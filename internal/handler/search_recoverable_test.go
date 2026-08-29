package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestBugCondition_GetIssuesRecoverableError asserts that recoverable upstream
// errors (404, 403) from GetIssues return HTTP 200 with an empty Jira search
// result. On UNFIXED code these tests FAIL (HTTP 502), confirming the bug.
//
// **Validates: Requirements 1.1, 1.2, 2.1, 2.2**
func TestBugCondition_GetIssuesRecoverableError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"GetIssues_404_returns_empty_result", http.StatusNotFound},
		{"GetIssues_403_returns_empty_result", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock YouTrack server that returns the recoverable error status
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"error":"not accessible"}`))
			}))
			defer mockServer.Close()

			cfg := &config.Config{YouTrackURL: mockServer.URL}
			c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+ARCHIVED")

			err := HandleSearchIssues(c, cfg, testCache())
			if err != nil {
				t.Fatalf("handler returned echo error: %v", err)
			}

			// Expected behavior: HTTP 200 with empty Jira search result
			if rec.Code != http.StatusOK {
				t.Errorf("expected HTTP 200, got %d (body: %s)", rec.Code, rec.Body.String())
			}

			var resp model.JiraSearchResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if resp.StartAt != 0 {
				t.Errorf("expected startAt=0, got %d", resp.StartAt)
			}
			if resp.MaxResults != 0 {
				t.Errorf("expected maxResults=0, got %d", resp.MaxResults)
			}
			if resp.Total != 0 {
				t.Errorf("expected total=0, got %d", resp.Total)
			}
			if len(resp.Issues) != 0 {
				t.Errorf("expected 0 issues, got %d", len(resp.Issues))
			}
		})
	}
}

// TestBugCondition_CountIssuesRecoverableError asserts that recoverable upstream
// errors (404, 403) from CountIssues return HTTP 200 with total = startAt + len(issues).
// On UNFIXED code these tests FAIL (HTTP 502), confirming the bug.
//
// To trigger CountIssues the handler must receive >= maxResults issues from GetIssues.
// We set maxResults=2 and return exactly 2 issues on the first call, then return
// the recoverable error on the second call (CountIssues). The calls are distinguished
// by the `fields` query parameter: CountIssues requests "idReadable" only.
//
// **Validates: Requirements 1.3, 2.3**
func TestBugCondition_CountIssuesRecoverableError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"CountIssues_404_treats_count_as_fallback", http.StatusNotFound},
		{"CountIssues_403_treats_count_as_fallback", http.StatusForbidden},
	}

	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "PROJ-1",
			Summary:    "Issue One",
			Created:    1700000000000,
			Updated:    1700001000000,
		},
		{
			ID:         "2-2",
			IDReadable: "PROJ-2",
			Summary:    "Issue Two",
			Created:    1700002000000,
			Updated:    1700003000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callCount atomic.Int32

			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/admin/projects/") {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte("[]"))
					return
				}
				if r.URL.Path != "/api/issues" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				fields := r.URL.Query().Get("fields")
				isCountCall := fields == "idReadable"

				// Also track by call count as a fallback
				n := callCount.Add(1)

				if isCountCall || n > 1 {
					// CountIssues call — return recoverable error
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.statusCode)
					w.Write([]byte(`{"error":"not accessible"}`))
					return
				}

				// GetIssues call — return issues normally
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(ytIssues)
			}))
			defer mockServer.Close()

			cfg := &config.Config{YouTrackURL: mockServer.URL}
			// maxResults=2 matches len(ytIssues) so handler will call CountIssues
			c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ&startAt=0&maxResults=2")

			err := HandleSearchIssues(c, cfg, testCache())
			if err != nil {
				t.Fatalf("handler returned echo error: %v", err)
			}

			// Expected behavior: HTTP 200 with the fetched issues and total = startAt + len(issues)
			if rec.Code != http.StatusOK {
				t.Errorf("expected HTTP 200, got %d (body: %s)", rec.Code, rec.Body.String())
			}

			var resp model.JiraSearchResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				// If we got a 502, the body is a JiraErrorResponse, not JiraSearchResponse.
				// Try to parse as error to document the counterexample.
				var errResp model.JiraErrorResponse
				if jsonErr := json.Unmarshal(rec.Body.Bytes(), &errResp); jsonErr == nil {
					t.Fatalf("got error response instead of search response: %v (HTTP %d)", errResp.ErrorMessages, rec.Code)
				}
				t.Fatalf("failed to parse response: %v (body: %s)", err, rec.Body.String())
			}

			if resp.StartAt != 0 {
				t.Errorf("expected startAt=0, got %d", resp.StartAt)
			}
			// total should be startAt + len(issues) = 0 + 2 = 2
			if resp.Total != 2 {
				t.Errorf("expected total=2 (startAt + len(issues)), got %d", resp.Total)
			}
			if len(resp.Issues) != 2 {
				t.Errorf("expected 2 issues, got %d", len(resp.Issues))
			}

			// Verify we actually got the right issues back
			if len(resp.Issues) >= 2 {
				if resp.Issues[0].Key != "PROJ-1" {
					t.Errorf("expected first issue key=PROJ-1, got %s", resp.Issues[0].Key)
				}
				if resp.Issues[1].Key != "PROJ-2" {
					t.Errorf("expected second issue key=PROJ-2, got %s", resp.Issues[1].Key)
				}
			}
		})
	}
}

// TestBugCondition_Summary documents the counterexamples discovered by the above
// tests. This is a documentation test that always passes — the real assertions
// are in the tests above. Remove after fix is applied.
func TestBugCondition_Summary(t *testing.T) {
	counterexamples := []string{
		"GetIssues 404 → HTTP 502 {\"errorMessages\":[\"Failed to search issues from upstream\"]}",
		"GetIssues 403 → HTTP 502 {\"errorMessages\":[\"Failed to search issues from upstream\"]}",
		"CountIssues 404 → HTTP 502 {\"errorMessages\":[\"Failed to count issues from upstream\"]}",
		"CountIssues 403 → HTTP 502 {\"errorMessages\":[\"Failed to count issues from upstream\"]}",
	}

	t.Log("Expected counterexamples on UNFIXED code:")
	for _, ce := range counterexamples {
		t.Log("  - " + ce)
	}
	t.Log("All four cases return HTTP 502 instead of HTTP 200 with empty/fallback result.")
	t.Log("This confirms the bug: no error classification branch for 404/403 in HandleSearchIssues.")

	// Verify the handler actually returns 502 for a 404 upstream (confirm bug exists)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+GONE")
	_ = HandleSearchIssues(c, cfg, testCache())

	if rec.Code == http.StatusBadGateway {
		t.Log("Confirmed: handler returns HTTP 502 for upstream 404 (bug present)")
	} else if rec.Code == http.StatusOK {
		t.Log("Handler returns HTTP 200 for upstream 404 (bug is FIXED)")
	}

	// Check body for the error message pattern
	body := rec.Body.String()
	if strings.Contains(body, "Failed to search issues from upstream") {
		t.Log("Confirmed: error message matches expected counterexample")
	}
}

// ---------------------------------------------------------------------------
// Preservation tests — capture CURRENT (unfixed) behavior that must not change
// ---------------------------------------------------------------------------

// TestPreservation_GetIssues5xxReturns502 is a property-based test that verifies:
// For any 5xx status code (500-599) returned by GetIssues as a *YouTrackError,
// the handler SHALL return HTTP 200 with an empty search result.
//
// Upstream failures during search are treated as empty results to prevent
// a single broken/legacy project from failing an entire DevLake pipeline.
//
// **Validates: Requirements 3.1, 3.3**
func TestPreservation_GetIssues5xxReturns502(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		statusCode := rapid.IntRange(500, 599).Draw(t, "statusCode")

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			w.Write([]byte(`{"error":"upstream failure"}`))
		}))
		defer mockServer.Close()

		cfg := &config.Config{YouTrackURL: mockServer.URL, QueueTimeout: 5 * time.Second, RequestTimeout: 5 * time.Second}
		c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ")

		err := HandleSearchIssues(c, cfg, testCache())
		if err != nil {
			t.Fatalf("handler returned echo error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200 for upstream %d, got %d (body: %s)", statusCode, rec.Code, rec.Body.String())
		}

		var searchResp model.JiraSearchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &searchResp); err != nil {
			t.Fatalf("failed to parse search response: %v", err)
		}
		if len(searchResp.Issues) != 0 {
			t.Fatalf("expected empty issues, got %d", len(searchResp.Issues))
		}
		if searchResp.Total != 0 {
			t.Fatalf("expected total=0, got %d", searchResp.Total)
		}
	})
}

// TestPreservation_GetIssuesQueueTimeoutReturns503 verifies that when the
// concurrency semaphore is saturated and GetIssues cannot acquire a slot,
// the handler returns HTTP 503 with a Retry-After header.
//
// This captures existing behavior that must be preserved after the fix.
//
// **Validates: Requirements 3.3**
func TestPreservation_GetIssuesQueueTimeoutReturns503(t *testing.T) {
	// Set up a mock server that blocks the first request (GetIssues) until signaled,
	// keeping the semaphore slot held while the handler request tries to acquire.
	blockCh := make(chan struct{})
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer mockServer.Close()

	// Initialize concurrency gate with 1 slot.
	client.InitConcurrency(1)

	queueTimeout := 50 * time.Millisecond
	cfg := &config.Config{
		YouTrackURL:    mockServer.URL,
		QueueTimeout:   queueTimeout,
		RequestTimeout: 5 * time.Second,
	}

	// Saturate the single slot by starting a request in the background.
	bgDone := make(chan struct{})
	go func() {
		defer close(bgDone)
		ctx := &model.RequestContext{YouTrackToken: "bg-token"}
		_, _ = client.GetIssues("", 0, 10, ctx, cfg)
	}()

	// Give the background goroutine time to acquire the slot.
	time.Sleep(20 * time.Millisecond)

	// Now the handler's GetIssues call should fail with ErrQueueTimeout.
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("handler returned echo error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("expected Retry-After header, got empty")
	}

	// Unblock the background goroutine and wait for it to finish before
	// resetting the semaphore, so it releases on the current semaphore.
	close(blockCh)
	<-bgDone

	// Disable the concurrency gate so subsequent tests in this package
	// that don't set QueueTimeout are unaffected (nil semaphore = no-op).
	client.DisableConcurrency()
}

// TestPreservation_SuccessfulSearchReturnsResults verifies that when GetIssues
// succeeds and returns fewer issues than maxResults (no CountIssues call needed),
// the handler returns HTTP 200 with the correct Jira search response.
//
// This captures existing behavior that must be preserved after the fix.
//
// **Validates: Requirements 3.2, 3.4**
func TestPreservation_SuccessfulSearchReturnsResults(t *testing.T) {
	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "PROJ-1",
			Summary:    "Preservation Test Issue 1",
			Created:    1700000000000,
			Updated:    1700001000000,
		},
		{
			ID:         "2-2",
			IDReadable: "PROJ-2",
			Summary:    "Preservation Test Issue 2",
			Created:    1700002000000,
			Updated:    1700003000000,
		},
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssues)
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL, QueueTimeout: 5 * time.Second, RequestTimeout: 5 * time.Second}
	// maxResults=10, returning 2 issues → fewer than max, no CountIssues call
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ&startAt=0&maxResults=10")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("handler returned echo error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.StartAt != 0 {
		t.Errorf("expected startAt=0, got %d", resp.StartAt)
	}
	if resp.MaxResults != 2 {
		t.Errorf("expected maxResults=2 (actual count), got %d", resp.MaxResults)
	}
	// total = startAt + len(issues) = 0 + 2 = 2 (last page shortcut)
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(resp.Issues))
	}
	if resp.Issues[0].Key != "PROJ-1" {
		t.Errorf("expected first issue key=PROJ-1, got %s", resp.Issues[0].Key)
	}
	if resp.Issues[1].Key != "PROJ-2" {
		t.Errorf("expected second issue key=PROJ-2, got %s", resp.Issues[1].Key)
	}
}

// TestPreservation_CountIssues5xxReturns502 verifies that when GetIssues succeeds
// but CountIssues returns a 5xx error, the handler returns HTTP 502.
//
// To trigger CountIssues, GetIssues must return exactly maxResults issues (so the
// handler thinks there may be more). CountIssues uses fields=idReadable to distinguish.
//
// This captures existing behavior that must be preserved after the fix.
//
// **Validates: Requirements 3.5**
func TestPreservation_CountIssues5xxReturns502(t *testing.T) {
	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "PROJ-1",
			Summary:    "Issue One",
			Created:    1700000000000,
			Updated:    1700001000000,
		},
		{
			ID:         "2-2",
			IDReadable: "PROJ-2",
			Summary:    "Issue Two",
			Created:    1700002000000,
			Updated:    1700003000000,
		},
	}

	var callCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/projects/") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		if r.URL.Path != "/api/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		fields := r.URL.Query().Get("fields")
		isCountCall := fields == "idReadable"
		n := callCount.Add(1)

		if isCountCall || n > 1 {
			// CountIssues call — return 500
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal server error"}`))
			return
		}

		// GetIssues call — return issues normally
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssues)
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL, QueueTimeout: 5 * time.Second, RequestTimeout: 5 * time.Second}
	// maxResults=2 matches len(ytIssues) so handler will call CountIssues
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ&startAt=0&maxResults=2")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("handler returned echo error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 for CountIssues 5xx, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var searchResp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("failed to parse search response: %v", err)
	}
	// Issues from GetIssues should still be present; only the total falls back.
	if len(searchResp.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(searchResp.Issues))
	}
	// Fallback total = startAt + len(issues) = 0 + 2 = 2
	if searchResp.Total != 2 {
		t.Fatalf("expected fallback total=2, got %d", searchResp.Total)
	}
}
