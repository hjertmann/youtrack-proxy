package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"pgregory.net/rapid"

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

	// Health probe and serverInfo registered exactly as main.go does (fixed).
	e.GET("/health", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.String(http.StatusOK, "OK")
	})

	// v2 API group with auth
	api := e.Group("/rest/api/2", authmw.BasicAuth(""))
	api.GET("/serverInfo", handler.HandleServerInfo)
	api.GET("/search", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return handler.HandleSearchIssues(c, cfg, resolvedCache)
	})

	// v3 API group with auth
	apiv3 := e.Group("/rest/api/3", authmw.BasicAuth(""))
	apiv3.GET("/serverInfo", handler.HandleServerInfo)
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

	api := e.Group("/rest/api/2", authmw.BasicAuth(""))

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

// --- Bug condition exploration tests: consistent-auth-and-health ---
//
// These encode the EXPECTED (correct) behavior and MUST FAIL on unfixed code:
//   - serverInfo is registered on the bare Echo instance (no auth group), so
//     unauthenticated requests return 200 instead of 401.
//   - /health sets no Cache-Control header.
// Failure here confirms the bug exists (isBugCondition returns true).

// TestBugCondition_V2ServerInfoRequiresAuth asserts GET /rest/api/2/serverInfo with
// no Authorization header returns 401. On unfixed code it returns 200 (serverInfo is
// registered outside the auth group). Validates: Requirements 1.1
func TestBugCondition_V2ServerInfoRequiresAuth(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/serverInfo", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /rest/api/2/serverInfo unauthenticated: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestBugCondition_V3ServerInfoRequiresAuth asserts GET /rest/api/3/serverInfo with
// no Authorization header returns 401. On unfixed code it returns 200. Validates: Requirements 1.2
func TestBugCondition_V3ServerInfoRequiresAuth(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/rest/api/3/serverInfo", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /rest/api/3/serverInfo unauthenticated: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestBugCondition_HealthSetsNoCacheHeader asserts GET /health carries a non-empty
// no-cache Cache-Control header. On unfixed code no such header is set. Validates: Requirements 1.3
func TestBugCondition_HealthSetsNoCacheHeader(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc == "" || !strings.Contains(cc, "no-cache") {
		t.Fatalf("GET /health: Cache-Control = %q, want a non-empty header containing \"no-cache\"", cc)
	}
}

// --- Preservation tests: consistent-auth-and-health ---
//
// Property 2: Preservation — already-protected routes still reject unauthenticated
// requests with 401, authenticated serverInfo still returns 200 with its metadata,
// and /health still returns 200 body "OK" with content type text/plain.
//
// These are observation-first tests: they encode the CURRENT (unfixed) baseline
// behavior and MUST PASS on unfixed code. They guard against regressions when the
// fix is applied. Validates: Requirements 2.3, 3.1, 3.2, 3.3, 3.4

// setupPreservationRouter mirrors setupRouter but also registers a representative
// /rest/agile/1.0 protected route, so the preservation property can cover all three
// already-protected route groups (v2, v3, agile) required by Requirement 3.1.
func setupPreservationRouter() *echo.Echo {
	e := setupRouter()

	cfg := &config.Config{YouTrackURL: "http://localhost:9999"}
	agile := e.Group("/rest/agile/1.0", authmw.BasicAuth(""))
	agile.GET("/board", func(c echo.Context) error {
		return handler.HandleListBoards(c, cfg)
	})

	return e
}

// alreadyProtectedRoutes is the set of routes that require Basic Auth on unfixed
// code. serverInfo is deliberately excluded (it is the buggy, unprotected route).
var alreadyProtectedRoutes = []string{
	"/rest/api/2/search",
	"/rest/api/2/search/jql",
	"/rest/api/3/search/jql",
	"/rest/agile/1.0/board",
}

// TestPreservation_ProtectedRoutesRejectUnauthenticated is a property-based test:
// for every already-protected route, an unauthenticated request returns 401. This
// preserves the auth-enforcement invariant for NOT isBugCondition(req).
// Validates: Requirements 3.1
func TestPreservation_ProtectedRoutesRejectUnauthenticated(t *testing.T) {
	e := setupPreservationRouter()

	rapid.Check(t, func(t *rapid.T) {
		path := rapid.SampledFrom(alreadyProtectedRoutes).Draw(t, "path")

		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s unauthenticated: status = %d, want %d", path, rec.Code, http.StatusUnauthorized)
		}
	})
}

// TestPreservation_AuthedServerInfoMetadata captures the authenticated serverInfo
// metadata shape (v2 and v3) on unfixed code: HTTP 200 with the ServerInfoResponse
// fields (base URL, version, deployment type, server title, build number).
// Validates: Requirements 2.3, 3.3
func TestPreservation_AuthedServerInfoMetadata(t *testing.T) {
	e := setupRouter()

	for _, path := range []string{"/rest/api/2/serverInfo", "/rest/api/3/serverInfo"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", basicAuthHeader("user@test.com", "test-token"))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s authenticated: status = %d, want %d\nBody: %s", path, rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp model.ServerInfoResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("GET %s: failed to unmarshal ServerInfoResponse: %v\nBody: %s", path, err, rec.Body.String())
		}

		if resp.BaseURL == "" {
			t.Errorf("GET %s: baseUrl is empty", path)
		}
		if resp.Version != "9.0.0" {
			t.Errorf("GET %s: version = %q, want %q", path, resp.Version, "9.0.0")
		}
		if resp.DeploymentType != "Server" {
			t.Errorf("GET %s: deploymentType = %q, want %q", path, resp.DeploymentType, "Server")
		}
		if resp.ServerTitle != "YouTrack Jira Proxy" {
			t.Errorf("GET %s: serverTitle = %q, want %q", path, resp.ServerTitle, "YouTrack Jira Proxy")
		}
		if resp.BuildNumber != 900000 {
			t.Errorf("GET %s: buildNumber = %d, want %d", path, resp.BuildNumber, 900000)
		}
	}
}

// TestPreservation_HealthBodyAndContentType captures /health on unfixed code:
// HTTP 200, body "OK", content type text/plain.
// Validates: Requirements 3.4
func TestPreservation_HealthBodyAndContentType(t *testing.T) {
	e := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("GET /health: body = %q, want %q", rec.Body.String(), "OK")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("GET /health: Content-Type = %q, want prefix %q", ct, "text/plain")
	}
}
