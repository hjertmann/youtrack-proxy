package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/handler"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
)

// setupRouter creates an Echo instance with the same route configuration as main.go
// for integration testing purposes.
func setupRouter() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	cfg := &config.Config{YouTrackURL: "http://localhost:9999"}
	resolvedCache := service.NewResolvedStateCache(1 * time.Hour)

	// v2 API group with auth
	api := e.Group("/rest/api/2", authmw.BasicAuth())
	api.GET("/search", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})

	// v3 API group with auth
	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth())
	apiv3.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})

	return e
}

// setupProjectRouter creates an Echo instance that mirrors main.go's project route
// registration for integration testing. It uses the provided YouTrack URL so tests
// can point it at a mock server.
func setupProjectRouter(youtrackURL string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	cfg := &config.Config{YouTrackURL: youtrackURL}

	api := e.Group("/rest/api/2", authmw.BasicAuth())

	// Projects — same registration order as main.go (WITH /project/recent route)
	api.GET("/project", func(c echo.Context) error {
		return handler.HandleListProjects(c, cfg)
	})
	api.GET("/project/recent", func(c echo.Context) error {
		return handler.HandleRecentProjects(c, cfg)
	})
	api.GET("/project/:projectIdOrKey", func(c echo.Context) error {
		return handler.HandleGetProject(c, cfg)
	})

	return e
}

// basicAuthHeader returns a Basic Auth header value for the given email and token.
func basicAuthHeader(email, token string) string {
	creds := email + ":" + token
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func TestAuthEnforcement_V3SearchJQL_Unauthenticated(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/rest/api/3/search/jql?jql=project+%3D+TEST", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for unauthenticated request to /rest/api/3/search/jql, got %d", rec.Code)
	}
}

func TestAuthEnforcement_V2SearchJQL_Unauthenticated(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/search/jql?jql=project+%3D+TEST", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for unauthenticated request to /rest/api/2/search/jql, got %d", rec.Code)
	}
}

// TestBugCondition_RecentEndpointReturns200 is a bug condition exploration test.
// It asserts the EXPECTED (correct) behavior: GET /rest/api/2/project/recent should
// return HTTP 200 with a JSON array of Jira projects identical to GET /project.
//
// On UNFIXED code this test MUST FAIL because no static route exists for /project/recent.
// Echo routes the request to /project/:projectIdOrKey with projectIdOrKey="recent",
// which causes a YouTrack lookup for a non-existent project key "recent", returning 404.
//
// Validates: Requirements 1.1, 1.2, 2.1, 2.2
func TestBugCondition_RecentEndpointReturns200(t *testing.T) {
	// Arrange: mock YouTrack server that returns projects for /api/admin/projects
	// and returns 404 for /api/admin/projects/recent (simulating real YouTrack behavior).
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/projects":
			// List all projects endpoint
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := `[
				{
					"id": "0-1",
					"name": "Demo Project",
					"shortName": "DEMO",
					"description": "A demo project",
					"leader": {"login": "admin", "fullName": "Admin User", "email": "admin@test.com", "banned": false},
					"$type": "Project"
				},
				{
					"id": "0-2",
					"name": "Test Project",
					"shortName": "TEST",
					"description": null,
					"leader": null,
					"$type": "Project"
				}
			]`
			w.Write([]byte(resp))
		case "/api/admin/projects/recent":
			// YouTrack has no project with key "recent" — returns 404
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Entity with id recent not found"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "not found"}`))
		}
	}))
	defer ytServer.Close()

	e := setupProjectRouter(ytServer.URL)

	// Act: send GET /rest/api/2/project/recent with valid Basic Auth
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/recent", nil)
	req.Header.Set("Authorization", basicAuthHeader("user@test.com", "test-token"))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Assert: Expected behavior is HTTP 200 with a JSON array of JiraProject objects
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rest/api/2/project/recent: status = %d, want %d\nBody: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	var projects []model.JiraProject
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("failed to unmarshal response body as []JiraProject: %v\nBody: %s",
			err, rec.Body.String())
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Verify the response matches what GET /rest/api/2/project would return
	if projects[0].Key != "DEMO" {
		t.Errorf("projects[0].Key = %q, want %q", projects[0].Key, "DEMO")
	}
	if projects[0].Name != "Demo Project" {
		t.Errorf("projects[0].Name = %q, want %q", projects[0].Name, "Demo Project")
	}
	if projects[1].Key != "TEST" {
		t.Errorf("projects[1].Key = %q, want %q", projects[1].Key, "TEST")
	}
	if projects[1].Name != "Test Project" {
		t.Errorf("projects[1].Name = %q, want %q", projects[1].Name, "Test Project")
	}
}
