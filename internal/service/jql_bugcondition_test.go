package service

import (
	"strings"
	"testing"
)

// --- TestParseJQL_UpdatedGE_BugCondition ---
// Bug condition exploration: these tests encode the EXPECTED behavior for
// "updated >= 'timestamp'" clauses. On unfixed code they fail (compile error:
// ParsedJQL has no field UpdatedGE), confirming the bug exists.
// Validates: Requirements 1.1, 1.2

func TestParseJQL_UpdatedGE_BugCondition(t *testing.T) {
	t.Run("updated >= single-quoted", func(t *testing.T) {
		p, err := ParseJQL("updated >= '2024-06-01 00:00'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.UpdatedGE != "2024-06-01 00:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2024-06-01 00:00")
		}
	})

	t.Run("updated >= double-quoted", func(t *testing.T) {
		p, err := ParseJQL(`updated >= "2024-03-01 12:00"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.UpdatedGE != "2024-03-01 12:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2024-03-01 12:00")
		}
	})

	t.Run("updated >= with project and ORDER BY updated", func(t *testing.T) {
		p, err := ParseJQL("project = TP AND updated >= '2024-01-15 09:30' ORDER BY updated ASC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "TP" {
			t.Errorf("Project = %q, want %q", p.Project, "TP")
		}
		if p.UpdatedGE != "2024-01-15 09:30" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2024-01-15 09:30")
		}
		if p.OrderBy != "updated" {
			t.Errorf("OrderBy = %q, want %q", p.OrderBy, "updated")
		}
		if p.OrderDir != "asc" {
			t.Errorf("OrderDir = %q, want %q", p.OrderDir, "asc")
		}
	})

	t.Run("updated >= combined with created >=", func(t *testing.T) {
		p, err := ParseJQL("project = TP AND created >= '2024-01-01 00:00' AND updated >= '2024-06-01 00:00'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.CreatedGE != "2024-01-01 00:00" {
			t.Errorf("CreatedGE = %q, want %q", p.CreatedGE, "2024-01-01 00:00")
		}
		if p.UpdatedGE != "2024-06-01 00:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2024-06-01 00:00")
		}
	})
}

// --- TestToYouTrackQuery_UpdatedGE ---
// Bug condition exploration: verifies ToYouTrackQuery emits "updated: {ts} .. *"
// when UpdatedGE is populated. Fails to compile on unfixed code (no UpdatedGE field).
// Validates: Requirements 2.1, 2.2

func TestToYouTrackQuery_UpdatedGE(t *testing.T) {
	t.Run("updated only", func(t *testing.T) {
		p := &ParsedJQL{UpdatedGE: "2024-06-01 00:00"}
		q := p.ToYouTrackQuery()
		expected := "updated: {2024-06-01T00:00} .. *"
		if q != expected {
			t.Errorf("query = %q, want %q", q, expected)
		}
	})

	t.Run("project + created + updated", func(t *testing.T) {
		p := &ParsedJQL{
			Project:   "TP",
			CreatedGE: "2024-01-01 00:00",
			UpdatedGE: "2024-06-01 00:00",
		}
		q := p.ToYouTrackQuery()
		if !strings.Contains(q, "project: {TP}") {
			t.Errorf("query %q missing project fragment", q)
		}
		if !strings.Contains(q, "created: {2024-01-01T00:00} .. *") {
			t.Errorf("query %q missing created fragment", q)
		}
		if !strings.Contains(q, "updated: {2024-06-01T00:00} .. *") {
			t.Errorf("query %q missing updated fragment", q)
		}
	})
}
