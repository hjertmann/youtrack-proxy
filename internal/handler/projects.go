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

// HandleListProjects returns all projects from YouTrack in Jira-compatible format.
func HandleListProjects(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	projects, err := client.GetProjects(requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.
			Error().
			Err(err).
			Msg("Error fetching projects from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve projects from upstream"},
		})
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraProjects := service.ConvertYTProjectsToJira(projects, baseURL)

	return c.JSON(http.StatusOK, jiraProjects)
}

// HandleGetProject returns a single project from YouTrack in Jira-compatible format.
func HandleGetProject(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	projectIDOrKey := c.Param("projectIdOrKey")
	if projectIDOrKey == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"Project ID or key is required"},
		})
	}

	project, err := client.GetProject(projectIDOrKey, requestCtx, cfg)
	if err != nil {
		if client.IsNotFound(err) {
			log.
				Debug().
				Str("projectIdOrKey", projectIDOrKey).
				Msg("Project not found in YouTrack")

			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Project not found"},
			})
		}

		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}

		log.
			Error().
			Err(err).
			Str("projectIdOrKey", projectIDOrKey).
			Msg("Error fetching project from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve project from upstream"},
		})
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraProject := service.ConvertYTProjectToJira(*project, baseURL)

	return c.JSON(http.StatusOK, jiraProject)
}

// HandleRecentProjects returns recent projects in Jira-compatible format.
// Since the proxy returns all available projects (no activity tracking), this
// delegates directly to HandleListProjects.
func HandleRecentProjects(c echo.Context, cfg *config.Config) error {
	return HandleListProjects(c, cfg)
}
