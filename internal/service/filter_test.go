package service

import (
	"errors"
	"testing"
)

// --- TestResolveFilterProject_UnknownID ---

func TestResolveFilterProject_UnknownID(t *testing.T) {
	// -1 is negative, so Decode returns ("", false) → ErrFilterNotFound.
	_, err := ResolveFilterProject(-1, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown filter ID, got nil")
	}
	if !errors.Is(err, ErrFilterNotFound) {
		t.Errorf("error = %v, want ErrFilterNotFound", err)
	}
}

// --- TestParseJQL_FilterMergePrecedence ---
// These tests verify the JQL parsing layer that feeds the merge logic in HandleSearchIssues.
// The handler merge rule: if parsed.Project != "", keep it (explicit takes precedence).

func TestParseJQL_FilterMergePrecedence(t *testing.T) {
	t.Run("filter with explicit project — explicit wins", func(t *testing.T) {
		p, err := ParseJQL("filter = 42 AND project = MYPROJ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 42 {
			t.Errorf("FilterID = %d, want 42", p.FilterID)
		}
		if p.Project != "MYPROJ" {
			t.Errorf("Project = %q, want %q", p.Project, "MYPROJ")
		}
		// Merge rule: parsed.Project is non-empty, so the handler keeps it.
	})

	t.Run("filter alone — project comes from resolved filter", func(t *testing.T) {
		p, err := ParseJQL("filter = 42")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 42 {
			t.Errorf("FilterID = %d, want 42", p.FilterID)
		}
		if p.Project != "" {
			t.Errorf("Project = %q, want empty (would be filled by filter resolution)", p.Project)
		}
		// Merge rule: parsed.Project is empty, so the handler fills it from the resolved filter.
	})

	t.Run("filter with updated clause — both extracted", func(t *testing.T) {
		p, err := ParseJQL("filter = 7 AND updated >= '2026-01-01 00:00'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 7 {
			t.Errorf("FilterID = %d, want 7", p.FilterID)
		}
		if p.UpdatedGE != "2026-01-01 00:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2026-01-01 00:00")
		}
		if p.Project != "" {
			t.Errorf("Project = %q, want empty", p.Project)
		}
	})

	t.Run("filter with parens and explicit project", func(t *testing.T) {
		p, err := ParseJQL("(filter = 10) AND (project = ZZZ)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 10 {
			t.Errorf("FilterID = %d, want 10", p.FilterID)
		}
		if p.Project != "ZZZ" {
			t.Errorf("Project = %q, want %q", p.Project, "ZZZ")
		}
	})
}
