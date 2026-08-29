package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
	"pgregory.net/rapid"
)

// setupRouterWithCurrentRoutes creates an Echo app that mirrors the user-related
// routes registered in main.go, including the api.GET("/user", ...) route.
func setupRouterWithCurrentRoutes(cfg *config.Config) *echo.Echo {
	e := echo.New()

	api := e.Group("/rest/api/2", authmw.BasicAuth(""))

	// Users — mirrors main.go routes including the /user route
	api.GET("/myself", func(c echo.Context) error {
		return HandleGetCurrentUser(c, cfg)
	})
	api.GET("/user", func(c echo.Context) error {
		return HandleGetUser(c, cfg)
	})
	api.GET("/user/picker", func(c echo.Context) error {
		return HandleUserPicker(c, cfg)
	})
	api.GET("/user/search", func(c echo.Context) error {
		return HandleSearchUsers(c, cfg)
	})

	return e
}

// TestPropertyBugCondition_GetUserRouteReturns404 validates Property 1: Bug Condition.
//
// The GET /rest/api/2/user endpoint is missing from the proxy router. For any of the
// three supported query parameters (username, key, accountId), a request to this path
// should return HTTP 200 with a single JiraUserResponse, but instead returns HTTP 404
// because the route is not registered.
//
// This test creates an Echo app with the CURRENT routes (no /user route), sets up a
// mock YouTrack server that returns a valid user, and sends requests through the
// router. The 404 response from Echo proves the route is missing.
//
// On UNFIXED code, this test FAILS — that failure confirms the bug exists.
//
// **Validates: Requirements 1.1, 1.2, 1.3**
func TestPropertyBugCondition_GetUserRouteReturns404(t *testing.T) {
	// The three query param variants that should all resolve a single user
	paramNames := []string{"username", "key", "accountId"}

	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid user data for the mock YouTrack response
		login := rapid.StringMatching(`[a-z][a-z0-9._]{2,15}`).Draw(t, "login")
		fullName := rapid.StringMatching(`[A-Z][a-z]+ [A-Z][a-z]+`).Draw(t, "fullName")
		email := rapid.StringMatching(`[a-z]{3,8}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email")
		banned := rapid.Bool().Draw(t, "banned")

		// Pick a random query param name from the three variants
		paramIdx := rapid.IntRange(0, len(paramNames)-1).Draw(t, "paramIdx")
		paramName := paramNames[paramIdx]

		// Mock YouTrack server returning a single user in an array (SearchUsers response)
		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/users" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"login":    login,
					"fullName": fullName,
					"email":    email,
					"banned":   banned,
					"$type":    "User",
				},
			})
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		app := setupRouterWithCurrentRoutes(cfg)

		// Send request to the missing /user endpoint
		target := "/rest/api/2/user?" + paramName + "=" + login
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", basicAuthHeader("test@example.com", "test-token"))
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		// EXPECTED behavior (after fix): HTTP 200 with a single JiraUserResponse
		// ACTUAL behavior (unfixed code): HTTP 404 because no route is registered
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /rest/api/2/user?%s=%s: expected status 200, got %d (body: %s)",
				paramName, login, rec.Code, rec.Body.String())
		}

		// If we got 200, verify the response is a single user object (not an array)
		var user model.JiraUserResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
			t.Fatalf("expected single JiraUserResponse object, got parse error: %v (body: %s)",
				err, rec.Body.String())
		}

		if user.Key != login {
			t.Fatalf("expected key=%q, got %q", login, user.Key)
		}
		if user.DisplayName != fullName {
			t.Fatalf("expected displayName=%q, got %q", fullName, user.DisplayName)
		}
		if user.EmailAddress != email {
			t.Fatalf("expected emailAddress=%q, got %q", email, user.EmailAddress)
		}
		if user.Active != !banned {
			t.Fatalf("expected active=%v, got %v", !banned, user.Active)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 2: Preservation — Existing User Endpoints Unchanged
// ---------------------------------------------------------------------------

// genYTUserJSON generates random YouTrack user data suitable for a mock response.
func genYTUserJSON(t *rapid.T, tag string) map[string]interface{} {
	return map[string]interface{}{
		"login":    rapid.StringMatching(`[a-z][a-z0-9._]{2,15}`).Draw(t, tag+"Login"),
		"fullName": rapid.StringMatching(`[A-Z][a-z]+ [A-Z][a-z]+`).Draw(t, tag+"FullName"),
		"email":    rapid.StringMatching(`[a-z]{3,8}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, tag+"Email"),
		"banned":   rapid.Bool().Draw(t, tag+"Banned"),
		"$type":    "User",
	}
}

// TestPreservation_SearchUsersReturnsArray verifies that GET /rest/api/2/user/search?username=<value>
// returns HTTP 200 with a JSON array of JiraUserResponse objects on unfixed code.
// For random YouTrack user payloads the response shape (array) and field mappings
// must match what ConvertYTUsersToJira produces.
//
// **Validates: Requirements 3.1, 3.4**
func TestPreservation_SearchUsersReturnsArray(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-5 random users
		numUsers := rapid.IntRange(1, 5).Draw(t, "numUsers")
		ytUsers := make([]map[string]interface{}, numUsers)
		for i := range ytUsers {
			ytUsers[i] = genYTUserJSON(t, "u"+string(rune('0'+i)))
		}

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/users" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ytUsers)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		app := setupRouterWithCurrentRoutes(cfg)

		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/user/search?username=test", nil)
		req.Header.Set("Authorization", basicAuthHeader("test@example.com", "test-token"))
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Must parse as an array
		var users []model.JiraUserResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
			t.Fatalf("response is not a JSON array of JiraUserResponse: %v", err)
		}

		if len(users) != numUsers {
			t.Fatalf("expected %d users, got %d", numUsers, len(users))
		}

		// Verify field mappings match ConvertYTUsersToJira
		for i, jiraUser := range users {
			ytLogin := ytUsers[i]["login"].(string)
			ytFullName := ytUsers[i]["fullName"].(string)
			ytEmail := ytUsers[i]["email"].(string)
			ytBanned := ytUsers[i]["banned"].(bool)

			expected := service.ConvertYTUserToJira(model.YTUser{
				Login:  ytLogin,
				Name:   ytFullName,
				Email:  ytEmail,
				Banned: ytBanned,
			})

			if jiraUser.Key != expected.Key {
				t.Fatalf("user[%d]: expected key=%q, got %q", i, expected.Key, jiraUser.Key)
			}
			if jiraUser.Name != expected.Name {
				t.Fatalf("user[%d]: expected name=%q, got %q", i, expected.Name, jiraUser.Name)
			}
			if jiraUser.DisplayName != expected.DisplayName {
				t.Fatalf("user[%d]: expected displayName=%q, got %q", i, expected.DisplayName, jiraUser.DisplayName)
			}
			if jiraUser.EmailAddress != expected.EmailAddress {
				t.Fatalf("user[%d]: expected emailAddress=%q, got %q", i, expected.EmailAddress, jiraUser.EmailAddress)
			}
			if jiraUser.Active != expected.Active {
				t.Fatalf("user[%d]: expected active=%v, got %v", i, expected.Active, jiraUser.Active)
			}
		}
	})
}

// TestPreservation_MyselfReturnsSingleUser verifies that GET /rest/api/2/myself
// returns HTTP 200 with a single JiraUserResponse object (not an array) on unfixed code.
// For random YouTrack user payloads the field mapping must match ConvertYTUserToJira.
//
// **Validates: Requirements 3.3**
func TestPreservation_MyselfReturnsSingleUser(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		u := genYTUserJSON(t, "me")

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/users/me" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(u)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		app := setupRouterWithCurrentRoutes(cfg)

		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/myself", nil)
		req.Header.Set("Authorization", basicAuthHeader("test@example.com", "test-token"))
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Must parse as a single object (not array)
		var jiraUser model.JiraUserResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &jiraUser); err != nil {
			t.Fatalf("response is not a single JiraUserResponse: %v", err)
		}

		// Verify it's NOT an array by checking array parse fails or has wrong shape
		var arr []json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &arr); err == nil {
			t.Fatal("response should be a single object, but parsed as an array")
		}

		expected := service.ConvertYTUserToJira(model.YTUser{
			Login:  u["login"].(string),
			Name:   u["fullName"].(string),
			Email:  u["email"].(string),
			Banned: u["banned"].(bool),
		})

		if jiraUser.Key != expected.Key {
			t.Fatalf("expected key=%q, got %q", expected.Key, jiraUser.Key)
		}
		if jiraUser.Name != expected.Name {
			t.Fatalf("expected name=%q, got %q", expected.Name, jiraUser.Name)
		}
		if jiraUser.DisplayName != expected.DisplayName {
			t.Fatalf("expected displayName=%q, got %q", expected.DisplayName, jiraUser.DisplayName)
		}
		if jiraUser.EmailAddress != expected.EmailAddress {
			t.Fatalf("expected emailAddress=%q, got %q", expected.EmailAddress, jiraUser.EmailAddress)
		}
		if jiraUser.Active != expected.Active {
			t.Fatalf("expected active=%v, got %v", expected.Active, jiraUser.Active)
		}
	})
}

// TestPreservation_SearchUsersMissingParamReturns400 verifies that
// GET /rest/api/2/user/search (no username param) returns HTTP 400 on unfixed code.
//
// **Validates: Requirements 3.4**
func TestPreservation_SearchUsersMissingParamReturns400(t *testing.T) {
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	app := setupRouterWithCurrentRoutes(cfg)

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/user/search", nil)
	req.Header.Set("Authorization", basicAuthHeader("test@example.com", "test-token"))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("response is not a JiraErrorResponse: %v", err)
	}

	if len(errResp.ErrorMessages) == 0 {
		t.Fatal("expected non-empty errorMessages")
	}
}
