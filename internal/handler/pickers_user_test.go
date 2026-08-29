package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// newYTUsersServer returns a test server that responds to /api/users with the given users JSON.
func newYTUsersServer(t *testing.T, users []model.YTUser, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(users)
	}))
}

// makeYTUsers generates n YTUser objects with predictable fields.
func makeYTUsers(n int) []model.YTUser {
	users := make([]model.YTUser, n)
	for i := range users {
		users[i] = model.YTUser{
			Login: fmt.Sprintf("user%d", i),
			Name:  fmt.Sprintf("User %d", i),
			Email: fmt.Sprintf("user%d@example.com", i),
		}
	}
	return users
}

func TestHandleUserPicker_ForwardsQuery(t *testing.T) {
	ytUsers := []model.YTUser{
		{Login: "alice", Name: "Alice Smith", Email: "alice@example.com"},
		{Login: "alicia", Name: "Alicia Keys", Email: "alicia@example.com"},
	}

	var capturedQuery string
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ytUsers)
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=alice")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if capturedQuery != "alice" {
		t.Errorf("expected query 'alice' forwarded to YouTrack, got %q", capturedQuery)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if resp.Header != "Showing users" {
		t.Errorf("expected header 'Showing users', got %q", resp.Header)
	}
	if resp.Users[0].Name != "alice" {
		t.Errorf("expected users[0].name 'alice', got %q", resp.Users[0].Name)
	}
	if resp.Users[0].DisplayName != "Alice Smith" {
		t.Errorf("expected users[0].displayName 'Alice Smith', got %q", resp.Users[0].DisplayName)
	}
}

func TestHandleUserPicker_EmptyQuery(t *testing.T) {
	// No mock server needed — handler short-circuits on empty query.
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 0 {
		t.Errorf("expected empty users array, got %d", len(resp.Users))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if resp.Header != "Showing users" {
		t.Errorf("expected header 'Showing users', got %q", resp.Header)
	}
}

func TestHandleUserPicker_EmptyQueryAbsent(t *testing.T) {
	// query param entirely absent.
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 0 {
		t.Errorf("expected empty users array, got %d", len(resp.Users))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

func TestHandleUserPicker_MaxResultsDefault(t *testing.T) {
	ytUsers := makeYTUsers(15)
	ytServer := newYTUsersServer(t, ytUsers, http.StatusOK)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=user")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 10 {
		t.Errorf("expected 10 users (default maxResults), got %d", len(resp.Users))
	}
	if resp.Total != 10 {
		t.Errorf("expected total 10, got %d", resp.Total)
	}
}

func TestHandleUserPicker_MaxResultsCustom(t *testing.T) {
	ytUsers := makeYTUsers(10)
	ytServer := newYTUsersServer(t, ytUsers, http.StatusOK)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=user&maxResults=3")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 3 {
		t.Errorf("expected 3 users, got %d", len(resp.Users))
	}
	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
}

func TestHandleUserPicker_MaxResultsInvalid(t *testing.T) {
	ytUsers := makeYTUsers(15)
	ytServer := newYTUsersServer(t, ytUsers, http.StatusOK)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	cases := []struct {
		name  string
		param string
	}{
		{"non-integer", "maxResults=abc"},
		{"negative", "maxResults=-5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=user&"+tc.param)

			err := HandleUserPicker(c, cfg)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			var resp model.JiraUserPickerResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("error parsing response: %v", err)
			}

			// Invalid maxResults should default to 10
			if len(resp.Users) != 10 {
				t.Errorf("[%s] expected 10 users (default), got %d", tc.name, len(resp.Users))
			}
		})
	}
}

func TestHandleUserPicker_UpstreamError(t *testing.T) {
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=alice")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("error parsing error response: %v", err)
	}

	if len(errResp.ErrorMessages) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(errResp.ErrorMessages))
	}
	if errResp.ErrorMessages[0] != "Failed to search users from upstream" {
		t.Errorf("expected 'Failed to search users from upstream', got %q", errResp.ErrorMessages[0])
	}
}

func TestHandleUserPicker_FieldMapping(t *testing.T) {
	ytUsers := []model.YTUser{
		{Login: "jdoe", Name: "John Doe", Email: "jdoe@example.com", Banned: false},
	}
	ytServer := newYTUsersServer(t, ytUsers, http.StatusOK)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/picker?query=jdoe")

	err := HandleUserPicker(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.JiraUserPickerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error parsing response: %v", err)
	}

	if len(resp.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(resp.Users))
	}

	u := resp.Users[0]

	// Login maps to both name and key
	if u.Name != "jdoe" {
		t.Errorf("expected name 'jdoe', got %q", u.Name)
	}
	if u.Key != "jdoe" {
		t.Errorf("expected key 'jdoe', got %q", u.Key)
	}

	// fullName maps to both html and displayName
	if u.HTML != "John Doe" {
		t.Errorf("expected html 'John Doe', got %q", u.HTML)
	}
	if u.DisplayName != "John Doe" {
		t.Errorf("expected displayName 'John Doe', got %q", u.DisplayName)
	}
}
