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

// setupRouterWithMockYouTrack creates an Echo instance with routes wired identically to main.go
// and a mock YouTrack server that returns the given issues.
func setupRouterWithMockYouTrack(t *testing.T, ytIssues []model.YTIssue) (*echo.Echo, *httptest.Server) {
	t.Helper()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/projects/") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		if r.URL.Path != "/api/issues" {
			t.Errorf("unexpected YouTrack path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssues)
	}))

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	cache := testCache()

	e := echo.New()

	// v2 API group (matches main.go)
	api := e.Group("/rest/api/2", authmw.BasicAuth())
	api.GET("/search", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})

	// v3 API group (matches main.go)
	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth())
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, cache)
	})

	return e, mockServer
}

// basicAuthHeader returns a valid Basic Auth header value for testing.
func basicAuthHeader(email, token string) string {
	creds := email + ":" + token
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func TestV3SearchJQL_ReachableAndReturnsCorrectResponse(t *testing.T) {
	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "PROJ-1",
			Summary:    "Test Issue 1",
			Created:    1700000000000,
			Updated:    1700001000000,
			Project: &model.YTProject{
				ID:        "0-0",
				Name:      "My Project",
				ShortName: "PROJ",
			},
		},
		{
			ID:         "2-2",
			IDReadable: "PROJ-2",
			Summary:    "Test Issue 2",
			Created:    1700002000000,
			Updated:    1700003000000,
			Project: &model.YTProject{
				ID:        "0-1",
				Name:      "My Project",
				ShortName: "PROJ",
			},
		},
	}

	e, mockServer := setupRouterWithMockYouTrack(t, ytIssues)
	defer mockServer.Close()

	req := httptest.NewRequest(http.MethodGet, "/rest/api/3/search/jql?jql=project+%3D+PROJ&startAt=0&maxResults=10", nil)
	req.Header.Set("Authorization", basicAuthHeader("user@example.com", "test-token"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify Content-Type is application/json
	ct := rec.Header().Get("Content-Type")
	if ct == "" || ct[:16] != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify JiraSearchResponse structure (Requirement 4.1)
	if resp.StartAt != 0 {
		t.Errorf("expected startAt=0, got %d", resp.StartAt)
	}
	if resp.MaxResults != 2 {
		t.Errorf("expected maxResults=2, got %d", resp.MaxResults)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(resp.Issues))
	}

	// Verify issue fields (Requirement 4.2)
	issue := resp.Issues[0]
	if issue.ID == "" {
		t.Error("expected issue id to be non-empty")
	}
	if issue.Key != "PROJ-1" {
		t.Errorf("expected issue key=PROJ-1, got %s", issue.Key)
	}
	if issue.Self == "" {
		t.Error("expected issue self URL to be non-empty")
	}
	if issue.Fields.Summary != "Test Issue 1" {
		t.Errorf("expected summary='Test Issue 1', got %s", issue.Fields.Summary)
	}
	if issue.Fields.Created == "" {
		t.Error("expected created field to be non-empty")
	}
	if issue.Fields.Updated == "" {
		t.Error("expected updated field to be non-empty")
	}
}
