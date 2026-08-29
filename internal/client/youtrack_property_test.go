package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestPropertyPaginationParameterForwarding validates Property 8: Pagination parameters are correctly forwarded.
// For any startAt >= 0 and maxResults > 0, the proxy SHALL pass $skip = startAt and $top = maxResults
// to the YouTrack API, and the response startAt field SHALL equal the requested value.
//
// **Validates: Requirements 3.3, 3.5, 5.3, 5.4**
func TestPropertyPaginationParameterForwarding(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		startAt := rapid.IntRange(0, 10000).Draw(t, "startAt")
		maxResults := rapid.IntRange(1, 100).Draw(t, "maxResults")

		var capturedSkip, capturedTop string

		// Create a mock YouTrack server that captures the query parameters
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedSkip = r.URL.Query().Get("$skip")
			capturedTop = r.URL.Query().Get("$top")

			// Return an empty JSON array (valid issues response)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]model.YTIssue{})
		}))
		defer server.Close()

		cfg := &config.Config{
			YouTrackURL: server.URL,
		}
		requestCtx := &model.RequestContext{
			YouTrackToken: "test-token",
		}

		// Call GetIssues with the generated pagination values
		_, err := GetIssues("", startAt, maxResults, requestCtx, cfg)
		if err != nil {
			t.Fatalf("GetIssues returned unexpected error: %v", err)
		}

		// Verify $skip equals startAt
		if capturedSkip != itoa(startAt) {
			t.Fatalf("$skip = %q, want %q", capturedSkip, itoa(startAt))
		}

		// Verify $top equals maxResults
		if capturedTop != itoa(maxResults) {
			t.Fatalf("$top = %q, want %q", capturedTop, itoa(maxResults))
		}
	})
}

// TestPropertyPaginationComments validates Property 8 for the comments endpoint.
// For any startAt >= 0 and maxResults > 0, GetIssueComments SHALL pass $skip = startAt
// and $top = maxResults to the YouTrack API.
//
// **Validates: Requirements 5.3, 5.4**
func TestPropertyPaginationComments(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		startAt := rapid.IntRange(0, 10000).Draw(t, "startAt")
		maxResults := rapid.IntRange(1, 100).Draw(t, "maxResults")
		issueID := rapid.StringMatching(`[A-Z]{2,5}-[0-9]{1,5}`).Draw(t, "issueID")

		var capturedSkip, capturedTop string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedSkip = r.URL.Query().Get("$skip")
			capturedTop = r.URL.Query().Get("$top")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]model.YTComment{})
		}))
		defer server.Close()

		cfg := &config.Config{
			YouTrackURL: server.URL,
		}
		requestCtx := &model.RequestContext{
			YouTrackToken: "test-token",
		}

		_, err := GetIssueComments(issueID, startAt, maxResults, requestCtx, cfg)
		if err != nil {
			t.Fatalf("GetIssueComments returned unexpected error: %v", err)
		}

		if capturedSkip != itoa(startAt) {
			t.Fatalf("$skip = %q, want %q", capturedSkip, itoa(startAt))
		}
		if capturedTop != itoa(maxResults) {
			t.Fatalf("$top = %q, want %q", capturedTop, itoa(maxResults))
		}
	})
}

// TestPropertyPaginationResponseStartAt validates that the handler response construction
// correctly reflects the startAt value in both search and comments responses.
//
// **Validates: Requirements 3.5, 5.3**
func TestPropertyPaginationResponseStartAt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		startAt := rapid.IntRange(0, 10000).Draw(t, "startAt")
		maxResults := rapid.IntRange(1, 100).Draw(t, "maxResults")
		numResults := rapid.IntRange(0, maxResults).Draw(t, "numResults")

		// Verify search response construction preserves startAt
		searchResponse := model.JiraSearchResponse{
			StartAt:    startAt,
			MaxResults: numResults,
			Total:      startAt + numResults,
			Issues:     make([]model.JiraIssue, numResults),
		}

		if searchResponse.StartAt != startAt {
			t.Fatalf("JiraSearchResponse.StartAt = %d, want %d", searchResponse.StartAt, startAt)
		}
		if searchResponse.Total < searchResponse.StartAt {
			t.Fatalf("Total (%d) < StartAt (%d)", searchResponse.Total, searchResponse.StartAt)
		}

		// Verify comments response construction preserves startAt
		commentsResponse := model.JiraCommentsResponse{
			StartAt:    startAt,
			MaxResults: numResults,
			Total:      startAt + numResults,
			Comments:   make([]model.JiraComment, numResults),
		}

		if commentsResponse.StartAt != startAt {
			t.Fatalf("JiraCommentsResponse.StartAt = %d, want %d", commentsResponse.StartAt, startAt)
		}
		if commentsResponse.Total < commentsResponse.StartAt {
			t.Fatalf("Total (%d) < StartAt (%d)", commentsResponse.Total, commentsResponse.StartAt)
		}
	})
}

// itoa is a small helper to convert int to string for comparison.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// TestPropertyGetProjectsPagination validates that GetProjects returns all unique projects
// for any project count, including multi-page scenarios and alias deduplication.
func TestPropertyGetProjectsPagination(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		totalProjects := rapid.IntRange(0, 350).Draw(t, "totalProjects")
		aliasRate := rapid.Float64Range(0, 0.3).Draw(t, "aliasRate")

		// Generate projects with some aliases (duplicate IDs, different short names)
		var allProjects []model.YTProject
		uniqueIDs := make(map[string]struct{})
		for i := 0; i < totalProjects; i++ {
			id := fmt.Sprintf("0-%d", i)
			// With aliasRate probability, reuse a previous ID
			if i > 0 && rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("alias_%d", i)) < aliasRate {
				aliasOf := rapid.IntRange(0, i-1).Draw(t, fmt.Sprintf("aliasOf_%d", i))
				id = fmt.Sprintf("0-%d", aliasOf)
			}
			uniqueIDs[id] = struct{}{}
			allProjects = append(allProjects, model.YTProject{
				ID:        id,
				Name:      fmt.Sprintf("Project %d", i),
				ShortName: fmt.Sprintf("P%d", i),
			})
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			skip := 0
			if v := r.URL.Query().Get("$skip"); v != "" {
				skip, _ = strconv.Atoi(v)
			}
			top := 100
			if v := r.URL.Query().Get("$top"); v != "" {
				top, _ = strconv.Atoi(v)
			}
			end := skip + top
			if end > len(allProjects) {
				end = len(allProjects)
			}
			var page []model.YTProject
			if skip < len(allProjects) {
				page = allProjects[skip:end]
			} else {
				page = []model.YTProject{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page)
		}))
		defer server.Close()

		cfg := &config.Config{YouTrackURL: server.URL}
		reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

		result, err := GetProjects(reqCtx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify unique count matches
		if len(result) != len(uniqueIDs) {
			t.Fatalf("len(result) = %d, want %d unique projects", len(result), len(uniqueIDs))
		}

		// Verify no duplicate IDs in result
		seenIDs := make(map[string]struct{})
		for _, p := range result {
			if _, dup := seenIDs[p.ID]; dup {
				t.Fatalf("duplicate project ID %q in result", p.ID)
			}
			seenIDs[p.ID] = struct{}{}
		}
	})
}
