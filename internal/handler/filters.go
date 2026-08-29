package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
	"github.com/rs/zerolog/log"
)

// JiraFilterSearchResponse represents a Jira-compatible paginated response
// for the filter search endpoint. The proxy always returns an empty result
// set since YouTrack has no equivalent of Jira saved filters.
type JiraFilterSearchResponse struct {
	Self       string `json:"self"`
	MaxResults int    `json:"maxResults"`
	StartAt    int    `json:"startAt"`
	Total      int    `json:"total"`
	IsLast     bool   `json:"isLast"`
	Values     []any  `json:"values"`
}

// HandleFilterSearch returns an empty paginated filter list. IntelliJ IDEA
// calls this endpoint during initialization; the stub prevents 404 errors.
func HandleFilterSearch(c echo.Context) error {
	startAt := 0
	if v := c.QueryParam("startAt"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			startAt = parsed
		}
	}

	maxResults := 100
	if v := c.QueryParam("maxResults"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			maxResults = parsed
		}
	}

	resp := JiraFilterSearchResponse{
		Self:       c.Request().URL.String(),
		MaxResults: maxResults,
		StartAt:    startAt,
		Total:      0,
		IsLast:     true,
		Values:     []any{},
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleGetFilter returns a synthetic Jira filter for a project. The filter ID
// is decoded back to a YouTrack project ID.
// DevLake fetches this to build the JQL for incremental issue sync.
func HandleGetFilter(c echo.Context, cfg *config.Config) error {
	filterIDStr := c.Param("filterId")
	filterID, err := strconv.ParseInt(filterIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"Invalid filter ID format"},
		})
	}

	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	shortName, err := service.ResolveFilterProject(filterID, requestCtx, cfg)
	if err != nil {
		if errors.Is(err, service.ErrFilterNotFound) {
			return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
				ErrorMessages: []string{"Filter not found"},
			})
		}
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Int64("filterID", filterID).Msg("Error resolving filter project")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve project from upstream"},
		})
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)

	return c.JSON(http.StatusOK, model.JiraFilterResponse{
		ID:   filterIDStr,
		Name: shortName + " Filter",
		JQL:  "project = " + shortName,
		Self: fmt.Sprintf("%s/rest/api/2/filter/%s", baseURL, filterIDStr),
	})
}
