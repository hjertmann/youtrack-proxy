package client

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

func TestIsRecoverableClientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"YouTrackError 404", &YouTrackError{StatusCode: 404, Message: "not found"}, true},
		{"YouTrackError 403", &YouTrackError{StatusCode: 403, Message: "forbidden"}, true},
		{"YouTrackError 500", &YouTrackError{StatusCode: 500, Message: "internal"}, false},
		{"ErrQueueTimeout", ErrQueueTimeout, false},
		{"plain error", fmt.Errorf("some error"), false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRecoverableClientError(tt.err); got != tt.want {
				t.Errorf("IsRecoverableClientError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// Validates: Requirements 2.1, 2.2, 2.3
func TestPropertyIsRecoverableClientError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		code := rapid.IntRange(100, 599).Draw(t, "statusCode")
		err := &YouTrackError{StatusCode: code, Message: "test"}
		got := IsRecoverableClientError(err)
		want := code == 403 || code == 404
		if got != want {
			t.Errorf("IsRecoverableClientError(YouTrackError{StatusCode: %d}) = %v, want %v", code, got, want)
		}
	})
}
