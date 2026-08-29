package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

func TestHandleGetIssueChangelog_MixedScalarAndArrayActivities(t *testing.T) {
	// Mock YouTrack server returning activities with mixed scalar and array added/removed fields.
	// This exercises the custom UnmarshalJSON on YTActivityItem for polymorphic values.
	activitiesJSON := `[
  {"id":"act-1","timestamp":1700000000000,"author":{"login":"user1","fullName":"User One"},"field":{"id":"f1","name":"Story Points","presentation":"Story Points"},"removed":5,"added":[{"name":"8","$type":"IssueCustomField"}],"$type":"CustomFieldActivityItem"},
  {"id":"act-2","timestamp":1700001000000,"author":{"login":"user2","fullName":"User Two"},"field":{"id":"f2","name":"Estimation","presentation":"Estimation"},"removed":[{"name":"Low","$type":"EnumBundleElement"}],"added":10,"$type":"CustomFieldActivityItem"},
  {"id":"act-3","timestamp":1700002000000,"author":{"login":"user1","fullName":"User One"},"field":{"id":"f3","name":"State","presentation":"State"},"removed":[{"name":"Open","$type":"StateBundleElement"}],"added":[{"name":"Fixed","$type":"StateBundleElement"}],"$type":"CustomFieldActivityItem"}
]`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/issues/PROJ-1/activities" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(activitiesJSON))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(
		http.MethodGet,
		"/rest/api/2/issue/:issueIdOrKey/changelog",
		"/rest/api/2/issue/PROJ-1/changelog",
		[]string{"issueIdOrKey"},
		[]string{"PROJ-1"},
	)

	err := HandleGetIssueChangelog(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert HTTP 200 (not 502 which would indicate unmarshal failure)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Assert valid JiraChangelogResponse JSON
	var resp model.JiraChangelogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response as JiraChangelogResponse: %v", err)
	}

	// We expect 3 history entries (each activity has a unique timestamp+author combination)
	if len(resp.Histories) != 3 {
		t.Fatalf("expected 3 histories, got %d", len(resp.Histories))
	}

	// History 0: act-1 — scalar removed (5), array added (["8"])
	if len(resp.Histories[0].Items) < 1 {
		t.Fatal("expected at least 1 item in history[0]")
	}
	item0 := resp.Histories[0].Items[0]
	if item0.FromString != "5" {
		t.Errorf("history[0] fromString: expected \"5\", got %q", item0.FromString)
	}
	if item0.ToString != "8" {
		t.Errorf("history[0] toString: expected \"8\", got %q", item0.ToString)
	}

	// History 1: act-2 — array removed (["Low"]), scalar added (10)
	if len(resp.Histories[1].Items) < 1 {
		t.Fatal("expected at least 1 item in history[1]")
	}
	item1 := resp.Histories[1].Items[0]
	if item1.FromString != "Low" {
		t.Errorf("history[1] fromString: expected \"Low\", got %q", item1.FromString)
	}
	if item1.ToString != "10" {
		t.Errorf("history[1] toString: expected \"10\", got %q", item1.ToString)
	}

	// History 2: act-3 — array removed (["Open"]), array added (["Fixed"]) (normal case)
	if len(resp.Histories[2].Items) < 1 {
		t.Fatal("expected at least 1 item in history[2]")
	}
	item2 := resp.Histories[2].Items[0]
	if item2.FromString != "Open" {
		t.Errorf("history[2] fromString: expected \"Open\", got %q", item2.FromString)
	}
	if item2.ToString != "Fixed" {
		t.Errorf("history[2] toString: expected \"Fixed\", got %q", item2.ToString)
	}
}

func TestHandleGetIssueChangelog_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{YouTrackURL: mockServer.URL}
	c, rec := issueTestContext(
		http.MethodGet,
		"/rest/api/2/issue/:issueIdOrKey/changelog",
		"/rest/api/2/issue/PROJ-999/changelog",
		[]string{"issueIdOrKey"},
		[]string{"PROJ-999"},
	)

	err := HandleGetIssueChangelog(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleGetIssueChangelog_EmptyId(t *testing.T) {
	cfg := &config.Config{YouTrackURL: "http://unused.example.com"}
	c, rec := issueTestContext(
		http.MethodGet,
		"/rest/api/2/issue/:issueIdOrKey/changelog",
		"/rest/api/2/issue//changelog",
		[]string{"issueIdOrKey"},
		[]string{""},
	)

	err := HandleGetIssueChangelog(c, cfg, testCache())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty ID, got %d", rec.Code)
	}
}
