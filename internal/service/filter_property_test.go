package service

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty5_FilterClauseWithExplicitProjectPrecedence validates Property 5:
// Filter clause extraction is idempotent with explicit project.
//
// For any JQL containing both `filter = N` and an explicit `project = X`,
// the parsed Project SHALL always equal X and FilterID SHALL always equal N.
// The handler-level merge (`if parsed.Project == "" { ... }`) is trivially
// correct because Project is already set — this test validates the parser
// extracts both fields without interference.
//
// **Validates: Requirements 3.2, 3.3**
func TestProperty5_FilterClauseWithExplicitProjectPrecedence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random project key: 2-6 uppercase letters.
		projectKey := rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "projectKey")

		// Generate a random positive filter ID.
		filterID := rapid.Int64Range(1, 999999).Draw(rt, "filterID")

		// Choose filter quoting style: bare, single-quoted, double-quoted.
		quoteStyle := rapid.SampledFrom([]string{"bare", "single", "double"}).Draw(rt, "quoteStyle")
		var filterClause string
		switch quoteStyle {
		case "bare":
			filterClause = fmt.Sprintf("filter = %d", filterID)
		case "single":
			filterClause = fmt.Sprintf("filter = '%d'", filterID)
		case "double":
			filterClause = fmt.Sprintf("filter = \"%d\"", filterID)
		}

		projectClause := fmt.Sprintf("project = %s", projectKey)

		// Vary clause order: filter first or project first.
		filterFirst := rapid.Bool().Draw(rt, "filterFirst")
		var jql string
		if filterFirst {
			jql = filterClause + " AND " + projectClause
		} else {
			jql = projectClause + " AND " + filterClause
		}

		// Optionally add other supported clauses to increase variety.
		if rapid.Bool().Draw(rt, "hasUpdated") {
			year := rapid.IntRange(2020, 2030).Draw(rt, "uYear")
			month := rapid.IntRange(1, 12).Draw(rt, "uMonth")
			day := rapid.IntRange(1, 28).Draw(rt, "uDay")
			hour := rapid.IntRange(0, 23).Draw(rt, "uHour")
			min := rapid.IntRange(0, 59).Draw(rt, "uMin")
			ts := fmt.Sprintf("%04d-%02d-%02d %02d:%02d", year, month, day, hour, min)
			jql += fmt.Sprintf(" AND updated >= '%s'", ts)
		}

		// Optionally wrap clauses in parentheses.
		if rapid.Bool().Draw(rt, "withParens") {
			// Wrap each AND-separated group in parens.
			parts := strings.Split(jql, " AND ")
			for i, p := range parts {
				parts[i] = "(" + p + ")"
			}
			jql = strings.Join(parts, " AND ")
		}

		parsed, err := ParseJQL(jql)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) error: %v", jql, err)
		}

		// Core assertion: explicit project always wins.
		if parsed.Project != projectKey {
			rt.Fatalf("Project: got %q, want %q (JQL: %s)", parsed.Project, projectKey, jql)
		}

		// Core assertion: filter ID always extracted.
		if parsed.FilterID != filterID {
			rt.Fatalf("FilterID: got %d, want %d (JQL: %s)", parsed.FilterID, filterID, jql)
		}
	})
}
