package service

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty2_QuotedContentSurvivesParenStripping validates Property 2:
// Quoted content survives parenthesis stripping.
//
// For any JQL string containing parentheses inside single or double quoted
// project values, the ParsedJQL result SHALL contain the quoted content
// unchanged, including the parentheses.
//
// **Validates: Requirements 1.3**
func TestProperty2_QuotedContentSurvivesParenStripping(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a project name that contains at least one parenthesis.
		// Avoid leading/trailing whitespace since ParseJQL trims extracted values.
		prefix := rapid.StringMatching(`[A-Za-z]{1,6}`).Draw(rt, "prefix")
		inner := rapid.StringMatching(`[A-Za-z0-9]{1,6}`).Draw(rt, "inner")
		suffix := rapid.StringMatching(`[A-Za-z]{0,6}`).Draw(rt, "suffix")

		// Choose paren style inside the name: "prefix(inner)suffix", "(inner)suffix", "prefix(inner)(extra)"
		parenStyle := rapid.IntRange(0, 2).Draw(rt, "parenStyle")
		var projectName string
		switch parenStyle {
		case 0:
			projectName = fmt.Sprintf("%s(%s)%s", prefix, inner, suffix)
		case 1:
			projectName = fmt.Sprintf("(%s)%s", inner, suffix)
		case 2:
			extra := rapid.StringMatching(`[A-Za-z0-9]{1,4}`).Draw(rt, "extra")
			projectName = fmt.Sprintf("%s(%s)(%s)", prefix, inner, extra)
		}

		// Choose quote style: single or double
		useSingle := rapid.Bool().Draw(rt, "useSingle")
		var quotedProject string
		if useSingle {
			quotedProject = fmt.Sprintf("'%s'", projectName)
		} else {
			quotedProject = fmt.Sprintf(`"%s"`, projectName)
		}

		jql := fmt.Sprintf("project = %s", quotedProject)

		// Parse without grouping parens — quoted parens must survive.
		parsed, err := ParseJQL(jql)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) returned error: %v", jql, err)
		}
		if parsed.Project != projectName {
			rt.Fatalf("without grouping parens: parsed.Project = %q, want %q (jql: %q)", parsed.Project, projectName, jql)
		}

		// Now wrap in one or more layers of grouping parens.
		layers := rapid.IntRange(1, 3).Draw(rt, "layers")
		wrapped := jql
		for i := 0; i < layers; i++ {
			wrapped = "(" + wrapped + ")"
		}

		parsed2, err := ParseJQL(wrapped)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) returned error: %v", wrapped, err)
		}
		if parsed2.Project != projectName {
			rt.Fatalf("with %d grouping paren layers: parsed.Project = %q, want %q (jql: %q)", layers, parsed2.Project, projectName, wrapped)
		}
	})
}
