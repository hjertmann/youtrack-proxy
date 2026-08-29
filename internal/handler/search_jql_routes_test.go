package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// setupRouterWithSearchRoutes creates an Echo instance with the v2 and v3 search/jql
// routes registered with BasicAuth middleware, mimicking the production router in main.go.
func setupRouterWithSearchRoutes(cfg *config.Config) *echo.Echo {
	e := echo.New()
	cache := testCache()

	api := e.Group("/rest/api/2", authmw.BasicAuth())
	api.GET("/search", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})

	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth())
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})

	return e
}

// validBasicAuth returns a valid Basic Auth header value for testing.
func validBasicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:test-token"))
}

func TestV2SearchJQL_ReachableAndReturnsCorrectResponse(t *testing.T) {
	// Mock YouTrack server that returns issues
	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "PROJ-1",
			Summary:    "Test Issue 1",
			Created:    1700000000000,
			Updated:    1700001000000,
		},
		{
			ID:         "2-2",
			IDReadable: "PROJ-2",
			Summary:    "Test Issue 2",
			Created:    1700002000000,
			Updated:    1700003000000,
		},
	}

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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssues)
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	e := setupRouterWithSearchRoutes(cfg)

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/search/jql?jql=project+%3D+PROJ&startAt=0&maxResults=10", nil)
	req.Header.Set("Authorization", validBasicAuth())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify Content-Type is JSON
	ct := rec.Header().Get("Content-Type")
	if ct == "" || (ct != "application/json" && ct != "application/json; charset=UTF-8" && ct != "application/json; charset=utf-8") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// Verify response parses as JiraSearchResponse
	var resp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response as JiraSearchResponse: %v", err)
	}

	// Verify JiraSearchResponse structure fields
	if resp.StartAt != 0 {
		t.Errorf("expected startAt=0, got %d", resp.StartAt)
	}
	if resp.MaxResults != 2 {
		t.Errorf("expected maxResults=2 (actual count returned), got %d", resp.MaxResults)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(resp.Issues))
	}

	// Verify issue structure includes expected fields (Requirement 4.2)
	issue := resp.Issues[0]
	if issue.Key != "PROJ-1" {
		t.Errorf("expected first issue key=PROJ-1, got %s", issue.Key)
	}
	if issue.Fields.Summary != "Test Issue 1" {
		t.Errorf("expected first issue summary='Test Issue 1', got %s", issue.Fields.Summary)
	}
}

func TestV2SearchJQL_EmptyResults(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	e := setupRouterWithSearchRoutes(cfg)

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/search/jql?jql=project+%3D+EMPTY", nil)
	req.Header.Set("Authorization", validBasicAuth())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(resp.Issues))
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Total)
	}
}
