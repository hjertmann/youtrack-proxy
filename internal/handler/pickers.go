package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// HandleListIssueTypes fetches Type custom field bundles from YouTrack,
// deduplicates by bundle value ID, and returns one issue type entry per
// unique value with a deterministic numeric ID.
func HandleListIssueTypes(c echo.Context, cfg *config.Config) error {
	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	allFields, err := client.GetAllProjectCustomFields(requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Msg("Error fetching issue type bundles from YouTrack")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to fetch issue types from upstream"},
		})
	}

	seen := make(map[string]bool)
	var result []model.JiraIssueType

	for _, f := range allFields {
		if f.Field.Name != "Type" || f.Bundle == nil {
			continue
		}
		for _, bv := range f.Bundle.Values {
			if seen[bv.ID] {
				continue
			}
			seen[bv.ID] = true

			numID, err := idmap.Encode(bv.ID)
			if err != nil {
				log.Error().Err(err).Str("bundleValueID", bv.ID).Msg("Error encoding issue type ID")
				return c.JSON(http.StatusInternalServerError, model.JiraErrorResponse{
					ErrorMessages: []string{"Internal ID encoding error"},
				})
			}

			strID := idmap.FormatID(numID)
			result = append(result, model.JiraIssueType{
				ID:          strID,
				Name:        bv.Name,
				Description: "",
				Subtask:     false,
				Self:        fmt.Sprintf("%s/rest/api/2/issuetype/%s", baseURL, strID),
			})
		}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleListStatuses fetches State custom field bundles from YouTrack,
// deduplicates by bundle value ID, and returns one status entry per unique
// value with a deterministic numeric ID and a statusCategory from
// service.MapStateToCategory. This ensures status IDs and categories match
// what the issue converter produces on fields.status.
func HandleListStatuses(c echo.Context, cfg *config.Config) error {
	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	allFields, err := client.GetAllProjectCustomFields(requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Msg("Error fetching state bundles from YouTrack")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to fetch statuses from upstream"},
		})
	}

	seen := make(map[string]bool)
	var result []model.JiraStatus

	for _, f := range allFields {
		if f.Field.Name != "State" || f.Bundle == nil {
			continue
		}
		// Build a local resolved set from this bundle's isResolved flags.
		resolvedSet := make(service.ResolvedStateSet, len(f.Bundle.Values))
		for _, bv := range f.Bundle.Values {
			if bv.IsResolved {
				resolvedSet[strings.ToLower(bv.Name)] = struct{}{}
			}
		}
		for _, bv := range f.Bundle.Values {
			if seen[bv.ID] {
				continue
			}
			seen[bv.ID] = true

			numID, err := idmap.Encode(bv.ID)
			if err != nil {
				log.Error().Err(err).Str("bundleValueID", bv.ID).Msg("Error encoding status ID")
				return c.JSON(http.StatusInternalServerError, model.JiraErrorResponse{
					ErrorMessages: []string{"Internal ID encoding error"},
				})
			}

			strID := idmap.FormatID(numID)
			cat := service.MapStateToCategory(bv.Name, resolvedSet)
			result = append(result, model.JiraStatus{
				ID:          strID,
				Name:        bv.Name,
				Description: "",
				Self:        fmt.Sprintf("%s/rest/api/2/status/%s", baseURL, strID),
				StatusCategory: model.JiraStatusCategory{
					ID:        cat.ID,
					Name:      cat.Name,
					Key:       cat.Key,
					ColorName: cat.ColorName,
				},
			})
		}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleUserPicker searches YouTrack users and returns results in Jira's
// user-picker envelope format. Returns an empty result for empty/absent query.
func HandleUserPicker(c echo.Context, cfg *config.Config) error {
	query := c.QueryParam("query")
	if query == "" {
		return c.JSON(http.StatusOK, model.JiraUserPickerResponse{
			Users:  []model.JiraUserPickerUser{},
			Total:  0,
			Header: "Showing users",
		})
	}

	// Parse and clamp maxResults: default 10, max 1000.
	maxResults := 10
	if mr := c.QueryParam("maxResults"); mr != "" {
		if v, err := strconv.Atoi(mr); err == nil && v > 0 {
			maxResults = v
		}
	}
	if maxResults > 1000 {
		maxResults = 1000
	}

	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	users, err := client.SearchUsers(query, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Str("query", query).Msg("Error searching users in YouTrack for picker")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to search users from upstream"},
		})
	}

	if len(users) > maxResults {
		users = users[:maxResults]
	}

	mapped := make([]model.JiraUserPickerUser, len(users))
	for i, u := range users {
		mapped[i] = model.JiraUserPickerUser{
			Name:        u.Login,
			Key:         u.Login,
			HTML:        u.Name,
			DisplayName: u.Name,
		}
	}

	return c.JSON(http.StatusOK, model.JiraUserPickerResponse{
		Users:  mapped,
		Total:  len(mapped),
		Header: "Showing users",
	})
}
