package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"github.com/rs/zerolog/log"
)

// unixMillisToISO8601 converts a Unix timestamp in milliseconds to an ISO 8601
// formatted string compatible with Jira's date format.
func unixMillisToISO8601(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000+0000")
}

// ConvertYTProjectToJira converts a YouTrack project to a Jira-compatible project response.
func ConvertYTProjectToJira(yt model.YTProject, baseURL string) model.JiraProject {
	description := ""
	if yt.Description != nil {
		description = *yt.Description
	}

	var lead *model.JiraUserResponse
	if yt.Leader != nil {
		l := ConvertYTUserToJira(*yt.Leader)
		lead = &l
	}

	selfURL := fmt.Sprintf("%s/rest/api/2/project/%s", baseURL, yt.ShortName)

	projectID := yt.ID
	if numID, err := idmap.Encode(yt.ID); err != nil {
		log.Warn().Err(err).Str("youtrackID", yt.ID).Msg("Failed to encode project ID, using YouTrack ID as fallback")
	} else {
		projectID = idmap.FormatID(numID)
	}

	// Provide default issue types so Jira clients (e.g. IntelliJ plugin) don't crash
	// when deserializing the project response expecting non-null IssueType.id values.
	issueTypes := []model.JiraIssueType{
		{Self: selfURL + "/issuetype/task", ID: "task", Name: "Task", Description: "A task", Subtask: false},
		{Self: selfURL + "/issuetype/bug", ID: "bug", Name: "Bug", Description: "A bug", Subtask: false},
		{Self: selfURL + "/issuetype/story", ID: "story", Name: "Story", Description: "A user story", Subtask: false},
		{Self: selfURL + "/issuetype/epic", ID: "epic", Name: "Epic", Description: "An epic", Subtask: false},
	}

	return model.JiraProject{
		Self:        selfURL,
		Id:          projectID,
		Key:         yt.ShortName,
		Name:        yt.Name,
		Description: description,
		Lead:        lead,
		IssueTypes:  issueTypes,
	}
}

// ConvertYTProjectsToJira converts a slice of YouTrack projects to Jira-compatible project responses.
func ConvertYTProjectsToJira(yts []model.YTProject, baseURL string) []model.JiraProject {
	projects := make([]model.JiraProject, 0, len(yts))
	for _, yt := range yts {
		projects = append(projects, ConvertYTProjectToJira(yt, baseURL))
	}
	return projects
}

// ConvertYTUserToJira converts a YouTrack user to a Jira user response.
// Maps login→key/name, name→displayName, email→emailAddress, !banned→active.
// Go zero values provide empty strings for missing fields and true for active
// (since banned defaults to false).
func ConvertYTUserToJira(yt model.YTUser) model.JiraUserResponse {
	return model.JiraUserResponse{
		AccountId:    yt.Login,
		Key:          yt.Login,
		Name:         yt.Login,
		DisplayName:  yt.Name,
		EmailAddress: yt.Email,
		Active:       !yt.Banned,
	}
}

// ConvertYTUsersToJira converts a slice of YouTrack users to Jira user responses.
func ConvertYTUsersToJira(yts []model.YTUser) []model.JiraUserResponse {
	results := make([]model.JiraUserResponse, len(yts))
	for i, yt := range yts {
		results[i] = ConvertYTUserToJira(yt)
	}
	return results
}

// ConvertYTCommentToJira converts a YouTrack comment to a Jira-compatible comment response.
// Maps text→body, author via ConvertYTUserToJira, and converts timestamps from unix ms to ISO 8601.
// If Updated is nil, the Created timestamp is used for both Created and Updated fields.
func ConvertYTCommentToJira(yt model.YTComment, issueID, baseURL string) model.JiraComment {
	body := ""
	if yt.Text != nil {
		body = *yt.Text
	}

	var author *model.JiraUserResponse
	if yt.Author != nil {
		a := ConvertYTUserToJira(*yt.Author)
		author = &a
	}

	created := unixMillisToISO8601(yt.Created)

	updated := created
	if yt.Updated != nil {
		updated = unixMillisToISO8601(*yt.Updated)
	}

	commentID := yt.ID
	if numID, err := idmap.Encode(yt.ID); err != nil {
		log.Warn().Err(err).Str("youtrackID", yt.ID).Msg("Failed to encode comment ID, using YouTrack ID as fallback")
	} else {
		commentID = idmap.FormatID(numID)
	}

	return model.JiraComment{
		ID:      commentID,
		Self:    fmt.Sprintf("%s/rest/api/2/issue/%s/comment/%s", baseURL, issueID, yt.ID),
		Body:    body,
		Author:  author,
		Created: created,
		Updated: updated,
	}
}

// ConvertYTCommentsToJira converts a slice of YouTrack comments to Jira-compatible comment responses.
func ConvertYTCommentsToJira(yts []model.YTComment, issueID, baseURL string) []model.JiraComment {
	comments := make([]model.JiraComment, len(yts))
	for i, yt := range yts {
		comments[i] = ConvertYTCommentToJira(yt, issueID, baseURL)
	}
	return comments
}

// ConvertYTIssueToJira converts a YouTrack issue to a Jira-compatible issue response.
// Maps basic fields (id, key, summary, description, timestamps), reporter, project,
// and custom fields (Type, Priority, State, Assignee) to their Jira equivalents.
// Numeric IDs are used for id, project.id, priority.id, and status includes
// statusCategory. Also adds creator, labels, and resolutiondate.
func ConvertYTIssueToJira(yt model.YTIssue, baseURL string, resolvedStates ResolvedStateSet) model.JiraIssue {
	fields := model.JiraIssueFields{
		Summary:     yt.Summary,
		Description: yt.Description,
		Created:     unixMillisToISO8601(yt.Created),
		Updated:     unixMillisToISO8601(yt.Updated),
		Labels:      make([]string, 0),
	}

	// Map reporter and creator (YouTrack doesn't distinguish them)
	if yt.Reporter != nil {
		reporter := ConvertYTUserToJira(*yt.Reporter)
		fields.Reporter = &reporter
		creator := reporter
		fields.Creator = &creator
	}

	// Map project
	if yt.Project != nil {
		project := ConvertYTProjectToJira(*yt.Project, baseURL)
		fields.Project = &project
	}

	// Map labels from tags
	for _, tag := range yt.Tags {
		fields.Labels = append(fields.Labels, tag.Name)
	}

	// Track state name for statusCategory and resolutiondate
	var stateName string

	// Map custom fields
	for _, cf := range yt.CustomFields {
		fieldKey, value := MapYTCustomFieldToJira(cf)
		switch fieldKey {
		case "issuetype":
			if value == nil {
				fields.IssueType = nil
			} else if nf, ok := value.(*model.JiraNamedField); ok {
				if numID, err := idmap.Encode(nf.ID); err != nil {
					log.Warn().Err(err).Str("youtrackID", nf.ID).Msg("Failed to encode issuetype ID, using YouTrack ID as fallback")
				} else {
					nf.ID = idmap.FormatID(numID)
				}
				fields.IssueType = nf
			}
		case "priority":
			if value == nil {
				fields.Priority = nil
			} else if nf, ok := value.(*model.JiraNamedField); ok {
				if numID, err := idmap.Encode(nf.ID); err != nil {
					log.Warn().Err(err).Str("youtrackID", nf.ID).Msg("Failed to encode priority ID, using YouTrack ID as fallback")
				} else {
					nf.ID = idmap.FormatID(numID)
				}
				fields.Priority = nf
			}
		case "status":
			if value == nil {
				fields.Status = nil
			} else if nf, ok := value.(*model.JiraNamedField); ok {
				stateName = nf.Name
				cat := MapStateToCategory(nf.Name, resolvedStates)
				statusID := nf.ID
				if numID, err := idmap.Encode(nf.ID); err != nil {
					log.Warn().Err(err).Str("youtrackID", nf.ID).Msg("Failed to encode status ID, using YouTrack ID as fallback")
				} else {
					statusID = idmap.FormatID(numID)
				}
				fields.Status = &model.JiraStatusField{
					ID:   statusID,
					Name: nf.Name,
					StatusCategory: model.JiraStatusCategory{
						ID:        cat.ID,
						Name:      cat.Name,
						Key:       cat.Key,
						ColorName: cat.ColorName,
					},
				}
			}
		case "assignee":
			if value == nil {
				fields.Assignee = nil
			} else if ur, ok := value.(*model.JiraUserResponse); ok {
				fields.Assignee = ur
			}
		default:
			// Unknown custom fields are skipped.
		}
	}

	// Ensure status, issuetype, and priority are never null in the response.
	if fields.Status == nil {
		cat := MapStateToCategory("Unknown", resolvedStates)
		fields.Status = &model.JiraStatusField{
			ID:   "unknown",
			Name: "Unknown",
			StatusCategory: model.JiraStatusCategory{
				ID:        cat.ID,
				Name:      cat.Name,
				Key:       cat.Key,
				ColorName: cat.ColorName,
			},
		}
	}
	if fields.IssueType == nil {
		fields.IssueType = &model.JiraNamedField{ID: "unknown", Name: "Unknown"}
	}
	if fields.Priority == nil {
		fields.Priority = &model.JiraNamedField{ID: "unknown", Name: "Unknown"}
	}

	// Set resolutiondate: if status category is "done" and issue has a resolved timestamp
	if stateName != "" && MapStateToCategory(stateName, resolvedStates).Key == "done" && yt.Resolved > 0 {
		rd := unixMillisToISO8601(yt.Resolved)
		fields.ResolutionDate = &rd
	}

	// Map issue ID to numeric ID
	issueID := yt.IDReadable
	if numID, err := idmap.Encode(yt.ID); err != nil {
		log.Warn().Err(err).Str("youtrackID", yt.ID).Msg("Failed to encode issue ID, using readable ID as fallback")
	} else {
		issueID = idmap.FormatID(numID)
	}

	return model.JiraIssue{
		ID:     issueID,
		Key:    yt.IDReadable,
		Self:   fmt.Sprintf("%s/rest/api/2/issue/%s", baseURL, yt.IDReadable),
		Fields: fields,
	}
}

// ConvertYTIssuesToJira converts a slice of YouTrack issues to Jira-compatible issue responses.
func ConvertYTIssuesToJira(yts []model.YTIssue, baseURL string, resolvedStates ResolvedStateSet) []model.JiraIssue {
	issues := make([]model.JiraIssue, 0, len(yts))
	for _, yt := range yts {
		issues = append(issues, ConvertYTIssueToJira(yt, baseURL, resolvedStates))
	}
	return issues
}

// MapYTCustomFieldToJira maps a YouTrack custom field to a Jira field key and value.
// Known fields (Type, Priority, State, Assignee) are mapped to their Jira equivalents.
// Unknown fields are mapped to customfield_<lowercase-hyphenated-name>.
// If the custom field value is null, the returned value is nil.
func MapYTCustomFieldToJira(cf model.YTCustomField) (fieldKey string, value interface{}) {
	switch cf.Name {
	case "Type":
		fieldKey = "issuetype"
	case "Priority":
		fieldKey = "priority"
	case "State":
		fieldKey = "status"
	case "Assignee":
		fieldKey = "assignee"
	default:
		fieldKey = "customfield_" + strings.ToLower(strings.ReplaceAll(cf.Name, " ", "-"))
	}

	// Handle null values — return nil for the Jira field.
	if cf.Value == nil {
		return fieldKey, nil
	}

	// Attempt to interpret value as a map (JSON object).
	valueMap, ok := cf.Value.(map[string]interface{})
	if !ok {
		// If not a map, return the raw value for unknown fields.
		return fieldKey, cf.Value
	}

	switch cf.Name {
	case "Type", "Priority", "State":
		name, _ := valueMap["name"].(string)
		id, _ := valueMap["id"].(string)
		if id == "" {
			// Generate a stable fallback ID from the name if YouTrack doesn't provide one.
			id = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		}
		return fieldKey, &model.JiraNamedField{ID: id, Name: name}
	case "Assignee":
		login, _ := valueMap["login"].(string)
		fullName, _ := valueMap["fullName"].(string)
		return fieldKey, &model.JiraUserResponse{
			AccountId:   login,
			Key:         login,
			Name:        login,
			DisplayName: fullName,
			Active:      true,
		}
	default:
		// Unknown fields: return the raw value.
		return fieldKey, cf.Value
	}
}

// BuildEditMetaResponse converts YouTrack project custom fields into a Jira-compatible
// editmeta response. It always includes summary and description as string-type editable
// fields, maps known enum fields (Type, Priority, State) with allowedValues from their
// bundles, and maps Assignee as a user-type field without pre-populated allowedValues.
func BuildEditMetaResponse(ytFields []model.YTProjectCustomField) model.JiraEditMetaResponse {
	fields := make(map[string]model.JiraEditMetaField)

	// Always include summary and description as editable string fields.
	fields["summary"] = model.JiraEditMetaField{
		Name:       "Summary",
		Schema:     model.JiraEditMetaFieldSchema{Type: "string", System: "summary"},
		Operations: []string{"set"},
	}
	fields["description"] = model.JiraEditMetaField{
		Name:       "Description",
		Schema:     model.JiraEditMetaFieldSchema{Type: "string", System: "description"},
		Operations: []string{"set"},
	}

	for _, cf := range ytFields {
		switch cf.Field.Name {
		case "Type":
			fields["issuetype"] = model.JiraEditMetaField{
				Name:          "Issue Type",
				Schema:        model.JiraEditMetaFieldSchema{Type: "issuetype", System: "issuetype"},
				Operations:    []string{"set"},
				AllowedValues: bundleValuesToAllowed(cf.Bundle),
			}
		case "Priority":
			fields["priority"] = model.JiraEditMetaField{
				Name:          "Priority",
				Schema:        model.JiraEditMetaFieldSchema{Type: "priority", System: "priority"},
				Operations:    []string{"set"},
				AllowedValues: bundleValuesToAllowed(cf.Bundle),
			}
		case "State":
			fields["status"] = model.JiraEditMetaField{
				Name:          "Status",
				Schema:        model.JiraEditMetaFieldSchema{Type: "status", System: "status"},
				Operations:    []string{"set"},
				AllowedValues: bundleValuesToAllowed(cf.Bundle),
			}
		case "Assignee":
			fields["assignee"] = model.JiraEditMetaField{
				Name:       "Assignee",
				Schema:     model.JiraEditMetaFieldSchema{Type: "user", System: "assignee"},
				Operations: []string{"set"},
			}
		}
	}

	return model.JiraEditMetaResponse{Fields: fields}
}

// bundleValuesToAllowed converts a YouTrack field bundle's values into the
// allowedValues slice expected by the Jira editmeta response format.
// Returns nil if the bundle is nil or has no values.
func bundleValuesToAllowed(bundle *model.YTFieldBundle) []map[string]interface{} {
	if bundle == nil || len(bundle.Values) == 0 {
		return nil
	}
	allowed := make([]map[string]interface{}, len(bundle.Values))
	for i, v := range bundle.Values {
		id := v.ID
		if id == "" {
			// Generate a stable fallback ID from the name.
			id = strings.ToLower(strings.ReplaceAll(v.Name, " ", "-"))
		}
		allowed[i] = map[string]interface{}{"id": id, "name": v.Name}
	}
	return allowed
}

// ytToJiraFieldNames maps YouTrack field names to their Jira-standard equivalents.
// Fields not in this map fall through using their YouTrack name as-is.
var ytToJiraFieldNames = map[string]string{
	"State":             "status",
	"Assignee":          "assignee",
	"Priority":          "priority",
	"Type":              "issuetype",
	"Fix versions":      "fixVersions",
	"Affected versions": "versions",
	"Estimation":        "timeoriginalestimate",
	"Spent time":        "timespent",
}

// DeriveResolutionDateFromActivities scans YouTrack activity items for the last
// transition into a done-category state and returns that timestamp in unix millis.
// Returns nil if no done-transition is found, allowing the caller to fall back
// to the stored yt.Resolved value.
func DeriveResolutionDateFromActivities(activities []model.YTActivityItem, resolvedStates ResolvedStateSet) *int64 {
	var lastDoneTs int64
	found := false
	for _, a := range activities {
		if a.Field == nil || a.Field.Name != "State" {
			continue
		}
		for _, v := range a.Added {
			if IsDoneState(v.Name, resolvedStates) {
				if !found || a.Timestamp > lastDoneTs {
					lastDoneTs = a.Timestamp
					found = true
				}
				break
			}
		}
	}
	if !found {
		return nil
	}
	return &lastDoneTs
}

// ConvertYTActivitiesToJiraChangelog converts YouTrack activity items into a Jira-compatible
// changelog response. Each activity item becomes a history entry with one item describing
// the field change (from/to values). Activities are grouped by timestamp and author to
// mirror how Jira batches simultaneous changes into a single history entry.
func ConvertYTActivitiesToJiraChangelog(activities []model.YTActivityItem, startAt int) model.JiraChangelogResponse {
	// Group activities by timestamp+author to consolidate simultaneous changes.
	type historyKey struct {
		Timestamp int64
		Login     string
	}

	keyOrder := make([]historyKey, 0)
	grouped := make(map[historyKey][]model.YTActivityItem)

	for _, a := range activities {
		login := ""
		if a.Author != nil {
			login = a.Author.Login
		}
		key := historyKey{Timestamp: a.Timestamp, Login: login}
		if _, exists := grouped[key]; !exists {
			keyOrder = append(keyOrder, key)
		}
		grouped[key] = append(grouped[key], a)
	}

	histories := make([]model.JiraHistory, 0, len(keyOrder))
	for _, key := range keyOrder {
		items := grouped[key]
		first := items[0]

		var author *model.JiraUserResponse
		if first.Author != nil {
			a := ConvertYTUserToJira(*first.Author)
			author = &a
		}

		historyItems := make([]model.JiraHistoryItem, 0, len(items))
		for _, act := range items {
			fieldName := ""
			if act.Field != nil {
				fieldName = act.Field.Name
			}

			// Map YouTrack field name to Jira-standard name; fall back to YouTrack name
			mappedName := fieldName
			if jiraName, ok := ytToJiraFieldNames[fieldName]; ok {
				mappedName = jiraName
			}

			fromString := joinFieldDiffNames(act.Removed)
			toString := joinFieldDiffNames(act.Added)

			historyItems = append(historyItems, model.JiraHistoryItem{
				Field:      mappedName,
				FieldID:    mappedName,
				FieldType:  "jira",
				From:       joinFieldDiffIDs(act.Removed),
				FromString: fromString,
				To:         joinFieldDiffIDs(act.Added),
				ToString:   toString,
			})
		}

		historyID := first.ID
		if numID, err := idmap.Encode(first.ID); err != nil {
			log.Warn().Err(err).Str("youtrackID", first.ID).Msg("Failed to encode activity ID, using YouTrack ID as fallback")
		} else {
			historyID = idmap.FormatID(numID)
		}

		histories = append(histories, model.JiraHistory{
			ID:      historyID,
			Author:  author,
			Created: unixMillisToISO8601(first.Timestamp),
			Items:   historyItems,
		})
	}

	total := startAt + len(histories)

	return model.JiraChangelogResponse{
		StartAt:    startAt,
		MaxResults: len(histories),
		Total:      total,
		IsLast:     true,
		Histories:  histories,
	}
}

// joinFieldDiffNames concatenates the names from a slice of YTFieldDiff values,
// separated by commas. Returns an empty string if the slice is empty.
func joinFieldDiffNames(diffs []model.YTFieldDiff) string {
	if len(diffs) == 0 {
		return ""
	}
	names := make([]string, len(diffs))
	for i, d := range diffs {
		names[i] = d.Name
	}
	return strings.Join(names, ", ")
}

// joinFieldDiffIDs concatenates the IDs from a slice of YTFieldDiff values,
// separated by commas. If a diff has an empty ID, the Name is used as a fallback.
// Returns an empty string if the slice is empty.
func joinFieldDiffIDs(diffs []model.YTFieldDiff) string {
	if len(diffs) == 0 {
		return ""
	}
	ids := make([]string, len(diffs))
	for i, d := range diffs {
		if d.ID != "" {
			ids[i] = d.ID
		} else {
			ids[i] = d.Name
		}
	}
	return strings.Join(ids, ", ")
}
