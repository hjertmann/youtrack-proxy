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

func setupTestContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	return c, rec
}

func TestHandleGetCurrentUser_Success(t *testing.T) {
	// Mock YouTrack API returning a user
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/me" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify auth header is forwarded
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected Authorization header: %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"login":    "jdoe",
			"fullName": "John Doe",
			"email":    "jdoe@example.com",
			"banned":   false,
			"$type":    "User",
		})
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/myself")

	err := HandleGetCurrentUser(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var user model.JiraUserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
		t.Fatalf("error parsing response JSON: %v", err)
	}

	if user.Key != "jdoe" {
		t.Errorf("expected key 'jdoe', got '%s'", user.Key)
	}
	if user.Name != "jdoe" {
		t.Errorf("expected name 'jdoe', got '%s'", user.Name)
	}
	if user.DisplayName != "John Doe" {
		t.Errorf("expected displayName 'John Doe', got '%s'", user.DisplayName)
	}
	if user.EmailAddress != "jdoe@example.com" {
		t.Errorf("expected emailAddress 'jdoe@example.com', got '%s'", user.EmailAddress)
	}
	if !user.Active {
		t.Error("expected active to be true")
	}
}

func TestHandleGetCurrentUser_UpstreamError(t *testing.T) {
	// Mock YouTrack API returning 500
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/myself")

	err := HandleGetCurrentUser(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("error parsing error response: %v", err)
	}

	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected error messages in response")
	}
}

func TestHandleSearchUsers_Success(t *testing.T) {
	// Mock YouTrack API returning a list of users
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify query param is forwarded
		query := r.URL.Query().Get("query")
		if query != "john" {
			t.Errorf("expected query param 'john', got '%s'", query)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"login":    "jdoe",
				"fullName": "John Doe",
				"email":    "jdoe@example.com",
				"banned":   false,
				"$type":    "User",
			},
			{
				"login":    "jsmith",
				"fullName": "John Smith",
				"email":    "jsmith@example.com",
				"banned":   false,
				"$type":    "User",
			},
		})
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/search?username=john")

	err := HandleSearchUsers(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var users []model.JiraUserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("error parsing response JSON: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	if users[0].Key != "jdoe" {
		t.Errorf("expected first user key 'jdoe', got '%s'", users[0].Key)
	}
	if users[1].Key != "jsmith" {
		t.Errorf("expected second user key 'jsmith', got '%s'", users[1].Key)
	}
}

func TestHandleSearchUsers_NoResults(t *testing.T) {
	// Mock YouTrack API returning empty array
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/search?username=nonexistent")

	err := HandleSearchUsers(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var users []model.JiraUserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("error parsing response JSON: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("expected empty array, got %d users", len(users))
	}
}

func TestHandleSearchUsers_MissingUsername(t *testing.T) {
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/search")

	err := HandleSearchUsers(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("error parsing error response: %v", err)
	}

	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected error messages in response")
	}
}

func TestHandleSearchUsers_UpstreamError(t *testing.T) {
	// Mock YouTrack API returning 500
	ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}
	c, rec := setupTestContext(http.MethodGet, "/rest/api/2/user/search?username=john")

	err := HandleSearchUsers(c, cfg)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", rec.Code)
	}

	var errResp model.JiraErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("error parsing error response: %v", err)
	}

	if len(errResp.ErrorMessages) == 0 {
		t.Error("expected error messages in response")
	}
}
