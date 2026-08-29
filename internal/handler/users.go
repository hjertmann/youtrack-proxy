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

// HandleGetCurrentUser returns the currently authenticated user in Jira-compatible format.
func HandleGetCurrentUser(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	user, err := client.GetCurrentUser(requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.
			Error().
			Err(err).
			Msg("Error fetching current user from YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve current user from upstream"},
		})
	}

	jiraUser := service.ConvertYTUserToJira(*user)

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	jiraUser.Self = fmt.Sprintf("%s/rest/api/2/user?username=%s", baseURL, jiraUser.Name)

	return c.JSON(http.StatusOK, jiraUser)
}

// HandleSearchUsers searches for users by username in YouTrack and returns
// results in Jira-compatible format. Returns an empty array if no matches found.
func HandleSearchUsers(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	username := c.QueryParam("username")
	if username == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"The username query parameter is required"},
		})
	}

	users, err := client.SearchUsers(username, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.
			Error().
			Err(err).
			Str("username", username).
			Msg("Error searching users in YouTrack")

		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to search users from upstream"},
		})
	}

	jiraUsers := service.ConvertYTUsersToJira(users)

	return c.JSON(http.StatusOK, jiraUsers)
}

// HandleGetUser looks up a single user by username, key, or accountId and
// returns a single Jira-compatible user object (not an array).
func HandleGetUser(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	// First non-empty query param wins — all three map to the same YT search.
	query := c.QueryParam("username")
	if query == "" {
		query = c.QueryParam("key")
	}
	if query == "" {
		query = c.QueryParam("accountId")
	}
	if query == "" {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"One of username, key, or accountId query parameter is required"},
		})
	}

	users, err := client.SearchUsers(query, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Str("query", query).Msg("Error looking up user in YouTrack")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve user from upstream"},
		})
	}

	if len(users) == 0 {
		return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
			ErrorMessages: []string{fmt.Sprintf("User not found: %s", query)},
		})
	}

	jiraUser := service.ConvertYTUserToJira(users[0])
	return c.JSON(http.StatusOK, jiraUser)
}
