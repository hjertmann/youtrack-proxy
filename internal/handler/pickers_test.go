package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// pickerContext creates an Echo context for picker handler tests.
func pickerContext(method, target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// mockYTServerForIssueTypes starts a mock YouTrack server that returns the given
// Type bundle values across two project endpoints.
func mockYTServerForIssueTypes(bundleValues []model.YTBundleValue) (*httptest.Server, *config.Config) {
	projectsJSON := `[{"id":"0-0","name":"Project One","shortName":"P1"},{"id":"0-1","name":"Project Two","shortName":"P2"}]`

	// Both projects return the same Type bundle
	customFieldsJSON := fmt.Sprintf(`[{"field":{"name":"Type","$type":"FieldType"},"bundle":{"values":%s,"$type":"EnumBundle"},"$type":"EnumProjectCustomField"}]`, mustMarshal(bundleValues))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/admin/projects" && !containsCustomFields(r.URL.Path):
			w.Write([]byte(projectsJSON))
		default:
			// Per-project custom fields
			w.Write([]byte(customFieldsJSON))
		}
	}))

	cfg := &config.Config{YouTrackURL: srv.URL}
	return srv, cfg
}

func containsCustomFields(path string) bool {
	return len(path) > len("/api/admin/projects") && path != "/api/admin/projects"
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func issueTypeContext(srv *httptest.Server) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/issuetype", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	return c, rec
}

func TestHandleListIssueTypes_DynamicTypes(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "69-1", Name: "Task"},
		{ID: "69-2", Name: "Bug"},
		{ID: "69-3", Name: "Story"},
	}
	srv, cfg := mockYTServerForIssueTypes(bundleValues)
	defer srv.Close()

	c, rec := issueTypeContext(srv)

	if err := HandleListIssueTypes(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var types []model.JiraIssueType
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("expected 3 issue types, got %d", len(types))
	}

	for _, it := range types {
		if it.Name != "Task" && it.Name != "Bug" && it.Name != "Story" {
			t.Errorf("unexpected issue type name %q", it.Name)
		}
		if it.Subtask {
			t.Errorf("type %q: Subtask = true, want false", it.Name)
		}
		if it.ID == "" {
			t.Errorf("type %q: ID is empty", it.Name)
		}
	}
}

func TestHandleListIssueTypes_DeduplicatesAcrossProjects(t *testing.T) {
	// Both projects will return the same bundle values — should be deduped
	bundleValues := []model.YTBundleValue{
		{ID: "69-1", Name: "Task"},
		{ID: "69-2", Name: "Bug"},
	}
	srv, cfg := mockYTServerForIssueTypes(bundleValues)
	defer srv.Close()

	c, rec := issueTypeContext(srv)

	if err := HandleListIssueTypes(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var types []model.JiraIssueType
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Even though 2 projects return same bundles, should only get 2 types
	if len(types) != 2 {
		t.Fatalf("expected 2 deduplicated issue types, got %d", len(types))
	}
}

func TestHandleListIssueTypes_SelfURL(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "69-1", Name: "Task"},
	}
	srv, cfg := mockYTServerForIssueTypes(bundleValues)
	defer srv.Close()

	c, rec := issueTypeContext(srv)

	if err := HandleListIssueTypes(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var types []model.JiraIssueType
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	baseURL := "http://example.com"
	for _, it := range types {
		want := baseURL + "/rest/api/2/issuetype/" + it.ID
		if it.Self != want {
			t.Errorf("self URL for %s = %q, want %q", it.Name, it.Self, want)
		}
	}
}

func TestHandleListIssueTypes_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{YouTrackURL: srv.URL}
	c, rec := issueTypeContext(srv)

	if err := HandleListIssueTypes(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleListIssueTypes_NumericIDs(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "69-1", Name: "Task"},
		{ID: "69-2", Name: "Bug"},
	}
	srv, cfg := mockYTServerForIssueTypes(bundleValues)
	defer srv.Close()

	c, rec := issueTypeContext(srv)

	if err := HandleListIssueTypes(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var types []model.JiraIssueType
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	for _, it := range types {
		// ID must be a valid positive integer string
		if it.ID == "" || it.ID[0] == '0' {
			t.Errorf("type %q: ID %q has leading zero or is empty", it.Name, it.ID)
		}
	}
}

// mockYTServerForStatuses starts a mock YouTrack server that returns the given
// State bundle values across two project endpoints.
func mockYTServerForStatuses(bundleValues []model.YTBundleValue) (*httptest.Server, *config.Config) {
	projectsJSON := `[{"id":"0-0","name":"Project One","shortName":"P1"},{"id":"0-1","name":"Project Two","shortName":"P2"}]`

	customFieldsJSON := fmt.Sprintf(`[{"field":{"name":"State","$type":"FieldType"},"bundle":{"values":%s,"$type":"EnumBundle"},"$type":"EnumProjectCustomField"}]`, mustMarshal(bundleValues))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/admin/projects" && !containsCustomFields(r.URL.Path):
			w.Write([]byte(projectsJSON))
		default:
			w.Write([]byte(customFieldsJSON))
		}
	}))

	cfg := &config.Config{YouTrackURL: srv.URL}
	return srv, cfg
}

func statusContext(srv *httptest.Server) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
	return c, rec
}

func TestHandleListStatuses_DynamicStatuses(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
		{ID: "71-2", Name: "In Progress"},
		{ID: "71-3", Name: "Fixed"},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var statuses []model.JiraStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Name != "Open" && s.Name != "In Progress" && s.Name != "Fixed" {
			t.Errorf("unexpected status name %q", s.Name)
		}
		if s.ID == "" {
			t.Errorf("status %q: ID is empty", s.Name)
		}
	}
}

func TestHandleListStatuses_CategoryMapping(t *testing.T) {
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

	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var statuses []model.JiraStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	wantCategory := map[string]model.JiraStatusCategory{
		"Open":        {ID: 1, Name: "To Do", Key: "new", ColorName: "blue-gray"},
		"In Progress": {ID: 4, Name: "In Progress", Key: "indeterminate", ColorName: "yellow"},
		"Fixed":       {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
		"Verified":    {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
		"Incomplete":  {ID: 1, Name: "To Do", Key: "new", ColorName: "blue-gray"},
		"Obsolete":    {ID: 3, Name: "Done", Key: "done", ColorName: "green"},
	}

	for _, s := range statuses {
		want, ok := wantCategory[s.Name]
		if !ok {
			t.Errorf("unexpected status name %q", s.Name)
			continue
		}
		got := s.StatusCategory
		if got.ID != want.ID || got.Name != want.Name || got.Key != want.Key || got.ColorName != want.ColorName {
			t.Errorf("status %q category = %+v, want %+v", s.Name, got, want)
		}
	}
}

func TestHandleListStatuses_SelfURL(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var statuses []model.JiraStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	baseURL := "http://example.com"
	for _, s := range statuses {
		want := baseURL + "/rest/api/2/status/" + s.ID
		if s.Self != want {
			t.Errorf("self URL for %s = %q, want %q", s.Name, s.Self, want)
		}
	}
}

func TestHandleListStatuses_DeduplicatesAcrossProjects(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
		{ID: "71-2", Name: "Fixed"},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var statuses []model.JiraStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Both projects return same bundles — should be deduped
	if len(statuses) != 2 {
		t.Fatalf("expected 2 deduplicated statuses, got %d", len(statuses))
	}
}

func TestHandleListStatuses_NumericIDs(t *testing.T) {
	bundleValues := []model.YTBundleValue{
		{ID: "71-1", Name: "Open"},
		{ID: "71-2", Name: "In Progress"},
	}
	srv, cfg := mockYTServerForStatuses(bundleValues)
	defer srv.Close()

	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var statuses []model.JiraStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	for _, s := range statuses {
		if s.ID == "" || s.ID[0] == '0' {
			t.Errorf("status %q: ID %q has leading zero or is empty", s.Name, s.ID)
		}
	}
}

func TestHandleListStatuses_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{YouTrackURL: srv.URL}
	c, rec := statusContext(srv)

	if err := HandleListStatuses(c, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
