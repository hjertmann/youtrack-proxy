package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestPropertyBugCondition_YouTrackRejectsIncorrectNameField validates Property 1: Bug Condition.
// The YouTrack API rejects requests containing `name` in the `fields` query parameter and only
// accepts `fullName` as the canonical field name for a user's display name.
//
// This test encodes the EXPECTED behavior after the fix: the proxy should send `fullName` in the
// fields parameter, receive a valid user response, and return it in Jira-compatible format.
//
// On UNFIXED code, this test FAILS because the proxy sends `name` instead of `fullName`, causing
// the mock YouTrack server to return 404, which results in a 502 from the proxy.
//
// **Validates: Requirements 1.1, 1.2, 2.1, 2.2**
func TestPropertyBugCondition_YouTrackRejectsIncorrectNameField(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid user data
		login := rapid.StringMatching(`[a-z][a-z0-9._]{2,15}`).Draw(t, "login")
		fullName := rapid.StringMatching(`[A-Z][a-z]+ [A-Z][a-z]+`).Draw(t, "fullName")
		email := rapid.StringMatching(`[a-z]{3,8}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email")
		banned := rapid.Bool().Draw(t, "banned")

		// Set up mock YouTrack server that:
		// - Returns 404 when `fields` param contains `name` (not `fullName`) — simulates real YouTrack behavior
		// - Returns valid user JSON when `fields` param contains `fullName`
		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/users/me" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			fieldsParam := r.URL.Query().Get("fields")

			// Check if fields param contains "name" but NOT "fullName"
			// This detects the bug: the unfixed code sends "login,name,email,banned"
			containsName := false
			for _, field := range strings.Split(fieldsParam, ",") {
				if field == "name" {
					containsName = true
					break
				}
			}
			containsFullName := strings.Contains(fieldsParam, "fullName")

			if containsName && !containsFullName {
				// Simulate YouTrack rejecting the unrecognized field
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"Not Found","error_description":"HTTP 404 Not Found"}`))
				return
			}

			if containsFullName {
				// Return valid user JSON when correct field name is used
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"login":    login,
					"fullName": fullName,
					"email":    email,
					"banned":   banned,
					"$type":    "User",
				})
				return
			}

			// Fallback: unexpected fields param
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}

		// Set up Echo context
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/myself", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		// Call the handler
		err := HandleGetCurrentUser(c, cfg)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}

		// Assert: response should be 200 (expected behavior after fix)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Assert: response body contains correct Jira-format user fields
		var user model.JiraUserResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
			t.Fatalf("error parsing response JSON: %v", err)
		}

		if user.Key != login {
			t.Fatalf("expected key=%q, got %q", login, user.Key)
		}
		if user.Name != login {
			t.Fatalf("expected name=%q, got %q", login, user.Name)
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

// TestBugCondition_YTUserDeserialization_FullNameKey verifies that the YTUser struct
// correctly deserializes JSON responses containing `"fullName"` as the key for the
// user's display name.
//
// On UNFIXED code, this test FAILS because the YTUser struct has `json:"name"` tag,
// so a response with `"fullName"` key will not populate the Name field.
//
// **Validates: Requirements 1.2, 2.2**
func TestBugCondition_YTUserDeserialization_FullNameKey(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random user data
		login := rapid.StringMatching(`[a-z][a-z0-9._]{2,15}`).Draw(t, "login")
		fullName := rapid.StringMatching(`[A-Z][a-z]+ [A-Z][a-z]+`).Draw(t, "fullName")
		email := rapid.StringMatching(`[a-z]{3,8}@[a-z]{3,8}\.[a-z]{2,4}`).Draw(t, "email")
		banned := rapid.Bool().Draw(t, "banned")

		// Simulate YouTrack API response that uses "fullName" as the key
		responseJSON := map[string]interface{}{
			"login":    login,
			"fullName": fullName,
			"email":    email,
			"banned":   banned,
			"$type":    "User",
		}

		jsonBytes, err := json.Marshal(responseJSON)
		if err != nil {
			t.Fatalf("failed to marshal test JSON: %v", err)
		}

		// Deserialize into YTUser struct
		var user model.YTUser
		if err := json.Unmarshal(jsonBytes, &user); err != nil {
			t.Fatalf("failed to unmarshal JSON into YTUser: %v", err)
		}

		// Assert: Name field should be populated from "fullName" key
		if user.Name != fullName {
			t.Fatalf("expected YTUser.Name=%q (from fullName key), got %q", fullName, user.Name)
		}

		// Also verify other fields deserialize correctly
		if user.Login != login {
			t.Fatalf("expected YTUser.Login=%q, got %q", login, user.Login)
		}
		if user.Email != email {
			t.Fatalf("expected YTUser.Email=%q, got %q", email, user.Email)
		}
		if user.Banned != banned {
			t.Fatalf("expected YTUser.Banned=%v, got %v", banned, user.Banned)
		}
	})
}
