package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// HandleServerInfo returns Jira-compatible server metadata for client discovery.
// This endpoint does not require authentication.
func HandleServerInfo(c echo.Context) error {
	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
	serverTime := time.Now().UTC().Format("2006-01-02T15:04:05.000+0000")

	return c.JSON(http.StatusOK, model.ServerInfoResponse{
		BaseURL:        baseURL,
		Version:        "9.0.0",
		VersionNumbers: []int{9, 0, 0},
		DeploymentType: "Server",
		ServerTitle:    "YouTrack Jira Proxy",
		ServerTime:     serverTime,
		BuildNumber:    900000,
	})
}
