package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedJQL holds the extracted components of a JQL string.
type ParsedJQL struct {
	Project   string   // from "project = X"
	CreatedGE string   // from "created >= 'timestamp'" — raw timestamp string
	UpdatedGE string   // from "updated >= 'timestamp'" — raw timestamp string
	IssueKeys []string // from "issue in (K1, K2, ...)"
	OrderBy   string   // "created" if ORDER BY created ASC/DESC
	OrderDir  string   // "asc" or "desc"
	FilterID  int64    // from "filter = N"
}

// Compiled regexes for JQL clause extraction.
var (
	// jqlProjectRegex matches "project = X", "project = 'X'", or "project = \"X\""
	// with optional whitespace around the equals sign. Case-insensitive on "project".
	jqlProjectRegex = regexp.MustCompile(`(?i)project\s*=\s*(?:"([^"]+)"|'([^']+)'|(\S+))`)

	// jqlCreatedGERegex matches "created >= 'YYYY-MM-DD HH:mm'" or "created >= \"...\""
	// Case-insensitive on "created".
	jqlCreatedGERegex = regexp.MustCompile(`(?i)created\s*>=\s*(?:'([^']+)'|"([^"]+)")`)

	// jqlUpdatedGERegex matches "updated >= 'YYYY-MM-DD HH:mm'" or "updated >= \"...\""
	// Case-insensitive on "updated".
	jqlUpdatedGERegex = regexp.MustCompile(`(?i)updated\s*>=\s*(?:'([^']+)'|"([^"]+)")`)

	// jqlIssueInRegex matches "issue in (KEY-1, KEY-2, ...)" with optional whitespace.
	// Case-insensitive on "issue in".
	jqlIssueInRegex = regexp.MustCompile(`(?i)issue\s+in\s*\(([^)]+)\)`)

	// jqlOrderByRegex matches "ORDER BY <field> ASC|DESC" at the end of the string.
	// Case-insensitive.
	jqlOrderByRegex = regexp.MustCompile(`(?i)order\s+by\s+(\w+)\s+(asc|desc)\s*$`)

	// jqlFilterRegex matches "filter = N", "filter = 'N'", or "filter = \"N\""
	// where N is a numeric filter ID. Case-insensitive on "filter".
	jqlFilterRegex = regexp.MustCompile(`(?i)filter\s*=\s*(?:"(\d+)"|'(\d+)'|(\d+))`)

	// issueKeyPattern validates that an issue key looks like "PROJ-123".
	issueKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-\d+$`)
)

// stripParentheses removes grouping parentheses that are not inside quoted
// strings and not part of "in (...)" value lists. Returns an error for
// empty "()" or unbalanced parentheses.
func stripParentheses(jql string) (string, error) {
	var result []byte
	inSingleQuote := false
	inDoubleQuote := false
	depth := 0       // grouping paren depth
	inListDepth := 0 // "in (...)" value-list paren depth

	for i := 0; i < len(jql); i++ {
		ch := jql[i]

		switch {
		case inSingleQuote:
			result = append(result, ch)
			if ch == '\'' {
				inSingleQuote = false
			}
		case inDoubleQuote:
			result = append(result, ch)
			if ch == '"' {
				inDoubleQuote = false
			}
		case ch == '\'':
			inSingleQuote = true
			result = append(result, ch)
		case ch == '"':
			inDoubleQuote = true
			result = append(result, ch)
		case ch == '(':
			// Check if preceded by "in" keyword — value-list paren, keep it
			if isInKeywordBefore(result) {
				inListDepth++
				result = append(result, ch)
				continue
			}
			// Check for empty grouping parens "()" — possibly with whitespace inside
			j := i + 1
			for j < len(jql) && jql[j] == ' ' {
				j++
			}
			if j < len(jql) && jql[j] == ')' {
				return "", fmt.Errorf("empty parentheses in JQL")
			}
			depth++
			// skip the grouping paren
		case ch == ')':
			if inListDepth > 0 {
				inListDepth--
				result = append(result, ch)
				continue
			}
			if depth <= 0 {
				return "", fmt.Errorf("unbalanced parentheses in JQL")
			}
			depth--
			// skip the grouping paren
		default:
			result = append(result, ch)
		}
	}

	if depth != 0 {
		return "", fmt.Errorf("unbalanced parentheses in JQL")
	}

	return string(result), nil
}

// isInKeywordBefore checks whether the bytes written so far end with the
// keyword "in" (case-insensitive) preceded by a word boundary (whitespace
// or start of string), allowing trailing whitespace. This identifies
// "in (...)" value-list parens.
func isInKeywordBefore(buf []byte) bool {
	n := len(buf)
	// Skip trailing whitespace
	end := n - 1
	for end >= 0 && (buf[end] == ' ' || buf[end] == '\t') {
		end--
	}
	if end < 1 {
		return false
	}
	// Last two non-whitespace chars must be "in" (case-insensitive)
	if (buf[end-1] != 'i' && buf[end-1] != 'I') || (buf[end] != 'n' && buf[end] != 'N') {
		return false
	}
	// Must be preceded by whitespace or be at the start
	if end-1 == 0 {
		return true
	}
	prev := buf[end-2]
	return prev == ' ' || prev == '\t' || prev == '\n'
}

// ParseJQL extracts known JQL clauses and returns a ParsedJQL.
// Returns an error if an unrecognized clause is found after stripping known ones.
func ParseJQL(jql string) (*ParsedJQL, error) {
	parsed := &ParsedJQL{}

	// Strip parentheses first, before any clause extraction.
	stripped, err := stripParentheses(jql)
	if err != nil {
		return nil, err
	}
	remaining := stripped

	// Extract ORDER BY first (always at the end), before stripping other clauses.
	if m := jqlOrderByRegex.FindStringSubmatchIndex(remaining); m != nil {
		parsed.OrderBy = strings.ToLower(remaining[m[2]:m[3]])
		parsed.OrderDir = strings.ToLower(remaining[m[4]:m[5]])
		remaining = remaining[:m[0]]
	}

	// Extract "project = X"
	if m := jqlProjectRegex.FindStringSubmatch(remaining); m != nil {
		for _, v := range m[1:] {
			if v != "" {
				parsed.Project = strings.TrimSpace(v)
				break
			}
		}
		remaining = jqlProjectRegex.ReplaceAllString(remaining, "")
	}

	// Extract "created >= 'timestamp'"
	if m := jqlCreatedGERegex.FindStringSubmatch(remaining); m != nil {
		for _, v := range m[1:] {
			if v != "" {
				parsed.CreatedGE = strings.ReplaceAll(strings.TrimSpace(v), "/", "-")
				break
			}
		}
		remaining = jqlCreatedGERegex.ReplaceAllString(remaining, "")
	}

	// Extract "updated >= 'timestamp'"
	if m := jqlUpdatedGERegex.FindStringSubmatch(remaining); m != nil {
		for _, v := range m[1:] {
			if v != "" {
				parsed.UpdatedGE = strings.ReplaceAll(strings.TrimSpace(v), "/", "-")
				break
			}
		}
		remaining = jqlUpdatedGERegex.ReplaceAllString(remaining, "")
	}

	// Extract "issue in (K1, K2, ...)"
	if m := jqlIssueInRegex.FindStringSubmatch(remaining); m != nil {
		keys := strings.Split(m[1], ",")
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if !issueKeyPattern.MatchString(k) {
				return nil, fmt.Errorf("invalid issue key: %s", k)
			}
			parsed.IssueKeys = append(parsed.IssueKeys, k)
		}
		remaining = jqlIssueInRegex.ReplaceAllString(remaining, "")
	}

	// Extract "filter = N"
	if m := jqlFilterRegex.FindStringSubmatch(remaining); m != nil {
		for _, v := range m[1:] {
			if v != "" {
				id, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid filter ID: %s", v)
				}
				parsed.FilterID = id
				break
			}
		}
		remaining = jqlFilterRegex.ReplaceAllString(remaining, "")
	}

	// Strip known connectors (AND, OR) and whitespace to check for leftover clauses.
	cleaned := stripConnectors(remaining)
	if cleaned != "" {
		return nil, fmt.Errorf("unsupported JQL clause: %s", strings.TrimSpace(remaining))
	}

	return parsed, nil
}

// stripConnectors removes AND/OR keywords and surrounding whitespace from a JQL fragment.
func stripConnectors(s string) string {
	// Remove AND/OR (case-insensitive, word boundaries)
	re := regexp.MustCompile(`(?i)\b(and|or)\b`)
	s = re.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// ToYouTrackQuery converts a ParsedJQL to a YouTrack query string.
func (p *ParsedJQL) ToYouTrackQuery() string {
	var parts []string

	if p.Project != "" {
		parts = append(parts, fmt.Sprintf("project: {%s}", p.Project))
	}

	if p.CreatedGE != "" {
		// Convert "YYYY-MM-DD HH:mm" to "YYYY-MM-DDTHH:mm" for YouTrack
		ts := strings.Replace(p.CreatedGE, " ", "T", 1)
		parts = append(parts, fmt.Sprintf("created: {%s} .. *", ts))
	}

	if p.UpdatedGE != "" {
		ts := strings.Replace(p.UpdatedGE, " ", "T", 1)
		parts = append(parts, fmt.Sprintf("updated: {%s} .. *", ts))
	}

	if len(p.IssueKeys) > 0 {
		parts = append(parts, "issue id: "+strings.Join(p.IssueKeys, ", "))
	}

	query := strings.Join(parts, " ")

	if p.OrderBy != "" {
		dir := p.OrderDir
		if dir == "" {
			dir = "asc"
		}
		query += " sort by: " + p.OrderBy + " " + dir
	}

	return strings.TrimSpace(query)
}

// ExtractProjectFromJQL extracts the project key from a JQL string containing
// a "project = X" clause. Returns the project key, or an empty string if no
// project clause is found. Delegates to ParseJQL for parsing.
func ExtractProjectFromJQL(jql string) string {
	// ParseJQL may return an error for unrecognized clauses, but
	// ExtractProjectFromJQL is a best-effort extractor — ignore errors.
	parsed, err := ParseJQL(jql)
	if err != nil {
		// Fall back to direct regex extraction for backward compatibility
		// when the JQL contains unsupported clauses alongside a project clause.
		m := jqlProjectRegex.FindStringSubmatch(jql)
		if m == nil {
			return ""
		}
		for _, v := range m[1:] {
			if v != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	return parsed.Project
}
