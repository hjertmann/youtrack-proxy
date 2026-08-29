package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"pgregory.net/rapid"
)

// filterSearchResponse represents the expected Jira-compatible paginated filter search response.
type filterSearchResponse struct {
	Self       string        `json:"self"`
	MaxResults int           `json:"maxResults"`
	StartAt    int           `json:"startAt"`
	Total      int           `json:"total"`
	IsLast     bool          `json:"isLast"`
	Values     []interface{} `json:"values"`
}

// setupFilterSearchRouter creates an Echo instance with the /rest/api/2 route group
// including BasicAuth middleware, mirroring the production router configuration.
// The /filter/search route is registered with the HandleFilterSearch handler.
func setupFilterSearchRouter() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	api := e.Group("/rest/api/2", authmw.BasicAuth(""))
	api.GET("/filter/search", HandleFilterSearch)

	return e
}

// filterSearchBasicAuth returns a valid Basic Auth header value for filter search tests.
func filterSearchBasicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("user:token123"))
}

// TestPropertyBugCondition_FilterSearchReturns404 validates the bug condition:
// For all authenticated GET requests to /rest/api/2/filter/search with any valid
// startAt/maxResults, the response SHALL return HTTP 200 with JSON body containing
// startAt reflecting the requested offset (default 0), maxResults reflecting the
// requested limit (default 100), total of 0, isLast set to true, and an empty values array.
//
// EXPECTED: This test FAILS on unfixed code (404 instead of 200) - confirming the bug exists.
//
// **Validates: Requirements 1.1, 1.2, 2.1, 2.2**
func TestPropertyBugCondition_FilterSearchReturns404(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		e := setupFilterSearchRouter()

		// Generate random startAt and maxResults values
		startAt := rapid.IntRange(0, 10000).Draw(t, "startAt")
		maxResults := rapid.IntRange(1, 1000).Draw(t, "maxResults")

		target := fmt.Sprintf("/rest/api/2/filter/search?startAt=%d&maxResults=%d", startAt, maxResults)
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", filterSearchBasicAuth())
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		// The expected behavior is HTTP 200 with a valid paginated response.
		// On unfixed code, this will be 404 - confirming the bug.
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d for GET %s", rec.Code, target)
		}

		var resp filterSearchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response body: %v", err)
		}

		if resp.StartAt != startAt {
			t.Fatalf("expected startAt=%d, got %d", startAt, resp.StartAt)
		}
		if resp.MaxResults != maxResults {
			t.Fatalf("expected maxResults=%d, got %d", maxResults, resp.MaxResults)
		}
		if resp.Total != 0 {
			t.Fatalf("expected total=0, got %d", resp.Total)
		}
		if !resp.IsLast {
			t.Fatalf("expected isLast=true, got false")
		}
		if resp.Values == nil || len(resp.Values) != 0 {
			t.Fatalf("expected empty values array, got %v", resp.Values)
		}
	})
}

// setupFilterSearchRouterWithRoute creates an Echo instance WITH the /filter/search
// route registered in the BasicAuth-protected group. This is used by the auth
// preservation test to confirm middleware enforcement.
func setupFilterSearchRouterWithRoute() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	api := e.Group("/rest/api/2", authmw.BasicAuth(""))
	api.GET("/filter/search", func(c echo.Context) error {
		// Minimal stub that returns 200 — the point is to test auth enforcement.
		return c.JSON(http.StatusOK, map[string]interface{}{
			"startAt":    0,
			"maxResults": 100,
			"total":      0,
			"isLast":     true,
			"values":     []interface{}{},
		})
	})
	return e
}

// TestPropertyPreservation_UnregisteredPathsReturn404 validates Property 2 (Preservation):
// For all generated paths that do NOT match /rest/api/2/filter/search, the unfixed router
// SHALL return HTTP 404, confirming that unregistered paths are properly rejected.
//
// This test captures the baseline routing behavior that must be preserved after the fix.
//
// **Validates: Requirements 3.1, 3.3**
func TestPropertyPreservation_UnregisteredPathsReturn404(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		e := setupFilterSearchRouter()

		// Generate random path segments that do NOT form "filter/search"
		segment := rapid.SampledFrom([]string{
			"nonexistent",
			"foobar",
			"unknown",
			"random-path",
			"filter",      // just "filter" without "/search"
			"filter/list", // filter sub-path that is NOT "search"
			"filter/other",
			"search/filter",  // reversed order
			"filters/search", // plural "filters" not "filter"
			"x/y/z",
			"abc123",
			"some-endpoint",
		}).Draw(t, "path_segment")

		target := fmt.Sprintf("/rest/api/2/%s", segment)
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", filterSearchBasicAuth())
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		// All unregistered paths must return 404 — this is the baseline
		// behavior that the fix must preserve.
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404 for unregistered path %s, got %d", target, rec.Code)
		}
	})
}

// TestPropertyPreservation_AuthMiddlewareEnforced validates Property 2 (Preservation):
// For any request to a route in the /rest/api/2 group WITHOUT valid BasicAuth credentials,
// the middleware SHALL return HTTP 401 Unauthorized.
//
// This confirms that when /filter/search is registered in the protected group,
// the BasicAuth middleware will correctly enforce authentication.
//
// **Validates: Requirements 3.4**
func TestPropertyPreservation_AuthMiddlewareEnforced(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		e := setupFilterSearchRouterWithRoute()

		// Generate various invalid/missing auth header values
		authType := rapid.IntRange(0, 4).Draw(t, "authType")
		var authHeader string
		setHeader := true

		switch authType {
		case 0:
			// No Authorization header at all
			setHeader = false
		case 1:
			// Bearer token (not Basic)
			authHeader = "Bearer sometoken123"
		case 2:
			// Malformed Basic (not valid base64)
			authHeader = "Basic !!!invalid!!!"
		case 3:
			// Basic auth with empty token (user:)
			authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte("user:"))
		case 4:
			// Completely random value
			authHeader = rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, "randomAuth")
		}

		target := "/rest/api/2/filter/search?startAt=0&maxResults=100"
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if setHeader {
			req.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		// Without valid BasicAuth, the middleware must reject with 400 or 401.
		// The middleware returns:
		// - 401 when no "Basic " prefix or empty token
		// - 400 when base64 is invalid or format is wrong
		// Both are acceptable "not 200" responses indicating auth enforcement.
		if rec.Code == http.StatusOK {
			t.Fatalf("expected auth rejection (non-200) for auth header %q, got %d", authHeader, rec.Code)
		}
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 401 or 400 for invalid auth, got %d with auth header %q", rec.Code, authHeader)
		}
	})
}
