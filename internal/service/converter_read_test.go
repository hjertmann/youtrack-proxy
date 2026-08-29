package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// --- TestUnixMillisToISO8601 ---

func TestUnixMillisToISO8601(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "zero value (epoch)",
			input:    0,
			expected: "1970-01-01T00:00:00.000+0000",
		},
		{
			name:     "known timestamp 2024-01-15T09:30:00Z",
			input:    1705311000000,
			expected: "2024-01-15T09:30:00.000+0000",
		},
		{
			name:     "timestamp with milliseconds",
			input:    1705311000123,
			expected: "2024-01-15T09:30:00.123+0000",
		},
		{
			name:     "negative timestamp (before epoch)",
			input:    -86400000,
			expected: "1969-12-31T00:00:00.000+0000",
		},
		{
			name:     "large timestamp (year 2100)",
			input:    4102444800000,
			expected: "2100-01-01T00:00:00.000+0000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unixMillisToISO8601(tt.input)
			if result != tt.expected {
				t.Errorf("unixMillisToISO8601(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- TestConvertYTProjectToJira ---

func TestConvertYTProjectToJira(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("project with all fields", func(t *testing.T) {
		desc := "A test project"
		yt := model.YTProject{
			ID:          "0-0",
			Name:        "Test Project",
			ShortName:   "TP",
			Description: &desc,
			Leader: &model.YTUser{
				Login:  "admin",
				Name:   "Admin User",
				Email:  "admin@example.com",
				Banned: false,
			},
			Type: "Project",
		}

		result := ConvertYTProjectToJira(yt, baseURL)

		if result.Id != "0" {
			t.Errorf("Id = %q, want %q", result.Id, "0")
		}
		if result.Key != "TP" {
			t.Errorf("Key = %q, want %q", result.Key, "TP")
		}
		if result.Name != "Test Project" {
			t.Errorf("Name = %q, want %q", result.Name, "Test Project")
		}
		if result.Description != "A test project" {
			t.Errorf("Description = %q, want %q", result.Description, "A test project")
		}
		if result.Self != "http://localhost:8080/rest/api/2/project/TP" {
			t.Errorf("Self = %q, want %q", result.Self, "http://localhost:8080/rest/api/2/project/TP")
		}
		if result.Lead == nil {
			t.Fatal("Lead is nil, expected non-nil")
		}
		if result.Lead.Key != "admin" {
			t.Errorf("Lead.Key = %q, want %q", result.Lead.Key, "admin")
		}
		if result.Lead.Name != "admin" {
			t.Errorf("Lead.Name = %q, want %q", result.Lead.Name, "admin")
		}
		if result.Lead.DisplayName != "Admin User" {
			t.Errorf("Lead.DisplayName = %q, want %q", result.Lead.DisplayName, "Admin User")
		}
		if result.Lead.EmailAddress != "admin@example.com" {
			t.Errorf("Lead.EmailAddress = %q, want %q", result.Lead.EmailAddress, "admin@example.com")
		}
		if result.Lead.Active != true {
			t.Errorf("Lead.Active = %v, want true", result.Lead.Active)
		}
	})

	t.Run("project with nil description", func(t *testing.T) {
		yt := model.YTProject{
			ID:          "1-1",
			Name:        "No Desc Project",
			ShortName:   "NDP",
			Description: nil,
			Leader:      nil,
		}

		result := ConvertYTProjectToJira(yt, baseURL)

		if result.Description != "" {
			t.Errorf("Description = %q, want empty string", result.Description)
		}
	})

	t.Run("project with nil leader", func(t *testing.T) {
		yt := model.YTProject{
			ID:          "2-2",
			Name:        "No Leader Project",
			ShortName:   "NLP",
			Description: nil,
			Leader:      nil,
		}

		result := ConvertYTProjectToJira(yt, baseURL)

		if result.Lead != nil {
			t.Errorf("Lead = %v, want nil", result.Lead)
		}
	})

	t.Run("project with banned leader", func(t *testing.T) {
		yt := model.YTProject{
			ID:        "3-3",
			Name:      "Banned Leader",
			ShortName: "BL",
			Leader: &model.YTUser{
				Login:  "banned_user",
				Name:   "Banned User",
				Banned: true,
			},
		}

		result := ConvertYTProjectToJira(yt, baseURL)

		if result.Lead == nil {
			t.Fatal("Lead is nil, expected non-nil")
		}
		if result.Lead.Active != false {
			t.Errorf("Lead.Active = %v, want false (user is banned)", result.Lead.Active)
		}
	})
}

// --- TestConvertYTProjectsToJira ---

func TestConvertYTProjectsToJira(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("empty slice", func(t *testing.T) {
		result := ConvertYTProjectsToJira([]model.YTProject{}, baseURL)
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})

	t.Run("multiple projects", func(t *testing.T) {
		yts := []model.YTProject{
			{ID: "0-0", Name: "First", ShortName: "F"},
			{ID: "1-1", Name: "Second", ShortName: "S"},
		}

		result := ConvertYTProjectsToJira(yts, baseURL)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Key != "F" {
			t.Errorf("result[0].Key = %q, want %q", result[0].Key, "F")
		}
		if result[1].Key != "S" {
			t.Errorf("result[1].Key = %q, want %q", result[1].Key, "S")
		}
	})
}

// --- TestMapYTCustomFieldToJira ---

func TestMapYTCustomFieldToJira(t *testing.T) {
	t.Run("Type field with value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Type",
			Value: map[string]interface{}{"name": "Bug"},
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "issuetype" {
			t.Errorf("key = %q, want %q", key, "issuetype")
		}
		nf, ok := val.(*model.JiraNamedField)
		if !ok {
			t.Fatalf("value type = %T, want *model.JiraNamedField", val)
		}
		if nf.Name != "Bug" {
			t.Errorf("Name = %q, want %q", nf.Name, "Bug")
		}
	})

	t.Run("Priority field with value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Priority",
			Value: map[string]interface{}{"name": "Critical"},
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "priority" {
			t.Errorf("key = %q, want %q", key, "priority")
		}
		nf, ok := val.(*model.JiraNamedField)
		if !ok {
			t.Fatalf("value type = %T, want *model.JiraNamedField", val)
		}
		if nf.Name != "Critical" {
			t.Errorf("Name = %q, want %q", nf.Name, "Critical")
		}
	})

	t.Run("State field with value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "State",
			Value: map[string]interface{}{"name": "In Progress"},
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "status" {
			t.Errorf("key = %q, want %q", key, "status")
		}
		nf, ok := val.(*model.JiraNamedField)
		if !ok {
			t.Fatalf("value type = %T, want *model.JiraNamedField", val)
		}
		if nf.Name != "In Progress" {
			t.Errorf("Name = %q, want %q", nf.Name, "In Progress")
		}
	})

	t.Run("Assignee field with value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name: "Assignee",
			Value: map[string]interface{}{
				"login":    "jdoe",
				"fullName": "John Doe",
			},
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "assignee" {
			t.Errorf("key = %q, want %q", key, "assignee")
		}
		ur, ok := val.(*model.JiraUserResponse)
		if !ok {
			t.Fatalf("value type = %T, want *model.JiraUserResponse", val)
		}
		if ur.Key != "jdoe" {
			t.Errorf("Key = %q, want %q", ur.Key, "jdoe")
		}
		if ur.Name != "jdoe" {
			t.Errorf("Name = %q, want %q", ur.Name, "jdoe")
		}
		if ur.DisplayName != "John Doe" {
			t.Errorf("DisplayName = %q, want %q", ur.DisplayName, "John Doe")
		}
		if ur.Active != true {
			t.Errorf("Active = %v, want true", ur.Active)
		}
	})

	t.Run("Type field with null value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Type",
			Value: nil,
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "issuetype" {
			t.Errorf("key = %q, want %q", key, "issuetype")
		}
		if val != nil {
			t.Errorf("value = %v, want nil", val)
		}
	})

	t.Run("Priority field with null value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Priority",
			Value: nil,
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "priority" {
			t.Errorf("key = %q, want %q", key, "priority")
		}
		if val != nil {
			t.Errorf("value = %v, want nil", val)
		}
	})

	t.Run("Assignee field with null value", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Assignee",
			Value: nil,
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "assignee" {
			t.Errorf("key = %q, want %q", key, "assignee")
		}
		if val != nil {
			t.Errorf("value = %v, want nil", val)
		}
	})

	t.Run("unknown field name formatting", func(t *testing.T) {
		tests := []struct {
			fieldName   string
			expectedKey string
		}{
			{"Sprint", "customfield_sprint"},
			{"Story Points", "customfield_story-points"},
			{"Due Date", "customfield_due-date"},
			{"Fix Versions", "customfield_fix-versions"},
		}

		for _, tt := range tests {
			t.Run(tt.fieldName, func(t *testing.T) {
				cf := model.YTCustomField{
					Name:  tt.fieldName,
					Value: "some-value",
				}

				key, _ := MapYTCustomFieldToJira(cf)
				if key != tt.expectedKey {
					t.Errorf("key = %q, want %q", key, tt.expectedKey)
				}
			})
		}
	})

	t.Run("unknown field with map value returns raw", func(t *testing.T) {
		cf := model.YTCustomField{
			Name:  "Custom Thing",
			Value: map[string]interface{}{"foo": "bar"},
		}

		key, val := MapYTCustomFieldToJira(cf)
		if key != "customfield_custom-thing" {
			t.Errorf("key = %q, want %q", key, "customfield_custom-thing")
		}
		valMap, ok := val.(map[string]interface{})
		if !ok {
			t.Fatalf("value type = %T, want map[string]interface{}", val)
		}
		if valMap["foo"] != "bar" {
			t.Errorf("value[foo] = %v, want %q", valMap["foo"], "bar")
		}
	})
}

// --- TestConvertYTIssueToJira ---

func TestConvertYTIssueToJira(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("issue with all custom fields", func(t *testing.T) {
		desc := "Issue description"
		yt := model.YTIssue{
			ID:          "2-123",
			IDReadable:  "TP-1",
			Summary:     "Test Issue",
			Description: &desc,
			Created:     1705311000000,
			Updated:     1705397400000,
			Reporter: &model.YTUser{
				Login: "reporter",
				Name:  "Reporter Name",
				Email: "reporter@example.com",
			},
			Project: &model.YTProject{
				ID:        "0-0",
				Name:      "Test Project",
				ShortName: "TP",
			},
			CustomFields: []model.YTCustomField{
				{Name: "Type", Value: map[string]interface{}{"name": "Bug"}},
				{Name: "Priority", Value: map[string]interface{}{"name": "High"}},
				{Name: "State", Value: map[string]interface{}{"name": "Open"}},
				{Name: "Assignee", Value: map[string]interface{}{"login": "dev", "fullName": "Developer"}},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.ID != "36028797018964091" {
			t.Errorf("ID = %q, want %q", result.ID, "36028797018964091")
		}
		if result.Key != "TP-1" {
			t.Errorf("Key = %q, want %q", result.Key, "TP-1")
		}
		if result.Self != "http://localhost:8080/rest/api/2/issue/TP-1" {
			t.Errorf("Self = %q, want %q", result.Self, "http://localhost:8080/rest/api/2/issue/TP-1")
		}
		if result.Fields.Summary != "Test Issue" {
			t.Errorf("Summary = %q, want %q", result.Fields.Summary, "Test Issue")
		}
		if result.Fields.Description == nil || *result.Fields.Description != "Issue description" {
			t.Errorf("Description = %v, want %q", result.Fields.Description, "Issue description")
		}
		if result.Fields.Created != "2024-01-15T09:30:00.000+0000" {
			t.Errorf("Created = %q, want %q", result.Fields.Created, "2024-01-15T09:30:00.000+0000")
		}
		if result.Fields.Updated != "2024-01-16T09:30:00.000+0000" {
			t.Errorf("Updated = %q, want %q", result.Fields.Updated, "2024-01-16T09:30:00.000+0000")
		}

		// Reporter
		if result.Fields.Reporter == nil {
			t.Fatal("Reporter is nil")
		}
		if result.Fields.Reporter.Key != "reporter" {
			t.Errorf("Reporter.Key = %q, want %q", result.Fields.Reporter.Key, "reporter")
		}

		// Project
		if result.Fields.Project == nil {
			t.Fatal("Project is nil")
		}
		if result.Fields.Project.Key != "TP" {
			t.Errorf("Project.Key = %q, want %q", result.Fields.Project.Key, "TP")
		}

		// Custom fields
		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil")
		}
		if result.Fields.IssueType.Name != "Bug" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "Bug")
		}

		if result.Fields.Priority == nil {
			t.Fatal("Priority is nil")
		}
		if result.Fields.Priority.Name != "High" {
			t.Errorf("Priority.Name = %q, want %q", result.Fields.Priority.Name, "High")
		}

		if result.Fields.Status == nil {
			t.Fatal("Status is nil")
		}
		if result.Fields.Status.Name != "Open" {
			t.Errorf("Status.Name = %q, want %q", result.Fields.Status.Name, "Open")
		}

		if result.Fields.Assignee == nil {
			t.Fatal("Assignee is nil")
		}
		if result.Fields.Assignee.Key != "dev" {
			t.Errorf("Assignee.Key = %q, want %q", result.Fields.Assignee.Key, "dev")
		}
		if result.Fields.Assignee.DisplayName != "Developer" {
			t.Errorf("Assignee.DisplayName = %q, want %q", result.Fields.Assignee.DisplayName, "Developer")
		}
	})

	t.Run("issue with null custom field values", func(t *testing.T) {
		yt := model.YTIssue{
			ID:         "2-456",
			IDReadable: "TP-2",
			Summary:    "Minimal Issue",
			Created:    1705311000000,
			Updated:    1705311000000,
			CustomFields: []model.YTCustomField{
				{Name: "Type", Value: nil},
				{Name: "Priority", Value: nil},
				{Name: "State", Value: nil},
				{Name: "Assignee", Value: nil},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil || result.Fields.IssueType.ID != "unknown" || result.Fields.IssueType.Name != "Unknown" {
			t.Errorf("IssueType = %v, want &{unknown Unknown}", result.Fields.IssueType)
		}
		if result.Fields.Priority == nil || result.Fields.Priority.ID != "unknown" || result.Fields.Priority.Name != "Unknown" {
			t.Errorf("Priority = %v, want &{unknown Unknown}", result.Fields.Priority)
		}
		if result.Fields.Status == nil || result.Fields.Status.ID != "unknown" || result.Fields.Status.Name != "Unknown" {
			t.Errorf("Status = %v, want &{unknown Unknown}", result.Fields.Status)
		}
		if result.Fields.Assignee != nil {
			t.Errorf("Assignee = %v, want nil", result.Fields.Assignee)
		}
	})

	t.Run("issue with nil description and no reporter", func(t *testing.T) {
		yt := model.YTIssue{
			ID:          "2-789",
			IDReadable:  "TP-3",
			Summary:     "No Desc",
			Description: nil,
			Created:     0,
			Updated:     0,
			Reporter:    nil,
			Project:     nil,
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.Description != nil {
			t.Errorf("Description = %v, want nil", result.Fields.Description)
		}
		if result.Fields.Reporter != nil {
			t.Errorf("Reporter = %v, want nil", result.Fields.Reporter)
		}
		if result.Fields.Project != nil {
			t.Errorf("Project = %v, want nil", result.Fields.Project)
		}
	})
}

// --- TestConvertYTIssuesToJira ---

func TestConvertYTIssuesToJira(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("empty slice", func(t *testing.T) {
		result := ConvertYTIssuesToJira([]model.YTIssue{}, baseURL, nil)
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})

	t.Run("multiple issues", func(t *testing.T) {
		yts := []model.YTIssue{
			{IDReadable: "TP-1", Summary: "First"},
			{IDReadable: "TP-2", Summary: "Second"},
		}

		result := ConvertYTIssuesToJira(yts, baseURL, nil)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Key != "TP-1" {
			t.Errorf("result[0].Key = %q, want %q", result[0].Key, "TP-1")
		}
		if result[1].Key != "TP-2" {
			t.Errorf("result[1].Key = %q, want %q", result[1].Key, "TP-2")
		}
	})
}

// --- TestExtractProjectFromJQL ---

func TestExtractProjectFromJQL(t *testing.T) {
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
			name:     "project clause with other clauses",
			jql:      "project = TP AND status = Open",
			expected: "TP",
		},
		{
			name:     "case insensitive PROJECT",
			jql:      "PROJECT = ABC",
			expected: "ABC",
		},
		{
			name:     "case insensitive mixed case",
			jql:      "Project = XYZ",
			expected: "XYZ",
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

// --- TestConvertYTCommentToJira ---

func TestConvertYTCommentToJira(t *testing.T) {
	baseURL := "http://localhost:8080"
	issueID := "TP-1"

	t.Run("comment with all fields", func(t *testing.T) {
		text := "This is a comment"
		updated := int64(1705397400000)
		yt := model.YTComment{
			ID: "4-1",
			Author: &model.YTUser{
				Login: "commenter",
				Name:  "Comment Author",
				Email: "comment@example.com",
			},
			Text:    &text,
			Created: 1705311000000,
			Updated: &updated,
		}

		result := ConvertYTCommentToJira(yt, issueID, baseURL)

		if result.ID != "72057594037927937" {
			t.Errorf("ID = %q, want %q", result.ID, "72057594037927937")
		}
		if result.Self != "http://localhost:8080/rest/api/2/issue/TP-1/comment/4-1" {
			t.Errorf("Self = %q, want %q", result.Self, "http://localhost:8080/rest/api/2/issue/TP-1/comment/4-1")
		}
		if result.Body != "This is a comment" {
			t.Errorf("Body = %q, want %q", result.Body, "This is a comment")
		}
		if result.Author == nil {
			t.Fatal("Author is nil")
		}
		if result.Author.Key != "commenter" {
			t.Errorf("Author.Key = %q, want %q", result.Author.Key, "commenter")
		}
		if result.Author.DisplayName != "Comment Author" {
			t.Errorf("Author.DisplayName = %q, want %q", result.Author.DisplayName, "Comment Author")
		}
		if result.Created != "2024-01-15T09:30:00.000+0000" {
			t.Errorf("Created = %q, want %q", result.Created, "2024-01-15T09:30:00.000+0000")
		}
		if result.Updated != "2024-01-16T09:30:00.000+0000" {
			t.Errorf("Updated = %q, want %q", result.Updated, "2024-01-16T09:30:00.000+0000")
		}
	})

	t.Run("comment with nil text", func(t *testing.T) {
		yt := model.YTComment{
			ID:      "4-2",
			Author:  &model.YTUser{Login: "user"},
			Text:    nil,
			Created: 1705311000000,
		}

		result := ConvertYTCommentToJira(yt, issueID, baseURL)

		if result.Body != "" {
			t.Errorf("Body = %q, want empty string", result.Body)
		}
	})

	t.Run("comment with nil author", func(t *testing.T) {
		text := "Anonymous comment"
		yt := model.YTComment{
			ID:      "4-3",
			Author:  nil,
			Text:    &text,
			Created: 1705311000000,
		}

		result := ConvertYTCommentToJira(yt, issueID, baseURL)

		if result.Author != nil {
			t.Errorf("Author = %v, want nil", result.Author)
		}
		if result.Body != "Anonymous comment" {
			t.Errorf("Body = %q, want %q", result.Body, "Anonymous comment")
		}
	})

	t.Run("comment with nil updated uses created", func(t *testing.T) {
		text := "Not updated"
		yt := model.YTComment{
			ID:      "4-4",
			Author:  &model.YTUser{Login: "user"},
			Text:    &text,
			Created: 1705311000000,
			Updated: nil,
		}

		result := ConvertYTCommentToJira(yt, issueID, baseURL)

		if result.Created != result.Updated {
			t.Errorf("Created (%q) != Updated (%q), expected same when Updated is nil", result.Created, result.Updated)
		}
	})
}

// --- TestConvertYTCommentsToJira ---

func TestConvertYTCommentsToJira(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("empty slice", func(t *testing.T) {
		result := ConvertYTCommentsToJira([]model.YTComment{}, "TP-1", baseURL)
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})

	t.Run("multiple comments", func(t *testing.T) {
		text1 := "First"
		text2 := "Second"
		yts := []model.YTComment{
			{ID: "4-1", Text: &text1, Created: 1705311000000},
			{ID: "4-2", Text: &text2, Created: 1705311000000},
		}

		result := ConvertYTCommentsToJira(yts, "TP-1", baseURL)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].ID != "72057594037927937" {
			t.Errorf("result[0].ID = %q, want %q", result[0].ID, "72057594037927937")
		}
		if result[1].ID != "72057594037927938" {
			t.Errorf("result[1].ID = %q, want %q", result[1].ID, "72057594037927938")
		}
	})
}

// --- TestConvertYTUserToJira ---

func TestConvertYTUserToJira(t *testing.T) {
	t.Run("normal active user", func(t *testing.T) {
		yt := model.YTUser{
			Login:  "john",
			Name:   "John Doe",
			Email:  "john@example.com",
			Banned: false,
		}

		result := ConvertYTUserToJira(yt)

		if result.Key != "john" {
			t.Errorf("Key = %q, want %q", result.Key, "john")
		}
		if result.Name != "john" {
			t.Errorf("Name = %q, want %q", result.Name, "john")
		}
		if result.DisplayName != "John Doe" {
			t.Errorf("DisplayName = %q, want %q", result.DisplayName, "John Doe")
		}
		if result.EmailAddress != "john@example.com" {
			t.Errorf("EmailAddress = %q, want %q", result.EmailAddress, "john@example.com")
		}
		if result.Active != true {
			t.Errorf("Active = %v, want true", result.Active)
		}
	})

	t.Run("banned user", func(t *testing.T) {
		yt := model.YTUser{
			Login:  "banned",
			Name:   "Banned User",
			Email:  "banned@example.com",
			Banned: true,
		}

		result := ConvertYTUserToJira(yt)

		if result.Active != false {
			t.Errorf("Active = %v, want false (user is banned)", result.Active)
		}
	})

	t.Run("user with empty fields", func(t *testing.T) {
		yt := model.YTUser{
			Login:  "",
			Name:   "",
			Email:  "",
			Banned: false,
		}

		result := ConvertYTUserToJira(yt)

		if result.Key != "" {
			t.Errorf("Key = %q, want empty", result.Key)
		}
		if result.Name != "" {
			t.Errorf("Name = %q, want empty", result.Name)
		}
		if result.DisplayName != "" {
			t.Errorf("DisplayName = %q, want empty", result.DisplayName)
		}
		if result.EmailAddress != "" {
			t.Errorf("EmailAddress = %q, want empty", result.EmailAddress)
		}
		if result.Active != true {
			t.Errorf("Active = %v, want true (banned defaults to false)", result.Active)
		}
	})
}

// --- TestConvertYTUsersToJira ---

func TestConvertYTUsersToJira(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := ConvertYTUsersToJira([]model.YTUser{})
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})

	t.Run("multiple users", func(t *testing.T) {
		yts := []model.YTUser{
			{Login: "alice", Name: "Alice"},
			{Login: "bob", Name: "Bob"},
		}

		result := ConvertYTUsersToJira(yts)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Key != "alice" {
			t.Errorf("result[0].Key = %q, want %q", result[0].Key, "alice")
		}
		if result[1].Key != "bob" {
			t.Errorf("result[1].Key = %q, want %q", result[1].Key, "bob")
		}
	})
}

// --- TestBugCondition_ChangelogFieldIdFix ---
// Bug Condition Exploration Test for the changelog-fieldid-fix spec.
//
// This test encodes the EXPECTED (correct) behavior for changelog history items.
// It is expected to FAIL on unfixed code, confirming the bug exists.
//
// Bug conditions tested:
// 1. `fieldId` JSON property is absent from serialized output (JiraHistoryItem has no FieldID field)
// 2. `from` and `to` are always empty strings even when fromString/toString are populated
// 3. YouTrack field names (State, Assignee, Priority) are not mapped to Jira-standard names (status, assignee, priority)
//
// **Validates: Requirements 1.1, 1.2, 1.3, 2.1, 2.2, 2.3**

func TestBugCondition_ChangelogFieldIdFix(t *testing.T) {
	// Build YouTrack activity items representing State, Assignee, and Priority changes
	activities := []model.YTActivityItem{
		{
			ID:        "activity-1",
			Timestamp: 1705311000000,
			Author:    &model.YTUser{Login: "admin", Name: "Admin User"},
			Field:     &model.YTFieldRef{ID: "field-state", Name: "State", Presentation: "State"},
			Added:     []model.YTFieldDiff{{Name: "In Progress"}},
			Removed:   []model.YTFieldDiff{{Name: "Open"}},
		},
		{
			ID:        "activity-2",
			Timestamp: 1705311000000,
			Author:    &model.YTUser{Login: "admin", Name: "Admin User"},
			Field:     &model.YTFieldRef{ID: "field-assignee", Name: "Assignee", Presentation: "Assignee"},
			Added:     []model.YTFieldDiff{{Name: "bob"}},
			Removed:   []model.YTFieldDiff{{Name: "alice"}},
		},
		{
			ID:        "activity-3",
			Timestamp: 1705311000000,
			Author:    &model.YTUser{Login: "admin", Name: "Admin User"},
			Field:     &model.YTFieldRef{ID: "field-priority", Name: "Priority", Presentation: "Priority"},
			Added:     []model.YTFieldDiff{{Name: "Critical"}},
			Removed:   []model.YTFieldDiff{{Name: "Normal"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	// All activities share the same timestamp+author, so they should be grouped into one history entry
	if len(result.Histories) != 1 {
		t.Fatalf("expected 1 history entry (same timestamp+author), got %d", len(result.Histories))
	}

	history := result.Histories[0]
	if len(history.Items) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(history.Items))
	}

	// Define expected mappings: YouTrack field name -> Jira-standard field name
	expectedMappings := []struct {
		ytFieldName     string
		jiraFieldName   string
		expectedFrom    string
		expectedTo      string
		fromStringValue string
		toStringValue   string
	}{
		{ytFieldName: "State", jiraFieldName: "status", expectedFrom: "Open", expectedTo: "In Progress", fromStringValue: "Open", toStringValue: "In Progress"},
		{ytFieldName: "Assignee", jiraFieldName: "assignee", expectedFrom: "alice", expectedTo: "bob", fromStringValue: "alice", toStringValue: "bob"},
		{ytFieldName: "Priority", jiraFieldName: "priority", expectedFrom: "Normal", expectedTo: "Critical", fromStringValue: "Normal", toStringValue: "Critical"},
	}

	for i, expected := range expectedMappings {
		item := history.Items[i]

		// Bug condition 3: Field name should be mapped to Jira-standard name
		// Currently the code passes YouTrack field name (e.g., "State") unmapped
		if item.Field != expected.jiraFieldName {
			t.Errorf("item[%d] Field = %q, want Jira-standard name %q (YouTrack name %q was not mapped)",
				i, item.Field, expected.jiraFieldName, expected.ytFieldName)
		}

		// Bug condition 2: From should be populated when FromString is non-empty
		if item.FromString != "" && item.From == "" {
			t.Errorf("item[%d] From is empty but FromString=%q — value ID should be populated",
				i, item.FromString)
		}

		// Bug condition 2: To should be populated when ToString is non-empty
		if item.ToString != "" && item.To == "" {
			t.Errorf("item[%d] To is empty but ToString=%q — value ID should be populated",
				i, item.ToString)
		}

		// Verify FromString and ToString are correctly populated (existing behavior)
		if item.FromString != expected.fromStringValue {
			t.Errorf("item[%d] FromString = %q, want %q", i, item.FromString, expected.fromStringValue)
		}
		if item.ToString != expected.toStringValue {
			t.Errorf("item[%d] ToString = %q, want %q", i, item.ToString, expected.toStringValue)
		}
	}
}

// TestBugCondition_ChangelogFieldIdMissingFromJSON demonstrates that the `fieldId` JSON property
// is entirely absent from the serialized changelog response because JiraHistoryItem has no FieldID field.
//
// **Validates: Requirements 1.1, 2.1**
func TestBugCondition_ChangelogFieldIdMissingFromJSON(t *testing.T) {
	activities := []model.YTActivityItem{
		{
			ID:        "activity-1",
			Timestamp: 1705311000000,
			Author:    &model.YTUser{Login: "admin", Name: "Admin User"},
			Field:     &model.YTFieldRef{ID: "field-state", Name: "State", Presentation: "State"},
			Added:     []model.YTFieldDiff{{Name: "In Progress"}},
			Removed:   []model.YTFieldDiff{{Name: "Open"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	if len(result.Histories) == 0 || len(result.Histories[0].Items) == 0 {
		t.Fatal("expected at least one history item")
	}

	item := result.Histories[0].Items[0]

	// Marshal the item to JSON and check for "fieldId" key
	jsonBytes, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal JiraHistoryItem: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Bug condition 1: fieldId should be present in the JSON output
	if !strings.Contains(jsonStr, `"fieldId"`) {
		t.Errorf("JSON output missing 'fieldId' property — JiraHistoryItem struct lacks FieldID field.\nGot: %s", jsonStr)
	}

	// If fieldId IS present, verify it has the correct mapped value
	if strings.Contains(jsonStr, `"fieldId"`) {
		// Parse the JSON to check the fieldId value
		var parsed map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}
		fieldId, ok := parsed["fieldId"]
		if !ok || fieldId == "" {
			t.Errorf("fieldId is present but empty or missing value")
		} else if fieldId != "status" {
			t.Errorf("fieldId = %q, want %q (State should map to status)", fieldId, "status")
		}
	}
}

// TestBugCondition_ChangelogUnmappedFieldFallback tests that fields without a known
// Jira mapping use the YouTrack field name as a fallback for both field and fieldId.
//
// **Validates: Requirements 2.3, 3.5**
func TestBugCondition_ChangelogUnmappedFieldFallback(t *testing.T) {
	activities := []model.YTActivityItem{
		{
			ID:        "activity-sprint",
			Timestamp: 1705311000000,
			Author:    &model.YTUser{Login: "admin", Name: "Admin User"},
			Field:     &model.YTFieldRef{ID: "field-sprint", Name: "Sprint", Presentation: "Sprint"},
			Added:     []model.YTFieldDiff{{Name: "Sprint 2"}},
			Removed:   []model.YTFieldDiff{{Name: "Sprint 1"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	if len(result.Histories) == 0 || len(result.Histories[0].Items) == 0 {
		t.Fatal("expected at least one history item")
	}

	item := result.Histories[0].Items[0]

	// For unmapped fields, the Field should remain as the YouTrack name (fallback)
	// This is the expected behavior for unknown fields — no mapping needed
	if item.Field != "Sprint" {
		t.Errorf("Field = %q, want %q (unmapped fields should use YouTrack name as fallback)", item.Field, "Sprint")
	}

	// Bug condition 2: From/To should still be populated for unmapped fields
	if item.FromString != "" && item.From == "" {
		t.Errorf("From is empty but FromString=%q — value ID should be populated even for unmapped fields", item.FromString)
	}
	if item.ToString != "" && item.To == "" {
		t.Errorf("To is empty but ToString=%q — value ID should be populated even for unmapped fields", item.ToString)
	}

	// Bug condition 1: fieldId should be present in JSON with the fallback value "Sprint"
	jsonBytes, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal JiraHistoryItem: %v", err)
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"fieldId"`) {
		t.Errorf("JSON output missing 'fieldId' property for unmapped field.\nGot: %s", jsonStr)
	}
}

// --- Preservation Property Tests for changelog-fieldid-fix spec ---
// These tests verify existing behavior of ConvertYTActivitiesToJiraChangelog on UNFIXED code.
// They establish a safety net that MUST CONTINUE TO PASS after the fix is applied.
//
// **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

// TestPreservation_FromStringToStringPopulation verifies that fromString equals the comma-joined
// names of Removed values, and toString equals the comma-joined names of Added values.
// This tests the existing behavior of joinFieldDiffNames on various inputs.
//
// **Validates: Requirements 3.1**
func TestPreservation_FromStringToStringPopulation(t *testing.T) {
	tests := []struct {
		name               string
		added              []model.YTFieldDiff
		removed            []model.YTFieldDiff
		expectedToString   string
		expectedFromString string
	}{
		{
			name:               "single added and removed",
			added:              []model.YTFieldDiff{{Name: "In Progress"}},
			removed:            []model.YTFieldDiff{{Name: "Open"}},
			expectedToString:   "In Progress",
			expectedFromString: "Open",
		},
		{
			name:               "multiple added values",
			added:              []model.YTFieldDiff{{Name: "Alpha"}, {Name: "Beta"}, {Name: "Gamma"}},
			removed:            []model.YTFieldDiff{},
			expectedToString:   "Alpha, Beta, Gamma",
			expectedFromString: "",
		},
		{
			name:               "multiple removed values",
			added:              []model.YTFieldDiff{},
			removed:            []model.YTFieldDiff{{Name: "X"}, {Name: "Y"}},
			expectedToString:   "",
			expectedFromString: "X, Y",
		},
		{
			name:               "empty added and removed",
			added:              []model.YTFieldDiff{},
			removed:            []model.YTFieldDiff{},
			expectedToString:   "",
			expectedFromString: "",
		},
		{
			name:               "nil added and removed",
			added:              nil,
			removed:            nil,
			expectedToString:   "",
			expectedFromString: "",
		},
		{
			name:               "single value with comma in name",
			added:              []model.YTFieldDiff{{Name: "Fix v1, v2"}},
			removed:            []model.YTFieldDiff{{Name: "v0"}},
			expectedToString:   "Fix v1, v2",
			expectedFromString: "v0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activities := []model.YTActivityItem{
				{
					ID:        "act-1",
					Timestamp: 1705311000000,
					Author:    &model.YTUser{Login: "user", Name: "User"},
					Field:     &model.YTFieldRef{ID: "f1", Name: "SomeField"},
					Added:     tt.added,
					Removed:   tt.removed,
				},
			}

			result := ConvertYTActivitiesToJiraChangelog(activities, 0)

			if len(result.Histories) != 1 {
				t.Fatalf("expected 1 history, got %d", len(result.Histories))
			}
			if len(result.Histories[0].Items) != 1 {
				t.Fatalf("expected 1 item, got %d", len(result.Histories[0].Items))
			}

			item := result.Histories[0].Items[0]
			if item.FromString != tt.expectedFromString {
				t.Errorf("FromString = %q, want %q", item.FromString, tt.expectedFromString)
			}
			if item.ToString != tt.expectedToString {
				t.Errorf("ToString = %q, want %q", item.ToString, tt.expectedToString)
			}
		})
	}
}

// TestPreservation_GroupingBySameTimestampAndAuthor verifies that activities sharing the
// same timestamp and author login are grouped into a single JiraHistory entry.
//
// **Validates: Requirements 3.2**
func TestPreservation_GroupingBySameTimestampAndAuthor(t *testing.T) {
	t.Run("same timestamp and author produces one history entry", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Open"}},
				Removed:   []model.YTFieldDiff{},
			},
			{
				ID:        "a2",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "Priority"},
				Added:     []model.YTFieldDiff{{Name: "High"}},
				Removed:   []model.YTFieldDiff{{Name: "Low"}},
			},
			{
				ID:        "a3",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "Assignee"},
				Added:     []model.YTFieldDiff{{Name: "bob"}},
				Removed:   []model.YTFieldDiff{},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history entry for same timestamp+author, got %d", len(result.Histories))
		}
		if len(result.Histories[0].Items) != 3 {
			t.Fatalf("expected 3 items in the grouped history, got %d", len(result.Histories[0].Items))
		}
	})

	t.Run("different timestamps produce separate history entries", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Open"}},
			},
			{
				ID:        "a2",
				Timestamp: 2000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "Priority"},
				Added:     []model.YTFieldDiff{{Name: "High"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 2 {
			t.Fatalf("expected 2 history entries for different timestamps, got %d", len(result.Histories))
		}
	})

	t.Run("different authors produce separate history entries", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "alice", Name: "Alice"},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Open"}},
			},
			{
				ID:        "a2",
				Timestamp: 1000000,
				Author:    &model.YTUser{Login: "bob", Name: "Bob"},
				Field:     &model.YTFieldRef{Name: "Priority"},
				Added:     []model.YTFieldDiff{{Name: "High"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 2 {
			t.Fatalf("expected 2 history entries for different authors, got %d", len(result.Histories))
		}
	})

	t.Run("nil author uses empty login for grouping", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1000000,
				Author:    nil,
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Open"}},
			},
			{
				ID:        "a2",
				Timestamp: 1000000,
				Author:    nil,
				Field:     &model.YTFieldRef{Name: "Priority"},
				Added:     []model.YTFieldDiff{{Name: "High"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		// Both have nil author (empty login) and same timestamp -> grouped
		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history entry for nil authors with same timestamp, got %d", len(result.Histories))
		}
		if len(result.Histories[0].Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result.Histories[0].Items))
		}
	})
}

// TestPreservation_PaginationMetadata verifies that pagination fields are calculated correctly:
// StartAt=startAt param, MaxResults=number of grouped histories, Total=startAt+grouped histories, IsLast=true.
//
// **Validates: Requirements 3.3**
func TestPreservation_PaginationMetadata(t *testing.T) {
	tests := []struct {
		name               string
		startAt            int
		activities         []model.YTActivityItem
		expectedHistories  int
		expectedStartAt    int
		expectedMaxResults int
		expectedTotal      int
		expectedIsLast     bool
	}{
		{
			name:               "empty activities with startAt=0",
			startAt:            0,
			activities:         []model.YTActivityItem{},
			expectedHistories:  0,
			expectedStartAt:    0,
			expectedMaxResults: 0,
			expectedTotal:      0,
			expectedIsLast:     true,
		},
		{
			name:    "single activity with startAt=0",
			startAt: 0,
			activities: []model.YTActivityItem{
				{ID: "a1", Timestamp: 1000, Author: &model.YTUser{Login: "u"}, Field: &model.YTFieldRef{Name: "F"}, Added: []model.YTFieldDiff{{Name: "v"}}},
			},
			expectedHistories:  1,
			expectedStartAt:    0,
			expectedMaxResults: 1,
			expectedTotal:      1,
			expectedIsLast:     true,
		},
		{
			name:    "single activity with startAt=5",
			startAt: 5,
			activities: []model.YTActivityItem{
				{ID: "a1", Timestamp: 1000, Author: &model.YTUser{Login: "u"}, Field: &model.YTFieldRef{Name: "F"}, Added: []model.YTFieldDiff{{Name: "v"}}},
			},
			expectedHistories:  1,
			expectedStartAt:    5,
			expectedMaxResults: 1,
			expectedTotal:      6,
			expectedIsLast:     true,
		},
		{
			name:    "three activities two groups with startAt=10",
			startAt: 10,
			activities: []model.YTActivityItem{
				{ID: "a1", Timestamp: 1000, Author: &model.YTUser{Login: "alice"}, Field: &model.YTFieldRef{Name: "State"}, Added: []model.YTFieldDiff{{Name: "Open"}}},
				{ID: "a2", Timestamp: 1000, Author: &model.YTUser{Login: "alice"}, Field: &model.YTFieldRef{Name: "Priority"}, Added: []model.YTFieldDiff{{Name: "High"}}},
				{ID: "a3", Timestamp: 2000, Author: &model.YTUser{Login: "alice"}, Field: &model.YTFieldRef{Name: "State"}, Added: []model.YTFieldDiff{{Name: "Closed"}}},
			},
			expectedHistories:  2,
			expectedStartAt:    10,
			expectedMaxResults: 2,
			expectedTotal:      12,
			expectedIsLast:     true,
		},
		{
			name:    "large startAt with startAt=100",
			startAt: 100,
			activities: []model.YTActivityItem{
				{ID: "a1", Timestamp: 5000, Author: &model.YTUser{Login: "x"}, Field: &model.YTFieldRef{Name: "F"}, Added: []model.YTFieldDiff{{Name: "v"}}},
				{ID: "a2", Timestamp: 6000, Author: &model.YTUser{Login: "y"}, Field: &model.YTFieldRef{Name: "G"}, Added: []model.YTFieldDiff{{Name: "w"}}},
				{ID: "a3", Timestamp: 7000, Author: &model.YTUser{Login: "z"}, Field: &model.YTFieldRef{Name: "H"}, Added: []model.YTFieldDiff{{Name: "q"}}},
			},
			expectedHistories:  3,
			expectedStartAt:    100,
			expectedMaxResults: 3,
			expectedTotal:      103,
			expectedIsLast:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertYTActivitiesToJiraChangelog(tt.activities, tt.startAt)

			if len(result.Histories) != tt.expectedHistories {
				t.Fatalf("Histories count = %d, want %d", len(result.Histories), tt.expectedHistories)
			}
			if result.StartAt != tt.expectedStartAt {
				t.Errorf("StartAt = %d, want %d", result.StartAt, tt.expectedStartAt)
			}
			if result.MaxResults != tt.expectedMaxResults {
				t.Errorf("MaxResults = %d, want %d", result.MaxResults, tt.expectedMaxResults)
			}
			if result.Total != tt.expectedTotal {
				t.Errorf("Total = %d, want %d", result.Total, tt.expectedTotal)
			}
			if result.IsLast != tt.expectedIsLast {
				t.Errorf("IsLast = %v, want %v", result.IsLast, tt.expectedIsLast)
			}
		})
	}
}

// TestPreservation_AuthorAndCreatedFields verifies that the author is correctly converted
// from YTUser to JiraUserResponse and that created is the ISO 8601 representation of the
// activity timestamp.
//
// **Validates: Requirements 3.4**
func TestPreservation_AuthorAndCreatedFields(t *testing.T) {
	t.Run("author converted from YTUser to JiraUserResponse", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1705311000000,
				Author:    &model.YTUser{Login: "dev", Name: "Developer", Email: "dev@example.com", Banned: false},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Done"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history, got %d", len(result.Histories))
		}
		history := result.Histories[0]

		if history.Author == nil {
			t.Fatal("Author is nil")
		}
		if history.Author.Key != "dev" {
			t.Errorf("Author.Key = %q, want %q", history.Author.Key, "dev")
		}
		if history.Author.Name != "dev" {
			t.Errorf("Author.Name = %q, want %q", history.Author.Name, "dev")
		}
		if history.Author.DisplayName != "Developer" {
			t.Errorf("Author.DisplayName = %q, want %q", history.Author.DisplayName, "Developer")
		}
		if history.Author.EmailAddress != "dev@example.com" {
			t.Errorf("Author.EmailAddress = %q, want %q", history.Author.EmailAddress, "dev@example.com")
		}
		if history.Author.Active != true {
			t.Errorf("Author.Active = %v, want true", history.Author.Active)
		}
	})

	t.Run("nil author produces nil JiraUserResponse", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1705311000000,
				Author:    nil,
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Open"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history, got %d", len(result.Histories))
		}
		if result.Histories[0].Author != nil {
			t.Errorf("Author = %v, want nil", result.Histories[0].Author)
		}
	})

	t.Run("created is ISO 8601 of activity timestamp", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1705311000000,
				Author:    &model.YTUser{Login: "u"},
				Field:     &model.YTFieldRef{Name: "F"},
				Added:     []model.YTFieldDiff{{Name: "v"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		expected := "2024-01-15T09:30:00.000+0000"
		if result.Histories[0].Created != expected {
			t.Errorf("Created = %q, want %q", result.Histories[0].Created, expected)
		}
	})

	t.Run("banned author has Active=false", func(t *testing.T) {
		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: 1705311000000,
				Author:    &model.YTUser{Login: "banned", Name: "Banned User", Banned: true},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: "Done"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if result.Histories[0].Author == nil {
			t.Fatal("Author is nil")
		}
		if result.Histories[0].Author.Active != false {
			t.Errorf("Author.Active = %v, want false (banned user)", result.Histories[0].Author.Active)
		}
	})
}

// TestPreservation_FieldTypeAlwaysJira verifies that the FieldType property is always "jira"
// for all history items regardless of field name.
//
// **Validates: Requirements 3.5**
func TestPreservation_FieldTypeAlwaysJira(t *testing.T) {
	activities := []model.YTActivityItem{
		{
			ID:        "a1",
			Timestamp: 1000,
			Author:    &model.YTUser{Login: "u"},
			Field:     &model.YTFieldRef{Name: "State"},
			Added:     []model.YTFieldDiff{{Name: "Open"}},
		},
		{
			ID:        "a2",
			Timestamp: 2000,
			Author:    &model.YTUser{Login: "u"},
			Field:     &model.YTFieldRef{Name: "CustomField"},
			Added:     []model.YTFieldDiff{{Name: "Value"}},
		},
		{
			ID:        "a3",
			Timestamp: 3000,
			Author:    &model.YTUser{Login: "u"},
			Field:     nil,
			Added:     []model.YTFieldDiff{{Name: "Something"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	for i, history := range result.Histories {
		for j, item := range history.Items {
			if item.FieldType != "jira" {
				t.Errorf("Histories[%d].Items[%d].FieldType = %q, want %q", i, j, item.FieldType, "jira")
			}
		}
	}
}

// TestPreservation_HistoryIDFromFirstActivityInGroup verifies that the history entry ID
// comes from the first activity item in the group.
//
// **Validates: Requirements 3.4**
func TestPreservation_HistoryIDFromFirstActivityInGroup(t *testing.T) {
	activities := []model.YTActivityItem{
		{
			ID:        "first-activity",
			Timestamp: 5000,
			Author:    &model.YTUser{Login: "user1"},
			Field:     &model.YTFieldRef{Name: "State"},
			Added:     []model.YTFieldDiff{{Name: "Open"}},
		},
		{
			ID:        "second-activity",
			Timestamp: 5000,
			Author:    &model.YTUser{Login: "user1"},
			Field:     &model.YTFieldRef{Name: "Priority"},
			Added:     []model.YTFieldDiff{{Name: "High"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	if len(result.Histories) != 1 {
		t.Fatalf("expected 1 history, got %d", len(result.Histories))
	}
	if result.Histories[0].ID != "first-activity" {
		t.Errorf("History.ID = %q, want %q (should be first activity's ID)", result.Histories[0].ID, "first-activity")
	}
}

// TestPreservation_NilFieldProducesEmptyFieldName verifies that when an activity has a nil
// Field reference, the history item's Field property is an empty string.
//
// **Validates: Requirements 3.5**
func TestPreservation_NilFieldProducesEmptyFieldName(t *testing.T) {
	activities := []model.YTActivityItem{
		{
			ID:        "a1",
			Timestamp: 1000,
			Author:    &model.YTUser{Login: "u"},
			Field:     nil,
			Added:     []model.YTFieldDiff{{Name: "value"}},
		},
	}

	result := ConvertYTActivitiesToJiraChangelog(activities, 0)

	if len(result.Histories) != 1 || len(result.Histories[0].Items) != 1 {
		t.Fatal("expected 1 history with 1 item")
	}
	if result.Histories[0].Items[0].Field != "" {
		t.Errorf("Field = %q, want empty string for nil Field ref", result.Histories[0].Items[0].Field)
	}
}

// --- TestConvertYTIssueToJira_IssuetypeIDMapping ---
// Tests for Requirement 4: Issue-Type ID Alignment.
// Validates that issuetype.id in converter output matches what HandleListIssueTypes would
// produce when both use the same IDMap instance.

func TestConvertYTIssueToJira_IssuetypeIDMapping(t *testing.T) {
	baseURL := "http://localhost:8080"

	t.Run("issuetype ID deterministically encoded", func(t *testing.T) {
		// Requirement 4.1, 4.2, 4.3: The converter encodes the bundle value ID
		// via idmap.Encode, producing a deterministic numeric ID.
		bundleValueID := "69-42"

		yt := model.YTIssue{
			ID:         "2-100",
			IDReadable: "TP-10",
			Summary:    "Test issue",
			Created:    1705311000000,
			Updated:    1705311000000,
			CustomFields: []model.YTCustomField{
				{
					Name:  "Type",
					Value: map[string]interface{}{"name": "Bug", "id": bundleValueID},
				},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil")
		}

		converterID := result.Fields.IssueType.ID

		// Verify against direct Encode call (what HandleListIssueTypes would do)
		numID, err := idmap.Encode(bundleValueID)
		if err != nil {
			t.Fatalf("idmap.Encode failed: %v", err)
		}
		pickerID := idmap.FormatID(numID)

		if converterID != pickerID {
			t.Errorf("converter issuetype.id = %q, picker issuetype.id = %q — they must match", converterID, pickerID)
		}

		// Verify the ID is not the raw bundle value ID
		if converterID == bundleValueID {
			t.Errorf("issuetype.id = %q, should have been encoded to a numeric ID", converterID)
		}

		// Verify the name is preserved
		if result.Fields.IssueType.Name != "Bug" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "Bug")
		}
	})

	t.Run("issuetype ID with name-based fallback uses raw fallback", func(t *testing.T) {
		// When the bundle value has no "id" field, MapYTCustomFieldToJira generates
		// a lowercased-hyphenated fallback from the name (e.g. "user-story").
		// Since "user-story" has no recognized prefix, Encode fails gracefully
		// and the fallback ID is used as-is.
		yt := model.YTIssue{
			ID:         "2-101",
			IDReadable: "TP-11",
			Summary:    "Test issue with name fallback",
			Created:    1705311000000,
			Updated:    1705311000000,
			CustomFields: []model.YTCustomField{
				{
					Name:  "Type",
					Value: map[string]interface{}{"name": "User Story"},
				},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil")
		}
		// "user-story" is not a valid YouTrack ID format, so Encode falls back
		// to using the raw ID string.
		if result.Fields.IssueType.ID != "user-story" {
			t.Errorf("IssueType.ID = %q, want %q", result.Fields.IssueType.ID, "user-story")
		}
		if result.Fields.IssueType.Name != "User Story" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "User Story")
		}
	})

	t.Run("nil Type custom field value produces unknown issuetype", func(t *testing.T) {
		yt := model.YTIssue{
			ID:         "2-200",
			IDReadable: "TP-20",
			Summary:    "Issue with nil Type",
			Created:    1705311000000,
			Updated:    1705311000000,
			CustomFields: []model.YTCustomField{
				{Name: "Type", Value: nil},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil, expected unknown fallback")
		}
		if result.Fields.IssueType.ID != "unknown" {
			t.Errorf("IssueType.ID = %q, want %q", result.Fields.IssueType.ID, "unknown")
		}
		if result.Fields.IssueType.Name != "Unknown" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "Unknown")
		}
	})

	t.Run("missing Type custom field produces unknown issuetype", func(t *testing.T) {
		yt := model.YTIssue{
			ID:           "2-201",
			IDReadable:   "TP-21",
			Summary:      "Issue with no custom fields",
			Created:      1705311000000,
			Updated:      1705311000000,
			CustomFields: []model.YTCustomField{},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil, expected unknown fallback")
		}
		if result.Fields.IssueType.ID != "unknown" {
			t.Errorf("IssueType.ID = %q, want %q", result.Fields.IssueType.ID, "unknown")
		}
		if result.Fields.IssueType.Name != "Unknown" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "Unknown")
		}
	})

	t.Run("all numeric YouTrack IDs encode successfully", func(t *testing.T) {
		// Any valid typeId-seqId format ID now encodes with the new bitfield
		// scheme — even those with previously "unrecognized" prefixes like 83.
		bundleValueID := "83-42"
		yt := model.YTIssue{
			ID:         "2-300",
			IDReadable: "TP-30",
			Summary:    "Issue with high type prefix",
			Created:    1705311000000,
			Updated:    1705311000000,
			CustomFields: []model.YTCustomField{
				{
					Name:  "Type",
					Value: map[string]interface{}{"name": "Task", "id": bundleValueID},
				},
			},
		}

		result := ConvertYTIssueToJira(yt, baseURL, nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil")
		}
		// "83-42" now encodes to 1495195076287004714 via bitfield packing (Mode A: typeId<<54 | seqId)
		if result.Fields.IssueType.ID != "1495195076287004714" {
			t.Errorf("IssueType.ID = %q, want %q (encoded via bitfield)", result.Fields.IssueType.ID, "1495195076287004714")
		}
		if result.Fields.IssueType.Name != "Task" {
			t.Errorf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, "Task")
		}
	})

	t.Run("multiple issues produce consistent deterministic IDs", func(t *testing.T) {
		// Requirement 4.2, 4.3: The same bundle value ID always produces the
		// same numeric ID (deterministic encoding, no state needed).
		makeIssue := func(readable, bundleID, typeName string) model.YTIssue {
			return model.YTIssue{
				ID:         "2-0",
				IDReadable: readable,
				Summary:    "Test",
				Created:    1705311000000,
				Updated:    1705311000000,
				CustomFields: []model.YTCustomField{
					{Name: "Type", Value: map[string]interface{}{"name": typeName, "id": bundleID}},
				},
			}
		}

		result1 := ConvertYTIssueToJira(makeIssue("TP-1", "69-1", "Bug"), baseURL, nil)
		result2 := ConvertYTIssueToJira(makeIssue("TP-2", "69-2", "Task"), baseURL, nil)
		result3 := ConvertYTIssueToJira(makeIssue("TP-3", "69-1", "Bug"), baseURL, nil)

		if result1.Fields.IssueType.ID == result2.Fields.IssueType.ID {
			t.Errorf("different bundle IDs produced same numeric ID: %q", result1.Fields.IssueType.ID)
		}
		if result1.Fields.IssueType.ID != result3.Fields.IssueType.ID {
			t.Errorf("same bundle ID produced different numeric IDs: %q vs %q", result1.Fields.IssueType.ID, result3.Fields.IssueType.ID)
		}
	})
}
