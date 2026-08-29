package client

import (
	"errors"
	"fmt"
)

// YouTrackError represents an error response from the YouTrack API.
type YouTrackError struct {
	StatusCode int
	Message    string
}

func (e *YouTrackError) Error() string {
	return fmt.Sprintf("YouTrack API error (status %d): %s", e.StatusCode, e.Message)
}

// ErrQueueTimeout is returned when a request cannot acquire a concurrency slot
// within the configured queue timeout.
var ErrQueueTimeout = errors.New("concurrency queue timeout")

// IsNotFound returns true if the error is a YouTrackError with HTTP status 404.
func IsNotFound(err error) bool {
	var ytErr *YouTrackError
	if errors.As(err, &ytErr) {
		return ytErr.StatusCode == 404
	}
	return false
}

// IsRecoverableClientError returns true if the error is a YouTrackError with
// HTTP status 404 or 403 — upstream says the resource doesn't exist or isn't
// accessible. These are not retryable and should be treated as empty results
// in search contexts.
func IsRecoverableClientError(err error) bool {
	var ytErr *YouTrackError
	if errors.As(err, &ytErr) {
		return ytErr.StatusCode == 404 || ytErr.StatusCode == 403
	}
	return false
}
