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
)

// TestGetFromYouTrack_Success verifies that a 200 response returns bytes and status correctly.
func TestGetFromYouTrack_Success(t *testing.T) {
	expected := `{"message":"ok"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expected))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	body, status, err := GetFromYouTrack("/api/test", "", nil, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if string(body) != expected {
		t.Fatalf("body = %q, want %q", string(body), expected)
	}
}

// TestGetFromYouTrack_Fields verifies the `fields` query param is set correctly.
func TestGetFromYouTrack_Fields(t *testing.T) {
	var capturedFields string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedFields = r.URL.Query().Get("fields")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, _, err := GetFromYouTrack("/api/test", "id,name,shortName", nil, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFields != "id,name,shortName" {
		t.Fatalf("fields = %q, want %q", capturedFields, "id,name,shortName")
	}
}

// TestGetFromYouTrack_QueryParams verifies additional query params are forwarded.
func TestGetFromYouTrack_QueryParams(t *testing.T) {
	var capturedQuery, capturedSkip string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		capturedSkip = r.URL.Query().Get("$skip")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	params := map[string]string{
		"query": "project: DEMO",
		"$skip": "10",
	}

	_, _, err := GetFromYouTrack("/api/issues", "id", params, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != "project: DEMO" {
		t.Fatalf("query = %q, want %q", capturedQuery, "project: DEMO")
	}
	if capturedSkip != "10" {
		t.Fatalf("$skip = %q, want %q", capturedSkip, "10")
	}
}

// TestGetFromYouTrack_AuthHeader verifies Authorization: Bearer header is set from requestCtx token.
func TestGetFromYouTrack_AuthHeader(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "my-secret-token"}

	_, _, err := GetFromYouTrack("/api/test", "", nil, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedAuth != "Bearer my-secret-token" {
		t.Fatalf("Authorization = %q, want %q", capturedAuth, "Bearer my-secret-token")
	}
}

// TestGetFromYouTrack_NotFound verifies that a 404 response returns *YouTrackError with IsNotFound == true.
func TestGetFromYouTrack_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Not Found"}`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, status, err := GetFromYouTrack("/api/issues/NONEXIST-1", "", nil, reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", status, http.StatusNotFound)
	}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound(err) = false, want true")
	}
}

// TestGetFromYouTrack_ServerError verifies that a 500 response returns *YouTrackError with StatusCode 500.
func TestGetFromYouTrack_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, status, err := GetFromYouTrack("/api/test", "", nil, reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
	}

	ytErr, ok := err.(*YouTrackError)
	if !ok {
		t.Fatalf("error type = %T, want *YouTrackError", err)
	}
	if ytErr.StatusCode != 500 {
		t.Fatalf("YouTrackError.StatusCode = %d, want 500", ytErr.StatusCode)
	}
	if ytErr.Message != "Internal Server Error" {
		t.Fatalf("YouTrackError.Message = %q, want %q", ytErr.Message, "Internal Server Error")
	}
}

// TestGetProjects_Success verifies that GetProjects returns parsed []YTProject from valid JSON.
func TestGetProjects_Success(t *testing.T) {
	projects := []model.YTProject{
		{
			ID:        "0-0",
			Name:      "Demo Project",
			ShortName: "DEMO",
			Type:      "Project",
		},
		{
			ID:        "0-1",
			Name:      "Second Project",
			ShortName: "SEC",
			Type:      "Project",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(projects)
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].ShortName != "DEMO" {
		t.Fatalf("result[0].ShortName = %q, want %q", result[0].ShortName, "DEMO")
	}
	if result[1].Name != "Second Project" {
		t.Fatalf("result[1].Name = %q, want %q", result[1].Name, "Second Project")
	}
}

// TestGetIssue_NotFound verifies that GetIssue returns an error where IsNotFound is true for 404.
func TestGetIssue_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Issue not found"}`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := GetIssue("NONEXIST-999", reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound(err) = false, want true")
	}
}

// TestCountIssues_Empty verifies CountIssues returns 0 when no issues match.
func TestCountIssues_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	total, err := CountIssues("project: EMPTY", reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
}

// TestCountIssues_SingleBatch verifies CountIssues counts correctly when all results fit in one batch.
func TestCountIssues_SingleBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 3 issues
		issues := []map[string]string{
			{"idReadable": "TEST-1"},
			{"idReadable": "TEST-2"},
			{"idReadable": "TEST-3"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(issues)
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	total, err := CountIssues("project: TEST", reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
}

// TestCountIssues_ForwardsQuery verifies the query parameter is passed to YouTrack.
func TestCountIssues_ForwardsQuery(t *testing.T) {
	var capturedQuery, capturedFields string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		capturedFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := CountIssues("project: {My Project}", reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != "project: {My Project}" {
		t.Fatalf("query = %q, want %q", capturedQuery, "project: {My Project}")
	}
	if capturedFields != "idReadable" {
		t.Fatalf("fields = %q, want %q", capturedFields, "idReadable")
	}
}

// TestCountIssues_UpstreamError verifies CountIssues returns an error on upstream failure.
func TestCountIssues_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := CountIssues("project: TEST", reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestCountIssues_EmptyQuery verifies CountIssues works without a query filter.
func TestCountIssues_EmptyQuery(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		issues := []map[string]string{{"idReadable": "X-1"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issues)
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	total, err := CountIssues("", reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if capturedQuery != "" {
		t.Fatalf("query = %q, want empty", capturedQuery)
	}
}

// TestGetIssues_Pagination verifies that $skip and $top query params are set correctly.
func TestGetIssues_Pagination(t *testing.T) {
	var capturedSkip, capturedTop string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSkip = r.URL.Query().Get("$skip")
		capturedTop = r.URL.Query().Get("$top")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]model.YTIssue{})
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := GetIssues("project: TEST", 25, 10, reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSkip != "25" {
		t.Fatalf("$skip = %q, want %q", capturedSkip, "25")
	}
	if capturedTop != "10" {
		t.Fatalf("$top = %q, want %q", capturedTop, "10")
	}
}

// TestGetAllProjectCustomFields_Success verifies that Type and State fields are collected across projects.
func TestGetAllProjectCustomFields_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/admin/projects":
			json.NewEncoder(w).Encode([]model.YTProject{
				{ID: "0-0", Name: "Alpha", ShortName: "ALPHA"},
				{ID: "0-1", Name: "Beta", ShortName: "BETA"},
			})
		case r.URL.Path == "/api/admin/projects/0-0/customFields":
			json.NewEncoder(w).Encode([]model.YTProjectCustomField{
				{Field: model.YTCustomFieldRef{Name: "Type"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "t1", Name: "Bug"}}}},
				{Field: model.YTCustomFieldRef{Name: "State"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "s1", Name: "Open"}}}},
				{Field: model.YTCustomFieldRef{Name: "Priority"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "p1", Name: "High"}}}},
			})
		case r.URL.Path == "/api/admin/projects/0-1/customFields":
			json.NewEncoder(w).Encode([]model.YTProjectCustomField{
				{Field: model.YTCustomFieldRef{Name: "Type"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "t2", Name: "Task"}}}},
				{Field: model.YTCustomFieldRef{Name: "State"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "s2", Name: "Done"}}}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetAllProjectCustomFields(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have 4 entries: Type+State from each of 2 projects (Priority excluded)
	if len(result) != 4 {
		t.Fatalf("len(result) = %d, want 4", len(result))
	}

	names := map[string]int{}
	for _, f := range result {
		names[f.Field.Name]++
	}
	if names["Type"] != 2 {
		t.Fatalf("Type count = %d, want 2", names["Type"])
	}
	if names["State"] != 2 {
		t.Fatalf("State count = %d, want 2", names["State"])
	}
	if names["Priority"] != 0 {
		t.Fatalf("Priority count = %d, want 0", names["Priority"])
	}
}

// TestGetAllProjectCustomFields_SkipsFailingProject verifies that a failing project is skipped.
func TestGetAllProjectCustomFields_SkipsFailingProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/admin/projects":
			json.NewEncoder(w).Encode([]model.YTProject{
				{ID: "0-0", Name: "Good", ShortName: "GOOD"},
				{ID: "0-1", Name: "Bad", ShortName: "BAD"},
			})
		case r.URL.Path == "/api/admin/projects/0-0/customFields":
			json.NewEncoder(w).Encode([]model.YTProjectCustomField{
				{Field: model.YTCustomFieldRef{Name: "Type"}, Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "t1", Name: "Bug"}}}},
			})
		case r.URL.Path == "/api/admin/projects/0-1/customFields":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`Internal Server Error`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetAllProjectCustomFields(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the good project's Type field should be returned
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Field.Name != "Type" {
		t.Fatalf("result[0].Field.Name = %q, want %q", result[0].Field.Name, "Type")
	}
}

// TestGetAllProjectCustomFields_GetProjectsFails verifies error propagation when GetProjects fails.
func TestGetAllProjectCustomFields_GetProjectsFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server down`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := GetAllProjectCustomFields(reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetAllProjectCustomFields_NoProjects verifies empty result when no projects exist.
func TestGetAllProjectCustomFields_NoProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]model.YTProject{})
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetAllProjectCustomFields(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(result))
	}
}

// --- Upstream Project Pagination Bug Tests ---

// makeYTProjects generates n YouTrack projects with unique IDs.
func makeYTProjects(n int) []model.YTProject {
	projects := make([]model.YTProject, n)
	for i := 0; i < n; i++ {
		projects[i] = model.YTProject{
			ID:        fmt.Sprintf("0-%d", i),
			Name:      fmt.Sprintf("Project %d", i),
			ShortName: fmt.Sprintf("P%d", i),
		}
	}
	return projects
}

// paginatedProjectServer returns an httptest.Server that serves projects
// with YouTrack-style $skip/$top pagination.
func paginatedProjectServer(allProjects []model.YTProject) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/projects" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		skip := 0
		if v := r.URL.Query().Get("$skip"); v != "" {
			skip, _ = strconv.Atoi(v)
		}
		top := 42 // YouTrack default
		if v := r.URL.Query().Get("$top"); v != "" {
			top, _ = strconv.Atoi(v)
		}
		end := skip + top
		if end > len(allProjects) {
			end = len(allProjects)
		}
		var page []model.YTProject
		if skip < len(allProjects) {
			page = allProjects[skip:end]
		} else {
			page = []model.YTProject{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	}))
}

// TestGetProjects_PaginatesUpstream verifies that GetProjects fetches all projects
// from a multi-page YouTrack response.
func TestGetProjects_PaginatesUpstream(t *testing.T) {
	all := makeYTProjects(155) // well over 100 page size
	server := paginatedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 155 {
		t.Fatalf("len(result) = %d, want 155", len(result))
	}
}

// TestGetProjects_DeduplicatesAliases verifies that projects with the same ID
// (aliases) are deduplicated to one entry.
func TestGetProjects_DeduplicatesAliases(t *testing.T) {
	all := []model.YTProject{
		{ID: "0-0", Name: "Project A", ShortName: "PA"},
		{ID: "0-1", Name: "Project B", ShortName: "PB"},
		{ID: "0-0", Name: "Project A Alias", ShortName: "PAA"}, // same ID as first
		{ID: "0-2", Name: "Project C", ShortName: "PC"},
	}
	server := paginatedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3 (dedup by ID)", len(result))
	}
	// First occurrence should be kept
	if result[0].ShortName != "PA" {
		t.Fatalf("result[0].ShortName = %q, want %q", result[0].ShortName, "PA")
	}
}

// TestGetProjects_EmptyInstance verifies GetProjects returns empty slice for zero projects.
func TestGetProjects_EmptyInstance(t *testing.T) {
	server := paginatedProjectServer([]model.YTProject{})
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(result))
	}
}

// TestGetProjects_SmallInstance verifies GetProjects works with fewer than page size projects.
func TestGetProjects_SmallInstance(t *testing.T) {
	all := makeYTProjects(10)
	server := paginatedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 10 {
		t.Fatalf("len(result) = %d, want 10", len(result))
	}
}

// TestGetProjects_ExactPageBoundary verifies behavior when project count equals page size exactly.
func TestGetProjects_ExactPageBoundary(t *testing.T) {
	all := makeYTProjects(100) // exactly projectPageSize
	server := paginatedProjectServer(all)
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	result, err := GetProjects(reqCtx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 100 {
		t.Fatalf("len(result) = %d, want 100", len(result))
	}
}

// TestGetProjects_ErrorOnSecondPage verifies error propagation when a mid-pagination page fails.
func TestGetProjects_ErrorOnSecondPage(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First page: return 100 projects (full page triggers next request)
			projects := makeYTProjects(100)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(projects)
			return
		}
		// Second page: server error
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	}))
	defer server.Close()

	cfg := &config.Config{YouTrackURL: server.URL}
	reqCtx := &model.RequestContext{YouTrackToken: "test-token"}

	_, err := GetProjects(reqCtx, cfg)
	if err == nil {
		t.Fatal("expected error on second page, got nil")
	}
}
