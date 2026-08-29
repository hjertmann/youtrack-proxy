package handler

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// serverInfoPropertyContext creates an Echo context with a custom scheme and host.
func serverInfoPropertyContext(method, scheme, host, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	target := fmt.Sprintf("%s://%s%s", scheme, host, path)
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	// httptest.NewRequest sets the URL but doesn't set the scheme in URL.Scheme,
	// so we explicitly set it via the TLS field for HTTPS detection by Echo.
	if scheme == "https" {
		req.TLS = &tls.ConnectionState{}
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// TestPropertyBaseUrlReflectsRequest validates Property 1: BaseUrl reflects incoming request.
// For any incoming HTTP request with a valid scheme and host, the baseUrl field in the
// ServerInfo response SHALL equal the concatenation of the request scheme, "://", and the request host.
//
// **Validates: Requirements 1.2**
func TestPropertyBaseUrlReflectsRequest(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		scheme := rapid.SampledFrom([]string{"http", "https"}).Draw(t, "scheme")

		// Generate random valid hostnames with optional ports
		hostname := rapid.SampledFrom([]string{
			"localhost",
			"example.com",
			"my-server.internal",
			"192.168.1.100",
			"10.0.0.1",
			"proxy.corp.example.org",
			"jira.company.io",
		}).Draw(t, "hostname")

		// Optionally add a port
		hasPort := rapid.Bool().Draw(t, "hasPort")
		host := hostname
		if hasPort {
			port := rapid.IntRange(1, 65535).Draw(t, "port")
			host = fmt.Sprintf("%s:%d", hostname, port)
		}

		c, rec := serverInfoPropertyContext(http.MethodGet, scheme, host, "/rest/api/2/serverInfo")

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

		expectedBaseURL := fmt.Sprintf("%s://%s", scheme, host)
		if resp.BaseURL != expectedBaseURL {
			t.Fatalf("baseUrl = %q, want %q", resp.BaseURL, expectedBaseURL)
		}
	})
}

// TestPropertyVersionNumbersConsistency validates Property 2: Version and versionNumbers consistency.
// For any ServerInfo response, the versionNumbers array SHALL contain exactly three integers that,
// when formatted as major.minor.patch, equal the version string.
//
// **Validates: Requirements 1.3, 1.4**
func TestPropertyVersionNumbersConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c, rec := serverInfoContext(http.MethodGet, "/rest/api/2/serverInfo")

		err := HandleServerInfo(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var resp model.ServerInfoResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		// versionNumbers must have exactly 3 elements
		if len(resp.VersionNumbers) != 3 {
			t.Fatalf("versionNumbers length = %d, want 3", len(resp.VersionNumbers))
		}

		// Formatting versionNumbers as major.minor.patch must equal version string
		formatted := fmt.Sprintf("%d.%d.%d", resp.VersionNumbers[0], resp.VersionNumbers[1], resp.VersionNumbers[2])
		if formatted != resp.Version {
			t.Fatalf("versionNumbers formatted as %q does not equal version %q", formatted, resp.Version)
		}
	})
}

// ponytail: 405 enforcement for non-GET methods is now handled by the Echo router
// (e.GET registration in main.go), not the handler. No handler-level test needed.

// TestPropertyAuthBypass validates Property 4: Auth bypass for any Authorization header value.
// For any string value provided as an Authorization header (including malformed, invalid, empty, or absent),
// a GET request to /rest/api/2/serverInfo SHALL receive a 200 status code with a valid ServerInfo response
// containing all required fields.
//
// **Validates: Requirements 2.1, 2.2, 2.3**
func TestPropertyAuthBypass(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a variety of Authorization header values
		authType := rapid.IntRange(0, 5).Draw(t, "authType")
		var authHeader string
		setHeader := true

		switch authType {
		case 0:
			// No header at all
			setHeader = false
		case 1:
			// Empty string
			authHeader = ""
		case 2:
			// Malformed Basic auth
			garbage := rapid.StringMatching(`[A-Za-z0-9!@#$%^&*]{1,30}`).Draw(t, "garbage")
			authHeader = "Basic " + garbage
		case 3:
			// Bearer token
			token := rapid.StringMatching(`[A-Za-z0-9._\-]{10,50}`).Draw(t, "token")
			authHeader = "Bearer " + token
		case 4:
			// Completely random garbage
			authHeader = rapid.StringMatching(`[A-Za-z0-9 ._\-/+=]{0,50}`).Draw(t, "randomAuth")
		case 5:
			// Valid-looking Basic auth (user:pass base64-encoded)
			authHeader = "Basic dXNlcjpwYXNz" // user:pass
		}

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/serverInfo", nil)
		if setHeader {
			req.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := HandleServerInfo(c)
		if err != nil {
			t.Fatalf("unexpected error with auth header %q: %v", authHeader, err)
		}

		// Must receive 200
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 with auth header %q, got %d", authHeader, rec.Code)
		}

		// Must have complete ServerInfo response
		var resp model.ServerInfoResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response with auth header %q: %v", authHeader, err)
		}

		// Verify all required fields are present
		if resp.BaseURL == "" {
			t.Fatalf("baseUrl is empty with auth header %q", authHeader)
		}
		if resp.Version == "" {
			t.Fatalf("version is empty with auth header %q", authHeader)
		}
		if len(resp.VersionNumbers) != 3 {
			t.Fatalf("versionNumbers length = %d with auth header %q, want 3", len(resp.VersionNumbers), authHeader)
		}
		if resp.DeploymentType == "" {
			t.Fatalf("deploymentType is empty with auth header %q", authHeader)
		}
		if resp.ServerTitle == "" {
			t.Fatalf("serverTitle is empty with auth header %q", authHeader)
		}
		if resp.ServerTime == "" {
			t.Fatalf("serverTime is empty with auth header %q", authHeader)
		}
	})
}

// TestPropertyServerTimeAccuracy validates Property 5: serverTime accuracy and format.
// For any GET request to /rest/api/2/serverInfo, the serverTime field SHALL be a valid ISO 8601
// timestamp with timezone offset +0000 and SHALL represent a time within 5 seconds of the actual
// UTC time at the moment the response is generated.
//
// **Validates: Requirements 4.2**
func TestPropertyServerTimeAccuracy(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
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

		// Verify the serverTime field ends with +0000 (timezone offset)
		if !strings.HasSuffix(resp.ServerTime, "+0000") {
			t.Fatalf("serverTime %q does not end with '+0000'", resp.ServerTime)
		}

		// Verify it's parseable using the expected ISO 8601 format
		parsed, err := time.Parse("2006-01-02T15:04:05.000+0000", resp.ServerTime)
		if err != nil {
			t.Fatalf("failed to parse serverTime %q: %v", resp.ServerTime, err)
		}

		// Verify parsed time is within 5 seconds of the actual time
		if parsed.Before(before.Add(-5 * time.Second)) {
			t.Fatalf("serverTime %v is more than 5 seconds before request start %v", parsed, before)
		}
		if parsed.After(after.Add(5 * time.Second)) {
			t.Fatalf("serverTime %v is more than 5 seconds after request end %v", parsed, after)
		}
	})
}
