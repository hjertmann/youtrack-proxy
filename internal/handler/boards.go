package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// HandleListBoards returns one synthetic Jira Agile board per YouTrack project.
// Board IDs are deterministic encodings of the YouTrack project ID.
func HandleListBoards(c echo.Context, cfg *config.Config) error {
	requestCtx := c.Get("requestCtx").(*model.RequestContext)

	projects, err := client.GetProjects(requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Msg("Error fetching projects from YouTrack for boards")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve projects from upstream"},
		})
	}

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)

	// Build boards, skipping projects whose ID cannot be encoded.
	var boards []model.JiraBoard
	for _, p := range projects {
		numID, err := idmap.Encode(p.ID)
		if err != nil {
			log.Warn().Err(err).Str("project", p.ShortName).Msg("skipping project for board listing")
			continue
		}
		boards = append(boards, model.JiraBoard{
			ID:   numID,
			Self: fmt.Sprintf("%s/rest/agile/1.0/board/%d", baseURL, numID),
			Name: p.Name,
			Type: "scrum",
			Location: model.JiraBoardLocation{
				ProjectID:   numID,
				ProjectName: p.Name,
				ProjectKey:  p.ShortName,
			},
		})
	}

	// Pagination
	startAt := 0
	if v := c.QueryParam("startAt"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			startAt = parsed
		}
	}
	maxResults := 50
	if v := c.QueryParam("maxResults"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxResults = parsed
		}
	}

	total := len(boards)
	if startAt > total {
		startAt = total
	}
	end := startAt + maxResults
	if end > total {
		end = total
	}
	page := boards[startAt:end]
	if page == nil {
		page = []model.JiraBoard{}
	}

	return c.JSON(http.StatusOK, model.JiraBoardResponse{
		MaxResults: maxResults,
		StartAt:    startAt,
		IsLast:     end >= total,
		Values:     page,
	})
}

// HandleBoardConfiguration returns the configuration for a synthetic board.
// The boardId is decoded back to a YouTrack project ID.
func HandleBoardConfiguration(c echo.Context, cfg *config.Config) error {
	boardID, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"Invalid board ID format"},
		})
	}

	ytID, ok := idmap.Decode(boardID)
	if !ok {
		return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
			ErrorMessages: []string{"Board not found"},
		})
	}

	// Fetch the project to get its name and short name.
	requestCtx := c.Get("requestCtx").(*model.RequestContext)
	project, err := client.GetProject(ytID, requestCtx, cfg)
	if err != nil {
		if resp, ok := handleQueueTimeout(c, err, cfg); ok {
			return resp
		}
		log.Error().Err(err).Str("ytID", ytID).Msg("Error fetching project for board config")
		return c.JSON(http.StatusBadGateway, model.JiraErrorResponse{
			ErrorMessages: []string{"Failed to retrieve project from upstream"},
		})
	}

	return c.JSON(http.StatusOK, model.JiraBoardConfig{
		ID:   boardID,
		Name: project.Name,
		Type: "scrum",
		Filter: model.JiraBoardFilter{
			ID: idmap.FormatID(boardID),
		},
		SubQuery: model.JiraBoardSubQuery{
			Query: "",
		},
	})
}

// HandleBoardSprints returns an empty sprint list for a synthetic board.
// YouTrack has no native sprint concept mapped here, so this is a stub.
func HandleBoardSprints(c echo.Context, _ *config.Config) error {
	boardID, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.JiraErrorResponse{
			ErrorMessages: []string{"Invalid board ID format"},
		})
	}

	if _, ok := idmap.Decode(boardID); !ok {
		return c.JSON(http.StatusNotFound, model.JiraErrorResponse{
			ErrorMessages: []string{"Board not found"},
		})
	}

	return c.JSON(http.StatusOK, model.JiraSprintResponse{
		MaxResults: 0,
		StartAt:    0,
		IsLast:     true,
		Values:     []model.JiraSprint{},
	})
}
