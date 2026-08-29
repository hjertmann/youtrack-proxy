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
	"pgregory.net/rapid"
)

// mockYouTrackProjectServer creates an httptest.Server that simulates the YouTrack
// project API. It recognizes a set of valid project keys and returns 404 for all
// others — mirroring real YouTrack behavior.
func mockYouTrackProjectServer(validProjects map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// List all projects endpoint
		if r.URL.Path == "/api/admin/projects" {
			var projects []string
			for key, name := range validProjects {
				projects = append(projects, fmt.Sprintf(
					`{"id":"0-%s","name":%q,"shortName":%q,"description":"Description of %s","leader":{"login":"admin","fullName":"Admin User","email":"admin@test.com","banned":false},"$type":"Project"}`,
					key, name, key, key,
				))
			}
			resp := "["
			for i, p := range projects {
				if i > 0 {
					resp += ","
				}
				resp += p
			}
			resp += "]"
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
			return
		}

		// Single project lookup: /api/admin/projects/<key>
		const prefix = "/api/admin/projects/"
		if len(r.URL.Path) > len(prefix) {
			key := r.URL.Path[len(prefix):]
			if name, ok := validProjects[key]; ok {
				resp := fmt.Sprintf(
					`{"id":"0-%s","name":%q,"shortName":%q,"description":"Description of %s","leader":{"login":"admin","fullName":"Admin User","email":"admin@test.com","banned":false},"$type":"Project"}`,
					key, name, key, key,
				)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(resp))
				return
			}
		}

		// Not found
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Entity not found"}`))
	}))
}

// TestPropertyPreservation_ValidProjectKeyReturns200 verifies that for any valid
// project key (not "recent"), HandleGetProject returns HTTP 200 with a single
// JiraProject containing the correct key, name, and lead.
//
// This tests existing behavior on unfixed code that must remain unchanged after the fix.
//
// **Validates: Requirements 3.1, 3.2**
func TestPropertyPreservation_ValidProjectKeyReturns200(t *testing.T) {
	validProjects := map[string]string{
		"DEMO":    "Demo Project",
		"TEST":    "Test Project",
		"ALPHA":   "Alpha Project",
		"MYPROJ":  "My Project",
		"XYZ":     "XYZ Project",
		"PROJECT": "Project Name",
	}

	ytServer := mockYouTrackProjectServer(validProjects)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	rapid.Check(t, func(t *rapid.T) {
		// Draw a valid project key from the known set, excluding "recent"
		keys := make([]string, 0, len(validProjects))
		for k := range validProjects {
			keys = append(keys, k)
		}
		key := rapid.SampledFrom(keys).Draw(t, "projectKey")

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/"+key, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
		c.SetParamNames("projectIdOrKey")
		c.SetParamValues(key)

		err := HandleGetProject(c, cfg)
		if err != nil {
			t.Fatalf("handler returned error for key %q: %v", key, err)
		}

		// Property: valid key → HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /project/%s: status = %d, want %d\nBody: %s",
				key, rec.Code, http.StatusOK, rec.Body.String())
		}

		// Property: response is a single JiraProject with matching key
		var project model.JiraProject
		if err := json.Unmarshal(rec.Body.Bytes(), &project); err != nil {
			t.Fatalf("failed to unmarshal response for key %q: %v", key, err)
		}

		if project.Key != key {
			t.Fatalf("project.Key = %q, want %q", project.Key, key)
		}
		expectedName := validProjects[key]
		if project.Name != expectedName {
			t.Fatalf("project.Name = %q, want %q", project.Name, expectedName)
		}
		if project.Lead == nil {
			t.Fatalf("project.Lead is nil for key %q", key)
		}
	})
}

// TestPropertyPreservation_InvalidProjectKeyReturns404 verifies that for any
// invalid/non-existent project key (not "recent"), HandleGetProject returns
// HTTP 404 with "Project not found" error message.
//
// This tests existing behavior on unfixed code that must remain unchanged after the fix.
//
// **Validates: Requirements 3.1, 3.2**
func TestPropertyPreservation_InvalidProjectKeyReturns404(t *testing.T) {
	// Only DEMO and TEST are valid — everything else should 404
	validProjects := map[string]string{
		"DEMO": "Demo Project",
		"TEST": "Test Project",
	}

	ytServer := mockYouTrackProjectServer(validProjects)
	defer ytServer.Close()

	cfg := &config.Config{YouTrackURL: ytServer.URL}

	rapid.Check(t, func(t *rapid.T) {
		// Generate invalid project keys that are NOT in the valid set and NOT "recent"
		invalidKey := rapid.SampledFrom([]string{
			"NONEXISTENT",
			"INVALID",
			"FOOBAR",
			"NOPROJECT",
			"ZZZ",
			"MISSING",
			"UNKNOWN",
			"NOPE",
			"BADKEY",
			"XYZZY",
		}).Draw(t, "invalidKey")

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project/"+invalidKey, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})
		c.SetParamNames("projectIdOrKey")
		c.SetParamValues(invalidKey)

		err := HandleGetProject(c, cfg)
		if err != nil {
			t.Fatalf("handler returned error for invalid key %q: %v", invalidKey, err)
		}

		// Property: invalid key → HTTP 404
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /project/%s: status = %d, want %d\nBody: %s",
				invalidKey, rec.Code, http.StatusNotFound, rec.Body.String())
		}

		// Property: error response contains "Project not found"
		var errResp model.JiraErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to unmarshal error response for key %q: %v", invalidKey, err)
		}

		found := false
		for _, msg := range errResp.ErrorMessages {
			if msg == "Project not found" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected error message 'Project not found' for key %q, got %v",
				invalidKey, errResp.ErrorMessages)
		}
	})
}

// TestPropertyPreservation_ListProjectsReturns200 verifies that HandleListProjects
// returns HTTP 200 with a JSON array of all projects. The count and keys in the
// response must match the upstream YouTrack data.
//
// This tests existing behavior on unfixed code that must remain unchanged after the fix.
//
// **Validates: Requirements 3.3**
func TestPropertyPreservation_ListProjectsReturns200(t *testing.T) {
	// Use varying project configurations to ensure the list endpoint is robust
	projectSets := []map[string]string{
		{"DEMO": "Demo Project", "TEST": "Test Project"},
		{"ALPHA": "Alpha", "BETA": "Beta", "GAMMA": "Gamma"},
		{"SINGLE": "Single Project"},
	}

	rapid.Check(t, func(t *rapid.T) {
		// Pick a random project set
		idx := rapid.IntRange(0, len(projectSets)-1).Draw(t, "projectSetIdx")
		validProjects := projectSets[idx]

		ytServer := mockYouTrackProjectServer(validProjects)
		defer ytServer.Close()

		cfg := &config.Config{YouTrackURL: ytServer.URL}

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/rest/api/2/project", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("requestCtx", &model.RequestContext{YouTrackToken: "test-token"})

		err := HandleListProjects(c, cfg)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}

		// Property: list endpoint → HTTP 200
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /project: status = %d, want %d\nBody: %s",
				rec.Code, http.StatusOK, rec.Body.String())
		}

		// Property: response is a JSON array with correct project count
		var projects []model.JiraProject
		if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
			t.Fatalf("failed to unmarshal response: %v\nBody: %s", err, rec.Body.String())
		}

		if len(projects) != len(validProjects) {
			t.Fatalf("got %d projects, want %d", len(projects), len(validProjects))
		}

		// Property: every project key in the response is from the valid set
		for _, p := range projects {
			if _, ok := validProjects[p.Key]; !ok {
				t.Fatalf("unexpected project key %q in response", p.Key)
			}
		}
	})
}
