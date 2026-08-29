package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// HandleSearchIssues searches for issues via YouTrack and returns results in Jira search response format.
func HandleSearchIssues(c echo.Context, cfg *config.Config, cache *service.ResolvedStateCache) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	jql := c.QueryParam("jql")
	startAt := parseIntParam(c.QueryParam("startAt"), 0)
	maxResults := parseIntParam(c.QueryParam("maxResults"), 50)
	// ponytail: hard cap prevents clients from requesting unbounded result sets; raise if needed.
	if maxResults > 1000 {
		maxResults = 1000
	}

	// Parse JQL into structured form and convert to YouTrack query
	var query string
	if jql != "" {
		parsed, err := service.ParseJQL(jql)
		if err != nil {
			return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
				ErrorMessages: []string{err.Error()},
			})
		}
		// Resolve filter clause to a project scope if present
		if parsed.FilterID != 0 {
			projectKey, err := service.ResolveFilterProject(parsed.FilterID, requestCtx, cfg)
			if err != nil {
				if errors.Is(err, service.ErrFilterNotFound) {
					return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
						ErrorMessages: []string{"filter not found"},
					})
				}
				if errors.Is(err, service.ErrInvalidFilterJQL) {
					return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
						ErrorMessages: []string{err.Error()},
					})
				}
				// Concurrency queue timeout
				if resp, ok := handleQueueTimeout(c, err, cfg); ok {
					return resp
				}
				// Upstream failure
				log.Error().Err(err).Int64("filterID", parsed.FilterID).
					Msg("Failed to resolve filter project from upstream")
				return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
					ErrorMessages: []string{"Failed to retrieve project from upstream"},
				})
			}
			// Merge: explicit project in JQL takes precedence
			if parsed.Project == "" {
				parsed.Project = projectKey
			}
		}

		query = parsed.ToYouTrackQuery()
	}

	issues, err := client.GetIssues(query, startAt, maxResults, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		// Any upstream failure during search returns an empty result set
		// instead of 502. This prevents a single broken/legacy project
		// from failing an entire DevLake pipeline.
		log.Warn().Err(err).Str("jql", jql).Str("query", query).
			Msg("Upstream error searching issues, returning empty result")
		return c.JSON(http.StatusOK, model.JiraSearchResponse{
			StartAt:    0,
			MaxResults: 0,
			Total:      0,
			Issues:     []model.JiraIssue{},
		})
	}

	// Accurate total: if we got fewer than requested, we're on the last page.
	// Otherwise, issue a count query.
	var total int
	if len(issues) < maxResults {
		total = startAt + len(issues)
	} else {
		total, err = client.CountIssues(query, requestCtx, cfg)
		if err != nil {
			if resp, ok := handleQueueTimeout(c, err, cfg); ok {
				return resp
			}
			log.Warn().Err(err).Str("query", query).Msg("Upstream error counting issues, using fallback total")
			total = startAt + len(issues)
		}
	}

	// Build a merged resolved state set across all projects in the result.
	fetchFn := func(pid string) (service.ResolvedStateSet, error) {
		fields, err := client.GetProjectCustomFields(pid, requestCtx, cfg)
		if err != nil {
			return nil, err
		}
		return service.BuildResolvedStateSet(fields), nil
	}
	mergedResolved := service.ResolvedStateSet{}
	seen := map[string]struct{}{}
	for _, iss := range issues {
		pid := ""
		if iss.Project != nil {
			pid = iss.Project.ID
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		for k, v := range cache.GetOrFetch(pid, fetchFn) {
			mergedResolved[k] = v
		}
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraIssues := service.ConvertYTIssuesToJira(issues, baseURL, mergedResolved)

	// Attach inline changelog when expand=changelog is requested (Req 13).
	expand := c.QueryParam("expand")
	if strings.Contains(expand, "changelog") {
		for i := range jiraIssues {
			activities, err := client.GetIssueActivities(jiraIssues[i].Key, 0, 100, requestCtx, cfg)
			if err != nil {
				log.Warn().Err(err).Str("issue", jiraIssues[i].Key).Msg("Failed to fetch activities for inline changelog")
				jiraIssues[i].Changelog = &model.JiraInlineChangelog{
					StartAt:    0,
					MaxResults: 100,
					Total:      0,
					Histories:  []model.JiraHistory{},
				}
				continue
			}
			changelog := service.ConvertYTActivitiesToJiraChangelog(activities, 0)
			jiraIssues[i].Changelog = &model.JiraInlineChangelog{
				StartAt:    0,
				MaxResults: 100,
				Total:      changelog.Total,
				Histories:  changelog.Histories,
			}
			// Override resolutiondate from changelog activities when a done-transition exists.
			if ts := service.DeriveResolutionDateFromActivities(activities, mergedResolved); ts != nil {
				rd := time.UnixMilli(*ts).UTC().Format("2006-01-02T15:04:05.000+0000")
				jiraIssues[i].Fields.ResolutionDate = &rd
			}
		}
	}

	return c.JSON(http.StatusOK, model.JiraSearchResponse{
		StartAt:    startAt,
		MaxResults: len(issues),
		Total:      total,
		Issues:     jiraIssues,
	})
}

// HandleGetIssue retrieves a single issue from YouTrack and returns it in Jira format.
func HandleGetIssue(c echo.Context, cfg *config.Config, cache *service.ResolvedStateCache) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	issueIDOrKey := c.Param("issueIdOrKey")
	if strings.TrimSpace(issueIDOrKey) == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"A valid issue identifier is required"},
		})
	}

	issue, err := client.GetIssue(issueIDOrKey, requestCtx, cfg)
	if err != nil {
		if client.IsNotFound(err) {
			log.
				Debug().
				Str("issueIdOrKey", issueIDOrKey).
				Msg("Issue not found in YouTrack")

			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Issue not found"},
			})
		}

		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}

		log.
			Error().
			Err(err).
			Str("issueIdOrKey", issueIDOrKey).
			Msg("Error fetching issue from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve issue from upstream"},
		})
	}

	projectID := ""
	if issue.Project != nil {
		projectID = issue.Project.ID
	}
	resolvedStates := cache.GetOrFetch(projectID, func(pid string) (service.ResolvedStateSet, error) {
		fields, err := client.GetProjectCustomFields(pid, requestCtx, cfg)
		if err != nil {
			return nil, err
		}
		return service.BuildResolvedStateSet(fields), nil
	})

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraIssue := service.ConvertYTIssueToJira(*issue, baseURL, resolvedStates)

	// Override resolutiondate for done-category issues using changelog activities.
	if jiraIssue.Fields.Status != nil && jiraIssue.Fields.Status.StatusCategory.Key == "done" {
		activities, err := client.GetIssueActivities(issueIDOrKey, 0, 100, requestCtx, cfg)
		if err != nil {
			log.Warn().Err(err).Str("issue", issueIDOrKey).Msg("Failed to fetch activities for resolutiondate override")
		} else if ts := service.DeriveResolutionDateFromActivities(activities, resolvedStates); ts != nil {
			rd := time.UnixMilli(*ts).UTC().Format("2006-01-02T15:04:05.000+0000")
			jiraIssue.Fields.ResolutionDate = &rd
		}
	}

	return c.JSON(http.StatusOK, jiraIssue)
}

// HandleGetIssueComments retrieves comments for an issue and returns them in Jira format.
func HandleGetIssueComments(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	issueIDOrKey := c.Param("issueIdOrKey")
	startAt := parseIntParam(c.QueryParam("startAt"), 0)
	maxResults := parseIntParam(c.QueryParam("maxResults"), 50)

	comments, err := client.GetIssueComments(issueIDOrKey, startAt, maxResults, requestCtx, cfg)
	if err != nil {
		if client.IsNotFound(err) {
			log.
				Debug().
				Str("issueIdOrKey", issueIDOrKey).
				Msg("Issue not found in YouTrack when fetching comments")

			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Issue not found"},
			})
		}

		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}

		log.
			Error().
			Err(err).
			Str("issueIdOrKey", issueIDOrKey).
			Msg("Error fetching issue comments from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve comments from upstream"},
		})
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraComments := service.ConvertYTCommentsToJira(comments, issueIDOrKey, baseURL)

	total := startAt + len(comments)

	return c.JSON(http.StatusOK, model.JiraCommentsResponse{
		StartAt:    startAt,
		MaxResults: len(comments),
		Total:      total,
		Comments:   jiraComments,
	})
}

// HandleGetIssueEditMeta retrieves editmeta for an issue, describing which fields
// are editable and their allowed values, returned in Jira REST API v2 format.
func HandleGetIssueEditMeta(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	issueIDOrKey := c.Param("issueIdOrKey")
	if strings.TrimSpace(issueIDOrKey) == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"A valid issue identifier is required"},
		})
	}

	issue, err := client.GetIssue(issueIDOrKey, requestCtx, cfg)
	if err != nil {
		if client.IsNotFound(err) {
			log.
				Debug().
				Str("issueIdOrKey", issueIDOrKey).
				Msg("Issue not found in YouTrack when fetching editmeta")

			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Issue not found"},
			})
		}

		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}

		log.
			Error().
			Err(err).
			Str("issueIdOrKey", issueIDOrKey).
			Msg("Error fetching issue from YouTrack for editmeta")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve issue from upstream"},
		})
	}

	projectID := ""
	if issue.Project != nil {
		projectID = issue.Project.ID
	}
	if projectID == "" {
		log.
			Error().
			Str("issueIdOrKey", issueIDOrKey).
			Msg("Issue has no associated project")

		return c.JSON(http.StatusInternalServerError, model.JiraErrorResponse{
			ErrorMessages: []string{"Issue has no associated project"},
		})
	}

	customFields, err := client.GetProjectCustomFields(projectID, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.
			Error().
			Err(err).
			Str("projectID", projectID).
			Msg("Error fetching project custom fields from YouTrack")

		return c.JSON(http.StatusInternalServerError, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve project field configuration"},
		})
	}

	editMeta := service.BuildEditMetaResponse(customFields)

	return c.JSON(http.StatusOK, editMeta)
}

// HandleGetIssueChangelog retrieves the change history for an issue from YouTrack
// and returns it in Jira's changelog response format.
func HandleGetIssueChangelog(c echo.Context, cfg *config.Config, cache *service.ResolvedStateCache) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	issueIDOrKey := c.Param("issueIdOrKey")
	if strings.TrimSpace(issueIDOrKey) == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"A valid issue identifier is required"},
		})
	}

	startAt := parseIntParam(c.QueryParam("startAt"), 0)
	maxResults := parseIntParam(c.QueryParam("maxResults"), 100)

	activities, err := client.GetIssueActivities(issueIDOrKey, startAt, maxResults, requestCtx, cfg)
	if err != nil {
		if client.IsNotFound(err) {
			log.
				Debug().
				Str("issueIdOrKey", issueIDOrKey).
				Msg("Issue not found in YouTrack when fetching changelog")

			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Issue not found"},
			})
		}

		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}

		log.
			Error().
			Err(err).
			Str("issueIdOrKey", issueIDOrKey).
			Msg("Error fetching issue changelog from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve changelog from upstream"},
		})
	}

	changelog := service.ConvertYTActivitiesToJiraChangelog(activities, startAt)

	return c.JSON(http.StatusOK, changelog)
}

// parseIntParam parses a string into a non-negative integer, returning the default value
// on failure or negative input.
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil || val < 0 {
		return defaultVal
	}
	return val
}
