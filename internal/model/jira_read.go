package model

// JiraIssue represents a Jira issue in REST API v2 response format.
type JiraIssue struct {
	ID        string               `json:"id"`
	Key       string               `json:"key"`
	Self      string               `json:"self"`
	Fields    JiraIssueFields      `json:"fields"`
	Changelog *JiraInlineChangelog `json:"changelog,omitempty"`
}

// JiraIssueFields contains the fields of a Jira issue response.
type JiraIssueFields struct {
	Summary        string            `json:"summary"`
	Description    *string           `json:"description"`
	IssueType      *JiraNamedField   `json:"issuetype"`
	Priority       *JiraNamedField   `json:"priority"`
	Status         *JiraStatusField  `json:"status"`
	Assignee       *JiraUserResponse `json:"assignee"`
	Reporter       *JiraUserResponse `json:"reporter"`
	Creator        *JiraUserResponse `json:"creator"`
	Project        *JiraProject      `json:"project"`
	Created        string            `json:"created"`
	Updated        string            `json:"updated"`
	Labels         []string          `json:"labels"`
	ResolutionDate *string           `json:"resolutiondate"`
}

// JiraStatusField represents a status with its category, used in issue responses.
type JiraStatusField struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Self           string             `json:"self,omitempty"`
	StatusCategory JiraStatusCategory `json:"statusCategory"`
}

// JiraInlineChangelog holds changelog entries embedded inline on an issue.
type JiraInlineChangelog struct {
	StartAt    int           `json:"startAt"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	Histories  []JiraHistory `json:"histories"`
}

// JiraNamedField represents a Jira field that has a name property (e.g., issuetype, priority, status).
type JiraNamedField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// JiraUserResponse represents a Jira user in REST API v2 response format.
type JiraUserResponse struct {
	Self         string            `json:"self,omitempty"`
	AccountId    string            `json:"accountId"`
	Key          string            `json:"key"`
	Name         string            `json:"name"`
	DisplayName  string            `json:"displayName"`
	EmailAddress string            `json:"emailAddress"`
	Active       bool              `json:"active"`
	AvatarUrls   map[string]string `json:"avatarUrls,omitempty"`
	TimeZone     string            `json:"timeZone,omitempty"`
}

// JiraComment represents a Jira issue comment in REST API v2 response format.
type JiraComment struct {
	ID      string            `json:"id"`
	Self    string            `json:"self"`
	Body    string            `json:"body"`
	Author  *JiraUserResponse `json:"author"`
	Created string            `json:"created"`
	Updated string            `json:"updated"`
}

// JiraSearchResponse represents the paginated response from Jira's issue search endpoint.
type JiraSearchResponse struct {
	StartAt    int         `json:"startAt"`
	MaxResults int         `json:"maxResults"`
	Total      int         `json:"total"`
	Issues     []JiraIssue `json:"issues"`
}

// JiraCommentsResponse represents the paginated response from Jira's issue comments endpoint.
type JiraCommentsResponse struct {
	StartAt    int           `json:"startAt"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	Comments   []JiraComment `json:"comments"`
}

// JiraErrorResponse represents a Jira error response.
type JiraErrorResponse struct {
	ErrorMessages []string `json:"errorMessages"`
}

// ServerInfoResponse represents the Jira REST API v2 serverInfo response.
type ServerInfoResponse struct {
	BaseURL        string `json:"baseUrl"`
	Version        string `json:"version"`
	VersionNumbers []int  `json:"versionNumbers"`
	DeploymentType string `json:"deploymentType"`
	ServerTitle    string `json:"serverTitle"`
	ServerTime     string `json:"serverTime"`
	BuildNumber    int    `json:"buildNumber"`
}

// JiraFieldSchema describes the schema metadata for a Jira field.
type JiraFieldSchema struct {
	Type   string `json:"type"`
	System string `json:"system,omitempty"`
}

// JiraField describes a single field definition in the Jira REST API v2 field list.
type JiraField struct {
	ID          string           `json:"id"`
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	Custom      bool             `json:"custom"`
	Orderable   bool             `json:"orderable"`
	Navigable   bool             `json:"navigable"`
	Searchable  bool             `json:"searchable"`
	ClauseNames []string         `json:"clauseNames"`
	Schema      *JiraFieldSchema `json:"schema,omitempty"`
}

// JiraEditMetaResponse represents the Jira REST API v2 editmeta response for an issue.
type JiraEditMetaResponse struct {
	Fields map[string]JiraEditMetaField `json:"fields"`
}

// JiraEditMetaField describes a single editable field in a Jira editmeta response.
type JiraEditMetaField struct {
	Name          string                   `json:"name"`
	Schema        JiraEditMetaFieldSchema  `json:"schema"`
	Operations    []string                 `json:"operations"`
	AllowedValues []map[string]interface{} `json:"allowedValues,omitempty"`
}

// JiraEditMetaFieldSchema describes the type metadata for an editable field.
type JiraEditMetaFieldSchema struct {
	Type   string `json:"type"`
	System string `json:"system,omitempty"`
	Items  string `json:"items,omitempty"`
}

// JiraChangelogResponse represents the paginated response from Jira's issue changelog endpoint.
type JiraChangelogResponse struct {
	StartAt    int           `json:"startAt"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	IsLast     bool          `json:"isLast"`
	Histories  []JiraHistory `json:"histories"`
}

// JiraHistory represents a single changelog entry (one timestamp, one author, one or more field changes).
type JiraHistory struct {
	ID      string            `json:"id"`
	Author  *JiraUserResponse `json:"author"`
	Created string            `json:"created"`
	Items   []JiraHistoryItem `json:"items"`
}

// JiraHistoryItem represents a single field change within a changelog history entry.
type JiraHistoryItem struct {
	Field      string `json:"field"`
	FieldID    string `json:"fieldId"`
	FieldType  string `json:"fieldtype"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

// JiraStatusCategory represents a Jira status category (To Do, In Progress, Done).
type JiraStatusCategory struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	ColorName string `json:"colorName"`
}

// JiraStatus represents a status in the Jira REST API v2 /status response.
type JiraStatus struct {
	Self           string             `json:"self"`
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	StatusCategory JiraStatusCategory `json:"statusCategory"`
}

// JiraUserPickerUser represents a single user in the Jira user picker response.
type JiraUserPickerUser struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	HTML        string `json:"html"`
	DisplayName string `json:"displayName"`
}

// JiraUserPickerResponse represents the Jira REST API v2 /user/picker response.
type JiraUserPickerResponse struct {
	Users  []JiraUserPickerUser `json:"users"`
	Total  int                  `json:"total"`
	Header string               `json:"header"`
}

// JiraBoardResponse is the paginated response for GET /rest/agile/1.0/board.
type JiraBoardResponse struct {
	MaxResults int         `json:"maxResults"`
	StartAt    int         `json:"startAt"`
	IsLast     bool        `json:"isLast"`
	Values     []JiraBoard `json:"values"`
}

// JiraBoard represents a single board entry.
type JiraBoard struct {
	ID       int64             `json:"id"`
	Self     string            `json:"self"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Location JiraBoardLocation `json:"location"`
}

// JiraBoardLocation identifies the project associated with a board.
type JiraBoardLocation struct {
	ProjectID   int64  `json:"projectId"`
	ProjectName string `json:"projectName"`
	ProjectKey  string `json:"projectKey"`
}

// JiraBoardConfig is the response for GET /rest/agile/1.0/board/{id}/configuration.
type JiraBoardConfig struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Filter   JiraBoardFilter   `json:"filter"`
	SubQuery JiraBoardSubQuery `json:"subQuery"`
}

// JiraBoardFilter holds the filter reference inside a board configuration.
type JiraBoardFilter struct {
	ID string `json:"id"`
}

// JiraBoardSubQuery holds the sub-query inside a board configuration.
type JiraBoardSubQuery struct {
	Query string `json:"query"`
}

// JiraSprintResponse is the paginated response for GET /rest/agile/1.0/board/{id}/sprint.
type JiraSprintResponse struct {
	MaxResults int          `json:"maxResults"`
	StartAt    int          `json:"startAt"`
	IsLast     bool         `json:"isLast"`
	Values     []JiraSprint `json:"values"`
}

// JiraSprint represents a single sprint entry.
type JiraSprint struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// JiraFilterResponse is the response for GET /rest/api/2/filter/{id}.
type JiraFilterResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	JQL  string `json:"jql"`
	Self string `json:"self"`
}

// JiraWorklogResponse is the response for GET /rest/api/2/issue/{key}/worklog.
type JiraWorklogResponse struct {
	StartAt    int   `json:"startAt"`
	MaxResults int   `json:"maxResults"`
	Total      int   `json:"total"`
	Worklogs   []any `json:"worklogs"`
}
