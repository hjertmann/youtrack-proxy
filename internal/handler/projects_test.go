package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// setupProjectTestContext creates an Echo context with the requestCtx already
// set (simulating the auth middleware) and returns the context plus the recorder.
func setupProjectTestContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	return c, rec
}

// TestHandleListProjects_Success verifies that when the mock YouTrack API returns
// a list of projects, the handler responds with 200 and properly formatted Jira projects.
func TestHandleListProjects_Success(t *testing.T) {
	// Arrange: mock YouTrack server returning two projects
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request hits the right endpoint
		if r.URL.Path != "/api/admin/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify auth header is forwarded
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := `[
			{
				"id": "0-1",
				"name": "Project Alpha",
				"shortName": "PA",
				"description": "Alpha project description",
				"leader": {"login": "admin", "fullName": "Admin User", "email": "admin@test.com", "banned": false},
				"$type": "Project"
			},
			{
				"id": "0-2",
				"name": "Project Beta",
				"shortName": "PB",
				"description": null,
				"leader": null,
				"$type": "Project"
			}
		]`
		w.Write([]byte(resp))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupProjectTestContext(http.MethodGet, "/rest/api/2/project")

	// Act
	err := HandleListProjects(c, cfg)

	// Assert
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var projects []model.JiraProject
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}

	// First project assertions
	if projects[0].Id != "1" {
		t.Errorf("projects[0].Id = %q, want %q", projects[0].Id, "1")
	}
	if projects[0].Key != "PA" {
		t.Errorf("projects[0].Key = %q, want %q", projects[0].Key, "PA")
	}
	if projects[0].Name != "Project Alpha" {
		t.Errorf("projects[0].Name = %q, want %q", projects[0].Name, "Project Alpha")
	}
	if projects[0].Description != "Alpha project description" {
		t.Errorf("projects[0].Description = %q, want %q", projects[0].Description, "Alpha project description")
	}
	if projects[0].Lead == nil {
		t.Fatal("projects[0].Lead is nil")
	}
	if projects[0].Lead.Key != "admin" {
		t.Errorf("projects[0].Lead.Key = %q, want %q", projects[0].Lead.Key, "admin")
	}
	if projects[0].Lead.DisplayName != "Admin User" {
		t.Errorf("projects[0].Lead.DisplayName = %q, want %q", projects[0].Lead.DisplayName, "Admin User")
	}

	// Second project: null description → empty string, null leader → nil
	if projects[1].Id != "2" {
		t.Errorf("projects[1].Id = %q, want %q", projects[1].Id, "2")
	}
	if projects[1].Key != "PB" {
		t.Errorf("projects[1].Key = %q, want %q", projects[1].Key, "PB")
	}
	if projects[1].Description != "" {
		t.Errorf("projects[1].Description = %q, want empty string", projects[1].Description)
	}
	if projects[1].Lead != nil {
		t.Errorf("projects[1].Lead = %v, want nil", projects[1].Lead)
	}

	// Verify self URL contains the request host
	if projects[0].Self == "" {
		t.Error("projects[0].Self is empty, expected a URL")
	}
}

// TestHandleListProjects_UpstreamError verifies that when the YouTrack API returns
// a 500 error, the handler responds with 502 and a Jira error response.
func TestHandleListProjects_UpstreamError(t *testing.T) {
	// Arrange: mock server returns 500
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupProjectTestContext(http.MethodGet, "/rest/api/2/project")

	// Act
	err := HandleListProjects(c, cfg)

	// Assert
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("errorMessages is empty, expected at least one message")
	}
}

// TestHandleGetProject_Success verifies that the handler returns a single project
// in Jira format when the YouTrack API returns a valid project response.
func TestHandleGetProject_Success(t *testing.T) {
	// Arrange: mock server returns a single project
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/projects/TP" {
			t.Errorf("unexpected path: %s, want /api/admin/projects/TP", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := `{
			"id": "0-5",
			"name": "Test Project",
			"shortName": "TP",
			"description": "A test project",
			"leader": {"login": "lead", "fullName": "Lead User", "email": "lead@test.com", "banned": false},
			"$type": "Project"
		}`
		w.Write([]byte(resp))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/TP", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	c.SetParamNames("projectIdOrKey")
	c.SetParamValues("TP")

	// Act
	err := HandleGetProject(c, cfg)

	// Assert
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var project model.JiraProject
	if err := json.Unmarshal(rec.Body.Bytes(), &project); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if project.Id != "5" {
		t.Errorf("Id = %q, want %q", project.Id, "5")
	}
	if project.Key != "TP" {
		t.Errorf("Key = %q, want %q", project.Key, "TP")
	}
	if project.Name != "Test Project" {
		t.Errorf("Name = %q, want %q", project.Name, "Test Project")
	}
	if project.Description != "A test project" {
		t.Errorf("Description = %q, want %q", project.Description, "A test project")
	}
	if project.Lead == nil {
		t.Fatal("Lead is nil")
	}
	if project.Lead.Key != "lead" {
		t.Errorf("Lead.Key = %q, want %q", project.Lead.Key, "lead")
	}
	if project.Lead.DisplayName != "Lead User" {
		t.Errorf("Lead.DisplayName = %q, want %q", project.Lead.DisplayName, "Lead User")
	}
	if project.Lead.Active != true {
		t.Errorf("Lead.Active = %v, want true", project.Lead.Active)
	}
}

// TestHandleGetProject_NotFound verifies that when the YouTrack API returns 404,
// the handler responds with 404 and a Jira error response.
func TestHandleGetProject_NotFound(t *testing.T) {
	// Arrange: mock server returns 404
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Entity not found"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/NONEXISTENT", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	c.SetParamNames("projectIdOrKey")
	c.SetParamValues("NONEXISTENT")

	// Act
	err := HandleGetProject(c, cfg)

	// Assert
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("errorMessages is empty, expected at least one message")
	}
	// Verify the error message mentions "not found"
	found := false
	for _, msg := range errResp.ErrorMessages {
		if msg == "Project not found" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error message 'Project not found', got %v", errResp.ErrorMessages)
	}
}

// TestHandleGetProject_UpstreamError verifies that when the YouTrack API returns
// a 500 server error, the handler responds with 502 and a Jira error response.
func TestHandleGetProject_UpstreamError(t *testing.T) {
	// Arrange: mock server returns 500
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/TP", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	c.SetParamNames("projectIdOrKey")
	c.SetParamValues("TP")

	// Act
	err := HandleGetProject(c, cfg)

	// Assert
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if len(errResp.ErrorMessages) == 0 {
		t.Error("errorMessages is empty, expected at least one message")
	}
}
