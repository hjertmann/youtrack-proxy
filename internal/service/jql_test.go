package service

import (
	"strings"
	"testing"
)

// --- TestParseJQL ---

func TestParseJQL(t *testing.T) {
	t.Run("project only", func(t *testing.T) {
		p, err := ParseJQL("project = TP")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "TP" {
			t.Errorf("Project = %q, want %q", p.Project, "TP")
		}
	})

	t.Run("project double-quoted", func(t *testing.T) {
		p, err := ParseJQL(`project = "My Project"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "My Project" {
			t.Errorf("Project = %q, want %q", p.Project, "My Project")
		}
	})

	t.Run("project single-quoted", func(t *testing.T) {
		p, err := ParseJQL("project = 'DEMO'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "DEMO" {
			t.Errorf("Project = %q, want %q", p.Project, "DEMO")
		}
	})

	t.Run("created >= clause", func(t *testing.T) {
		p, err := ParseJQL("created >= '2024-01-15 09:30'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.CreatedGE != "2024-01-15 09:30" {
			t.Errorf("CreatedGE = %q, want %q", p.CreatedGE, "2024-01-15 09:30")
		}
	})

	t.Run("created >= with double quotes", func(t *testing.T) {
		p, err := ParseJQL(`created >= "2024-06-01 00:00"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.CreatedGE != "2024-06-01 00:00" {
			t.Errorf("CreatedGE = %q, want %q", p.CreatedGE, "2024-06-01 00:00")
		}
	})

	t.Run("issue in clause", func(t *testing.T) {
		p, err := ParseJQL("issue in (TP-1, TP-2, TP-3)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(p.IssueKeys) != 3 {
			t.Fatalf("IssueKeys len = %d, want 3", len(p.IssueKeys))
		}
		expected := []string{"TP-1", "TP-2", "TP-3"}
		for i, k := range expected {
			if p.IssueKeys[i] != k {
				t.Errorf("IssueKeys[%d] = %q, want %q", i, p.IssueKeys[i], k)
			}
		}
	})

	t.Run("ORDER BY created ASC", func(t *testing.T) {
		p, err := ParseJQL("project = TP ORDER BY created ASC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.OrderBy != "created" {
			t.Errorf("OrderBy = %q, want %q", p.OrderBy, "created")
		}
		if p.OrderDir != "asc" {
			t.Errorf("OrderDir = %q, want %q", p.OrderDir, "asc")
		}
	})

	t.Run("ORDER BY created DESC", func(t *testing.T) {
		p, err := ParseJQL("project = TP ORDER BY created DESC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.OrderDir != "desc" {
			t.Errorf("OrderDir = %q, want %q", p.OrderDir, "desc")
		}
	})

	t.Run("all clauses combined", func(t *testing.T) {
		jql := "project = TP AND created >= '2024-01-15 09:30' AND issue in (TP-1, TP-2) ORDER BY created ASC"
		p, err := ParseJQL(jql)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "TP" {
			t.Errorf("Project = %q, want %q", p.Project, "TP")
		}
		if p.CreatedGE != "2024-01-15 09:30" {
			t.Errorf("CreatedGE = %q, want %q", p.CreatedGE, "2024-01-15 09:30")
		}
		if len(p.IssueKeys) != 2 {
			t.Fatalf("IssueKeys len = %d, want 2", len(p.IssueKeys))
		}
		if p.OrderBy != "created" {
			t.Errorf("OrderBy = %q, want %q", p.OrderBy, "created")
		}
		if p.OrderDir != "asc" {
			t.Errorf("OrderDir = %q, want %q", p.OrderDir, "asc")
		}
	})

	t.Run("empty JQL", func(t *testing.T) {
		p, err := ParseJQL("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "" || p.CreatedGE != "" || len(p.IssueKeys) != 0 || p.OrderBy != "" {
			t.Errorf("expected empty ParsedJQL for empty input, got %+v", p)
		}
	})

	t.Run("unrecognized clause returns error", func(t *testing.T) {
		_, err := ParseJQL("status = Open AND assignee = john")
		if err == nil {
			t.Fatal("expected error for unrecognized JQL clause, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported JQL clause") {
			t.Errorf("error = %q, want to contain 'unsupported JQL clause'", err.Error())
		}
	})

	t.Run("project with unrecognized clause returns error", func(t *testing.T) {
		_, err := ParseJQL("project = TP AND status = Open")
		if err == nil {
			t.Fatal("expected error for unrecognized JQL clause, got nil")
		}
	})

	t.Run("case insensitive project", func(t *testing.T) {
		p, err := ParseJQL("PROJECT = ABC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "ABC" {
			t.Errorf("Project = %q, want %q", p.Project, "ABC")
		}
	})

	t.Run("case insensitive order by", func(t *testing.T) {
		p, err := ParseJQL("project = TP order by created asc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.OrderBy != "created" {
			t.Errorf("OrderBy = %q, want %q", p.OrderBy, "created")
		}
	})

	t.Run("issue in with spaces around keys", func(t *testing.T) {
		p, err := ParseJQL("issue in ( TP-1 , TP-2 )")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(p.IssueKeys) != 2 {
			t.Fatalf("IssueKeys len = %d, want 2", len(p.IssueKeys))
		}
		if p.IssueKeys[0] != "TP-1" || p.IssueKeys[1] != "TP-2" {
			t.Errorf("IssueKeys = %v, want [TP-1 TP-2]", p.IssueKeys)
		}
	})
}

// --- TestToYouTrackQuery ---

func TestToYouTrackQuery(t *testing.T) {
	t.Run("project only", func(t *testing.T) {
		p := &ParsedJQL{Project: "TP"}
		q := p.ToYouTrackQuery()
		if q != "project: {TP}" {
			t.Errorf("query = %q, want %q", q, "project: {TP}")
		}
	})

	t.Run("created >= only", func(t *testing.T) {
		p := &ParsedJQL{CreatedGE: "2024-01-15 09:30"}
		q := p.ToYouTrackQuery()
		if q != "created: {2024-01-15T09:30} .. *" {
			t.Errorf("query = %q, want %q", q, "created: {2024-01-15T09:30} .. *")
		}
	})

	t.Run("issue keys only", func(t *testing.T) {
		p := &ParsedJQL{IssueKeys: []string{"TP-1", "TP-2"}}
		q := p.ToYouTrackQuery()
		if q != "issue id: TP-1, TP-2" {
			t.Errorf("query = %q, want %q", q, "issue id: TP-1, TP-2")
		}
	})

	t.Run("order by only", func(t *testing.T) {
		p := &ParsedJQL{OrderBy: "created", OrderDir: "asc"}
		q := p.ToYouTrackQuery()
		if q != "sort by: created asc" {
			t.Errorf("query = %q, want %q", q, "sort by: created asc")
		}
	})

	t.Run("all clauses combined", func(t *testing.T) {
		p := &ParsedJQL{
			Project:   "TP",
			CreatedGE: "2024-01-15 09:30",
			IssueKeys: []string{"TP-1", "TP-2"},
			OrderBy:   "created",
			OrderDir:  "asc",
		}
		q := p.ToYouTrackQuery()
		expected := "project: {TP} created: {2024-01-15T09:30} .. * issue id: TP-1, TP-2 sort by: created asc"
		if q != expected {
			t.Errorf("query = %q, want %q", q, expected)
		}
	})

	t.Run("empty parsed JQL", func(t *testing.T) {
		p := &ParsedJQL{}
		q := p.ToYouTrackQuery()
		if q != "" {
			t.Errorf("query = %q, want empty string", q)
		}
	})

	t.Run("order by defaults dir to asc", func(t *testing.T) {
		p := &ParsedJQL{OrderBy: "created"}
		q := p.ToYouTrackQuery()
		if q != "sort by: created asc" {
			t.Errorf("query = %q, want %q", q, "sort by: created asc")
		}
	})
}

// --- TestParseJQLRoundTrip ---

func TestParseJQLRoundTrip(t *testing.T) {
	t.Run("project + order round-trips to youtrack query", func(t *testing.T) {
		p, err := ParseJQL("project = TP ORDER BY created ASC")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		q := p.ToYouTrackQuery()
		if !strings.Contains(q, "project: {TP}") {
			t.Errorf("query %q missing project fragment", q)
		}
		if !strings.Contains(q, "sort by: created asc") {
			t.Errorf("query %q missing sort fragment", q)
		}
	})

	t.Run("full JQL round-trips", func(t *testing.T) {
		jql := "project = TP AND created >= '2024-01-15 09:30' AND issue in (TP-1, TP-2) ORDER BY created ASC"
		p, err := ParseJQL(jql)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		q := p.ToYouTrackQuery()
		if !strings.Contains(q, "project: {TP}") {
			t.Errorf("query %q missing project fragment", q)
		}
		if !strings.Contains(q, "created: {2024-01-15T09:30} .. *") {
			t.Errorf("query %q missing created fragment", q)
		}
		if !strings.Contains(q, "issue id: TP-1, TP-2") {
			t.Errorf("query %q missing issue id fragment", q)
		}
		if !strings.Contains(q, "sort by: created asc") {
			t.Errorf("query %q missing sort fragment", q)
		}
	})
}

// --- TestExtractProjectFromJQL (backward compatibility) ---
// These tests mirror the existing TestExtractProjectFromJQL from converter_read_test.go
// to ensure the delegating implementation maintains backward compatibility.

func TestExtractProjectFromJQL_Delegation(t *testing.T) {
	tests := []struct {
		name     string
		jql      string
		expected string
	}{
		{
			name:     "unquoted project key",
			jql:      "project = TP",
			expected: "TP",
		},
		{
			name:     "double-quoted project key",
			jql:      `project = "My Project"`,
			expected: "My Project",
		},
		{
			name:     "single-quoted project key",
			jql:      "project = 'DEMO'",
			expected: "DEMO",
		},
		{
			name:     "no project clause",
			jql:      "status = Open AND assignee = john",
			expected: "",
		},
		{
			name:     "empty string",
			jql:      "",
			expected: "",
		},
		{
			name:     "project clause with unsupported clauses (fallback)",
			jql:      "project = TP AND status = Open",
			expected: "TP",
		},
		{
			name:     "case insensitive PROJECT",
			jql:      "PROJECT = ABC",
			expected: "ABC",
		},
		{
			name:     "extra whitespace around equals",
			jql:      "project  =  SPACE",
			expected: "SPACE",
		},
		{
			name:     "project clause at end",
			jql:      "status = Open AND project = END",
			expected: "END",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractProjectFromJQL(tt.jql)
			if result != tt.expected {
				t.Errorf("ExtractProjectFromJQL(%q) = %q, want %q", tt.jql, result, tt.expected)
			}
		})
	}
}

// ponytail: Bug condition tests (TestParseJQL_UpdatedGE_BugCondition, TestToYouTrackQuery_UpdatedGE)
// moved to jql_bugcondition_test.go behind //go:build bugfix_updated tag.
// They reference ParsedJQL.UpdatedGE which doesn't exist yet, so they break compilation.
// They'll be moved back once the fix lands in task 3.

// --- TestParseJQL_ParenthesisStripping ---

func TestParseJQL_ParenthesisStripping(t *testing.T) {
	t.Run("single parens stripped", func(t *testing.T) {
		p, err := ParseJQL("(project = PDA)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "PDA" {
			t.Errorf("Project = %q, want %q", p.Project, "PDA")
		}
	})

	t.Run("nested parens stripped", func(t *testing.T) {
		p, err := ParseJQL("((project = PDA))")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "PDA" {
			t.Errorf("Project = %q, want %q", p.Project, "PDA")
		}
	})

	t.Run("multi-group parens stripped", func(t *testing.T) {
		p, err := ParseJQL("(project = PDA) AND (updated >= '2026-01-01 00:00')")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "PDA" {
			t.Errorf("Project = %q, want %q", p.Project, "PDA")
		}
		if p.UpdatedGE != "2026-01-01 00:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2026-01-01 00:00")
		}
	})

	t.Run("quoted parens preserved", func(t *testing.T) {
		p, err := ParseJQL("project = '(TEST)'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Project != "(TEST)" {
			t.Errorf("Project = %q, want %q", p.Project, "(TEST)")
		}
	})

	t.Run("empty parens error", func(t *testing.T) {
		_, err := ParseJQL("()")
		if err == nil {
			t.Fatal("expected error for empty parens, got nil")
		}
		if !strings.Contains(err.Error(), "empty parentheses") {
			t.Errorf("error = %q, want to contain 'empty parentheses'", err.Error())
		}
	})

	t.Run("unbalanced open paren error", func(t *testing.T) {
		_, err := ParseJQL("(project = PDA")
		if err == nil {
			t.Fatal("expected error for unbalanced paren, got nil")
		}
		if !strings.Contains(err.Error(), "unbalanced parentheses") {
			t.Errorf("error = %q, want to contain 'unbalanced parentheses'", err.Error())
		}
	})

	t.Run("unbalanced close paren error", func(t *testing.T) {
		_, err := ParseJQL("project = PDA)")
		if err == nil {
			t.Fatal("expected error for unbalanced paren, got nil")
		}
		if !strings.Contains(err.Error(), "unbalanced parentheses") {
			t.Errorf("error = %q, want to contain 'unbalanced parentheses'", err.Error())
		}
	})
}

// --- TestParseJQL_SlashDateNormalization ---

func TestParseJQL_SlashDateNormalization(t *testing.T) {
	t.Run("updated with slash dates normalized", func(t *testing.T) {
		p, err := ParseJQL("updated >= '2026/08/27 07:52'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.UpdatedGE != "2026-08-27 07:52" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2026-08-27 07:52")
		}
	})

	t.Run("created with slash dates normalized", func(t *testing.T) {
		p, err := ParseJQL("created >= '2026/08/27 07:52'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.CreatedGE != "2026-08-27 07:52" {
			t.Errorf("CreatedGE = %q, want %q", p.CreatedGE, "2026-08-27 07:52")
		}
	})

	t.Run("dash dates unchanged", func(t *testing.T) {
		p, err := ParseJQL("updated >= '2026-08-27 07:52'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.UpdatedGE != "2026-08-27 07:52" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2026-08-27 07:52")
		}
	})
}

// --- TestToYouTrackQuery_SlashDateRoundTrip ---

func TestToYouTrackQuery_SlashDateRoundTrip(t *testing.T) {
	t.Run("updated >= only", func(t *testing.T) {
		p := &ParsedJQL{UpdatedGE: "2026-08-27 07:52"}
		q := p.ToYouTrackQuery()
		if q != "updated: {2026-08-27T07:52} .. *" {
			t.Errorf("query = %q, want %q", q, "updated: {2026-08-27T07:52} .. *")
		}
	})

	t.Run("slash date round-trip via parse and convert", func(t *testing.T) {
		p, err := ParseJQL("updated >= '2026/08/27 07:52'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		q := p.ToYouTrackQuery()
		if q != "updated: {2026-08-27T07:52} .. *" {
			t.Errorf("query = %q, want %q", q, "updated: {2026-08-27T07:52} .. *")
		}
	})
}

// --- TestParseJQL_FilterClause ---

func TestParseJQL_FilterClause(t *testing.T) {
	t.Run("filter unquoted", func(t *testing.T) {
		p, err := ParseJQL("filter = 123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 123 {
			t.Errorf("FilterID = %d, want %d", p.FilterID, 123)
		}
	})

	t.Run("filter single-quoted", func(t *testing.T) {
		p, err := ParseJQL("filter = '123'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 123 {
			t.Errorf("FilterID = %d, want %d", p.FilterID, 123)
		}
	})

	t.Run("filter double-quoted", func(t *testing.T) {
		p, err := ParseJQL(`filter = "123"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 123 {
			t.Errorf("FilterID = %d, want %d", p.FilterID, 123)
		}
	})

	t.Run("filter combined with updated", func(t *testing.T) {
		p, err := ParseJQL("filter = 42 AND updated >= '2026-01-01 00:00'")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.FilterID != 42 {
			t.Errorf("FilterID = %d, want %d", p.FilterID, 42)
		}
		if p.UpdatedGE != "2026-01-01 00:00" {
			t.Errorf("UpdatedGE = %q, want %q", p.UpdatedGE, "2026-01-01 00:00")
		}
	})
}
