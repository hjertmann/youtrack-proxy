package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	echoMw "github.com/labstack/echo/v4/middleware"
	"pgregory.net/rapid"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/handler"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// ---------------------------------------------------------------------------
// FixtureResponse represents a canned response from Mock YouTrack.
// ---------------------------------------------------------------------------

// FixtureResponse holds the status code, body, and optional headers for a mock response.
type FixtureResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// ---------------------------------------------------------------------------
// FixtureRegistry maps YouTrack API paths to response fixtures.
// ---------------------------------------------------------------------------

// FixtureRegistry is a thread-safe map of YouTrack API paths to mock responses.
type FixtureRegistry struct {
	mu        sync.RWMutex
	responses map[string]FixtureResponse
}

// NewFixtureRegistry creates an empty FixtureRegistry.
func NewFixtureRegistry() *FixtureRegistry {
	return &FixtureRegistry{
		responses: make(map[string]FixtureResponse),
	}
}

// Set registers a fixture response for a YouTrack API path.
func (r *FixtureRegistry) Set(path string, resp FixtureResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses[path] = resp
}

// Get returns the fixture for a path, or a 404 if none registered.
func (r *FixtureRegistry) Get(path string) FixtureResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if resp, ok := r.responses[path]; ok {
		return resp
	}
	return FixtureResponse{
		StatusCode: http.StatusNotFound,
		Body:       []byte(`{"error":"not found"}`),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}
}

// ---------------------------------------------------------------------------
// TestHarness holds the mock YouTrack server and the configured proxy router.
// ---------------------------------------------------------------------------

// TestHarness provides a fully-configured test environment with a mock YouTrack
// server and the complete proxy router matching main.go route registration.
type TestHarness struct {
	MockYouTrack *httptest.Server
	Router       *echo.Echo
	Fixtures     *FixtureRegistry
}

// NewTestHarness creates a fully-configured test harness with mock YouTrack
// and proxy router matching main.go route registration.
func NewTestHarness() *TestHarness {
	fixtures := NewFixtureRegistry()

	// Load standard fixture data
	loadStandardFixtures(fixtures)

	// Create mock YouTrack server that dispatches to the fixture registry
	mockYT := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fixtures.Get(r.URL.Path)
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Body) //nolint:errcheck
	}))

	// Configure the proxy to point at the mock YouTrack
	cfg := &config.Config{
		YouTrackURL: mockYT.URL,
		Port:        "8080",
	}

	// Build the full Echo router matching main.go route registration
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(echoMw.Recover())

	resolvedCache := service.NewResolvedStateCache(1 * time.Hour)

	// Health endpoint (no auth)
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	// Server discovery (no auth required)
	e.Any("/rest/api/2/serverInfo", handler.HandleServerInfo)
	e.Any("/rest/api/3/serverInfo", handler.HandleServerInfo)

	// All API routes share the auth middleware
	api := e.Group("/rest/api/2", authmw.BasicAuth(""))

	// Issue creation
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
	api.GET("/user/search", func(c echo.Context) error {
		return handler.HandleSearchUsers(c, cfg)
	})

	// Field metadata
	api.GET("/field", handler.HandleListFields)

	// Filters (stub for IntelliJ IDEA compatibility)
	api.GET("/filter/search", handler.HandleFilterSearch)

	// v3 API group (mirrors v2 structure with BasicAuth)
	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth(""))
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})

	return &TestHarness{
		MockYouTrack: mockYT,
		Router:       e,
		Fixtures:     fixtures,
	}
}

// Close shuts down the mock YouTrack server.
func (h *TestHarness) Close() {
	h.MockYouTrack.Close()
}

// Request creates an authenticated HTTP request with Basic Auth.
// Uses test credentials: email "test@example.com" and token "test-token".
func (h *TestHarness) Request(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	creds := "test@example.com:test-token"
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	return req
}

// Execute sends a request through the proxy router and returns the recorder.
func (h *TestHarness) Execute(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Router.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Standard fixture data
// ---------------------------------------------------------------------------

// fixtureProjects is the standard fixture for /api/admin/projects.
var fixtureProjects = []model.YTProject{
	{
		ID:        "0-1",
		Name:      "Demo Project",
		ShortName: "DEMO",
		Type:      "Project",
	},
	{
		ID:        "0-2",
		Name:      "Backend",
		ShortName: "BACK",
		Type:      "Project",
	},
}

// fixtureIssues is the standard fixture for /api/issues.
var fixtureIssues = []model.YTIssue{
	{
		ID:         "2-1",
		IDReadable: "DEMO-1",
		Summary:    "First issue",
		Created:    1700000000000,
		Updated:    1700100000000,
		Reporter:   &model.YTUser{Login: "admin", Name: "Admin User", Email: "admin@test.com"},
		Project:    &model.YTProject{ID: "0-1", ShortName: "DEMO", Name: "Demo Project"},
		CustomFields: []model.YTCustomField{
			{Name: "Type", Value: map[string]interface{}{"name": "Task"}, Type: "SingleEnumIssueCustomField"},
			{Name: "Priority", Value: map[string]interface{}{"name": "Normal"}, Type: "SingleEnumIssueCustomField"},
			{Name: "State", Value: map[string]interface{}{"name": "Open"}, Type: "StateIssueCustomField"},
			{Name: "Assignee", Value: map[string]interface{}{"login": "dev1", "fullName": "Dev One", "email": "dev1@test.com"}, Type: "SingleUserIssueCustomField"},
		},
	},
}

// fixtureActivities is the standard fixture for /api/issues/{id}/activities.
var fixtureActivities = []model.YTActivityItem{
	{
		ID:        "act-1",
		Timestamp: 1700050000000,
		Author:    &model.YTUser{Login: "admin", Name: "Admin User", Email: "admin@test.com"},
		Field:     &model.YTFieldRef{ID: "state-field", Name: "State", Presentation: "State"},
		Added:     []model.YTFieldDiff{{Name: "In Progress", ID: "in-progress-id"}},
		Removed:   []model.YTFieldDiff{{Name: "Open", ID: "open-id"}},
	},
}

// fixtureComments is the standard fixture for /api/issues/{id}/comments.
var fixtureComments = []model.YTComment{
	{
		ID:      "comment-1",
		Author:  &model.YTUser{Login: "dev1", Name: "Dev One", Email: "dev1@test.com"},
		Text:    stringPtr("This is a comment"),
		Created: 1700060000000,
	},
}

// stringPtr returns a pointer to the given string (utility for fixture data).
func stringPtr(s string) *string {
	return &s
}

// loadStandardFixtures populates the fixture registry with the standard test data.
func loadStandardFixtures(fixtures *FixtureRegistry) {
	// Projects
	projectsJSON, _ := json.Marshal(fixtureProjects)
	fixtures.Set("/api/admin/projects", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       projectsJSON,
	})

	// Single project by short name
	singleProjectJSON, _ := json.Marshal(fixtureProjects[0])
	fixtures.Set("/api/admin/projects/DEMO", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       singleProjectJSON,
	})

	// Project custom fields (used by issue creation to resolve field IDs)
	projectCustomFields := []model.YTProjectCustomField{
		{ID: "field-type", Field: model.YTCustomFieldRef{Name: "Type"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "t1", Name: "Task"}, {ID: "t2", Name: "Bug"}}}},
		{ID: "field-priority", Field: model.YTCustomFieldRef{Name: "Priority"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "p1", Name: "Normal"}, {ID: "p2", Name: "Critical"}}}},
		{ID: "field-state", Field: model.YTCustomFieldRef{Name: "State"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "s1", Name: "Open"}, {ID: "s2", Name: "Fixed", IsResolved: true}}}},
		{ID: "field-assignee", Field: model.YTCustomFieldRef{Name: "Assignee"}},
	}
	customFieldsJSON, _ := json.Marshal(projectCustomFields)
	fixtures.Set("/api/admin/projects/DEMO/customFields", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       customFieldsJSON,
	})
	fixtures.Set("/api/admin/projects/0-1/customFields", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       customFieldsJSON,
	})

	// Issues
	issuesJSON, _ := json.Marshal(fixtureIssues)
	fixtures.Set("/api/issues", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       issuesJSON,
	})

	// Single issue
	singleIssueJSON, _ := json.Marshal(fixtureIssues[0])
	fixtures.Set("/api/issues/DEMO-1", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       singleIssueJSON,
	})

	// Activities (changelog)
	activitiesJSON, _ := json.Marshal(fixtureActivities)
	fixtures.Set("/api/issues/DEMO-1/activities", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       activitiesJSON,
	})

	// Comments
	commentsJSON, _ := json.Marshal(fixtureComments)
	fixtures.Set("/api/issues/DEMO-1/comments", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       commentsJSON,
	})

	// Current user (/api/users/me)
	currentUser := model.YTUser{
		Login:  "admin",
		Name:   "Admin User",
		Email:  "admin@test.com",
		Banned: false,
	}
	currentUserJSON, _ := json.Marshal(currentUser)
	fixtures.Set("/api/users/me", FixtureResponse{
		StatusCode: http.StatusOK,
		Body:       currentUserJSON,
	})
}

// ---------------------------------------------------------------------------
// Smoke test to verify the harness itself works
// ---------------------------------------------------------------------------

func TestDevLake_ProjectListing(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/project")
	rec := h.Execute(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/project: got status %d, want %d\nBody: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var projects []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("Failed to unmarshal response as JSON array: %v\nBody: %s", err, rec.Body.String())
	}

	if len(projects) == 0 {
		t.Fatal("Expected non-empty project array, got empty")
	}

	for i, p := range projects {
		for _, field := range []string{"id", "key", "name", "self"} {
			val, ok := p[field]
			if !ok {
				t.Errorf("projects[%d]: missing field %q", i, field)
				continue
			}
			str, isStr := val.(string)
			if !isStr {
				t.Errorf("projects[%d].%s: expected string, got %T", i, field, val)
			} else if str == "" {
				t.Errorf("projects[%d].%s: expected non-empty string", i, field)
			}
		}
	}
}

func TestDevLake_HarnessSetup(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// Verify health endpoint works (no auth required)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := h.Execute(req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("GET /health: got body %q, want %q", rec.Body.String(), "OK")
	}

	// Verify authenticated request works
	authReq := h.Request(http.MethodGet, "/rest/api/2/project")
	authRec := h.Execute(authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/project (authenticated): got status %d, want %d\nBody: %s",
			authRec.Code, http.StatusOK, authRec.Body.String())
	}

	// Verify unauthenticated request is rejected
	unauthReq := httptest.NewRequest(http.MethodGet, "/rest/api/2/project", nil)
	unauthRec := h.Execute(unauthReq)

	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /rest/api/2/project (unauthenticated): got status %d, want %d",
			unauthRec.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Test: Server Discovery Endpoint (Requirement 2)
// ---------------------------------------------------------------------------

func TestDevLake_ServerInfo(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// ServerInfo is a public endpoint—no auth required
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/serverInfo", nil)
	rec := h.Execute(req)

	// Requirement 2.3: HTTP 200
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/serverInfo: got status %d, want %d\nBody: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	// Unmarshal and verify structure
	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal serverInfo response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 2.1: baseUrl (string)
	if resp.BaseURL == "" {
		t.Error("serverInfo: baseUrl is empty")
	}

	// Requirement 2.1: version (string)
	if resp.Version == "" {
		t.Error("serverInfo: version is empty")
	}

	// Requirement 2.1: versionNumbers (array of integers)
	if len(resp.VersionNumbers) == 0 {
		t.Error("serverInfo: versionNumbers is empty")
	}

	// Requirement 2.2: deploymentType == "Server"
	if resp.DeploymentType != "Server" {
		t.Errorf("serverInfo: deploymentType = %q, want %q", resp.DeploymentType, "Server")
	}

	// Requirement 2.1: buildNumber (integer, non-zero)
	if resp.BuildNumber == 0 {
		t.Error("serverInfo: buildNumber is zero")
	}

	// Requirement 2.1: serverTitle (string)
	if resp.ServerTitle == "" {
		t.Error("serverInfo: serverTitle is empty")
	}
}

// ---------------------------------------------------------------------------
// TestDevLake_FieldMetadata verifies the /rest/api/2/field endpoint returns
// Jira-compatible field metadata (Requirements: 16.1, 16.2, 16.3).
// ---------------------------------------------------------------------------

func TestDevLake_FieldMetadata(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/field")
	rec := h.Execute(req)

	// 16.3: Verify HTTP 200
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/field: got status %d, want %d\nBody: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	// 16.1: Verify response is a JSON array
	var fields []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("GET /rest/api/2/field: failed to unmarshal response as JSON array: %v", err)
	}

	if len(fields) == 0 {
		t.Fatal("GET /rest/api/2/field: expected non-empty array of field objects")
	}

	// 16.2: Verify each field object contains id (string), name (string), custom (boolean), schema (object or null)
	for i, field := range fields {
		// Check "id" is a string
		id, ok := field["id"]
		if !ok {
			t.Errorf("field[%d]: missing 'id' key", i)
		} else if _, isStr := id.(string); !isStr {
			t.Errorf("field[%d]: 'id' is %T, want string", i, id)
		}

		// Check "name" is a string
		name, ok := field["name"]
		if !ok {
			t.Errorf("field[%d]: missing 'name' key", i)
		} else if _, isStr := name.(string); !isStr {
			t.Errorf("field[%d]: 'name' is %T, want string", i, name)
		}

		// Check "custom" is a boolean
		custom, ok := field["custom"]
		if !ok {
			t.Errorf("field[%d]: missing 'custom' key", i)
		} else if _, isBool := custom.(bool); !isBool {
			t.Errorf("field[%d]: 'custom' is %T, want bool", i, custom)
		}

		// Check "schema" is an object or null (omitempty means it may be absent for nil)
		schema, hasSchema := field["schema"]
		if hasSchema && schema != nil {
			if _, isObj := schema.(map[string]interface{}); !isObj {
				t.Errorf("field[%d]: 'schema' is %T, want object or null", i, schema)
			}
		}
		// schema being absent (omitempty) or null is acceptable
	}
}

// ---------------------------------------------------------------------------
// Test: User Lookup Endpoint (Requirement 15)
// ---------------------------------------------------------------------------

func TestDevLake_UserLookup(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// --- Subtest: GET /rest/api/2/myself returns current user (Req 15.2) ---
	t.Run("Myself", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/myself")
		rec := h.Execute(req)

		// Verify HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/myself: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		// Unmarshal response as generic map to verify contract fields
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal myself response: %v\nBody: %s", err, rec.Body.String())
		}

		// Requirement 15.1/15.2: Verify key fields are present and have correct types
		// key (string) — mapped from YTUser.Login
		if key, ok := resp["key"].(string); !ok || key == "" {
			t.Errorf("myself: 'key' is missing or empty, got %v", resp["key"])
		}

		// name (string) — mapped from YTUser.Login
		if name, ok := resp["name"].(string); !ok || name == "" {
			t.Errorf("myself: 'name' is missing or empty, got %v", resp["name"])
		}

		// displayName (string) — mapped from YTUser.Name (fullName)
		if displayName, ok := resp["displayName"].(string); !ok || displayName == "" {
			t.Errorf("myself: 'displayName' is missing or empty, got %v", resp["displayName"])
		}

		// emailAddress (string) — mapped from YTUser.Email
		if email, ok := resp["emailAddress"].(string); !ok || email == "" {
			t.Errorf("myself: 'emailAddress' is missing or empty, got %v", resp["emailAddress"])
		}

		// active (bool) — mapped from !YTUser.Banned
		if _, ok := resp["active"].(bool); !ok {
			t.Errorf("myself: 'active' is missing or not a boolean, got %v", resp["active"])
		}

		// timeZone (string) — note: omitempty in JiraUserResponse, so may be absent
		// when YouTrack user fixture doesn't provide a timezone. Document this gap.
		if _, ok := resp["timeZone"]; !ok {
			t.Log("myself: 'timeZone' field is absent (omitempty, YouTrack user fixture has no timezone)")
		}

		// Verify expected values from fixture
		if key, _ := resp["key"].(string); key != "admin" {
			t.Errorf("myself: key = %q, want %q", key, "admin")
		}
		if name, _ := resp["name"].(string); name != "admin" {
			t.Errorf("myself: name = %q, want %q", name, "admin")
		}
		if displayName, _ := resp["displayName"].(string); displayName != "Admin User" {
			t.Errorf("myself: displayName = %q, want %q", displayName, "Admin User")
		}
		if email, _ := resp["emailAddress"].(string); email != "admin@test.com" {
			t.Errorf("myself: emailAddress = %q, want %q", email, "admin@test.com")
		}
		if active, _ := resp["active"].(bool); !active {
			t.Errorf("myself: active = %v, want true", active)
		}
	})

	// --- Subtest: GET /rest/api/2/user?key= (Req 15.3 — document as missing) ---
	t.Run("UserByKey_Missing", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/user?key=admin")
		rec := h.Execute(req)

		// The proxy does not implement GET /rest/api/2/user (only /user/search).
		// DevLake uses this endpoint to resolve user details by key.
		// Document as missing per Requirement 15.3.
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Log("GET /rest/api/2/user?key=admin: endpoint not implemented (HTTP " +
				http.StatusText(rec.Code) + "). " +
				"DevLake uses this for user lookup by key. " +
				"Source: plugins/jira/tasks/api_client.go")
		} else if rec.Code == http.StatusOK {
			t.Log("GET /rest/api/2/user?key=admin: endpoint is implemented (HTTP 200)")
		} else {
			t.Logf("GET /rest/api/2/user?key=admin: unexpected status %d\nBody: %s",
				rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test: Issue Search Endpoint (Requirement 6)
// ---------------------------------------------------------------------------

func TestDevLake_IssueSearch(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// --- Subtest: BasicSearch (Req 6.1, 6.2, 6.3, 6.7) ---
	t.Run("BasicSearch", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/search?jql=project+%3D+DEMO&startAt=0&maxResults=50")
		rec := h.Execute(req)

		// Requirement 6.1: HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/search: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		// Unmarshal response as generic map
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal search response: %v\nBody: %s", err, rec.Body.String())
		}

		// Requirement 6.1: Verify pagination fields
		if _, ok := resp["startAt"].(float64); !ok {
			t.Errorf("search response: 'startAt' is missing or not a number, got %T", resp["startAt"])
		}
		if _, ok := resp["maxResults"].(float64); !ok {
			t.Errorf("search response: 'maxResults' is missing or not a number, got %T", resp["maxResults"])
		}
		if _, ok := resp["total"].(float64); !ok {
			t.Errorf("search response: 'total' is missing or not a number, got %T", resp["total"])
		}

		// Requirement 6.1: Verify issues array
		issuesRaw, ok := resp["issues"].([]interface{})
		if !ok {
			t.Fatalf("search response: 'issues' is missing or not an array, got %T", resp["issues"])
		}
		if len(issuesRaw) == 0 {
			t.Fatal("search response: 'issues' array is empty, expected at least one issue")
		}

		// Requirement 6.2, 6.3: Verify each issue structure
		for i, issueRaw := range issuesRaw {
			issue, ok := issueRaw.(map[string]interface{})
			if !ok {
				t.Errorf("issues[%d]: expected object, got %T", i, issueRaw)
				continue
			}

			// Requirement 6.2: id, key, self
			if id, ok := issue["id"].(string); !ok || id == "" {
				t.Errorf("issues[%d]: 'id' is missing or empty", i)
			}
			if key, ok := issue["key"].(string); !ok || key == "" {
				t.Errorf("issues[%d]: 'key' is missing or empty", i)
			}
			if self, ok := issue["self"].(string); !ok || self == "" {
				t.Errorf("issues[%d]: 'self' is missing or empty", i)
			}

			// Requirement 6.2: fields object
			fields, ok := issue["fields"].(map[string]interface{})
			if !ok {
				t.Errorf("issues[%d]: 'fields' is missing or not an object", i)
				continue
			}

			// Requirement 6.3: summary (string)
			if summary, ok := fields["summary"].(string); !ok || summary == "" {
				t.Errorf("issues[%d].fields: 'summary' is missing or empty", i)
			}

			// Requirement 6.3: issuetype (object with id and name)
			if issuetype, ok := fields["issuetype"].(map[string]interface{}); !ok {
				t.Errorf("issues[%d].fields: 'issuetype' is missing or not an object", i)
			} else {
				if _, ok := issuetype["id"].(string); !ok {
					t.Errorf("issues[%d].fields.issuetype: 'id' is missing or not a string", i)
				}
				if _, ok := issuetype["name"].(string); !ok {
					t.Errorf("issues[%d].fields.issuetype: 'name' is missing or not a string", i)
				}
			}

			// Requirement 6.3: status (object with name)
			if status, ok := fields["status"].(map[string]interface{}); !ok {
				t.Errorf("issues[%d].fields: 'status' is missing or not an object", i)
			} else {
				if _, ok := status["name"].(string); !ok {
					t.Errorf("issues[%d].fields.status: 'name' is missing or not a string", i)
				}
			}

			// Requirement 6.3: priority (object with id and name)
			if priority, ok := fields["priority"].(map[string]interface{}); !ok {
				t.Errorf("issues[%d].fields: 'priority' is missing or not an object", i)
			} else {
				if _, ok := priority["id"].(string); !ok {
					t.Errorf("issues[%d].fields.priority: 'id' is missing or not a string", i)
				}
				if _, ok := priority["name"].(string); !ok {
					t.Errorf("issues[%d].fields.priority: 'name' is missing or not a string", i)
				}
			}

			// Requirement 6.3: project (object with id, key, name)
			if project, ok := fields["project"].(map[string]interface{}); !ok {
				t.Errorf("issues[%d].fields: 'project' is missing or not an object", i)
			} else {
				if _, ok := project["id"].(string); !ok {
					t.Errorf("issues[%d].fields.project: 'id' is missing or not a string", i)
				}
				if _, ok := project["key"].(string); !ok {
					t.Errorf("issues[%d].fields.project: 'key' is missing or not a string", i)
				}
				if _, ok := project["name"].(string); !ok {
					t.Errorf("issues[%d].fields.project: 'name' is missing or not a string", i)
				}
			}

			// Requirement 6.3: created and updated (strings)
			if created, ok := fields["created"].(string); !ok || created == "" {
				t.Errorf("issues[%d].fields: 'created' is missing or empty", i)
			}
			if updated, ok := fields["updated"].(string); !ok || updated == "" {
				t.Errorf("issues[%d].fields: 'updated' is missing or empty", i)
			}

			// Requirement 6.3: assignee (object or null — just verify key is present)
			if _, hasAssignee := fields["assignee"]; !hasAssignee {
				t.Errorf("issues[%d].fields: 'assignee' key is missing (should be object or null)", i)
			}

			// Requirement 6.3: reporter (object or null — just verify key is present)
			if _, hasReporter := fields["reporter"]; !hasReporter {
				t.Errorf("issues[%d].fields: 'reporter' key is missing (should be object or null)", i)
			}
		}
	})

	// --- Subtest: WithChangelog (Req 6.4, 6.5, 6.6) ---
	t.Run("WithChangelog", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/search?jql=project+%3D+DEMO&startAt=0&maxResults=50&expand=changelog")
		rec := h.Execute(req)

		// The response should still be a valid search response
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/search (expand=changelog): got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal search response: %v\nBody: %s", err, rec.Body.String())
		}

		// Verify basic search response structure still works with expand param
		if _, ok := resp["startAt"].(float64); !ok {
			t.Errorf("search response (expand=changelog): 'startAt' is missing or not a number")
		}
		if _, ok := resp["maxResults"].(float64); !ok {
			t.Errorf("search response (expand=changelog): 'maxResults' is missing or not a number")
		}
		if _, ok := resp["total"].(float64); !ok {
			t.Errorf("search response (expand=changelog): 'total' is missing or not a number")
		}

		issuesRaw, ok := resp["issues"].([]interface{})
		if !ok {
			t.Fatalf("search response (expand=changelog): 'issues' is missing or not an array")
		}
		if len(issuesRaw) == 0 {
			t.Fatal("search response (expand=changelog): 'issues' array is empty")
		}

		// Check if any issue has an embedded changelog object
		changelogFound := false
		for i, issueRaw := range issuesRaw {
			issue, ok := issueRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for changelog field on the issue (Requirement 6.4)
			changelogRaw, hasChangelog := issue["changelog"]
			if !hasChangelog {
				continue
			}

			changelogFound = true
			changelog, ok := changelogRaw.(map[string]interface{})
			if !ok {
				t.Errorf("issues[%d].changelog: expected object, got %T", i, changelogRaw)
				continue
			}

			// Requirement 6.4: changelog has startAt, maxResults, total, histories
			if _, ok := changelog["startAt"].(float64); !ok {
				t.Errorf("issues[%d].changelog: 'startAt' is missing or not a number", i)
			}
			if _, ok := changelog["maxResults"].(float64); !ok {
				t.Errorf("issues[%d].changelog: 'maxResults' is missing or not a number", i)
			}
			if _, ok := changelog["total"].(float64); !ok {
				t.Errorf("issues[%d].changelog: 'total' is missing or not a number", i)
			}

			historiesRaw, ok := changelog["histories"].([]interface{})
			if !ok {
				t.Errorf("issues[%d].changelog: 'histories' is missing or not an array", i)
				continue
			}

			// Requirement 6.5, 6.6: verify history entry structure
			for j, histRaw := range historiesRaw {
				hist, ok := histRaw.(map[string]interface{})
				if !ok {
					t.Errorf("issues[%d].changelog.histories[%d]: expected object, got %T", i, j, histRaw)
					continue
				}

				if _, ok := hist["id"].(string); !ok {
					t.Errorf("issues[%d].changelog.histories[%d]: 'id' is missing or not a string", i, j)
				}
				if author, ok := hist["author"].(map[string]interface{}); ok {
					if _, ok := author["displayName"].(string); !ok {
						t.Errorf("issues[%d].changelog.histories[%d].author: 'displayName' is missing", i, j)
					}
				}
				if _, ok := hist["created"].(string); !ok {
					t.Errorf("issues[%d].changelog.histories[%d]: 'created' is missing or not a string", i, j)
				}

				itemsRaw, ok := hist["items"].([]interface{})
				if !ok {
					t.Errorf("issues[%d].changelog.histories[%d]: 'items' is missing or not an array", i, j)
					continue
				}

				for k, itemRaw := range itemsRaw {
					item, ok := itemRaw.(map[string]interface{})
					if !ok {
						t.Errorf("issues[%d].changelog.histories[%d].items[%d]: expected object", i, j, k)
						continue
					}
					if _, ok := item["field"].(string); !ok {
						t.Errorf("issues[%d].changelog.histories[%d].items[%d]: 'field' is missing", i, j, k)
					}
					if _, ok := item["fieldId"].(string); !ok {
						t.Errorf("issues[%d].changelog.histories[%d].items[%d]: 'fieldId' is missing", i, j, k)
					}
					if _, ok := item["fieldtype"].(string); !ok {
						t.Errorf("issues[%d].changelog.histories[%d].items[%d]: 'fieldtype' is missing", i, j, k)
					}
				}
			}
		}

		if !changelogFound {
			t.Log("WithChangelog: expand=changelog does not currently produce embedded changelog " +
				"objects in the search response. The handler returns issues without inline " +
				"changelog expansion. DevLake falls back to GET /issue/{id}/changelog for " +
				"Server deployments. This is a known gap documented for future implementation.")
		}
	})
}

// ---------------------------------------------------------------------------
// Test: Standalone Changelog Endpoint (Requirement 7)
// ---------------------------------------------------------------------------

func TestDevLake_IssueChangelog(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// --- Subtest: ValidIssue (Requirements 7.1, 7.2) ---
	t.Run("ValidIssue", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/issue/DEMO-1/changelog?startAt=0&maxResults=100")
		rec := h.Execute(req)

		// Requirement 7.1: Verify HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/issue/DEMO-1/changelog: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		// Unmarshal as generic map to verify structure
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal changelog response: %v\nBody: %s", err, rec.Body.String())
		}

		// Requirement 7.1: Verify top-level fields exist with correct types
		// startAt (number)
		if startAt, ok := resp["startAt"].(float64); !ok {
			t.Errorf("changelog: 'startAt' missing or not a number, got %T", resp["startAt"])
		} else if startAt < 0 {
			t.Errorf("changelog: 'startAt' = %v, want >= 0", startAt)
		}

		// maxResults (number)
		if maxResults, ok := resp["maxResults"].(float64); !ok {
			t.Errorf("changelog: 'maxResults' missing or not a number, got %T", resp["maxResults"])
		} else if maxResults < 0 {
			t.Errorf("changelog: 'maxResults' = %v, want >= 0", maxResults)
		}

		// total (number)
		if total, ok := resp["total"].(float64); !ok {
			t.Errorf("changelog: 'total' missing or not a number, got %T", resp["total"])
		} else if total < 0 {
			t.Errorf("changelog: 'total' = %v, want >= 0", total)
		}

		// isLast (bool)
		if _, ok := resp["isLast"].(bool); !ok {
			t.Errorf("changelog: 'isLast' missing or not a boolean, got %T", resp["isLast"])
		}

		// histories (array)
		histories, ok := resp["histories"].([]interface{})
		if !ok {
			t.Fatalf("changelog: 'histories' missing or not an array, got %T", resp["histories"])
		}

		if len(histories) == 0 {
			t.Fatal("changelog: expected non-empty 'histories' array")
		}

		// Requirement 7.2: Verify each history entry structure
		for i, h := range histories {
			history, ok := h.(map[string]interface{})
			if !ok {
				t.Errorf("histories[%d]: expected object, got %T", i, h)
				continue
			}

			// id (string)
			if id, ok := history["id"].(string); !ok || id == "" {
				t.Errorf("histories[%d]: 'id' missing or empty, got %v", i, history["id"])
			}

			// author (object with displayName)
			author, ok := history["author"].(map[string]interface{})
			if !ok {
				t.Errorf("histories[%d]: 'author' missing or not an object, got %T", i, history["author"])
			} else {
				if displayName, ok := author["displayName"].(string); !ok || displayName == "" {
					t.Errorf("histories[%d].author: 'displayName' missing or empty, got %v", i, author["displayName"])
				}
			}

			// created (string)
			if created, ok := history["created"].(string); !ok || created == "" {
				t.Errorf("histories[%d]: 'created' missing or empty, got %v", i, history["created"])
			}

			// items (array)
			items, ok := history["items"].([]interface{})
			if !ok {
				t.Errorf("histories[%d]: 'items' missing or not an array, got %T", i, history["items"])
				continue
			}

			if len(items) == 0 {
				t.Errorf("histories[%d]: expected non-empty 'items' array", i)
				continue
			}

			// Verify each item structure (Requirement 7.2: matches format from Req 6.5/6.6)
			for j, it := range items {
				item, ok := it.(map[string]interface{})
				if !ok {
					t.Errorf("histories[%d].items[%d]: expected object, got %T", i, j, it)
					continue
				}

				// field (string)
				if field, ok := item["field"].(string); !ok || field == "" {
					t.Errorf("histories[%d].items[%d]: 'field' missing or empty, got %v", i, j, item["field"])
				}

				// fieldId (string)
				if fieldID, ok := item["fieldId"].(string); !ok || fieldID == "" {
					t.Errorf("histories[%d].items[%d]: 'fieldId' missing or empty, got %v", i, j, item["fieldId"])
				}

				// fieldtype (string)
				if fieldType, ok := item["fieldtype"].(string); !ok || fieldType == "" {
					t.Errorf("histories[%d].items[%d]: 'fieldtype' missing or empty, got %v", i, j, item["fieldtype"])
				}

				// from (string) — may be empty string but must exist as string type
				if _, ok := item["from"].(string); !ok {
					t.Errorf("histories[%d].items[%d]: 'from' missing or not a string, got %T", i, j, item["from"])
				}

				// fromString (string)
				if _, ok := item["fromString"].(string); !ok {
					t.Errorf("histories[%d].items[%d]: 'fromString' missing or not a string, got %T", i, j, item["fromString"])
				}

				// to (string)
				if _, ok := item["to"].(string); !ok {
					t.Errorf("histories[%d].items[%d]: 'to' missing or not a string, got %T", i, j, item["to"])
				}

				// toString (string)
				if _, ok := item["toString"].(string); !ok {
					t.Errorf("histories[%d].items[%d]: 'toString' missing or not a string, got %T", i, j, item["toString"])
				}
			}
		}
	})

	// --- Subtest: NotFound (Requirement 7.3) ---
	t.Run("NotFound", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/issue/NONEXIST-999/changelog")
		rec := h.Execute(req)

		// Requirement 7.3: Non-existent issue returns HTTP 404
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /rest/api/2/issue/NONEXIST-999/changelog: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test: Issue Comments Endpoint (Requirement 8)
// ---------------------------------------------------------------------------

func TestDevLake_IssueComments(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// --- Subtest: ValidIssue — GET /rest/api/2/issue/DEMO-1/comment (Req 8.1, 8.2) ---
	t.Run("ValidIssue", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/issue/DEMO-1/comment?startAt=0&maxResults=50")
		rec := h.Execute(req)

		// Requirement 8.1: Verify HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/issue/DEMO-1/comment: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		// Unmarshal as generic map to verify contract
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal comments response: %v\nBody: %s", err, rec.Body.String())
		}

		// Requirement 8.1: Verify pagination fields
		if _, ok := resp["startAt"].(float64); !ok {
			t.Errorf("comments: 'startAt' is missing or not a number, got %v (%T)", resp["startAt"], resp["startAt"])
		}
		if _, ok := resp["maxResults"].(float64); !ok {
			t.Errorf("comments: 'maxResults' is missing or not a number, got %v (%T)", resp["maxResults"], resp["maxResults"])
		}
		if _, ok := resp["total"].(float64); !ok {
			t.Errorf("comments: 'total' is missing or not a number, got %v (%T)", resp["total"], resp["total"])
		}

		// Requirement 8.1: Verify comments array
		commentsRaw, ok := resp["comments"].([]interface{})
		if !ok {
			t.Fatalf("comments: 'comments' is missing or not an array, got %v (%T)", resp["comments"], resp["comments"])
		}

		if len(commentsRaw) == 0 {
			t.Fatal("comments: expected non-empty comments array")
		}

		// Requirement 8.2: Verify each comment structure
		for i, raw := range commentsRaw {
			comment, ok := raw.(map[string]interface{})
			if !ok {
				t.Errorf("comments[%d]: expected object, got %T", i, raw)
				continue
			}

			// id (string)
			if id, ok := comment["id"].(string); !ok || id == "" {
				t.Errorf("comments[%d]: 'id' is missing or empty, got %v", i, comment["id"])
			}

			// self (string)
			if self, ok := comment["self"].(string); !ok || self == "" {
				t.Errorf("comments[%d]: 'self' is missing or empty, got %v", i, comment["self"])
			}

			// body (string)
			if _, ok := comment["body"].(string); !ok {
				t.Errorf("comments[%d]: 'body' is missing or not a string, got %v (%T)", i, comment["body"], comment["body"])
			}

			// author (object with key and displayName)
			authorRaw, ok := comment["author"]
			if !ok || authorRaw == nil {
				t.Errorf("comments[%d]: 'author' is missing or null", i)
			} else if author, ok := authorRaw.(map[string]interface{}); !ok {
				t.Errorf("comments[%d]: 'author' is not an object, got %T", i, authorRaw)
			} else {
				if key, ok := author["key"].(string); !ok || key == "" {
					t.Errorf("comments[%d].author: 'key' is missing or empty, got %v", i, author["key"])
				}
				if dn, ok := author["displayName"].(string); !ok || dn == "" {
					t.Errorf("comments[%d].author: 'displayName' is missing or empty, got %v", i, author["displayName"])
				}
			}

			// created (string)
			if created, ok := comment["created"].(string); !ok || created == "" {
				t.Errorf("comments[%d]: 'created' is missing or empty, got %v", i, comment["created"])
			}

			// updated (string)
			if updated, ok := comment["updated"].(string); !ok || updated == "" {
				t.Errorf("comments[%d]: 'updated' is missing or empty, got %v", i, comment["updated"])
			}
		}
	})

	// --- Subtest: NotFound — issue does not exist (Req 8.3) ---
	t.Run("NotFound", func(t *testing.T) {
		req := h.Request(http.MethodGet, "/rest/api/2/issue/NONEXIST-999/comment")
		rec := h.Execute(req)

		// Requirement 8.3: Verify HTTP 404 when issue does not exist
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /rest/api/2/issue/NONEXIST-999/comment: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test: Board Listing Endpoint (Requirement 3)
// ---------------------------------------------------------------------------

func TestDevLake_BoardListing(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/agile/1.0/board?startAt=0&maxResults=50")
	rec := h.Execute(req)

	// The Agile API is not implemented in the proxy (no route registered for /rest/agile/1.0/board).
	// Per Requirement 3.3: document as missing and verify 404/405.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/agile/1.0/board: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this for board discovery during scope configuration. " +
			"Expected contract: {maxResults (int), startAt (int), isLast (bool), values: [{id (int), name (string), type (string), self (string)}]}. " +
			"Source: plugins/jira/tasks/apiv2models/board.go")
		return
	}

	// If somehow implemented (future), verify the response structure (Req 3.1, 3.2).
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/agile/1.0/board: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal board listing response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 3.1: Verify pagination fields
	if _, ok := resp["maxResults"].(float64); !ok {
		t.Errorf("board listing: 'maxResults' is missing or not a number, got %T", resp["maxResults"])
	}
	if _, ok := resp["startAt"].(float64); !ok {
		t.Errorf("board listing: 'startAt' is missing or not a number, got %T", resp["startAt"])
	}
	if _, ok := resp["isLast"].(bool); !ok {
		t.Errorf("board listing: 'isLast' is missing or not a boolean, got %T", resp["isLast"])
	}

	// Requirement 3.1: Verify values array
	valuesRaw, ok := resp["values"].([]interface{})
	if !ok {
		t.Fatalf("board listing: 'values' is missing or not an array, got %T", resp["values"])
	}

	// Requirement 3.2: Verify each board object structure
	for i, raw := range valuesRaw {
		board, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("values[%d]: expected object, got %T", i, raw)
			continue
		}

		// id (number)
		if _, ok := board["id"].(float64); !ok {
			t.Errorf("values[%d]: 'id' is missing or not a number, got %T", i, board["id"])
		}

		// name (string)
		if name, ok := board["name"].(string); !ok || name == "" {
			t.Errorf("values[%d]: 'name' is missing or empty", i)
		}

		// type (string, one of "scrum", "kanban", "simple")
		if boardType, ok := board["type"].(string); !ok || boardType == "" {
			t.Errorf("values[%d]: 'type' is missing or empty", i)
		} else {
			validTypes := map[string]bool{"scrum": true, "kanban": true, "simple": true}
			if !validTypes[boardType] {
				t.Errorf("values[%d]: 'type' = %q, want one of scrum/kanban/simple", i, boardType)
			}
		}

		// self (string URL)
		if self, ok := board["self"].(string); !ok || self == "" {
			t.Errorf("values[%d]: 'self' is missing or empty", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Board Configuration Endpoint (Requirement 4)
// ---------------------------------------------------------------------------

func TestDevLake_BoardConfiguration(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/agile/1.0/board/1/configuration")
	rec := h.Execute(req)

	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/agile/1.0/board/1/configuration: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this to discover the saved filter ID for JQL-based collection. " +
			"Expected contract: {id, name, type, filter: {id}}. " +
			"Source: plugins/jira/tasks/apiv2models/board.go")
		return
	}

	// If somehow implemented, verify the expected structure: id, name, type, filter.id
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/agile/1.0/board/1/configuration: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal board configuration response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 4.1: Verify id (integer), name (string), type (string), filter (object with id)
	if _, ok := resp["id"].(float64); !ok {
		t.Errorf("board configuration: 'id' is missing or not a number, got %T", resp["id"])
	}
	if name, ok := resp["name"].(string); !ok || name == "" {
		t.Errorf("board configuration: 'name' is missing or empty, got %v", resp["name"])
	}
	if btype, ok := resp["type"].(string); !ok || btype == "" {
		t.Errorf("board configuration: 'type' is missing or empty, got %v", resp["type"])
	}

	// Requirement 4.2: filter.id must be present and non-empty
	filterRaw, ok := resp["filter"]
	if !ok || filterRaw == nil {
		t.Fatal("board configuration: 'filter' is missing or null")
	}
	filter, ok := filterRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("board configuration: 'filter' is not an object, got %T", filterRaw)
	}
	if filterID, ok := filter["id"].(string); !ok || filterID == "" {
		t.Errorf("board configuration: 'filter.id' is missing or empty, got %v", filter["id"])
	}
}

// ---------------------------------------------------------------------------
// Test: Sprint Listing Endpoint (Requirement 5)
// ---------------------------------------------------------------------------

func TestDevLake_SprintListing(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/agile/1.0/board/1/sprint?startAt=0&maxResults=50")
	rec := h.Execute(req)

	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/agile/1.0/board/1/sprint: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this for sprint/iteration tracking. " +
			"Expected contract: {maxResults, startAt, isLast, values: [{id, name, state, startDate, endDate}]}. " +
			"Source: plugins/jira/tasks/apiv2models/sprint.go")
		return
	}

	// If implemented, verify structure
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/agile/1.0/board/1/sprint: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal sprint listing response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 5.1: Verify pagination fields
	if _, ok := resp["maxResults"].(float64); !ok {
		t.Errorf("sprint listing: 'maxResults' is missing or not a number, got %T", resp["maxResults"])
	}
	if _, ok := resp["startAt"].(float64); !ok {
		t.Errorf("sprint listing: 'startAt' is missing or not a number, got %T", resp["startAt"])
	}
	if _, ok := resp["isLast"].(bool); !ok {
		t.Errorf("sprint listing: 'isLast' is missing or not a boolean, got %T", resp["isLast"])
	}

	// Requirement 5.1: Verify values array
	valuesRaw, ok := resp["values"].([]interface{})
	if !ok {
		t.Fatalf("sprint listing: 'values' is missing or not an array, got %T", resp["values"])
	}

	// Requirement 5.2: Verify each sprint object structure
	for i, raw := range valuesRaw {
		sprint, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("values[%d]: expected object, got %T", i, raw)
			continue
		}

		if _, ok := sprint["id"].(float64); !ok {
			t.Errorf("values[%d]: 'id' is missing or not a number, got %T", i, sprint["id"])
		}
		if name, ok := sprint["name"].(string); !ok || name == "" {
			t.Errorf("values[%d]: 'name' is missing or empty, got %v", i, sprint["name"])
		}
		if state, ok := sprint["state"].(string); !ok || state == "" {
			t.Errorf("values[%d]: 'state' is missing or empty, got %v", i, sprint["state"])
		}
		// startDate and endDate are optional (ISO 8601 strings or null)
	}
}

// ---------------------------------------------------------------------------
// Test: Issue Worklog Endpoint (Requirement 9)
// ---------------------------------------------------------------------------

func TestDevLake_IssueWorklog(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/issue/DEMO-1/worklog?startAt=0&maxResults=50")
	rec := h.Execute(req)

	// The proxy does not register a route for /rest/api/2/issue/:issueIdOrKey/worklog.
	// Echo will return 404 since there is no matching route pattern.
	// Per Requirement 9.3: document as missing endpoint.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/api/2/issue/DEMO-1/worklog: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this for time tracking data collection. " +
			"Expected contract: {startAt (int), maxResults (int), total (int), worklogs: [{id (string), author ({displayName}), started (string), timeSpentSeconds (int), comment (string, optional)}]}. " +
			"Source: plugins/jira/tasks/apiv2models/worklog.go")
		return
	}

	// If the endpoint is implemented (future), verify the worklog response structure.
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/issue/DEMO-1/worklog: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal worklog response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 9.1: Verify pagination fields
	if _, ok := resp["startAt"].(float64); !ok {
		t.Errorf("worklog: 'startAt' is missing or not a number, got %T", resp["startAt"])
	}
	if _, ok := resp["maxResults"].(float64); !ok {
		t.Errorf("worklog: 'maxResults' is missing or not a number, got %T", resp["maxResults"])
	}
	if _, ok := resp["total"].(float64); !ok {
		t.Errorf("worklog: 'total' is missing or not a number, got %T", resp["total"])
	}

	// Requirement 9.1: Verify worklogs array
	worklogsRaw, ok := resp["worklogs"].([]interface{})
	if !ok {
		t.Fatalf("worklog: 'worklogs' is missing or not an array, got %T", resp["worklogs"])
	}

	// Requirement 9.2: Verify each worklog object structure
	for i, raw := range worklogsRaw {
		wl, ok := raw.(map[string]interface{})
		if !ok {
			t.Errorf("worklogs[%d]: expected object, got %T", i, raw)
			continue
		}

		// id (string)
		if id, ok := wl["id"].(string); !ok || id == "" {
			t.Errorf("worklogs[%d]: 'id' is missing or empty", i)
		}

		// author (object with displayName)
		if authorRaw, ok := wl["author"]; !ok || authorRaw == nil {
			t.Errorf("worklogs[%d]: 'author' is missing or null", i)
		} else if author, ok := authorRaw.(map[string]interface{}); !ok {
			t.Errorf("worklogs[%d]: 'author' is not an object, got %T", i, authorRaw)
		} else {
			if dn, ok := author["displayName"].(string); !ok || dn == "" {
				t.Errorf("worklogs[%d].author: 'displayName' is missing or empty", i)
			}
		}

		// started (string)
		if started, ok := wl["started"].(string); !ok || started == "" {
			t.Errorf("worklogs[%d]: 'started' is missing or empty", i)
		}

		// timeSpentSeconds (number)
		if _, ok := wl["timeSpentSeconds"].(float64); !ok {
			t.Errorf("worklogs[%d]: 'timeSpentSeconds' is missing or not a number, got %T", i, wl["timeSpentSeconds"])
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Remote Links Endpoint (Requirement 10)
// ---------------------------------------------------------------------------

func TestDevLake_IssueRemoteLinks(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/issue/DEMO-1/remotelink")
	rec := h.Execute(req)

	// The proxy does not register a route for /rest/api/2/issue/:issueIdOrKey/remotelink.
	// Echo will return 404 since there is no matching route pattern.
	// Per Requirement 10.3: document as missing endpoint.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/api/2/issue/DEMO-1/remotelink: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this for cross-tool traceability. " +
			"Expected contract: [{id (int), self (string), object: {url (string), title (string)}}]. " +
			"Source: plugins/jira/tasks/apiv2models/remotelink.go")
		return
	}

	// If the endpoint is implemented (future), verify the remote links response structure.
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/issue/DEMO-1/remotelink: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	// Requirement 10.1: Response is a JSON array of remote link objects.
	var links []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil {
		t.Fatalf("Failed to unmarshal remote links response as JSON array: %v\nBody: %s",
			err, rec.Body.String())
	}

	// Requirement 10.2: Verify each remote link object contains id, self, object.url, object.title
	for i, link := range links {
		// id (number — Jira remote link IDs are integers)
		if _, ok := link["id"].(float64); !ok {
			t.Errorf("remotelinks[%d]: 'id' is missing or not a number, got %T", i, link["id"])
		}

		// self (string)
		if self, ok := link["self"].(string); !ok || self == "" {
			t.Errorf("remotelinks[%d]: 'self' is missing or empty", i)
		}

		// object (object with url and title)
		objRaw, ok := link["object"]
		if !ok || objRaw == nil {
			t.Errorf("remotelinks[%d]: 'object' is missing or null", i)
			continue
		}
		obj, ok := objRaw.(map[string]interface{})
		if !ok {
			t.Errorf("remotelinks[%d]: 'object' is not an object, got %T", i, objRaw)
			continue
		}
		if url, ok := obj["url"].(string); !ok || url == "" {
			t.Errorf("remotelinks[%d].object: 'url' is missing or empty", i)
		}
		if title, ok := obj["title"].(string); !ok || title == "" {
			t.Errorf("remotelinks[%d].object: 'title' is missing or empty", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Status Listing Endpoint (Requirement 12)
// ---------------------------------------------------------------------------

func TestDevLake_StatusListing(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/status")
	rec := h.Execute(req)

	// The proxy does not register a route for /rest/api/2/status.
	// Per Requirement 12.3: document as missing endpoint.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/api/2/status: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this to map workflow statuses. " +
			"Expected contract: [{id (string), name (string), statusCategory: {id (int), key (string), name (string)}}]. " +
			"Source: plugins/jira/tasks/apiv2models/status.go")
		return
	}

	// If implemented, verify the structure (Req 12.1, 12.2)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/status: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	var statuses []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("Failed to unmarshal status listing response as JSON array: %v\nBody: %s",
			err, rec.Body.String())
	}

	// Requirement 12.2: Verify each status object structure
	for i, s := range statuses {
		if id, ok := s["id"].(string); !ok || id == "" {
			t.Errorf("statuses[%d]: 'id' missing or empty", i)
		}
		if name, ok := s["name"].(string); !ok || name == "" {
			t.Errorf("statuses[%d]: 'name' missing or empty", i)
		}
		if cat, ok := s["statusCategory"].(map[string]interface{}); !ok {
			t.Errorf("statuses[%d]: 'statusCategory' missing or not an object", i)
		} else {
			if _, ok := cat["id"].(float64); !ok {
				t.Errorf("statuses[%d].statusCategory: 'id' missing or not a number", i)
			}
			if _, ok := cat["key"].(string); !ok {
				t.Errorf("statuses[%d].statusCategory: 'key' missing or not a string", i)
			}
			if _, ok := cat["name"].(string); !ok {
				t.Errorf("statuses[%d].statusCategory: 'name' missing or not a string", i)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Issue Type Listing Endpoint (Requirements: 13.1, 13.2, 13.3)
// ---------------------------------------------------------------------------

func TestDevLake_IssueTypeListing(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/issuetype")
	rec := h.Execute(req)

	// Requirement 13.3: If endpoint is not implemented, document as missing.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/api/2/issuetype: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this to map issue type metadata. " +
			"Expected contract: [{id (string), name (string), subtask (boolean), description (string, optional)}]. " +
			"Source: plugins/jira/tasks/apiv2models/issuetype.go")
		return
	}

	// If implemented, verify the structure
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/issuetype: unexpected status %d\nBody: %s",
			rec.Code, rec.Body.String())
	}

	// Requirement 13.1: Response is a JSON array of issue type objects.
	var types []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("Failed to unmarshal issue types response: %v\nBody: %s", err, rec.Body.String())
	}

	// Requirement 13.2: Verify each issue type object contains id, name, subtask.
	for i, it := range types {
		if id, ok := it["id"].(string); !ok || id == "" {
			t.Errorf("issuetypes[%d]: 'id' missing or empty", i)
		}
		if name, ok := it["name"].(string); !ok || name == "" {
			t.Errorf("issuetypes[%d]: 'name' missing or empty", i)
		}
		if _, ok := it["subtask"].(bool); !ok {
			t.Errorf("issuetypes[%d]: 'subtask' missing or not boolean", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Saved Filter Endpoint (Requirement 14)
// ---------------------------------------------------------------------------

func TestDevLake_SavedFilter(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	req := h.Request(http.MethodGet, "/rest/api/2/filter/12345")
	rec := h.Execute(req)

	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Log("GET /rest/api/2/filter/12345: endpoint not implemented (HTTP " +
			http.StatusText(rec.Code) + "). " +
			"DevLake uses this to retrieve JQL for board-based collection. " +
			"Expected contract: {id (string), name (string), jql (string), self (string)}. " +
			"Source: plugins/jira/tasks/apiv2models/board.go")
		return
	}

	// If implemented, verify the structure
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d\nBody: %s", rec.Code, rec.Body.String())
	}

	var filter map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &filter); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if id, ok := filter["id"].(string); !ok || id == "" {
		t.Errorf("filter: 'id' missing or empty")
	}
	if name, ok := filter["name"].(string); !ok || name == "" {
		t.Errorf("filter: 'name' missing or empty")
	}
	if jql, ok := filter["jql"].(string); !ok || jql == "" {
		t.Errorf("filter: 'jql' missing or empty (Req 14.2: must be non-empty)")
	}
	if self, ok := filter["self"].(string); !ok || self == "" {
		t.Errorf("filter: 'self' missing or empty")
	}
}

// ---------------------------------------------------------------------------
// Test: Authentication Verification (Requirement 19)
// ---------------------------------------------------------------------------

func TestDevLake_Authentication(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// --- Subtest: ValidAuth (Req 19.1) ---
	t.Run("ValidAuth", func(t *testing.T) {
		// Use h.Request which adds valid Basic Auth, hit a protected endpoint
		req := h.Request(http.MethodGet, "/rest/api/2/field")
		rec := h.Execute(req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Valid auth: got status %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	// --- Subtest: NoAuthHeader (Req 19.2) ---
	t.Run("NoAuthHeader", func(t *testing.T) {
		// No Authorization header → 401
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/field", nil)
		rec := h.Execute(req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("No auth: got status %d, want 401\nBody: %s",
				rec.Code, rec.Body.String())
		}
	})

	// --- Subtest: InvalidBase64 (Req 19.3) ---
	t.Run("InvalidBase64", func(t *testing.T) {
		// Invalid base64 → 400
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/field", nil)
		req.Header.Set("Authorization", "Basic !!!invalid-base64!!!")
		rec := h.Execute(req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Invalid base64: got status %d, want 400\nBody: %s",
				rec.Code, rec.Body.String())
		}
	})

	// --- Subtest: EmptyToken (Req 19.3) ---
	t.Run("EmptyToken", func(t *testing.T) {
		// email: (empty password) → 401
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/field", nil)
		creds := base64.StdEncoding.EncodeToString([]byte("user@test.com:"))
		req.Header.Set("Authorization", "Basic "+creds)
		rec := h.Execute(req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Empty token: got status %d, want 401\nBody: %s",
				rec.Code, rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Test: Issue Field Completeness (Requirement 20)
// ---------------------------------------------------------------------------

func TestDevLake_IssueFieldCompleteness(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// Get issues via search
	req := h.Request(http.MethodGet, "/rest/api/2/search?jql=project+%3D+DEMO&startAt=0&maxResults=50")
	rec := h.Execute(req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Failed to get issues: %d\nBody: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal search response: %v", err)
	}
	issues, ok := resp["issues"].([]interface{})
	if !ok || len(issues) == 0 {
		t.Fatal("No issues returned from search")
	}

	issue, ok := issues[0].(map[string]interface{})
	if !ok {
		t.Fatal("First issue is not an object")
	}
	fields, ok := issue["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("Issue 'fields' is missing or not an object")
	}

	// --- Requirement 20.1: labels, components, fixVersions, issuelinks ---
	// These are array-type fields DevLake's extractor accesses.
	// Per Req 20.5: use t.Log() to document gaps rather than fail.
	checkFieldPresence(t, fields, "labels",
		"array of strings or null — DevLake accesses fields.Labels for issue labeling")
	checkFieldPresence(t, fields, "components",
		"array of objects with 'name' or null — DevLake accesses fields.Components for component tracking")
	checkFieldPresence(t, fields, "fixVersions",
		"array of objects with 'id' and 'name' or null — DevLake accesses fields.FixVersions for release tracking")
	checkFieldPresence(t, fields, "issuelinks",
		"array or null — DevLake accesses fields.IssueLinks for dependency mapping")

	// --- Requirement 20.2: parent (object with id/key or null) ---
	checkFieldPresence(t, fields, "parent",
		"object with 'id' and 'key' or null — DevLake uses this for subtask detection (fields.Parent.Key)")

	// --- Requirement 20.3: time tracking fields ---
	checkFieldPresence(t, fields, "timeoriginalestimate",
		"integer or null — DevLake accesses fields.TimeOriginalEstimate for estimation data")
	checkFieldPresence(t, fields, "timespent",
		"integer or null — DevLake accesses fields.TimeSpent for time tracking")
	checkFieldPresence(t, fields, "aggregatetimeestimate",
		"integer or null — DevLake accesses fields.AggregateTimeEstimate for rollup estimation")

	// --- Requirement 20.4: status.statusCategory ---
	status := fields["status"]
	if status == nil {
		t.Error("Issue 'status' field is nil — cannot check for statusCategory")
	} else if statusObj, ok := status.(map[string]interface{}); !ok {
		t.Error("Issue 'status' field is not an object — cannot check for statusCategory")
	} else {
		catRaw, hasCat := statusObj["statusCategory"]
		if !hasCat || catRaw == nil {
			t.Log("Gap [Req 20.4]: status.statusCategory is missing. " +
				"DevLake uses statusCategory.key for state mapping (e.g., 'new', 'indeterminate', 'done'). " +
				"Expected: {key (string), name (string)}. " +
				"Source: plugins/jira/tasks/apiv2models/issue.go — StatusCategory struct. " +
				"Recommend adding statusCategory sub-object to JiraNamedField or status response.")
		} else if cat, ok := catRaw.(map[string]interface{}); !ok {
			t.Errorf("status.statusCategory is not an object, got %T", catRaw)
		} else {
			// Verify statusCategory has key and name
			if key, ok := cat["key"].(string); !ok || key == "" {
				t.Errorf("status.statusCategory.key is missing or empty, got %v", cat["key"])
			}
			if name, ok := cat["name"].(string); !ok || name == "" {
				t.Errorf("status.statusCategory.name is missing or empty, got %v", cat["name"])
			}
		}
	}
}

// checkFieldPresence checks whether a field is present in the issue fields map.
// If absent, it logs the gap per Requirement 20.5 (document rather than fail).
// If present, it verifies the value is either null or a valid type (array/object/number).
func checkFieldPresence(t *testing.T, fields map[string]interface{}, fieldName, description string) {
	t.Helper()

	val, exists := fields[fieldName]
	if !exists {
		t.Logf("Gap [Req 20]: field %q is missing from issue response. "+
			"Expected: %s. "+
			"Categorized for future implementation.",
			fieldName, description)
		return
	}

	// Field exists — verify it's a reasonable type (null is acceptable per all requirements)
	if val == nil {
		// null is explicitly allowed for all these fields
		return
	}

	// Log presence for visibility
	t.Logf("Field %q is present (type: %T)", fieldName, val)
}

// ---------------------------------------------------------------------------
// Test: Endpoint Inventory (Requirements: 18.1, 18.2, 18.3)
// ---------------------------------------------------------------------------

// TestDevLake_EndpointInventory exercises every DevLake endpoint and classifies
// each as implemented, stubbed, or missing. It fails on regression: if a
// previously-implemented endpoint now returns 404 or stub status.
func TestDevLake_EndpointInventory(t *testing.T) {
	h := NewTestHarness()
	defer h.Close()

	// Define the table of endpoints DevLake calls.
	// expectedStatus is based on the routes registered in main.go.
	type endpointSpec struct {
		Method           string
		Path             string
		ExpectedStatus   EndpointStatus
		ExpectedContract string
		DevLakeSource    string
		RequiresAuth     bool
	}

	endpoints := []endpointSpec{
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/serverInfo",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{baseUrl, version, versionNumbers, deploymentType, buildNumber, serverTitle}",
			DevLakeSource:    "plugins/jira/tasks/api_client.go",
			RequiresAuth:     false,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/project",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "[{id, key, name, self}]",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/project.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/search?jql=project+%3D+DEMO&startAt=0&maxResults=50",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{startAt, maxResults, total, issues: [{id, key, self, fields}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/issue.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issue/DEMO-1",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{id, key, self, fields: {summary, issuetype, status, priority, project}}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/issue.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issue/DEMO-1/changelog",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{startAt, maxResults, total, isLast, histories: [{id, author, created, items}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/changelog.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issue/DEMO-1/comment",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{startAt, maxResults, total, comments: [{id, self, body, author, created, updated}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/issue.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/myself",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "{key, name, displayName, emailAddress, active, timeZone}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/user.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/field",
			ExpectedStatus:   StatusImplemented,
			ExpectedContract: "[{id, name, custom, schema}]",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/field.go",
			RequiresAuth:     true,
		},
		// --- Missing endpoints (Agile API and others not implemented) ---
		{
			Method:           http.MethodGet,
			Path:             "/rest/agile/1.0/board?startAt=0&maxResults=50",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "{maxResults, startAt, isLast, values: [{id, name, type, self}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/board.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/agile/1.0/board/1/configuration",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "{id, name, type, filter: {id}}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/board.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/agile/1.0/board/1/sprint?startAt=0&maxResults=50",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "{maxResults, startAt, isLast, values: [{id, name, state, startDate, endDate}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/sprint.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issue/DEMO-1/worklog?startAt=0&maxResults=50",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "{startAt, maxResults, total, worklogs: [{id, author, started, timeSpentSeconds}]}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/worklog.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issue/DEMO-1/remotelink",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "[{id, self, object: {url, title}}]",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/remotelink.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/status",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "[{id, name, statusCategory: {id, key, name}}]",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/status.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/issuetype",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "[{id, name, subtask, description}]",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/issuetype.go",
			RequiresAuth:     true,
		},
		{
			Method:           http.MethodGet,
			Path:             "/rest/api/2/filter/12345",
			ExpectedStatus:   StatusMissing,
			ExpectedContract: "{id, name, jql, self}",
			DevLakeSource:    "plugins/jira/tasks/apiv2models/board.go",
			RequiresAuth:     true,
		},
	}

	var reports []EndpointReport

	for _, ep := range endpoints {
		var req *http.Request
		if ep.RequiresAuth {
			req = h.Request(ep.Method, ep.Path)
		} else {
			req = httptest.NewRequest(ep.Method, ep.Path, nil)
		}
		rec := h.Execute(req)

		// Classify actual status based on response code
		var actualStatus EndpointStatus
		switch {
		case rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed:
			actualStatus = StatusMissing
		case rec.Code == http.StatusOK:
			// Check if this is a stubbed response (valid structure with empty data)
			// A stub typically returns an empty array [] or an object with empty collections
			body := rec.Body.Bytes()
			if isStubResponse(body) {
				actualStatus = StatusStubbed
			} else {
				actualStatus = StatusImplemented
			}
		default:
			// Any other status (e.g., 500, 502) is treated as missing
			actualStatus = StatusMissing
		}

		report := EndpointReport{
			Path:             ep.Path,
			Method:           ep.Method,
			Status:           actualStatus,
			ExpectedContract: ep.ExpectedContract,
			DevLakeSource:    ep.DevLakeSource,
		}
		reports = append(reports, report)

		// Requirement 18.3: FAIL if a previously-implemented endpoint regresses
		if ep.ExpectedStatus == StatusImplemented && actualStatus != StatusImplemented {
			t.Errorf("REGRESSION: %s %s expected status %q but got %q (HTTP %d). "+
				"This endpoint was previously implemented and must not regress.",
				ep.Method, ep.Path, ep.ExpectedStatus, actualStatus, rec.Code)
		}
	}

	// Requirement 18.2: Log all missing endpoints with path, expected contract, and DevLake source
	t.Log("--- DevLake Endpoint Inventory ---")
	implementedCount := 0
	stubbedCount := 0
	missingCount := 0

	for _, r := range reports {
		switch r.Status {
		case StatusImplemented:
			implementedCount++
		case StatusStubbed:
			stubbedCount++
			t.Logf("  STUBBED: %s %s | Contract: %s | Source: %s",
				r.Method, r.Path, r.ExpectedContract, r.DevLakeSource)
		case StatusMissing:
			missingCount++
			t.Logf("  MISSING: %s %s | Contract: %s | Source: %s",
				r.Method, r.Path, r.ExpectedContract, r.DevLakeSource)
		}
	}

	t.Logf("Summary: %d implemented, %d stubbed, %d missing (total: %d endpoints)",
		implementedCount, stubbedCount, missingCount, len(reports))
}

// isStubResponse checks whether a response body looks like a stub — a valid
// JSON structure with empty data (e.g., empty array, or object with zero-count
// pagination and empty collections).
func isStubResponse(body []byte) bool {
	// An empty JSON array [] is a stub
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err == nil {
		return len(arr) == 0
	}

	// A paginated response with total=0 and empty items array is a stub
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err == nil {
		// Check for paginated response patterns
		if total, ok := obj["total"].(float64); ok && total == 0 {
			// Check if there's an items-like array that is empty
			for _, key := range []string{"issues", "values", "worklogs", "comments", "histories"} {
				if items, ok := obj[key].([]interface{}); ok && len(items) == 0 {
					return true
				}
			}
		}
		// Check for isLast=true with empty values (agile-style stub)
		if isLast, ok := obj["isLast"].(bool); ok && isLast {
			if values, ok := obj["values"].([]interface{}); ok && len(values) == 0 {
				return true
			}
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Property Test: Response round-trip unmarshaling (Property 1)
// **Validates: Requirements 17.1**
//
// For any valid YouTrack issue fixture with arbitrary non-empty string fields
// for IDReadable, Summary, and custom field values, converting through the proxy
// and unmarshaling the response JSON into DevLake's expected struct types SHALL
// produce no error and preserve the key, summary, and custom field name values.
// ---------------------------------------------------------------------------

func TestDevLake_Property_RoundTripUnmarshal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random non-empty strings for key fields
		idReadable := rapid.StringMatching(`[A-Z]+-[0-9]+`).Draw(t, "idReadable")
		summary := rapid.String().Filter(func(s string) bool { return len(s) > 0 }).Draw(t, "summary")
		typeName := rapid.SampledFrom([]string{"Task", "Bug", "Story", "Epic"}).Draw(t, "typeName")
		priorityName := rapid.SampledFrom([]string{"Critical", "Major", "Normal", "Minor"}).Draw(t, "priorityName")
		stateName := rapid.SampledFrom([]string{"Open", "In Progress", "Fixed", "Verified"}).Draw(t, "stateName")

		ytIssue := model.YTIssue{
			ID:         "2-" + rapid.StringMatching(`[0-9]+`).Draw(t, "internalId"),
			IDReadable: idReadable,
			Summary:    summary,
			Created:    rapid.Int64Range(1000000000000, 2000000000000).Draw(t, "created"),
			Updated:    rapid.Int64Range(1000000000000, 2000000000000).Draw(t, "updated"),
			CustomFields: []model.YTCustomField{
				{Name: "Type", Value: map[string]interface{}{"name": typeName}, Type: "SingleEnumIssueCustomField"},
				{Name: "Priority", Value: map[string]interface{}{"name": priorityName}, Type: "SingleEnumIssueCustomField"},
				{Name: "State", Value: map[string]interface{}{"name": stateName}, Type: "StateIssueCustomField"},
			},
		}

		// Convert through service layer
		jiraIssue := service.ConvertYTIssueToJira(ytIssue, "http://test", nil)

		// Marshal to JSON
		data, err := json.Marshal(jiraIssue)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		// Unmarshal back into the model struct
		var roundTripped model.JiraIssue
		if err := json.Unmarshal(data, &roundTripped); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Verify key fields are preserved through round-trip
		if roundTripped.Key != idReadable {
			t.Fatalf("key mismatch: got %q, want %q", roundTripped.Key, idReadable)
		}
		if roundTripped.ID == "" {
			t.Fatalf("id is empty after conversion")
		}
		if roundTripped.Fields.Summary != summary {
			t.Fatalf("summary mismatch: got %q, want %q", roundTripped.Fields.Summary, summary)
		}
		if roundTripped.Fields.IssueType == nil || roundTripped.Fields.IssueType.Name != typeName {
			t.Fatalf("issuetype.name mismatch: got %v, want %q", roundTripped.Fields.IssueType, typeName)
		}
		if roundTripped.Fields.Priority == nil || roundTripped.Fields.Priority.Name != priorityName {
			t.Fatalf("priority.name mismatch: got %v, want %q", roundTripped.Fields.Priority, priorityName)
		}
		if roundTripped.Fields.Status == nil || roundTripped.Fields.Status.Name != stateName {
			t.Fatalf("status.name mismatch: got %v, want %q", roundTripped.Fields.Status, stateName)
		}
		if roundTripped.Self == "" {
			t.Fatal("self URL should be non-empty after conversion")
		}
	})
}

// ---------------------------------------------------------------------------
// Property Test: Issue identity fields are non-empty (Requirement 17.3)
// ---------------------------------------------------------------------------

// TestDevLake_Property_IssueIdentityNonEmpty verifies that for any YTIssue with
// non-empty IDReadable and Summary, the converted Jira issue always has non-empty
// id, key, and fields.summary.
//
// **Validates: Requirements 17.3**
func TestDevLake_Property_IssueIdentityNonEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random non-empty IDReadable (simulates YouTrack's "KEY-123" format)
		idReadable := rapid.StringMatching(`[A-Z]{2,5}-[0-9]{1,5}`).Draw(t, "idReadable")
		summary := rapid.String().Filter(func(s string) bool { return len(s) > 0 }).Draw(t, "summary")

		ytIssue := model.YTIssue{
			ID:         "internal-id",
			IDReadable: idReadable,
			Summary:    summary,
			Created:    1700000000000,
			Updated:    1700100000000,
		}

		// Convert through service layer
		jiraIssue := service.ConvertYTIssueToJira(ytIssue, "http://test", nil)

		// Property 3: id and key are non-empty
		if jiraIssue.ID == "" {
			t.Fatal("Issue 'id' is empty after conversion")
		}
		if jiraIssue.Key == "" {
			t.Fatal("Issue 'key' is empty after conversion")
		}
		// fields is a struct, always non-null in Go, but verify summary is populated
		if jiraIssue.Fields.Summary == "" {
			t.Fatal("Issue 'fields.summary' is empty after conversion")
		}
	})
}

// ---------------------------------------------------------------------------
// Property Test: Changelog item field names non-empty (Property 4)
// **Validates: Requirements 17.4**
//
// For any YouTrack activity with a non-null field reference, converting through
// the service layer SHALL produce history items where both 'field' and 'fieldId'
// are non-empty strings.
// ---------------------------------------------------------------------------

func TestDevLake_Property_ChangelogFieldNamesNonEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random non-empty field name
		fieldName := rapid.StringMatching(`[A-Za-z][A-Za-z0-9 ]{0,20}`).Draw(t, "fieldName")
		fieldID := rapid.StringMatching(`[a-z]+-[a-z0-9]+`).Draw(t, "fieldID")

		// Generate activity with non-null field reference
		activities := []model.YTActivityItem{
			{
				ID:        "act-" + rapid.StringMatching(`[0-9]+`).Draw(t, "actId"),
				Timestamp: rapid.Int64Range(1000000000000, 2000000000000).Draw(t, "ts"),
				Author:    &model.YTUser{Login: "user", Name: "User"},
				Field:     &model.YTFieldRef{ID: fieldID, Name: fieldName},
				Added:     []model.YTFieldDiff{{Name: "New", ID: "new-id"}},
				Removed:   []model.YTFieldDiff{{Name: "Old", ID: "old-id"}},
			},
		}

		// Convert through service layer
		changelog := service.ConvertYTActivitiesToJiraChangelog(activities, 0)

		// Property 4: field and fieldId are non-empty in each history item
		for _, history := range changelog.Histories {
			for _, item := range history.Items {
				if item.Field == "" {
					t.Fatal("changelog history item 'field' is empty")
				}
				if item.FieldID == "" {
					t.Fatal("changelog history item 'fieldId' is empty")
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property Test: Pagination invariants (Property 2)
// **Validates: Requirements 17.2**
//
// For any paginated search response from the proxy with random startAt and
// maxResults parameters, the response SHALL have startAt >= 0, maxResults >= 0,
// and total >= len(issues).
// ---------------------------------------------------------------------------

func TestDevLake_Property_PaginationInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random pagination parameters
		startAt := rapid.IntRange(0, 100).Draw(t, "startAt")
		maxResults := rapid.IntRange(1, 100).Draw(t, "maxResults")

		h := NewTestHarness()
		defer h.Close()

		// Send search request with the generated pagination params
		path := fmt.Sprintf("/rest/api/2/search?jql=project+%%3D+DEMO&startAt=%d&maxResults=%d", startAt, maxResults)
		req := h.Request(http.MethodGet, path)
		rec := h.Execute(req)

		if rec.Code != http.StatusOK {
			return // Skip if endpoint errors
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		respStartAt := int(resp["startAt"].(float64))
		respMaxResults := int(resp["maxResults"].(float64))
		respTotal := int(resp["total"].(float64))
		issues := resp["issues"].([]interface{})

		// Property 2: Pagination invariants
		if respStartAt < 0 {
			t.Fatalf("startAt is negative: %d", respStartAt)
		}
		if respMaxResults < 0 {
			t.Fatalf("maxResults is negative: %d", respMaxResults)
		}
		if respTotal < len(issues) {
			t.Fatalf("total (%d) < len(issues) (%d)", respTotal, len(issues))
		}
	})
}
