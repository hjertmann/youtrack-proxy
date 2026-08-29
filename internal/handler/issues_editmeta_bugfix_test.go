package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	authmw "github.com/hjertmann/youtrack-proxy/internal/middleware"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/hjertmann/youtrack-proxy/internal/service"
	"pgregory.net/rapid"
)

// authHeaderConst is a valid Basic Auth header: user@example.com:test-token
const authHeaderConst = "Basic dXNlckBleGFtcGxlLmNvbTp0ZXN0LXRva2Vu"

// setupEditmtaRouter creates an Echo instance with the same route configuration as main.go
// for testing that the editmeta endpoint is registered and responds correctly.
func setupEditmetaRouter(cfg *config.Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	resolvedCache := service.NewResolvedStateCache(1 * time.Hour)
	api := e.Group("/rest/api/2", authmw.BasicAuth())

	// Issue creation
	api.POST("/issue", func(c echo.Context) error {
		return HandleCreateIssue(c, cfg)
	})

	// Issue search and retrieval
	api.GET("/search", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/search/jql", func(c echo.Context) error {
		return HandleSearchIssues(c, cfg, resolvedCache)
	})
	api.GET("/issue/:issueIdOrKey/editmeta", func(c echo.Context) error {
		return HandleGetIssueEditMeta(c, cfg)
	})
	api.GET("/issue/:issueIdOrKey", func(c echo.Context) error {
		return HandleGetIssue(c, cfg, resolvedCache)
	})
	api.GET("/issue/:issueIdOrKey/comment", func(c echo.Context) error {
		return HandleGetIssueComments(c, cfg)
	})

	// Field metadata
	api.GET("/field", HandleListFields)

	return e
}

// TestPropertyBugCondition_EditmetaReturnsValidResponse validates Property 1: Bug Condition.
// For any GET request to /rest/api/2/issue/{issueKey}/editmeta where the issue exists,
// the proxy SHALL return HTTP 200 with a JSON body containing a `fields` object where each
// supported field entry includes `name`, `schema`, and `operations` properties, and enum-type
// fields (issuetype, priority, status) include an `allowedValues` array.
//
// On UNFIXED code, this test FAILS because no route is registered for the editmeta path,
// causing Echo to return its default 404 response.
//
// **Validates: Requirements 1.1, 2.1, 2.3**
func TestPropertyBugCondition_EditmetaReturnsValidResponse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid issue keys
		projectKey := rapid.SampledFrom([]string{"PROJ", "TEST", "DEV", "TEAM", "APP"}).Draw(t, "projectKey")
		issueNum := rapid.IntRange(1, 999).Draw(t, "issueNum")
		issueKey := fmt.Sprintf("%s-%d", projectKey, issueNum)

		// Set up mock YouTrack server that returns a valid issue (to confirm issue exists)
		// and project custom fields
		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle issue fetch (to validate issue exists)
			expectedIssuePath := fmt.Sprintf("/api/issues/%s", issueKey)
			if r.URL.Path == expectedIssuePath {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(model.YTIssue{
					ID:         "2-1",
					IDReadable: issueKey,
					Summary:    "Test Issue",
					Created:    1700000000000,
					Updated:    1700001000000,
					Project: &model.YTProject{
						ID:        "0-0",
						Name:      "Test Project",
						ShortName: projectKey,
					},
				})
				return
			}

			// Handle project custom fields fetch
			if r.URL.Path == "/api/admin/projects/0-0/customFields" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[
					{
						"field": {"name": "Type", "$type": "CustomField"},
						"bundle": {"values": [{"name": "Bug"}, {"name": "Task"}, {"name": "Feature"}]},
						"$type": "EnumProjectCustomField"
					},
					{
						"field": {"name": "Priority", "$type": "CustomField"},
						"bundle": {"values": [{"name": "Critical"}, {"name": "Major"}, {"name": "Normal"}, {"name": "Minor"}]},
						"$type": "EnumProjectCustomField"
					},
					{
						"field": {"name": "State", "$type": "CustomField"},
						"bundle": {"values": [{"name": "Open"}, {"name": "In Progress"}, {"name": "Fixed"}, {"name": "Verified"}]},
						"$type": "StateProjectCustomField"
					},
					{
						"field": {"name": "Assignee", "$type": "CustomField"},
						"$type": "UserProjectCustomField"
					}
				]`))
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		e := setupEditmetaRouter(cfg)

		// Send GET request to the editmeta endpoint
		target := fmt.Sprintf("/rest/api/2/issue/%s/editmeta", issueKey)
		req := httptest.NewRequest(http.MethodGet, target, nil)
		// Valid Basic Auth header: user@example.com:test-token
		req.Header.Set("Authorization", "Basic dXNlckBleGFtcGxlLmNvbTp0ZXN0LXRva2Vu")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// Assert: response should be 200 with valid editmeta JSON
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for GET %s, got %d (body: %s)",
				target, rec.Code, rec.Body.String())
		}

		// Parse response body
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		// Assert: response contains "fields" object
		fieldsRaw, exists := body["fields"]
		if !exists {
			t.Fatalf("response missing 'fields' key, got: %s", rec.Body.String())
		}

		fields, ok := fieldsRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("'fields' is not an object, got: %T", fieldsRaw)
		}

		// Assert: fields map is not empty
		if len(fields) == 0 {
			t.Fatalf("'fields' map is empty")
		}

		// Assert: each field entry includes name, schema, and operations
		for fieldID, fieldRaw := range fields {
			field, ok := fieldRaw.(map[string]interface{})
			if !ok {
				t.Fatalf("field %q is not an object", fieldID)
			}

			if _, hasName := field["name"]; !hasName {
				t.Fatalf("field %q missing 'name' property", fieldID)
			}
			if _, hasSchema := field["schema"]; !hasSchema {
				t.Fatalf("field %q missing 'schema' property", fieldID)
			}
			if _, hasOps := field["operations"]; !hasOps {
				t.Fatalf("field %q missing 'operations' property", fieldID)
			}
		}

		// Assert: enum-type fields include allowedValues
		enumFieldIDs := []string{"issuetype", "priority", "status"}
		for _, enumID := range enumFieldIDs {
			fieldRaw, exists := fields[enumID]
			if !exists {
				t.Fatalf("expected enum field %q not present in fields", enumID)
			}
			field := fieldRaw.(map[string]interface{})
			allowedValues, hasAV := field["allowedValues"]
			if !hasAV {
				t.Fatalf("enum field %q missing 'allowedValues'", enumID)
			}
			avSlice, ok := allowedValues.([]interface{})
			if !ok {
				t.Fatalf("enum field %q 'allowedValues' is not an array", enumID)
			}
			if len(avSlice) == 0 {
				t.Fatalf("enum field %q 'allowedValues' is empty", enumID)
			}
		}
	})
}

// TestPropertyPreservation_IssueRetrievalUnchanged validates Property 2: Preservation.
// For any GET request to /rest/api/2/issue/{key} (without /editmeta), the proxy returns
// HTTP 200 with a Jira-compatible issue JSON containing the expected fields.
// This test confirms baseline behavior on UNFIXED code.
//
// **Validates: Requirements 3.1**
func TestPropertyPreservation_IssueRetrievalUnchanged(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid issue keys
		projectKey := rapid.SampledFrom([]string{"PROJ", "TEST", "DEV", "TEAM", "APP"}).Draw(t, "projectKey")
		issueNum := rapid.IntRange(1, 999).Draw(t, "issueNum")
		issueKey := fmt.Sprintf("%s-%d", projectKey, issueNum)

		desc := "A generated issue description"
		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedPath := fmt.Sprintf("/api/issues/%s", issueKey)
			if r.URL.Path == expectedPath {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(model.YTIssue{
					ID:          "2-1",
					IDReadable:  issueKey,
					Summary:     "Test Issue",
					Description: &desc,
					Created:     1700000000000,
					Updated:     1700001000000,
					Reporter: &model.YTUser{
						Login: "testuser",
						Name:  "Test User",
					},
					Project: &model.YTProject{
						ID:        "0-0",
						Name:      "Test Project",
						ShortName: projectKey,
					},
					CustomFields: []model.YTCustomField{
						{Name: "Type", Value: map[string]interface{}{"name": "Bug", "id": "bug-id"}},
						{Name: "Priority", Value: map[string]interface{}{"name": "Normal", "id": "normal-id"}},
						{Name: "State", Value: map[string]interface{}{"name": "Open", "id": "open-id"}},
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		e := setupEditmetaRouter(cfg)

		// Send GET request to the issue endpoint (NOT editmeta)
		target := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", authHeaderConst)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// Assert: response should be 200
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for GET %s, got %d (body: %s)",
				target, rec.Code, rec.Body.String())
		}

		// Parse response and verify Jira issue structure
		var jiraIssue model.JiraIssue
		if err := json.Unmarshal(rec.Body.Bytes(), &jiraIssue); err != nil {
			t.Fatalf("failed to parse Jira issue response: %v", err)
		}

		// Verify key fields are present
		if jiraIssue.Key != issueKey {
			t.Fatalf("expected issue key=%s, got %s", issueKey, jiraIssue.Key)
		}
		if jiraIssue.Fields.Summary != "Test Issue" {
			t.Fatalf("expected summary='Test Issue', got %s", jiraIssue.Fields.Summary)
		}
		if jiraIssue.Fields.Description == nil || *jiraIssue.Fields.Description != desc {
			t.Fatalf("expected description=%q, got %v", desc, jiraIssue.Fields.Description)
		}
		if jiraIssue.Self == "" {
			t.Fatalf("expected non-empty self URL")
		}
		// Verify custom fields are mapped
		if jiraIssue.Fields.IssueType == nil || jiraIssue.Fields.IssueType.Name != "Bug" {
			t.Fatalf("expected issuetype.name='Bug', got %v", jiraIssue.Fields.IssueType)
		}
		if jiraIssue.Fields.Priority == nil || jiraIssue.Fields.Priority.Name != "Normal" {
			t.Fatalf("expected priority.name='Normal', got %v", jiraIssue.Fields.Priority)
		}
		if jiraIssue.Fields.Status == nil || jiraIssue.Fields.Status.Name != "Open" {
			t.Fatalf("expected status.name='Open', got %v", jiraIssue.Fields.Status)
		}
	})
}

// TestPropertyPreservation_CommentRetrievalUnchanged validates Property 2: Preservation.
// For any GET request to /rest/api/2/issue/{key}/comment, the proxy returns HTTP 200
// with a Jira-compatible comments JSON response.
// This test confirms baseline behavior on UNFIXED code.
//
// **Validates: Requirements 3.2**
func TestPropertyPreservation_CommentRetrievalUnchanged(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid issue keys
		projectKey := rapid.SampledFrom([]string{"PROJ", "TEST", "DEV", "TEAM", "APP"}).Draw(t, "projectKey")
		issueNum := rapid.IntRange(1, 999).Draw(t, "issueNum")
		issueKey := fmt.Sprintf("%s-%d", projectKey, issueNum)

		commentText := "This is a test comment"
		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedPath := fmt.Sprintf("/api/issues/%s/comments", issueKey)
			if r.URL.Path == expectedPath {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]model.YTComment{
					{
						ID: "comment-1",
						Author: &model.YTUser{
							Login: "commenter",
							Name:  "Comment Author",
						},
						Text:    &commentText,
						Created: 1700000000000,
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		e := setupEditmetaRouter(cfg)

		// Send GET request to the comment endpoint
		target := fmt.Sprintf("/rest/api/2/issue/%s/comment", issueKey)
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", authHeaderConst)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// Assert: response should be 200
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for GET %s, got %d (body: %s)",
				target, rec.Code, rec.Body.String())
		}

		// Parse response and verify Jira comments structure
		var resp model.JiraCommentsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse Jira comments response: %v", err)
		}

		// Verify structure
		if len(resp.Comments) != 1 {
			t.Fatalf("expected 1 comment, got %d", len(resp.Comments))
		}
		if resp.Comments[0].ID != "comment-1" {
			t.Fatalf("expected comment ID='comment-1', got %s", resp.Comments[0].ID)
		}
		if resp.Comments[0].Body != commentText {
			t.Fatalf("expected comment body=%q, got %q", commentText, resp.Comments[0].Body)
		}
		if resp.Comments[0].Author == nil || resp.Comments[0].Author.Name != "commenter" {
			t.Fatalf("expected comment author name='commenter', got %v", resp.Comments[0].Author)
		}
		if resp.StartAt != 0 {
			t.Fatalf("expected startAt=0, got %d", resp.StartAt)
		}
	})
}

// TestPropertyPreservation_IssueCreationUnchanged validates Property 2: Preservation.
// For any POST request to /rest/api/2/issue, the proxy creates an issue via YouTrack
// and returns a Jira-compatible response with HTTP 201.
// This test confirms baseline behavior on UNFIXED code.
//
// **Validates: Requirements 3.3**
func TestPropertyPreservation_IssueCreationUnchanged(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		projectKey := rapid.SampledFrom([]string{"PROJ", "TEST", "DEV"}).Draw(t, "projectKey")
		summary := rapid.SampledFrom([]string{"Bug report", "Feature request", "Task item", "Improvement"}).Draw(t, "summary")

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle project custom fields lookup (needed for dynamic field ID resolution)
			if strings.HasSuffix(r.URL.Path, "/customFields") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]model.YTProjectCustomField{
					{ID: "field-type", Field: model.YTCustomFieldRef{Name: "Type"}},
					{ID: "field-priority", Field: model.YTCustomFieldRef{Name: "Priority"}},
					{ID: "field-assignee", Field: model.YTCustomFieldRef{Name: "Assignee"}},
				})
				return
			}

			// Handle issue creation
			if r.URL.Path == "/api/issues" && r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(model.YouTrackResponse{
					ID:   fmt.Sprintf("%s-1", projectKey),
					Type: "Issue",
				})
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		e := setupEditmetaRouter(cfg)

		// Build Jira create issue request body
		body := fmt.Sprintf(`{
			"fields": {
				"project": {"key": "%s"},
				"summary": "%s",
				"description": "Test description",
				"issuetype": {"name": "Task"}
			}
		}`, projectKey, summary)

		req := httptest.NewRequest(http.MethodPost, "/rest/api/2/issue", strings.NewReader(body))
		req.Header.Set("Authorization", authHeaderConst)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// Assert: response should be 201 Created
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status 201 for POST /rest/api/2/issue, got %d (body: %s)",
				rec.Code, rec.Body.String())
		}

		// Parse response and verify Jira create-issue response structure
		var resp model.JiraResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse Jira create issue response: %v", err)
		}

		// Verify response has key and ID
		if resp.Key == "" {
			t.Fatalf("expected non-empty key in create issue response")
		}
		if resp.ID == "" {
			t.Fatalf("expected non-empty id in create issue response")
		}
		if resp.Self == "" {
			t.Fatalf("expected non-empty self URL in create issue response")
		}
	})
}

// TestPropertyPreservation_FieldListUnchanged validates Property 2: Preservation.
// For any GET request to /rest/api/2/field, the proxy returns HTTP 200 with a static
// array of Jira field definitions. This endpoint makes no upstream calls.
// This test confirms baseline behavior on UNFIXED code.
//
// **Validates: Requirements 3.4**
func TestPropertyPreservation_FieldListUnchanged(t *testing.T) {
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	e := setupEditmetaRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/rest/api/2/field", nil)
	req.Header.Set("Authorization", authHeaderConst)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Assert: response should be 200
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for GET /rest/api/2/field, got %d (body: %s)",
			rec.Code, rec.Body.String())
	}

	// Parse response as array of JiraField
	var fields []model.JiraField
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("failed to parse field list response: %v", err)
	}

	// Verify field list is not empty and contains expected system fields
	if len(fields) == 0 {
		t.Fatalf("field list is empty")
	}

	// Verify known system fields are present
	expectedFields := map[string]bool{
		"summary":     false,
		"description": false,
		"issuetype":   false,
		"priority":    false,
		"status":      false,
		"assignee":    false,
		"reporter":    false,
		"project":     false,
		"created":     false,
		"updated":     false,
	}

	for _, field := range fields {
		if _, ok := expectedFields[field.ID]; ok {
			expectedFields[field.ID] = true
		}
		// Every field must have an ID, key, and name
		if field.ID == "" {
			t.Fatalf("field has empty ID")
		}
		if field.Key == "" {
			t.Fatalf("field %s has empty key", field.ID)
		}
		if field.Name == "" {
			t.Fatalf("field %s has empty name", field.ID)
		}
	}

	for fieldID, found := range expectedFields {
		if !found {
			t.Fatalf("expected system field %q not found in field list", fieldID)
		}
	}
}

// TestPropertyPreservation_RouteIsolation validates Property 2: Preservation.
// Verifies that for various issue keys, the /issue/:key route and /issue/:key/comment route
// do NOT accidentally match or conflict with an editmeta-style path.
// Specifically tests that route matching for existing endpoints is not disturbed.
//
// **Validates: Requirements 3.1, 3.2**
func TestPropertyPreservation_RouteIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid issue keys, including ones that could be tricky for routing
		projectKey := rapid.SampledFrom([]string{"PROJ", "EDIT", "META", "EDITMETA", "TEST"}).Draw(t, "projectKey")
		issueNum := rapid.IntRange(1, 999).Draw(t, "issueNum")
		issueKey := fmt.Sprintf("%s-%d", projectKey, issueNum)

		ytServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Respond to issue requests
			if strings.HasPrefix(r.URL.Path, "/api/issues/") && !strings.Contains(r.URL.Path, "/comments") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(model.YTIssue{
					ID:         "2-1",
					IDReadable: issueKey,
					Summary:    "Route Isolation Test",
					Created:    1700000000000,
					Updated:    1700001000000,
					Project: &model.YTProject{
						ID:        "0-0",
						Name:      "Test Project",
						ShortName: projectKey,
					},
				})
				return
			}
			// Respond to comment requests
			if strings.Contains(r.URL.Path, "/comments") {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("[]"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}
		e := setupEditmetaRouter(cfg)

		// Test 1: GET /issue/{key} should return 200 (issue retrieval)
		issueTarget := fmt.Sprintf("/rest/api/2/issue/%s", issueKey)
		req1 := httptest.NewRequest(http.MethodGet, issueTarget, nil)
		req1.Header.Set("Authorization", authHeaderConst)
		rec1 := httptest.NewRecorder()
		e.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d (body: %s)", issueTarget, rec1.Code, rec1.Body.String())
		}

		// Test 2: GET /issue/{key}/comment should return 200 (comments retrieval)
		commentTarget := fmt.Sprintf("/rest/api/2/issue/%s/comment", issueKey)
		req2 := httptest.NewRequest(http.MethodGet, commentTarget, nil)
		req2.Header.Set("Authorization", authHeaderConst)
		rec2 := httptest.NewRecorder()
		e.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d (body: %s)", commentTarget, rec2.Code, rec2.Body.String())
		}

		// Verify the issue response has the correct key (route dispatched to the right handler)
		var jiraIssue model.JiraIssue
		if err := json.Unmarshal(rec1.Body.Bytes(), &jiraIssue); err != nil {
			t.Fatalf("failed to parse issue response: %v", err)
		}
		if jiraIssue.Key != issueKey {
			t.Fatalf("issue response key mismatch: expected %s, got %s", issueKey, jiraIssue.Key)
		}

		// Verify comments response is valid
		var commentsResp model.JiraCommentsResponse
		if err := json.Unmarshal(rec2.Body.Bytes(), &commentsResp); err != nil {
			t.Fatalf("failed to parse comments response: %v", err)
		}
	})
}
