package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// handleQueueTimeout checks whether err is a concurrency queue timeout. If so
// it writes an HTTP 503 with a Retry-After header and returns true. Callers
// should return the echo error when true.
func handleQueueTimeout(c echo.Context, err error, cfg *config.Config) (error, bool) {
	if errors.Is(err, client.ErrQueueTimeout) {
		c.Response().Header().Set("Retry-After", strconv.Itoa(int(cfg.QueueTimeout.Seconds())))
		return c.JSON(http.StatusServiceUnavailable, model.JiraErrorResponse{
			ErrorMessages: []string{"Server is at capacity, please retry"},
		}), true
	}
	return nil, false
}
