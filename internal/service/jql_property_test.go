package service

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyParenStrippingPreservesParseResult validates Property 1: Parenthesis stripping preserves parse result.
// For any valid JQL string that parses successfully without parentheses, wrapping any combination
// of its clauses in balanced parentheses (one or more layers) SHALL produce the same ParsedJQL
// field values.
//
// **Validates: Requirements 1.1, 1.2, 1.5**
func TestPropertyParenStrippingPreservesParseResult(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Decide which clauses to include (at least one).
		hasProject := rapid.Bool().Draw(rt, "hasProject")
		hasCreated := rapid.Bool().Draw(rt, "hasCreated")
		hasUpdated := rapid.Bool().Draw(rt, "hasUpdated")
		hasIssueIn := rapid.Bool().Draw(rt, "hasIssueIn")
		hasOrderBy := rapid.Bool().Draw(rt, "hasOrderBy")

		if !hasProject && !hasCreated && !hasUpdated && !hasIssueIn && !hasOrderBy {
			hasProject = true
		}

		var clauses []string
		var wrappedClauses []string

		// Helper: wrap a clause string in 1-3 layers of parens.
		wrapInParens := func(clause string, label string) string {
			layers := rapid.IntRange(1, 3).Draw(rt, label+"Layers")
			for range layers {
				clause = "(" + clause + ")"
			}
			return clause
		}

		if hasProject {
			key := rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "projectKey")
			clause := fmt.Sprintf("project = %s", key)
			clauses = append(clauses, clause)
			wrappedClauses = append(wrappedClauses, wrapInParens(clause, "project"))
		}

		if hasCreated {
			year := rapid.IntRange(2020, 2030).Draw(rt, "cYear")
			month := rapid.IntRange(1, 12).Draw(rt, "cMonth")
			day := rapid.IntRange(1, 28).Draw(rt, "cDay")
			hour := rapid.IntRange(0, 23).Draw(rt, "cHour")
			min := rapid.IntRange(0, 59).Draw(rt, "cMin")
			ts := fmt.Sprintf("%04d-%02d-%02d %02d:%02d", year, month, day, hour, min)
			clause := fmt.Sprintf("created >= '%s'", ts)
			clauses = append(clauses, clause)
			wrappedClauses = append(wrappedClauses, wrapInParens(clause, "created"))
		}

		if hasUpdated {
			year := rapid.IntRange(2020, 2030).Draw(rt, "uYear")
			month := rapid.IntRange(1, 12).Draw(rt, "uMonth")
			day := rapid.IntRange(1, 28).Draw(rt, "uDay")
			hour := rapid.IntRange(0, 23).Draw(rt, "uHour")
			min := rapid.IntRange(0, 59).Draw(rt, "uMin")
			ts := fmt.Sprintf("%04d-%02d-%02d %02d:%02d", year, month, day, hour, min)
			clause := fmt.Sprintf("updated >= '%s'", ts)
			clauses = append(clauses, clause)
			wrappedClauses = append(wrappedClauses, wrapInParens(clause, "updated"))
		}

		if hasIssueIn {
			keyCount := rapid.IntRange(1, 5).Draw(rt, "keyCount")
			var keys []string
			for i := range keyCount {
				prefix := rapid.StringMatching(`[A-Z]{2,4}`).Draw(rt, fmt.Sprintf("kPfx%d", i))
				num := rapid.IntRange(1, 9999).Draw(rt, fmt.Sprintf("kNum%d", i))
				keys = append(keys, fmt.Sprintf("%s-%d", prefix, num))
			}
			clause := fmt.Sprintf("issue in (%s)", strings.Join(keys, ", "))
			clauses = append(clauses, clause)
			wrappedClauses = append(wrappedClauses, wrapInParens(clause, "issueIn"))
		}

		bareJQL := strings.Join(clauses, " AND ")
		wrappedJQL := strings.Join(wrappedClauses, " AND ")

		// Append ORDER BY if selected (ORDER BY is not wrapped in parens — it's a suffix, not a clause).
		if hasOrderBy {
			dir := rapid.SampledFrom([]string{"ASC", "DESC"}).Draw(rt, "orderDir")
			suffix := " ORDER BY created " + dir
			bareJQL += suffix
			wrappedJQL += suffix
		}

		// Parse both versions.
		expected, err := ParseJQL(bareJQL)
		if err != nil {
			rt.Fatalf("ParseJQL(bare %q) error: %v", bareJQL, err)
		}

		actual, err := ParseJQL(wrappedJQL)
		if err != nil {
			rt.Fatalf("ParseJQL(wrapped %q) error: %v", wrappedJQL, err)
		}

		// Assert all fields are identical.
		if expected.Project != actual.Project {
			rt.Fatalf("Project: bare=%q wrapped=%q\nbare JQL: %s\nwrapped JQL: %s",
				expected.Project, actual.Project, bareJQL, wrappedJQL)
		}
		if expected.CreatedGE != actual.CreatedGE {
			rt.Fatalf("CreatedGE: bare=%q wrapped=%q", expected.CreatedGE, actual.CreatedGE)
		}
		if expected.UpdatedGE != actual.UpdatedGE {
			rt.Fatalf("UpdatedGE: bare=%q wrapped=%q", expected.UpdatedGE, actual.UpdatedGE)
		}
		if len(expected.IssueKeys) != len(actual.IssueKeys) {
			rt.Fatalf("IssueKeys length: bare=%d wrapped=%d", len(expected.IssueKeys), len(actual.IssueKeys))
		}
		for i := range expected.IssueKeys {
			if expected.IssueKeys[i] != actual.IssueKeys[i] {
				rt.Fatalf("IssueKeys[%d]: bare=%q wrapped=%q", i, expected.IssueKeys[i], actual.IssueKeys[i])
			}
		}
		if expected.OrderBy != actual.OrderBy {
			rt.Fatalf("OrderBy: bare=%q wrapped=%q", expected.OrderBy, actual.OrderBy)
		}
		if expected.OrderDir != actual.OrderDir {
			rt.Fatalf("OrderDir: bare=%q wrapped=%q", expected.OrderDir, actual.OrderDir)
		}
		if expected.FilterID != actual.FilterID {
			rt.Fatalf("FilterID: bare=%d wrapped=%d", expected.FilterID, actual.FilterID)
		}
	})
}

// TestPropertyJQLClauseComposition validates Property 4: JQL Clause Composition.
// For any combination of supported JQL clauses (project, created >=, issue in, ORDER BY),
// ParseJQL followed by ToYouTrackQuery produces a YouTrack query string containing the
// corresponding YouTrack fragment for each clause present in the input.
//
// **Validates: Requirements 3.1, 3.2, 3.3, 3.4**
func TestPropertyJQLClauseComposition(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Decide which clauses to include (at least one to avoid empty JQL edge case).
		hasProject := rapid.Bool().Draw(rt, "hasProject")
		hasCreated := rapid.Bool().Draw(rt, "hasCreated")
		hasIssueIn := rapid.Bool().Draw(rt, "hasIssueIn")
		hasOrderBy := rapid.Bool().Draw(rt, "hasOrderBy")

		// Ensure at least one clause is present.
		if !hasProject && !hasCreated && !hasIssueIn && !hasOrderBy {
			hasProject = true
		}

		var clauses []string

		// Generate project clause.
		var projectKey string
		if hasProject {
			projectKey = rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "projectKey")
			clauses = append(clauses, fmt.Sprintf("project = %s", projectKey))
		}

		// Generate created >= clause with a valid-ish timestamp.
		var createdTS string
		if hasCreated {
			year := rapid.IntRange(2020, 2030).Draw(rt, "year")
			month := rapid.IntRange(1, 12).Draw(rt, "month")
			day := rapid.IntRange(1, 28).Draw(rt, "day")
			hour := rapid.IntRange(0, 23).Draw(rt, "hour")
			min := rapid.IntRange(0, 59).Draw(rt, "minute")
			createdTS = fmt.Sprintf("%04d-%02d-%02d %02d:%02d", year, month, day, hour, min)
			clauses = append(clauses, fmt.Sprintf("created >= '%s'", createdTS))
		}

		// Generate issue in clause with 1-5 random issue keys.
		var issueKeys []string
		if hasIssueIn {
			keyCount := rapid.IntRange(1, 5).Draw(rt, "keyCount")
			for i := range keyCount {
				prefix := rapid.StringMatching(`[A-Z]{2,4}`).Draw(rt, fmt.Sprintf("keyPrefix%d", i))
				num := rapid.IntRange(1, 9999).Draw(rt, fmt.Sprintf("keyNum%d", i))
				issueKeys = append(issueKeys, fmt.Sprintf("%s-%d", prefix, num))
			}
			clauses = append(clauses, fmt.Sprintf("issue in (%s)", strings.Join(issueKeys, ", ")))
		}

		// Build the WHERE part joined with AND.
		jql := strings.Join(clauses, " AND ")

		// Append ORDER BY if selected.
		var orderDir string
		if hasOrderBy {
			orderDir = rapid.SampledFrom([]string{"ASC", "DESC"}).Draw(rt, "orderDir")
			jql += " ORDER BY created " + orderDir
		}

		// Parse the JQL — should succeed for any combination of supported clauses.
		parsed, err := ParseJQL(jql)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) returned error: %v", jql, err)
		}

		// Convert to YouTrack query.
		ytQuery := parsed.ToYouTrackQuery()

		// Verify each clause that was included produces the expected fragment.
		if hasProject {
			expected := fmt.Sprintf("project: {%s}", projectKey)
			if !strings.Contains(ytQuery, expected) {
				rt.Fatalf("YouTrack query %q missing project fragment %q (input: %q)", ytQuery, expected, jql)
			}
		}

		if hasCreated {
			// The converter replaces space with T in the timestamp.
			expectedTS := strings.Replace(createdTS, " ", "T", 1)
			expected := fmt.Sprintf("created: {%s} .. *", expectedTS)
			if !strings.Contains(ytQuery, expected) {
				rt.Fatalf("YouTrack query %q missing created fragment %q (input: %q)", ytQuery, expected, jql)
			}
		}

		if hasIssueIn {
			expected := "issue id: " + strings.Join(issueKeys, ", ")
			if !strings.Contains(ytQuery, expected) {
				rt.Fatalf("YouTrack query %q missing issue id fragment %q (input: %q)", ytQuery, expected, jql)
			}
		}

		if hasOrderBy {
			expected := "sort by: created " + strings.ToLower(orderDir)
			if !strings.Contains(ytQuery, expected) {
				rt.Fatalf("YouTrack query %q missing sort fragment %q (input: %q)", ytQuery, expected, jql)
			}
		}

		// When no project was included, project should be empty in parsed result.
		if !hasProject && parsed.Project != "" {
			rt.Fatalf("parsed.Project = %q, expected empty when no project clause", parsed.Project)
		}

		// When no created was included, CreatedGE should be empty.
		if !hasCreated && parsed.CreatedGE != "" {
			rt.Fatalf("parsed.CreatedGE = %q, expected empty when no created clause", parsed.CreatedGE)
		}

		// When no issue in was included, IssueKeys should be empty.
		if !hasIssueIn && len(parsed.IssueKeys) != 0 {
			rt.Fatalf("parsed.IssueKeys = %v, expected empty when no issue in clause", parsed.IssueKeys)
		}

		// When no order by was included, OrderBy should be empty.
		if !hasOrderBy && parsed.OrderBy != "" {
			rt.Fatalf("parsed.OrderBy = %q, expected empty when no ORDER BY clause", parsed.OrderBy)
		}
	})
}
