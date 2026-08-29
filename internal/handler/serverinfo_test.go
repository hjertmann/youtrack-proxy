package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// serverInfoContext creates an Echo context for serverInfo tests.
func serverInfoContext(method, target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// serverInfoContextWithAuth creates an Echo context with an Authorization header.
func serverInfoContextWithAuth(method, target, authHeader string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

func TestServerInfo_GETReturns200WithAllRequiredFields(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify all required fields are present and non-zero
	if resp.BaseURL == "" {
		t.Error("expected baseUrl to be non-empty")
	}
	if resp.Version == "" {
		t.Error("expected version to be non-empty")
	}
	if len(resp.VersionNumbers) != 3 {
		t.Errorf("expected versionNumbers to have 3 elements, got %d", len(resp.VersionNumbers))
	}
	if resp.DeploymentType == "" {
		t.Error("expected deploymentType to be non-empty")
	}
	if resp.ServerTitle == "" {
		t.Error("expected serverTitle to be non-empty")
	}
	if resp.ServerTime == "" {
		t.Error("expected serverTime to be non-empty")
	}
	if resp.BuildNumber == 0 {
		t.Error("expected buildNumber to be non-zero")
	}
}

func TestServerInfo_ContentTypeIsJSON(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected Content-Type to start with application/json, got %q", ct)
	}
	// If charset is present, it must be utf-8
	if strings.Contains(ct, "charset") && !strings.Contains(ct, "charset=utf-8") && !strings.Contains(ct, "charset=UTF-8") {
		t.Errorf("expected charset=utf-8 if present, got %q", ct)
	}
}

func TestServerInfo_DeploymentTypeIsServer(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.DeploymentType != "Server" {
		t.Errorf("expected deploymentType='Server', got %q", resp.DeploymentType)
	}
}

func TestServerInfo_ServerTitleIsYouTrackJiraProxy(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ServerTitle != "YouTrack Jira Proxy" {
		t.Errorf("expected serverTitle='YouTrack Jira Proxy', got %q", resp.ServerTitle)
	}
}

func TestServerInfo_BuildNumberIs900000(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.BuildNumber != 900000 {
		t.Errorf("expected buildNumber=900000, got %d", resp.BuildNumber)
	}
}

func TestServerInfo_VersionIs9_0_0(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Version != "9.0.0" {
		t.Errorf("expected version='9.0.0', got %q", resp.Version)
	}

	expected := []int{9, 0, 0}
	if len(resp.VersionNumbers) != 3 {
		t.Fatalf("expected versionNumbers length=3, got %d", len(resp.VersionNumbers))
	}
	for i, v := range expected {
		if resp.VersionNumbers[i] != v {
			t.Errorf("expected versionNumbers[%d]=%d, got %d", i, v, resp.VersionNumbers[i])
		}
	}
}

func TestServerInfo_BaseUrlReflectsRequest(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// httptest.NewRequest defaults to scheme "http" and host "example.com"
	expected := "http://example.com"
	if resp.BaseURL != expected {
		t.Errorf("expected baseUrl=%q, got %q", expected, resp.BaseURL)
	}
}

func TestServerInfo_ServerTimeIsValidAndClose(t *testing.T) {
	before := time.Now().UTC()

	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := time.Now().UTC()

	var resp model.ServerInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Parse the serverTime using the expected format
	parsed, err := time.Parse("2006-01-02T15:04:05.000+0000", resp.ServerTime)
	if err != nil {
		t.Fatalf("failed to parse serverTime %q: %v", resp.ServerTime, err)
	}

	// Verify time is within 5 seconds of actual time
	if parsed.Before(before.Add(-5*time.Second)) || parsed.After(after.Add(5*time.Second)) {
		t.Errorf("serverTime %v is not within 5 seconds of request time (before=%v, after=%v)", parsed, before, after)
	}
}

func TestServerInfo_ResponseIsJSONObject(t *testing.T) {
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := rec.Body.Bytes()
	// Verify response starts with '{' — it's a JSON object, not an array
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		t.Errorf("expected response to be a JSON object starting with '{', got %q", trimmed[:min(20, len(trimmed))])
	}

	// Also verify it actually unmarshals into a map (object)
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("response is not a valid JSON object: %v", err)
	}
}

func TestServerInfo_NoAuthHeaderReturns200(t *testing.T) {
	// No Authorization header set
	c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 without auth header, got %d", rec.Code)
	}
}

func TestServerInfo_ValidAuthHeaderReturns200(t *testing.T) {
	c, rec := serverInfoContextWithAuth(http.MethodGet, "/rest/api/2/serverInfo", "Basic dXNlcjpwYXNz")

	err := HandleServerInfo(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 with valid auth header, got %d", rec.Code)
	}
}

func TestServerInfo_InvalidAuthHeaderReturns200(t *testing.T) {
	testCases := []struct {
		name   string
		header string
	}{
		{"malformed basic", "Basic not-valid-base64!!!"},
		{"garbage string", "totally-invalid-auth-header"},
		{"empty value", ""},
		{"bearer token", "Bearer some-jwt-token"},
		{"partial basic", "Basic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := serverInfoContextWithAuth(http.MethodGet, "/rest/api/2/serverInfo", tc.header)

			err := HandleServerInfo(c)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200 with auth header %q, got %d", tc.header, rec.Code)
			}
		})
	}
}
