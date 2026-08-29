package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
	"pgregory.net/rapid"
)

// genScheme generates "http" or "https".
func genScheme(t *rapid.T) string {
	return rapid.SampledFrom([]string{"http", "https"}).Draw(t, "scheme")
}

// genHost generates a plausible hostname like "foo.com" or "tracker.io".
func genHost(t *rapid.T) string {
	return rapid.StringMatching(`[a-z][a-z0-9]{2,15}\.(com|org|io|net)`).Draw(t, "host")
}

// pickerEchoContext creates an Echo context with the given scheme and host.
// Echo's c.Scheme() reads X-Forwarded-Proto, so we set that header.
func pickerEchoContext(scheme, host, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", host, path), nil)
	req.Header.Set("X-Forwarded-Proto", scheme)
	req.Host = host
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// TestIssueTypeSelfURL_Property validates that for any scheme+host combination,
// the self field of every dynamically-fetched issue type
// equals {scheme}://{host}/rest/api/2/issuetype/{id}.
//
// **Validates: Requirements 9.4**
func TestIssueTypeSelfURL_Property(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "69-1", Name: "Task"},
		{ID: "69-2", Name: "Bug"},
		{ID: "69-3", Name: "Story"},
	}
	srv, cfg := mockYTServerForIssueTypes(bundleValues)
	defer srv.Close()

	rapid.Check(t, func(t *rapid.T) {
		scheme := genScheme(t)
		host := genHost(t)

		c, rec := pickerEchoContext(scheme, host, "/rest/api/2/issuetype")
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		if err := HandleListIssueTypes(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var types []model.JiraIssueType
		if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		for _, it := range types {
			want := fmt.Sprintf("%s://%s/rest/api/2/issuetype/%s", scheme, host, it.ID)
			if it.Self != want {
				t.Fatalf("issue type %q: self = %q, want %q", it.Name, it.Self, want)
			}
		}
	})
}

// TestStatusSelfURL_Property validates Property 2:
// For any scheme+host combination, the self field of every dynamically-fetched status
// equals {scheme}://{host}/rest/api/2/status/{id}.
//
// **Validates: Requirements 10.1**
func TestStatusSelfURL_Property(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
		{ID: "71-2", Name: "In Progress"},
		{ID: "71-3", Name: "Fixed"},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	rapid.Check(t, func(t *rapid.T) {
		scheme := genScheme(t)
		host := genHost(t)

		c, rec := pickerEchoContext(scheme, host, "/rest/api/2/status")
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		if err := HandleListStatuses(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var sts []model.JiraStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &sts); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		for _, s := range sts {
			want := fmt.Sprintf("%s://%s/rest/api/2/status/%s", scheme, host, s.ID)
			if s.Self != want {
				t.Fatalf("status %q: self = %q, want %q", s.Name, s.Self, want)
			}
		}
	})
}

// TestStatusCategoryMapping_Property validates Property 3:
// Every dynamically-fetched status maps to one of exactly 3 valid categories
// and matches the defined mapping, regardless of request scheme or host.
//
// **Validates: Requirements 4.5, 10.3**
func TestStatusCategoryMapping_Property(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
		{ID: "71-2", Name: "In Progress"},
		{ID: "71-3", Name: "Fixed", IsResolved: true},
		{ID: "71-4", Name: "Verified", IsResolved: true},
		{ID: "71-5", Name: "Incomplete"},
		{ID: "71-6", Name: "Obsolete", IsResolved: true},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	// Expected mapping: status name → category.
	wantCategory := map[string]model.JiraStatusCategory{
		"Open":        {ID: 1, Name: "To Do", Key: "new", ColorName: "blue-gray"},
		"In Progress": {ID: 4, Name: "In Progress", Key: "indeterminate", ColorName: "yellow"},
		"Fixed":       {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
		"Verified":    {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
		"Incomplete":  {ID: 1, Name: "To Do", Key: "new", ColorName: "blue-gray"},
		"Obsolete":    {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
	}

	rapid.Check(t, func(t *rapid.T) {
		scheme := genScheme(t)
		host := genHost(t)

		c, rec := pickerEchoContext(scheme, host, "/rest/api/2/status")
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		if err := HandleListStatuses(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}

		var sts []model.JiraStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &sts); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		for _, s := range sts {
			want, ok := wantCategory[s.Name]
			if !ok {
				t.Fatalf("unexpected status name %q", s.Name)
			}
			got := s.StatusCategory
			if got.ID != want.ID || got.Name != want.Name || got.Key != want.Key || got.ColorName != want.ColorName {
				t.Fatalf("status %q: category = %+v, want %+v", s.Name, got, want)
			}
		}
	})
}

// genStateName generates a plausible state name: either one from newStates
// (to cover the CatToDo branch) or a random alphabetic string.
func genStateName(t *rapid.T) string {
	knownNames := []string{
		"open", "submitted", "incomplete", "new", "reopened", "to do", "backlog",
		"Fixed", "Verified", "Obsolete", "In Progress", "Won't fix", "Duplicate",
		"Review", "Waiting", "CustomState",
	}
	return rapid.SampledFrom(knownNames).Draw(t, "stateName")
}

// TestPropertyDynamic_StatusListingCategoryMatchesIsResolved validates Property 5:
// For any State bundle value with name N and boolean isResolved, the status listing
// handler SHALL assign:
//   - CatDone when isResolved == true
//   - CatToDo when isResolved == false and strings.ToLower(N) is in newStates
//   - CatInProgress otherwise
//
// This is equivalent to calling MapStateToCategory(N, resolvedSet) where resolvedSet
// is built from the same bundle.
//
// Feature: dynamic-resolved-states, Property 5: Status Listing Category Matches isResolved
//
// **Validates: Requirements 5.2, 5.3, 5.4**
func TestPropertyDynamic_StatusListingCategoryMatchesIsResolved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1-8 bundle values with unique IDs, random names & isResolved flags.
		count := rapid.IntRange(1, 8).Draw(t, "count")
		bundleValues := make([]model.YTBundleValue, count)
		for i := 0; i < count; i++ {
			bundleValues[i] = model.YTBundleValue{
				ID:         fmt.Sprintf("71-%d", i+1),
				Name:       genStateName(t),
				IsResolved: rapid.Bool().Draw(t, fmt.Sprintf("isResolved-%d", i)),
			}
		}

		// Build the expected resolved set from the same bundle (mirrors handler logic).
		expectedResolved := make(service.ResolvedStateSet, len(bundleValues))
		for _, bv := range bundleValues {
			if bv.IsResolved {
				expectedResolved[strings.ToLower(bv.Name)] = struct{}{}
			}
		}

		srv, cfg := mockYTServerForStatuses(bundleValues)
		defer srv.Close()

		scheme := genScheme(t)
		host := genHost(t)
		c, rec := pickerEchoContext(scheme, host, "/rest/api/2/status")
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		if err := HandleListStatuses(c, cfg); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var statuses []model.JiraStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		// Build a name→JiraStatus map from the response for lookup.
		// Because the mock returns the same bundle for two projects, dedup by
		// bundle value ID means each original bundle value appears exactly once.
		statusByName := make(map[string]model.JiraStatus)
		for _, s := range statuses {
			statusByName[s.Name] = s
		}

		for _, bv := range bundleValues {
			s, ok := statusByName[bv.Name]
			if !ok {
				// Dedup may drop a duplicate name that had a different ID; skip.
				continue
			}
			want := service.MapStateToCategory(bv.Name, expectedResolved)
			got := s.StatusCategory
			if got.ID != want.ID || got.Name != want.Name || got.Key != want.Key || got.ColorName != want.ColorName {
				t.Fatalf("status %q (isResolved=%v): category = {ID:%d Name:%q Key:%q Color:%q}, want {ID:%d Name:%q Key:%q Color:%q}",
					bv.Name, bv.IsResolved,
					got.ID, got.Name, got.Key, got.ColorName,
					want.ID, want.Name, want.Key, want.ColorName)
			}
		}
	})
}
