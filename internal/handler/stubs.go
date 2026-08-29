package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// HandleGetWorklog returns a static empty worklog response.
// DevLake expects this endpoint to exist; YouTrack worklogs are not mapped.
func HandleGetWorklog(c echo.Context) error {
	return c.JSON(http.StatusOK, model.JiraWorklogResponse{
		StartAt:    0,
		MaxResults: 1048576,
		Total:      0,
		Worklogs:   []any{},
	})
}

// HandleGetRemoteLinks returns a static empty array.
// DevLake expects this endpoint to exist; YouTrack remote links are not mapped.
func HandleGetRemoteLinks(c echo.Context) error {
	return c.JSON(http.StatusOK, []any{})
}
