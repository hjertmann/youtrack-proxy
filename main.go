// Command jira-youtrack-proxy starts an HTTP server that accepts Jira REST API
// v2 requests and translates them into YouTrack API calls.
package main

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/handler"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to build config")
		panic(1)
	}

	client.InitConcurrency(int64(cfg.MaxConcurrency))
	client.InitHTTPClient(*cfg)

	resolvedCache := service.NewResolvedStateCache(1 * time.Hour)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Health probe — no auth, no YouTrack call.
	e.GET("/health", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.String(http.StatusOK, "OK")
	})

	// Jira REST API v2 — all routes require Basic Auth
	api := e.Group("/rest/api/2", authmw.BasicAuth(cfg.AuthUsername))

	// Server discovery
	api.GET("/serverInfo", handler.HandleServerInfo)

	api.POST("/issue", func(c echo.Context) error {
		return handler.HandleCreateIssue(c, cfg)
	})

	// Projects
	api.GET("/project", func(c echo.Context) error {
		return handler.HandleListProjects(c, cfg)
	})
	api.GET("/project/recent", func(c echo.Context) error {
		return handler.HandleRecentProjects(c, cfg)
	})
	api.GET("/project/:projectIdOrKey", func(c echo.Context) error {
		return handler.HandleGetProject(c, cfg)
	})

	// Issue search and retrieval
	api.GET("/search", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/issue/:issueIdOrKey/editmeta", func(c echo.Context) error {
		return handler.HandleGetIssueEditMeta(c, cfg)
	})
	api.GET("/issue/:issueIdOrKey/changelog", func(c echo.Context) error {
		return handler.HandleGetIssueChangelog(c, cfg, resolvedCache)
	})
	api.GET("/issue/:issueIdOrKey/worklog", handler.HandleGetWorklog)
	api.GET("/issue/:issueIdOrKey/remotelink", handler.HandleGetRemoteLinks)
	api.GET("/issue/:issueIdOrKey", func(c echo.Context) error {
		return handler.HandleGetIssue(c, cfg, resolvedCache)
	})
	api.GET("/issue/:issueIdOrKey/comment", func(c echo.Context) error {
		return handler.HandleGetIssueComments(c, cfg)
	})

	// Users
	api.GET("/myself", func(c echo.Context) error {
		return handler.HandleGetCurrentUser(c, cfg)
	})
	api.GET("/user", func(c echo.Context) error {
		return handler.HandleGetUser(c, cfg)
	})
	api.GET("/user/picker", func(c echo.Context) error {
		return handler.HandleUserPicker(c, cfg)
	})
	api.GET("/user/search", func(c echo.Context) error {
		return handler.HandleSearchUsers(c, cfg)
	})

	// Field metadata
	api.GET("/field", handler.HandleListFields)

	// Picker metadata
	api.GET("/issuetype", func(c echo.Context) error {
		return handler.HandleListIssueTypes(c, cfg)
	})
	api.GET("/status", func(c echo.Context) error {
		return handler.HandleListStatuses(c, cfg)
	})

	// Filters (IntelliJ IDEA compatibility)
	api.GET("/filter/search", handler.HandleFilterSearch)
	api.GET("/filter/:filterId", func(c echo.Context) error {
		return handler.HandleGetFilter(c, cfg)
	})

	// Agile API (synthetic boards)
	agile := e.Group("/rest/agile/1.0", authmw.BasicAuth(cfg.AuthUsername))
	agile.GET("/board", func(c echo.Context) error {
		return handler.HandleListBoards(c, cfg)
	})
	agile.GET("/board/:boardId/configuration", func(c echo.Context) error {
		return handler.HandleBoardConfiguration(c, cfg)
	})
	agile.GET("/board/:boardId/sprint", func(c echo.Context) error {
		return handler.HandleBoardSprints(c, cfg)
	})

	// v3 API (mirrors v2 search)
	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth(cfg.AuthUsername))
	apiv3.GET("/serverInfo", handler.HandleServerInfo)
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})

	log.Info().Str("port", cfg.Port).Str("youtrackURL", cfg.YouTrackURL).Msg("Starting proxy server")
	e.Logger.Fatal(e.Start(":" + cfg.Port))
}
