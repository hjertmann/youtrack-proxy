// Package model defines shared request and response types for the Jira-to-YouTrack
// proxy. Write-direction (creation) structs live here; read-direction structs
// live in jira_read.go and youtrack_read.go.
package model

// ---------------------------------------------------------------------------
// Jira inbound request types (issue creation)
// ---------------------------------------------------------------------------

// JiraCreateIssueRequest is the top-level Jira REST API v2 issue creation body.
type JiraCreateIssueRequest struct {
	Fields JiraFields `json:"fields"`
}

// JiraFields contains the fields of a Jira issue creation request.
type JiraFields struct {
	Project     JiraProject  `json:"project"`
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	IssueType   JiraType     `json:"issuetype"`
	Priority    JiraPriority `json:"priority,omitempty"`
	Assignee    JiraUser     `json:"assignee,omitempty"`
}

// JiraType identifies an issue type by name.
type JiraType struct {
	Name string `json:"name"`
}

// JiraPriority identifies a priority by name.
type JiraPriority struct {
	Name string `json:"name"`
}

// JiraUser identifies a user by name (login).
type JiraUser struct {
	Name string `json:"name"`
}

// JiraResponse is the Jira-shaped response returned after issue creation.
type JiraResponse struct {
	Key  string `json:"key"`
	ID   string `json:"id"`
	Self string `json:"self"`
}

// JiraProject represents a project, used both in creation requests and in
// read responses (the read path adds extra fields via JSON tags).
type JiraProject struct {
	Self        string            `json:"self"`
	Id          string            `json:"id"`
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Lead        *JiraUserResponse `json:"lead,omitempty"`
	IssueTypes  []JiraIssueType   `json:"issueTypes,omitempty"`
}

// JiraIssueType represents an issue type in a Jira project response.
type JiraIssueType struct {
	Self        string `json:"self"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Subtask     bool   `json:"subtask"`
}

// ---------------------------------------------------------------------------
// Authentication context
// ---------------------------------------------------------------------------

// RequestContext carries per-request auth data extracted by the middleware.
type RequestContext struct {
	YouTrackToken string
}

// ---------------------------------------------------------------------------
// YouTrack outbound request types (issue creation)
// ---------------------------------------------------------------------------

// YouTrackCreateIssueRequest is the payload POSTed to YouTrack's /api/issues.
type YouTrackCreateIssueRequest struct {
	Summary      string                `json:"summary"`
	Description  string                `json:"description,omitempty"`
	Project      YouTrackProject       `json:"project"`
	CustomFields []YouTrackCustomField `json:"customFields,omitempty"`
}

// YouTrackProject identifies a project by its short name / ID.
type YouTrackProject struct {
	ID string `json:"id"`
}

// YouTrackCustomField sets a single custom field value on a new issue.
type YouTrackCustomField struct {
	ID    string             `json:"id"`
	Type  string             `json:"$type"`
	Value YouTrackFieldValue `json:"value"`
}

// YouTrackFieldValue is the value payload for an enum or user custom field.
type YouTrackFieldValue struct {
	Name string `json:"name"`
}

// YouTrackResponse is the minimal response from YouTrack after issue creation.
type YouTrackResponse struct {
	ID   string `json:"id"`
	Type string `json:"$type"`
}
