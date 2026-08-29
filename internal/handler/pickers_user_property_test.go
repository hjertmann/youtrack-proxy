package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"

	"pgregory.net/rapid"
)

// TestUserPickerMaxResults_Property validates that for any maxResults value the
// response user count respects the clamping rules defined in the design.
//
// **Validates: Requirements 3.4, 3.5**
func TestUserPickerMaxResults_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random number of upstream users (0-50).
		nUsers := rapid.IntRange(0, 50).Draw(t, "nUsers")
		ytUsers := makeYTUsers(nUsers)

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(ytUsers)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}

		// Generate a maxResults value from a wide range including invalid values.
		mrValue := rapid.IntRange(-100, 2000).Draw(t, "maxResults")
		target := fmt.Sprintf("/rest/api/2/user/picker?query=user&maxResults=%d", mrValue)

		c, rec := setupTestContext(http.MethodGet, target)
		if err := HandleUserPicker(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp model.JiraUserPickerResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse error: %v", err)
		}

		// Determine the effective limit.
		effectiveLimit := 10 // default for absent/zero/negative
		if mrValue > 0 && mrValue <= 1000 {
			effectiveLimit = mrValue
		} else if mrValue > 1000 {
			effectiveLimit = 1000
		}

		wantCount := nUsers
		if wantCount > effectiveLimit {
			wantCount = effectiveLimit
		}

		if len(resp.Users) != wantCount {
			t.Fatalf("maxResults=%d, nUsers=%d: expected %d users, got %d",
				mrValue, nUsers, wantCount, len(resp.Users))
		}
		if resp.Total != wantCount {
			t.Fatalf("maxResults=%d, nUsers=%d: expected total %d, got %d",
				mrValue, nUsers, wantCount, resp.Total)
		}
	})
}

// TestUserPickerEmptyQuery_Property validates that when the query parameter is
// empty or absent the handler always returns an empty user list, regardless of
// any other parameter values.
//
// **Validates: Requirements 3.6**
func TestUserPickerEmptyQuery_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := &config.Config{YouTrackURL: "http://unused.example.com"}

		// Generate a random maxResults value — should be irrelevant since
		// the handler short-circuits on empty query.
		mrValue := rapid.IntRange(-100, 2000).Draw(t, "maxResults")

		// Choose between empty query param and absent query param.
		absent := rapid.Bool().Draw(t, "absentQuery")
		var target string
		if absent {
			target = fmt.Sprintf("/rest/api/2/user/picker?maxResults=%d", mrValue)
		} else {
			target = fmt.Sprintf("/rest/api/2/user/picker?query=&maxResults=%d", mrValue)
		}

		c, rec := setupTestContext(http.MethodGet, target)
		if err := HandleUserPicker(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp model.JiraUserPickerResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse error: %v", err)
		}

		if len(resp.Users) != 0 {
			t.Fatalf("expected empty users array, got %d users", len(resp.Users))
		}
		if resp.Total != 0 {
			t.Fatalf("expected total 0, got %d", resp.Total)
		}
		if resp.Header != "Showing users" {
			t.Fatalf("expected header 'Showing users', got %q", resp.Header)
		}
	})
}

// TestUserPickerFieldMapping_Property validates that for any YTUser the handler
// maps login→name, login→key, fullName→html, fullName→displayName.
//
// **Validates: Requirements 3.3**
func TestUserPickerFieldMapping_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		login := rapid.StringMatching("[a-z][a-z0-9]{2,15}").Draw(t, "login")
		fullName := rapid.StringMatching("[A-Z][a-z]{2,10} [A-Z][a-z]{2,10}").Draw(t, "fullName")

		ytUser := model.YTUser{
			Login: login,
			Name:  fullName,
			Email: login + "@example.com",
		}

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]model.YTUser{ytUser})
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query="+login)

		if err := HandleUserPicker(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp model.JiraUserPickerResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse error: %v", err)
		}

		if len(resp.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(resp.Users))
		}

		u := resp.Users[0]
		if u.Name != login {
			t.Fatalf("expected name=%q, got %q", login, u.Name)
		}
		if u.Key != login {
			t.Fatalf("expected key=%q, got %q", login, u.Key)
		}
		if u.HTML != fullName {
			t.Fatalf("expected html=%q, got %q", fullName, u.HTML)
		}
		if u.DisplayName != fullName {
			t.Fatalf("expected displayName=%q, got %q", fullName, u.DisplayName)
		}
	})
}
