package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// YTProject represents a project returned by the YouTrack API.
type YTProject struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ShortName   string  `json:"shortName"`
	Description *string `json:"description"`
	Leader      *YTUser `json:"leader"`
	Type        string  `json:"$type"`
}

// YTIssue represents an issue returned by the YouTrack API.
type YTIssue struct {
	ID           string          `json:"id"`
	IDReadable   string          `json:"idReadable"`
	Summary      string          `json:"summary"`
	Description  *string         `json:"description"`
	Created      int64           `json:"created"`
	Updated      int64           `json:"updated"`
	Reporter     *YTUser         `json:"reporter"`
	Project      *YTProject      `json:"project"`
	CustomFields []YTCustomField `json:"customFields"`
	Resolved     int64           `json:"resolved"`
	Tags         []YTTag         `json:"tags"`
	Type         string          `json:"$type"`
}

// YTTag represents a tag on a YouTrack issue.
type YTTag struct {
	Name string `json:"name"`
	Type string `json:"$type"`
}

// YTCustomField represents a custom field on a YouTrack issue.
// Value is interface{} because it can be an object (with name/login fields) or null.
type YTCustomField struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
	Type  string      `json:"$type"`
}

// YTUser represents a user returned by the YouTrack API.
type YTUser struct {
	Login  string `json:"login"`
	Name   string `json:"fullName"`
	Email  string `json:"email"`
	Banned bool   `json:"banned"`
	Type   string `json:"$type"`
}

// YTComment represents a comment on a YouTrack issue.
type YTComment struct {
	ID      string  `json:"id"`
	Author  *YTUser `json:"author"`
	Text    *string `json:"text"`
	Created int64   `json:"created"`
	Updated *int64  `json:"updated"`
	Type    string  `json:"$type"`
}

// YTBundleValue represents a single allowed value within a YouTrack field bundle.
type YTBundleValue struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsResolved bool   `json:"isResolved"`
	Type       string `json:"$type"`
}

// YTFieldBundle represents the bundle of allowed values for a YouTrack project custom field.
type YTFieldBundle struct {
	Values []YTBundleValue `json:"values"`
	Type   string          `json:"$type"`
}

// YTCustomFieldRef represents the field reference within a project custom field configuration.
type YTCustomFieldRef struct {
	Name string `json:"name"`
	Type string `json:"$type"`
}

// YTProjectCustomField represents a custom field configuration for a YouTrack project,
// as returned by the /api/admin/projects/{id}/customFields endpoint.
type YTProjectCustomField struct {
	ID     string           `json:"id"`
	Field  YTCustomFieldRef `json:"field"`
	Bundle *YTFieldBundle   `json:"bundle"`
	Type   string           `json:"$type"`
}

// YTActivityItem represents a single activity (history change) on a YouTrack issue.
type YTActivityItem struct {
	ID        string        `json:"id"`
	Timestamp int64         `json:"timestamp"`
	Author    *YTUser       `json:"author"`
	Field     *YTFieldRef   `json:"field"`
	Added     []YTFieldDiff `json:"added"`
	Removed   []YTFieldDiff `json:"removed"`
	Type      string        `json:"$type"`
}

// YTFieldRef identifies which field was changed in an activity item.
type YTFieldRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Presentation string `json:"presentation"`
	Type         string `json:"$type"`
}

// YTFieldDiff represents an added or removed value in an activity item.
type YTFieldDiff struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"$type"`
}

// ytActivityItemAlias is a type alias used to avoid infinite recursion when
// defining a custom UnmarshalJSON on YTActivityItem.
type ytActivityItemAlias YTActivityItem

// UnmarshalJSON implements custom JSON unmarshaling for YTActivityItem to handle
// polymorphic "added" and "removed" fields. The YouTrack API may return these as
// JSON arrays of objects, scalar numbers, scalar strings, or null.
func (item *YTActivityItem) UnmarshalJSON(data []byte) error {
	// Temporary struct that captures added/removed as raw JSON
	var raw struct {
		ytActivityItemAlias
		Added   json.RawMessage `json:"added"`
		Removed json.RawMessage `json:"removed"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Copy all non-polymorphic fields from the alias
	*item = YTActivityItem(raw.ytActivityItemAlias)

	// Parse the polymorphic added field
	added, err := parseFieldDiffValue(raw.Added)
	if err != nil {
		return fmt.Errorf("parsing added field: %w", err)
	}
	item.Added = added

	// Parse the polymorphic removed field
	removed, err := parseFieldDiffValue(raw.Removed)
	if err != nil {
		return fmt.Errorf("parsing removed field: %w", err)
	}
	item.Removed = removed

	return nil
}

// parseFieldDiffValue parses a raw JSON value that may be an array of YTFieldDiff
// objects, a scalar number, a scalar string, a boolean, or null.
// It returns the appropriate []YTFieldDiff representation.
func parseFieldDiffValue(raw json.RawMessage) ([]YTFieldDiff, error) {
	// Handle nil or JSON null
	if raw == nil || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	switch trimmed[0] {
	case '[':
		// Array: unmarshal as []YTFieldDiff (existing behavior)
		var diffs []YTFieldDiff
		if err := json.Unmarshal(raw, &diffs); err != nil {
			return nil, err
		}
		return diffs, nil

	case '"':
		// String scalar: use the string value directly as Name and ID
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return []YTFieldDiff{{Name: s, ID: s}}, nil

	case 't', 'f':
		// Boolean scalar: convert to "true" or "false"
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		return []YTFieldDiff{{Name: strconv.FormatBool(b), ID: strconv.FormatBool(b)}}, nil

	default:
		// Numeric scalar (digit or '-')
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		// Format without trailing zeros
		name := strconv.FormatFloat(f, 'f', -1, 64)
		return []YTFieldDiff{{Name: name, ID: name}}, nil
	}
}
