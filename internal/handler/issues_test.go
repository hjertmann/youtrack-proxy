package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// issueTestContext creates an Echo context with path params and requestCtx set.
func issueTestContext(method, path, target string, paramNames, paramValues []string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(path)
	c.SetParamNames(paramNames...)
	c.SetParamValues(paramValues...)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	return c, rec
}

// testCache creates a ResolvedStateCache for use in handler tests.
func testCache() *service.ResolvedStateCache {
	return service.NewResolvedStateCache(time.Hour)
}

func TestHandleSearchIssues_Success(t *testing.T) {
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
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ&startAt=0&maxResults=10")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
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
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(resp.Issues))
	}
	if resp.Issues[0].Key != "PROJ-1" {
		t.Errorf("expected first issue key=PROJ-1, got %s", resp.Issues[0].Key)
	}
}

func TestHandleSearchIssues_DefaultPagination(t *testing.T) {
	// Verify that when startAt and maxResults are not provided, defaults are used (0 and 50)
	var capturedSkip, capturedTop string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSkip = r.URL.Query().Get("$skip")
		capturedTop = r.URL.Query().Get("$top")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if capturedSkip != "0" {
		t.Errorf("expected $skip=0, got %s", capturedSkip)
	}
	if capturedTop != "50" {
		t.Errorf("expected $top=50, got %s", capturedTop)
	}
}

func TestHandleSearchIssues_UpstreamError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=project+%3D+PROJ")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var searchResp model.JiraSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("failed to parse search response: %v", err)
	}
	if len(searchResp.Issues) != 0 {
		t.Fatalf("expected empty issues, got %d", len(searchResp.Issues))
	}
}

func TestHandleSearchIssues_EmptyJQL(t *testing.T) {
	// Empty JQL should still return a 200 with all issues (no query filter sent to YouTrack)
	var capturedQuery string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/search?jql=")

	err := HandleSearchIssues(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// With empty JQL, no query param should be sent to YouTrack
	if capturedQuery != "" {
		t.Errorf("expected empty query param, got %q", capturedQuery)
	}
}

func TestHandleGetIssue_Success(t *testing.T) {
	desc := "Test description"
	ytIssue := model.YTIssue{
		ID:          "2-1",
		IDReadable:  "PROJ-42",
		Summary:     "Found a bug",
		Description: &desc,
		Created:     1700000000000,
		Updated:     1700001000000,
		Reporter: &model.YTUser{
			Login: "john",
			Name:  "John Doe",
		},
		Project: &model.YTProject{
			ID:        "0-0",
			Name:      "My Project",
			ShortName: "PROJ",
		},
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/projects/") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		if r.URL.Path != "/api/issues/PROJ-42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssue)
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey", "/rest/api/2/issue/PROJ-42", []string{"issueIdOrKey"}, []string{"PROJ-42"})

	err := HandleGetIssue(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var jiraIssue model.JiraIssue
	if err := json.Unmarshal(rec.Body.Bytes(), &jiraIssue); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if jiraIssue.Key != "PROJ-42" {
		t.Errorf("expected key=PROJ-42, got %s", jiraIssue.Key)
	}
	if jiraIssue.Fields.Summary != "Found a bug" {
		t.Errorf("expected summary='Found a bug', got %s", jiraIssue.Fields.Summary)
	}
	if jiraIssue.Fields.Description == nil || *jiraIssue.Fields.Description != "Test description" {
		t.Errorf("expected description='Test description', got %v", jiraIssue.Fields.Description)
	}
	if jiraIssue.Fields.Reporter == nil || jiraIssue.Fields.Reporter.Name != "john" {
		t.Errorf("expected reporter name='john', got %v", jiraIssue.Fields.Reporter)
	}
}

func TestHandleGetIssue_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey", "/rest/api/2/issue/PROJ-999", []string{"issueIdOrKey"}, []string{"PROJ-999"})

	err := HandleGetIssue(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected at least one error message")
	}
}

func TestHandleGetIssue_EmptyId(t *testing.T) {
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}

	// Test with empty string
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey", "/rest/api/2/issue/", []string{"issueIdOrKey"}, []string{""})

	err := HandleGetIssue(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty ID, got %d", rec.Code)
	}

	// Test with whitespace-only string
	c2, rec2 := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey", "/rest/api/2/issue/%20%20", []string{"issueIdOrKey"}, []string{"  "})

	err = HandleGetIssue(c2, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for whitespace-only ID, got %d", rec2.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected at least one error message")
	}
}

func TestHandleGetIssue_UpstreamError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey", "/rest/api/2/issue/PROJ-1", []string{"issueIdOrKey"}, []string{"PROJ-1"})

	err := HandleGetIssue(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected at least one error message")
	}
}

func TestHandleGetIssueComments_Success(t *testing.T) {
	commentText := "This is a comment"
	ytComments := []model.YTComment{
		{
			ID: "comment-1",
			Author: &model.YTUser{
				Login: "jane",
				Name:  "Jane Doe",
			},
			Text:    &commentText,
			Created: 1700000000000,
		},
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/issues/PROJ-1/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytComments)
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey/comment", "/rest/api/2/issue/PROJ-1/comment?startAt=0&maxResults=10", []string{"issueIdOrKey"}, []string{"PROJ-1"})

	err := HandleGetIssueComments(c, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraCommentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.StartAt != 0 {
		t.Errorf("expected startAt=0, got %d", resp.StartAt)
	}
	if resp.MaxResults != 1 {
		t.Errorf("expected maxResults=1 (actual count), got %d", resp.MaxResults)
	}
	if resp.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Total)
	}
	if len(resp.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(resp.Comments))
	}
	if resp.Comments[0].ID != "comment-1" {
		t.Errorf("expected comment id=comment-1, got %s", resp.Comments[0].ID)
	}
	if resp.Comments[0].Body != "This is a comment" {
		t.Errorf("expected comment body='This is a comment', got %s", resp.Comments[0].Body)
	}
	if resp.Comments[0].Author == nil || resp.Comments[0].Author.Name != "jane" {
		t.Errorf("expected author name='jane', got %v", resp.Comments[0].Author)
	}
}

func TestHandleGetIssueComments_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey/comment", "/rest/api/2/issue/PROJ-999/comment", []string{"issueIdOrKey"}, []string{"PROJ-999"})

	err := HandleGetIssueComments(c, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected at least one error message")
	}
}

func TestHandleGetIssueComments_UpstreamError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"server error"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(http.MethodGet, "/rest/api/2/issue/:issueIdOrKey/comment", "/rest/api/2/issue/PROJ-1/comment", []string{"issueIdOrKey"}, []string{"PROJ-1"})

	err := HandleGetIssueComments(c, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected at least one error message")
	}
}

func TestSearchResponseParity_AllThreePaths(t *testing.T) {
	// Mock YouTrack server that returns consistent issue data
	ytIssues := []model.YTIssue{
		{
			ID:         "2-1",
			IDReadable: "TEST-1",
			Summary:    "Parity Test Issue",
			Created:    1700000000000,
			Updated:    1700001000000,
		},
	}

	mockYT := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ytIssues)
	}))
	defer mockYT.Close()

	cfg := &config.Config{YouTrackURL: mockYT.URL}

	// Set up a full Echo instance with the three search routes and auth middleware
	e := echo.New()
	api := e.Group("/rest/api/2", authmw.BasicAuth())
	api.GET("/search", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, testCache())
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, testCache())
	})

	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth())
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, testCache())
	})

	paths := []string{
		"/rest/api/2/search?jql=project+%3D+TEST&startAt=0&maxResults=10",
		"/rest/api/2/search/jql?jql=project+%3D+TEST&startAt=0&maxResults=10",
		"/rest/api/3/search/jql?jql=project+%3D+TEST&startAt=0&maxResults=10",
	}

	// Valid Basic Auth header: user@example.com:test-token
	authHeader := "Basic dXNlckBleGFtcGxlLmNvbTp0ZXN0LXRva2Vu"

	var responses []string
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %s returned status %d, expected 200", path, rec.Code)
		}

		// Verify response is valid JiraSearchResponse
		var resp model.JiraSearchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("path %s: failed to parse response: %v", path, err)
		}

		responses = append(responses, rec.Body.String())
	}

	// All three responses must be identical
	if responses[0] != responses[1] {
		t.Errorf("response from /rest/api/2/search differs from /rest/api/2/search/jql:\n  v2/search:     %s\n  v2/search/jql: %s", responses[0], responses[1])
	}
	if responses[0] != responses[2] {
		t.Errorf("response from /rest/api/2/search differs from /rest/api/3/search/jql:\n  v2/search:     %s\n  v3/search/jql: %s", responses[0], responses[2])
	}
}
