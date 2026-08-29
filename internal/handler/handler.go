// Package handler implements Echo HTTP route handlers that translate Jira REST
// API v2 requests into YouTrack API calls.
package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// HandleCreateIssue accepts a Jira-shaped issue creation request, converts it
// to the YouTrack format, creates the issue upstream, and returns a Jira-shaped
// response.
func HandleCreateIssue(c echo.Context, cfg *config.Config) error {
	rctx := c.Get("requestCtx").(*model.RequestContext)

	var jiraReq model.JiraCreateIssueRequest
	if err := c.Bind(&jiraReq); err != nil {
		log.Error().Err(err).Msg("Error parsing Jira request")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON",
		})
	}

	projectKey := jiraReq.Fields.Project.Key
	if projectKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Project key is required",
		})
	}

	projectFields, err := client.GetProjectCustomFields(projectKey, rctx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Str("project", projectKey).
			Msg("Error fetching project custom fields for issue creation")
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Failed to retrieve project field configuration from upstream",
		})
	}

	ytReq, err := service.ConvertJiraToYouTrack(jiraReq, projectFields)
	if err != nil {
		log.Error().Err(err).Msg("Error converting request")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid issue fields",
		})
	}

	log.Debug().Interface("request", ytReq).Msg("YouTrack Request")

	ytResp, err := client.CreateYouTrackIssue(ytReq, rctx, cfg)
	if err != nil {
		log.Error().Err(err).Msg("Error creating YouTrack issue")
		return c.NoContent(http.StatusInternalServerError)
	}

	log.Debug().Interface("response", ytResp).Msg("YouTrack Response")

	return c.JSON(http.StatusCreated, model.JiraResponse{
		Key:  ytResp.ID,
		ID:   ytResp.ID,
		Self: fmt.Sprintf("%s/api/issues/%s", cfg.YouTrackURL, ytResp.ID),
	})
}
