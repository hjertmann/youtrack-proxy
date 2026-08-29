package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// makeProjects generates n unique YTProject values with sequential IDs.
func makeProjects(n int) []model.YTProject {
	projects := make([]model.YTProject, n)
	for i := range n {
		projects[i] = model.YTProject{
			ID:        fmt.Sprintf("0-%d", i),
			Name:      fmt.Sprintf("Project %d", i),
			ShortName: fmt.Sprintf("P%d", i),
		}
	}
	return projects
}

// pagedProjectServer returns an httptest.Server that serves projects from `all`
// using YouTrack-style $skip/$top pagination. Default page size when $top is
// absent mirrors YouTrack's real behaviour (~42).
func pagedProjectServer(all []model.YTProject) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		skip := 0
		if v := r.URL.Query().Get("$skip"); v != "" {
			skip, _ = strconv.Atoi(v)
		}
		top := 42 // YouTrack default when $top is absent
		if v := r.URL.Query().Get("$top"); v != "" {
			top, _ = strconv.Atoi(v)
		}

		end := skip + top
		if end > len(all) {
			end = len(all)
		}
		if skip > len(all) {
			skip = len(all)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(all[skip:end])
	}))
}

// ---------------------------------------------------------------------------
// Task 1: Bug condition exploration tests
// ---------------------------------------------------------------------------

// TestBugCondition_TruncatesLargeInstances demonstrates that GetProjects returns
// ALL projects from a multi-page YouTrack instance. On the unfixed code this
// would only return the first page (~42). After the fix it returns all.
//
// **Validates: Requirements 1.1, 2.1, 2.2, 2.3, 2.4**
func TestBugCondition_TruncatesLargeInstances(t *testing.T) {
	all := makeProjects(55)
	server := pagedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 55 {
		t.Fatalf("GetProjects returned %d projects, want 55", len(result))
	}
}

// TestBugCondition_DeduplicatesAliases demonstrates that GetProjects deduplicates
// alias projects (different ShortName, same internal ID).
//
// **Validates: Requirements 1.5, 2.5**
func TestBugCondition_DeduplicatesAliases(t *testing.T) {
	projects := []model.YTProject{
		{ID: "0-0", Name: "Project Alpha", ShortName: "ALPHA"},
		{ID: "0-1", Name: "Project Beta", ShortName: "BETA"},
		{ID: "0-0", Name: "Project Alpha Alias", ShortName: "ALPHA2"}, // alias of ALPHA
	}
	server := pagedProjectServer(projects)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("GetProjects returned %d projects, want 2 (deduplicated)", len(result))
	}

	// First occurrence should be kept
	if result[0].ShortName != "ALPHA" {
		t.Fatalf("result[0].ShortName = %q, want ALPHA", result[0].ShortName)
	}
	if result[1].ShortName != "BETA" {
		t.Fatalf("result[1].ShortName = %q, want BETA", result[1].ShortName)
	}
}

// TestBugCondition_ExactPageBoundary verifies that when the project count equals
// the page size exactly, GetProjects still returns all projects (it must make one
// extra request that returns an empty page).
//
// **Validates: Requirements 2.1**
func TestBugCondition_ExactPageBoundary(t *testing.T) {
	// 100 projects = exactly one full page at projectPageSize=100
	all := makeProjects(100)
	server := pagedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 100 {
		t.Fatalf("GetProjects returned %d projects, want 100", len(result))
	}
}

// TestBugCondition_ErrorOnPage2Propagates verifies that an error on a subsequent
// page propagates correctly to the caller.
func TestBugCondition_ErrorOnPage2Propagates(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First page: return 100 projects (full page)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(makeProjects(100))
			return
		}
		// Second page: upstream error
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := GetProjects(reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error on page 2 failure, got nil")
	}
}

// TestBugCondition_LargeInstance_250Projects verifies pagination across multiple
// pages with 250 projects (3 pages: 100+100+50).
func TestBugCondition_LargeInstance_250Projects(t *testing.T) {
	all := makeProjects(250)
	server := pagedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 250 {
		t.Fatalf("GetProjects returned %d projects, want 250", len(result))
	}

	// Verify ordering is preserved
	for i, p := range result {
		expected := fmt.Sprintf("0-%d", i)
		if p.ID != expected {
			t.Fatalf("result[%d].ID = %q, want %q", i, p.ID, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 2: Preservation property tests
// ---------------------------------------------------------------------------

// TestPropertyPreservation_SmallInstanceReturnsSameProjects verifies that for any
// project count under the page size, GetProjects returns exactly those projects
// in the same order.
//
// **Validates: Requirements 3.1, 3.3, 3.5**
func TestPropertyPreservation_SmallInstanceReturnsSameProjects(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(0, 42).Draw(t, "projectCount")
		all := makeProjects(count)

		server := pagedProjectServer(all)
		defer server.Close()

		cfg := &config.Config{YouTrackURL: server.URL}
		reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

		result, err := GetProjects(reqCtx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != count {
			t.Fatalf("GetProjects returned %d projects, want %d", len(result), count)
		}

		// Verify same order and same content
		for i := range result {
			if result[i].ID != all[i].ID {
				t.Fatalf("result[%d].ID = %q, want %q", i, result[i].ID, all[i].ID)
			}
			if result[i].ShortName != all[i].ShortName {
				t.Fatalf("result[%d].ShortName = %q, want %q", i, result[i].ShortName, all[i].ShortName)
			}
			if result[i].Name != all[i].Name {
				t.Fatalf("result[%d].Name = %q, want %q", i, result[i].Name, all[i].Name)
			}
		}
	})
}

// TestPropertyPreservation_EmptyInstanceReturnsEmpty verifies that GetProjects
// returns an empty slice when no projects exist.
//
// **Validates: Requirements 3.1**
func TestPropertyPreservation_EmptyInstanceReturnsEmpty(t *testing.T) {
	server := pagedProjectServer(nil)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetProjects returned %d projects, want 0", len(result))
	}
}

// TestPropertyPreservation_SignatureUnchanged is a compile-time check that
// GetProjects still has the expected signature. If this test compiles, the
// signature is preserved.
//
// **Validates: Requirements 3.3**
func TestPropertyPreservation_SignatureUnchanged(t *testing.T) {
	// This is a compile-time assertion — the function variable assignment
	// fails to compile if the signature changes.
	var _ func(*model.RequestContext, *config.Config) ([]model.YTProject, error) = GetProjects
}

// TestPropertyPreservation_AllSizesReturnCorrectCount is a property-based test
// that generates random project counts (0–500) with random alias rates and
// verifies the result count equals unique project count.
//
// **Validates: Requirements 2.1, 2.5, 3.1**
func TestPropertyPreservation_AllSizesReturnCorrectCount(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		totalUnique := rapid.IntRange(0, 300).Draw(t, "totalUnique")
		aliasCount := rapid.IntRange(0, totalUnique/4).Draw(t, "aliasCount") // up to 25% aliases

		// Build project list with some aliases mixed in
		all := makeProjects(totalUnique)
		for i := range aliasCount {
			// Alias: same ID as project 0, different ShortName
			alias := model.YTProject{
				ID:        all[0].ID,
				Name:      fmt.Sprintf("Alias %d", i),
				ShortName: fmt.Sprintf("ALIAS%d", i),
			}
			all = append(all, alias)
		}

		server := pagedProjectServer(all)
		defer server.Close()

		cfg := &config.Config{YouTrackURL: server.URL}
		reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

		result, err := GetProjects(reqCtx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != totalUnique {
			t.Fatalf("GetProjects returned %d projects, want %d unique", len(result), totalUnique)
		}

		// Verify no duplicate IDs
		seen := make(map[string]struct{})
		for _, p := range result {
			if _, dup := seen[p.ID]; dup {
				t.Fatalf("duplicate project ID %q in result", p.ID)
			}
			seen[p.ID] = struct{}{}
		}
	})
}

// TestPropertyPreservation_NonProjectAPIUnchanged verifies that GetFromYouTrack
// for non-project paths is not affected by the pagination change.
//
// **Validates: Requirements 3.5**
func TestPropertyPreservation_NonProjectAPIUnchanged(t *testing.T) {
	expected := `{"message":"ok"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no $skip/$top on non-project calls
		if r.URL.Query().Get("$skip") != "" || r.URL.Query().Get("$top") != "" {
			t.Error("$skip/$top should not be present on non-project API calls")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(expected))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	body, _, err := GetFromYouTrack("/api/issues", "id", nil, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != expected {
		t.Fatalf("body = %q, want %q", string(body), expected)
	}
}
