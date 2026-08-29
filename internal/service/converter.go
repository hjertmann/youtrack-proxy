// Package service contains business logic for translating between Jira and
// YouTrack domain concepts.
package service

import (
	"fmt"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// ResolveFieldID looks up the project-level custom field ID for a given field
// name (e.g. "Type", "Priority", "Assignee"). Returns "" when not found.
func ResolveFieldID(fields []model.YTProjectCustomField, name string) string {
	for _, f := range fields {
		if f.Field.Name == name {
			return f.ID
		}
	}
	return ""
}

// ConvertJiraToYouTrack translates a Jira issue creation request into the
// corresponding YouTrack payload. Field IDs are resolved dynamically from
// the project's custom field configuration.
func ConvertJiraToYouTrack(req model.JiraCreateIssueRequest, projectFields []model.YTProjectCustomField) (*model.YouTrackCreateIssueRequest, error) {
	key := req.Fields.Project.Key
	if key == "" {
		return nil, fmt.Errorf("project key is required")
	}

	out := &model.YouTrackCreateIssueRequest{
		Summary:     req.Fields.Summary,
		Description: req.Fields.Description,
		Project:     model.YouTrackProject{ID: key},
	}

	if name := req.Fields.IssueType.Name; name != "" {
		if id := ResolveFieldID(projectFields, "Type"); id != "" {
			out.CustomFields = append(out.CustomFields, model.YouTrackCustomField{
				ID:    id,
				Type:  "SingleEnumIssueCustomField",
				Value: model.YouTrackFieldValue{Name: translateIssueType(name)},
			})
		}
	}

	if name := req.Fields.Priority.Name; name != "" {
		if id := ResolveFieldID(projectFields, "Priority"); id != "" {
			out.CustomFields = append(out.CustomFields, model.YouTrackCustomField{
				ID:    id,
				Type:  "SingleEnumIssueCustomField",
				Value: model.YouTrackFieldValue{Name: translatePriority(name)},
			})
		}
	}

	if name := req.Fields.Assignee.Name; name != "" {
		if id := ResolveFieldID(projectFields, "Assignee"); id != "" {
			out.CustomFields = append(out.CustomFields, model.YouTrackCustomField{
				ID:    id,
				Type:  "SingleUserIssueCustomField",
				Value: model.YouTrackFieldValue{Name: name},
			})
		}
	}

	return out, nil
}

// translateIssueType maps Jira issue type names to YouTrack equivalents.
func translateIssueType(jiraType string) string {
	switch jiraType {
	case "Story", "Feature":
		return "Feature"
	case "Epic":
		return "Epic"
	default:
		return "Task"
	}
}

// translatePriority maps Jira priority names to YouTrack equivalents.
func translatePriority(jiraPriority string) string {
	switch jiraPriority {
	case "Highest":
		return "Critical"
	case "High":
		return "Major"
	case "Medium":
		return "Normal"
	case "Low", "Lowest":
		return "Minor"
	default:
		return "Normal"
	}
}
