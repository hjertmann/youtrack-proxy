package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestPropertyProjectConversion validates Property 1: Project conversion preserves identity fields.
// For any YouTrack project with non-empty id, shortName, and name fields, converting it to a Jira
// project SHALL produce a result where id equals the YouTrack id, key equals shortName, and name
// equals the YouTrack name.
//
// **Validates: Requirements 1.2, 2.2**
func TestPropertyProjectConversion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate non-empty identity fields
		id := rapid.StringMatching(`[a-z]{2,5}-[a-z]{2,5}`).Draw(t, "id")
		shortName := rapid.StringMatching(`[A-Z]{2,10}`).Draw(t, "shortName")
		name := rapid.StringMatching(`[A-Za-z0-9 ]{1,30}`).Draw(t, "name")

		// Optionally generate description (nil or non-nil)
		hasDescription := rapid.Bool().Draw(t, "hasDescription")
		var description *string
		if hasDescription {
			desc := rapid.String().Draw(t, "description")
			description = &desc
		}

		// Optionally generate leader (nil or non-nil)
		hasLeader := rapid.Bool().Draw(t, "hasLeader")
		var leader *model.YTUser
		if hasLeader {
			leader = &model.YTUser{
				Login:  rapid.StringMatching(`[a-z]{3,12}`).Draw(t, "leaderLogin"),
				Name:   rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "leaderName"),
				Email:  rapid.StringMatching(`[a-z]+@[a-z]+\.[a-z]{2,4}`).Draw(t, "leaderEmail"),
				Banned: rapid.Bool().Draw(t, "leaderBanned"),
			}
		}

		baseURL := rapid.SampledFrom([]string{
			"http://localhost:8080",
			"https://proxy.example.com",
			"http://127.0.0.1:3000",
		}).Draw(t, "baseURL")

		yt := model.YTProject{
			ID:          id,
			ShortName:   shortName,
			Name:        name,
			Description: description,
			Leader:      leader,
			Type:        "Project",
		}

		result := ConvertYTProjectToJira(yt, baseURL)

		// Assert identity fields are preserved
		if result.Id != yt.ID {
			t.Fatalf("expected Id=%q, got %q", yt.ID, result.Id)
		}
		if result.Key != yt.ShortName {
			t.Fatalf("expected Key=%q, got %q", yt.ShortName, result.Key)
		}
		if result.Name != yt.Name {
			t.Fatalf("expected Name=%q, got %q", yt.Name, result.Name)
		}

		// Assert Self URL contains the shortName
		expectedSelf := fmt.Sprintf("%s/rest/api/2/project/%s", baseURL, shortName)
		if result.Self != expectedSelf {
			t.Fatalf("expected Self=%q, got %q", expectedSelf, result.Self)
		}

		// Assert description handling: nil -> empty string
		if description == nil {
			if result.Description != "" {
				t.Fatalf("expected empty Description for nil input, got %q", result.Description)
			}
		} else {
			if result.Description != *description {
				t.Fatalf("expected Description=%q, got %q", *description, result.Description)
			}
		}

		// Assert leader mapping
		if leader == nil {
			if result.Lead != nil {
				t.Fatal("expected nil Lead for nil leader input")
			}
		} else {
			if result.Lead == nil {
				t.Fatal("expected non-nil Lead for non-nil leader input")
			}
			if result.Lead.Key != leader.Login {
				t.Fatalf("expected Lead.Key=%q, got %q", leader.Login, result.Lead.Key)
			}
			if result.Lead.Name != leader.Login {
				t.Fatalf("expected Lead.Name=%q, got %q", leader.Login, result.Lead.Name)
			}
			if result.Lead.DisplayName != leader.Name {
				t.Fatalf("expected Lead.DisplayName=%q, got %q", leader.Name, result.Lead.DisplayName)
			}
			if result.Lead.EmailAddress != leader.Email {
				t.Fatalf("expected Lead.EmailAddress=%q, got %q", leader.Email, result.Lead.EmailAddress)
			}
			if result.Lead.Active != !leader.Banned {
				t.Fatalf("expected Lead.Active=%v, got %v", !leader.Banned, result.Lead.Active)
			}
		}
	})
}

// TestPropertyIssueCustomFieldMapping validates Property 2: Issue field mapping round-trip consistency.
// For any YouTrack issue containing custom fields named "Type", "Priority", and "State" with non-null
// values, the converter SHALL produce a Jira issue where fields.issuetype.name, fields.priority.name,
// and fields.status.name equal the respective YouTrack custom field value names.
//
// **Validates: Requirements 9.1, 9.2, 9.3**
func TestPropertyIssueCustomFieldMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		typeName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(t, "typeName")
		priorityName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(t, "priorityName")
		stateName := rapid.StringMatching(`[A-Za-z]{1,20}`).Draw(t, "stateName")

		yt := model.YTIssue{
			IDReadable: "TEST-1",
			Summary:    "Test",
			CustomFields: []model.YTCustomField{
				{Name: "Type", Value: map[string]interface{}{"name": typeName}},
				{Name: "Priority", Value: map[string]interface{}{"name": priorityName}},
				{Name: "State", Value: map[string]interface{}{"name": stateName}},
			},
		}

		result := ConvertYTIssueToJira(yt, "http://localhost", nil)

		if result.Fields.IssueType == nil {
			t.Fatal("IssueType is nil")
		}
		if result.Fields.IssueType.Name != typeName {
			t.Fatalf("IssueType.Name = %q, want %q", result.Fields.IssueType.Name, typeName)
		}
		if result.Fields.Priority == nil {
			t.Fatal("Priority is nil")
		}
		if result.Fields.Priority.Name != priorityName {
			t.Fatalf("Priority.Name = %q, want %q", result.Fields.Priority.Name, priorityName)
		}
		if result.Fields.Status == nil {
			t.Fatal("Status is nil")
		}
		if result.Fields.Status.Name != stateName {
			t.Fatalf("Status.Name = %q, want %q", result.Fields.Status.Name, stateName)
		}
	})
}

// TestPropertyNullCustomFields validates Property 3: Null custom field values map to null Jira fields.
// For any YouTrack issue where a known custom field (Type, Priority, State, Assignee) has a null
// value, the corresponding Jira field SHALL be null in the converted output.
//
// **Validates: Requirements 9.6**
func TestPropertyNullCustomFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Randomly select which fields are null (at least one must be null)
		typeNull := rapid.Bool().Draw(t, "typeNull")
		priorityNull := rapid.Bool().Draw(t, "priorityNull")
		stateNull := rapid.Bool().Draw(t, "stateNull")
		assigneeNull := rapid.Bool().Draw(t, "assigneeNull")

		// Ensure at least one is null
		if !typeNull && !priorityNull && !stateNull && !assigneeNull {
			typeNull = true
		}

		var customFields []model.YTCustomField
		if typeNull {
			customFields = append(customFields, model.YTCustomField{Name: "Type", Value: nil})
		} else {
			customFields = append(customFields, model.YTCustomField{Name: "Type", Value: map[string]interface{}{"name": "Bug"}})
		}
		if priorityNull {
			customFields = append(customFields, model.YTCustomField{Name: "Priority", Value: nil})
		} else {
			customFields = append(customFields, model.YTCustomField{Name: "Priority", Value: map[string]interface{}{"name": "High"}})
		}
		if stateNull {
			customFields = append(customFields, model.YTCustomField{Name: "State", Value: nil})
		} else {
			customFields = append(customFields, model.YTCustomField{Name: "State", Value: map[string]interface{}{"name": "Open"}})
		}
		if assigneeNull {
			customFields = append(customFields, model.YTCustomField{Name: "Assignee", Value: nil})
		} else {
			customFields = append(customFields, model.YTCustomField{Name: "Assignee", Value: map[string]interface{}{"login": "user", "fullName": "User Name"}})
		}

		yt := model.YTIssue{
			IDReadable:   "TEST-1",
			Summary:      "Test",
			CustomFields: customFields,
		}

		result := ConvertYTIssueToJira(yt, "http://localhost", nil)

		if typeNull {
			if result.Fields.IssueType == nil || result.Fields.IssueType.ID != "unknown" || result.Fields.IssueType.Name != "Unknown" {
				t.Fatalf("IssueType should be default {unknown Unknown} when Type value is null, got %v", result.Fields.IssueType)
			}
		}
		if priorityNull {
			if result.Fields.Priority == nil || result.Fields.Priority.ID != "unknown" || result.Fields.Priority.Name != "Unknown" {
				t.Fatalf("Priority should be default {unknown Unknown} when Priority value is null, got %v", result.Fields.Priority)
			}
		}
		if stateNull {
			if result.Fields.Status == nil || result.Fields.Status.ID != "unknown" || result.Fields.Status.Name != "Unknown" {
				t.Fatalf("Status should be default {unknown Unknown} when State value is null, got %v", result.Fields.Status)
			}
		}
		if assigneeNull && result.Fields.Assignee != nil {
			t.Fatal("Assignee should be nil when Assignee value is null")
		}
	})
}

// TestPropertyCommentConversion verifies Property 6: Comment conversion preserves content.
// For any YouTrack comment with non-null text, author, and created fields, converting it
// to a Jira comment SHALL produce a result where body equals the YouTrack text,
// author.name equals the YouTrack author login, and created is a valid ISO 8601 string
// derived from the YouTrack timestamp.
//
// **Validates: Requirements 5.2**
func TestPropertyCommentConversion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := rapid.String().Draw(t, "text")
		login := rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "login")
		authorName := rapid.String().Draw(t, "authorName")
		created := rapid.Int64Range(0, 4102444800000).Draw(t, "created")
		issueID := rapid.StringMatching(`[A-Z]{2,5}-[0-9]{1,5}`).Draw(t, "issueID")
		baseURL := "http://localhost:8080"

		yt := model.YTComment{
			ID:      "comment-1",
			Author:  &model.YTUser{Login: login, Name: authorName},
			Text:    &text,
			Created: created,
		}

		result := ConvertYTCommentToJira(yt, issueID, baseURL)

		if result.Body != text {
			t.Fatalf("Body = %q, want %q", result.Body, text)
		}
		if result.Author == nil {
			t.Fatal("Author is nil")
		}
		if result.Author.Name != login {
			t.Fatalf("Author.Name = %q, want %q", result.Author.Name, login)
		}
		expectedCreated := unixMillisToISO8601(created)
		if result.Created != expectedCreated {
			t.Fatalf("Created = %q, want %q", result.Created, expectedCreated)
		}
	})
}

// TestPropertyUserConversion validates Property 7: User conversion with defaults.
// For any YouTrack user (including those with empty or missing string fields), converting to a Jira
// user SHALL produce a result where key equals login (or empty string if missing), name equals login
// (or empty string if missing), displayName equals the YouTrack name (or empty string if missing),
// emailAddress equals email (or empty string if missing), and active equals !banned (defaulting to
// true if banned is absent).
//
// **Validates: Requirements 6.2, 6.3, 7.2**
func TestPropertyUserConversion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		login := rapid.String().Draw(t, "login")
		name := rapid.String().Draw(t, "name")
		email := rapid.String().Draw(t, "email")
		banned := rapid.Bool().Draw(t, "banned")

		yt := model.YTUser{
			Login:  login,
			Name:   name,
			Email:  email,
			Banned: banned,
		}

		result := ConvertYTUserToJira(yt)

		if result.Key != login {
			t.Fatalf("Key = %q, want %q", result.Key, login)
		}
		if result.Name != login {
			t.Fatalf("Name = %q, want %q", result.Name, login)
		}
		if result.DisplayName != name {
			t.Fatalf("DisplayName = %q, want %q", result.DisplayName, name)
		}
		if result.EmailAddress != email {
			t.Fatalf("EmailAddress = %q, want %q", result.EmailAddress, email)
		}
		if result.Active != !banned {
			t.Fatalf("Active = %v, want %v", result.Active, !banned)
		}
	})
}

// TestPropertyUnknownCustomFieldKeyFormat validates Property 4: Unknown custom fields use standardized key format.
// For any YouTrack issue custom field whose name is not in the known set {Type, Priority, State, Assignee},
// the converter SHALL include it in the Jira fields object with a key formatted as `customfield_` followed
// by a lowercase, hyphen-separated version of the field name.
//
// **Validates: Requirements 9.5**
func TestPropertyUnknownCustomFieldKeyFormat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a field name that is NOT one of the known fields
		// Use multi-word names with spaces to test hyphenation
		word1 := rapid.StringMatching(`[A-Z][a-z]{2,8}`).Draw(t, "word1")
		word2 := rapid.StringMatching(`[A-Z][a-z]{2,8}`).Draw(t, "word2")
		fieldName := word1 + " " + word2

		// Ensure it's not a known field name
		if fieldName == "Type" || fieldName == "Priority" || fieldName == "State" || fieldName == "Assignee" {
			return // skip this iteration
		}

		cf := model.YTCustomField{
			Name:  fieldName,
			Value: "test-value",
		}

		key, _ := MapYTCustomFieldToJira(cf)

		// Key should start with "customfield_"
		if !strings.HasPrefix(key, "customfield_") {
			t.Fatalf("key %q doesn't start with 'customfield_'", key)
		}

		// Key should be all lowercase
		suffix := strings.TrimPrefix(key, "customfield_")
		if suffix != strings.ToLower(suffix) {
			t.Fatalf("key suffix %q is not lowercase", suffix)
		}

		// Key should not contain spaces (replaced with hyphens)
		if strings.Contains(suffix, " ") {
			t.Fatalf("key suffix %q contains spaces", suffix)
		}
	})
}

// --- Preservation Property-Based Tests for changelog-fieldid-fix spec ---
// These property tests verify preservation behavior using generated inputs.
// They MUST PASS on unfixed code and continue to pass after the fix.
//
// **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**

// TestPropertyPreservation_FromStringEqualsJoinedRemovedNames verifies that for any activity,
// fromString equals the comma-joined Names of Removed diffs.
//
// **Validates: Requirements 3.1**
func TestPropertyPreservation_FromStringEqualsJoinedRemovedNames(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 1–5 removed diff names
		numRemoved := rapid.IntRange(0, 5).Draw(t, "numRemoved")
		removed := make([]model.YTFieldDiff, numRemoved)
		removedNames := make([]string, numRemoved)
		for i := 0; i < numRemoved; i++ {
			name := rapid.StringMatching(`[A-Za-z0-9 ]{1,15}`).Draw(t, fmt.Sprintf("removedName_%d", i))
			removed[i] = model.YTFieldDiff{Name: name}
			removedNames[i] = name
		}

		// Generate 0–3 added diffs (don't care about these for this property)
		numAdded := rapid.IntRange(0, 3).Draw(t, "numAdded")
		added := make([]model.YTFieldDiff, numAdded)
		for i := 0; i < numAdded; i++ {
			added[i] = model.YTFieldDiff{Name: rapid.StringMatching(`[A-Za-z]{1,10}`).Draw(t, fmt.Sprintf("addedName_%d", i))}
		}

		activities := []model.YTActivityItem{
			{
				ID:        "act-1",
				Timestamp: rapid.Int64Range(0, 4102444800000).Draw(t, "timestamp"),
				Author:    &model.YTUser{Login: rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "login")},
				Field:     &model.YTFieldRef{Name: rapid.StringMatching(`[A-Za-z]{1,15}`).Draw(t, "fieldName")},
				Added:     added,
				Removed:   removed,
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 || len(result.Histories[0].Items) != 1 {
			t.Fatalf("expected 1 history with 1 item, got %d histories", len(result.Histories))
		}

		item := result.Histories[0].Items[0]
		expectedFromString := strings.Join(removedNames, ", ")
		if item.FromString != expectedFromString {
			t.Fatalf("FromString = %q, want %q", item.FromString, expectedFromString)
		}
	})
}

// TestPropertyPreservation_ToStringEqualsJoinedAddedNames verifies that for any activity,
// toString equals the comma-joined Names of Added diffs.
//
// **Validates: Requirements 3.1**
func TestPropertyPreservation_ToStringEqualsJoinedAddedNames(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate 0–5 added diff names
		numAdded := rapid.IntRange(0, 5).Draw(t, "numAdded")
		added := make([]model.YTFieldDiff, numAdded)
		addedNames := make([]string, numAdded)
		for i := 0; i < numAdded; i++ {
			name := rapid.StringMatching(`[A-Za-z0-9 ]{1,15}`).Draw(t, fmt.Sprintf("addedName_%d", i))
			added[i] = model.YTFieldDiff{Name: name}
			addedNames[i] = name
		}

		activities := []model.YTActivityItem{
			{
				ID:        "act-1",
				Timestamp: rapid.Int64Range(0, 4102444800000).Draw(t, "timestamp"),
				Author:    &model.YTUser{Login: rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "login")},
				Field:     &model.YTFieldRef{Name: rapid.StringMatching(`[A-Za-z]{1,15}`).Draw(t, "fieldName")},
				Added:     added,
				Removed:   []model.YTFieldDiff{},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 || len(result.Histories[0].Items) != 1 {
			t.Fatalf("expected 1 history with 1 item, got %d histories", len(result.Histories))
		}

		item := result.Histories[0].Items[0]
		expectedToString := strings.Join(addedNames, ", ")
		if item.ToString != expectedToString {
			t.Fatalf("ToString = %q, want %q", item.ToString, expectedToString)
		}
	})
}

// TestPropertyPreservation_SameTimestampAuthorGrouped verifies that activities sharing the same
// timestamp and author login are always grouped into a single JiraHistory entry.
//
// **Validates: Requirements 3.2**
func TestPropertyPreservation_SameTimestampAuthorGrouped(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		timestamp := rapid.Int64Range(0, 4102444800000).Draw(t, "timestamp")
		login := rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "login")
		numActivities := rapid.IntRange(1, 5).Draw(t, "numActivities")

		activities := make([]model.YTActivityItem, numActivities)
		for i := 0; i < numActivities; i++ {
			activities[i] = model.YTActivityItem{
				ID:        fmt.Sprintf("act-%d", i),
				Timestamp: timestamp,
				Author:    &model.YTUser{Login: login, Name: "User"},
				Field:     &model.YTFieldRef{Name: fmt.Sprintf("Field%d", i)},
				Added:     []model.YTFieldDiff{{Name: fmt.Sprintf("val%d", i)}},
			}
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		// All activities have the same timestamp+login, so should produce exactly 1 history entry
		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history entry for same timestamp+login, got %d", len(result.Histories))
		}
		if len(result.Histories[0].Items) != numActivities {
			t.Fatalf("expected %d items in grouped history, got %d", numActivities, len(result.Histories[0].Items))
		}
	})
}

// TestPropertyPreservation_PaginationFormula verifies the pagination formula:
// StartAt=startAt, MaxResults=len(histories), Total=startAt+len(histories), IsLast=true.
//
// **Validates: Requirements 3.3**
func TestPropertyPreservation_PaginationFormula(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		startAt := rapid.IntRange(0, 1000).Draw(t, "startAt")

		// Generate distinct groups by using different timestamps
		numGroups := rapid.IntRange(0, 5).Draw(t, "numGroups")
		activities := make([]model.YTActivityItem, 0)
		for g := 0; g < numGroups; g++ {
			ts := int64((g + 1) * 1000000) // distinct timestamps
			activities = append(activities, model.YTActivityItem{
				ID:        fmt.Sprintf("act-%d", g),
				Timestamp: ts,
				Author:    &model.YTUser{Login: "user"},
				Field:     &model.YTFieldRef{Name: "Field"},
				Added:     []model.YTFieldDiff{{Name: "val"}},
			})
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, startAt)

		if result.StartAt != startAt {
			t.Fatalf("StartAt = %d, want %d", result.StartAt, startAt)
		}
		if result.MaxResults != numGroups {
			t.Fatalf("MaxResults = %d, want %d", result.MaxResults, numGroups)
		}
		expectedTotal := startAt + numGroups
		if result.Total != expectedTotal {
			t.Fatalf("Total = %d, want %d", result.Total, expectedTotal)
		}
		if result.IsLast != true {
			t.Fatalf("IsLast = %v, want true", result.IsLast)
		}
	})
}

// TestPropertyPreservation_AuthorConversion verifies that the author in each history entry
// is correctly converted from YTUser to JiraUserResponse (login→key/name, name→displayName,
// email→emailAddress, !banned→active).
//
// **Validates: Requirements 3.4**
func TestPropertyPreservation_AuthorConversion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		login := rapid.StringMatching(`[a-z]{1,12}`).Draw(t, "login")
		name := rapid.StringMatching(`[A-Za-z ]{1,20}`).Draw(t, "name")
		email := rapid.StringMatching(`[a-z]+@[a-z]+\.[a-z]{2,4}`).Draw(t, "email")
		banned := rapid.Bool().Draw(t, "banned")

		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: rapid.Int64Range(0, 4102444800000).Draw(t, "ts"),
				Author:    &model.YTUser{Login: login, Name: name, Email: email, Banned: banned},
				Field:     &model.YTFieldRef{Name: "F"},
				Added:     []model.YTFieldDiff{{Name: "v"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history, got %d", len(result.Histories))
		}
		author := result.Histories[0].Author
		if author == nil {
			t.Fatal("Author is nil")
		}
		if author.Key != login {
			t.Fatalf("Author.Key = %q, want %q", author.Key, login)
		}
		if author.Name != login {
			t.Fatalf("Author.Name = %q, want %q", author.Name, login)
		}
		if author.DisplayName != name {
			t.Fatalf("Author.DisplayName = %q, want %q", author.DisplayName, name)
		}
		if author.EmailAddress != email {
			t.Fatalf("Author.EmailAddress = %q, want %q", author.EmailAddress, email)
		}
		if author.Active != !banned {
			t.Fatalf("Author.Active = %v, want %v", author.Active, !banned)
		}
	})
}

// TestPropertyPreservation_CreatedIsISO8601 verifies that the Created field in each history entry
// is the ISO 8601 representation of the activity's timestamp.
//
// **Validates: Requirements 3.4**
func TestPropertyPreservation_CreatedIsISO8601(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		timestamp := rapid.Int64Range(0, 4102444800000).Draw(t, "timestamp")

		activities := []model.YTActivityItem{
			{
				ID:        "a1",
				Timestamp: timestamp,
				Author:    &model.YTUser{Login: "user"},
				Field:     &model.YTFieldRef{Name: "F"},
				Added:     []model.YTFieldDiff{{Name: "v"}},
			},
		}

		result := ConvertYTActivitiesToJiraChangelog(activities, 0)

		if len(result.Histories) != 1 {
			t.Fatalf("expected 1 history, got %d", len(result.Histories))
		}

		expectedCreated := unixMillisToISO8601(timestamp)
		if result.Histories[0].Created != expectedCreated {
			t.Fatalf("Created = %q, want %q", result.Histories[0].Created, expectedCreated)
		}
	})
}

// TestPropertyResolutionDateCorrectness validates Property 6: Resolution Date Correctness.
// For any YTIssue with a state name and a Resolved timestamp, if
// MapStateToCategory(stateName).Key == "done" and Resolved > 0, then resolutiondate
// in the converted JiraIssue SHALL be a valid ISO 8601 string. For all other
// combinations (non-done state, or Resolved == 0), resolutiondate SHALL be null.
//
// **Validates: Requirements 5.1, 5.2, 5.3**
func TestPropertyResolutionDateCorrectness(t *testing.T) {
	// Pool of known state names across all categories to get good coverage.
	knownDone := []string{"Fixed", "Verified", "Obsolete", "Done", "Closed", "Resolved", "Complete", "Completed"}
	knownNew := []string{"Open", "Submitted", "Incomplete", "New", "Reopened", "To Do", "Backlog"}
	knownInProgress := []string{"In Progress", "In Review", "Testing", "Waiting"}

	allKnown := make([]string, 0, len(knownDone)+len(knownNew)+len(knownInProgress))
	allKnown = append(allKnown, knownDone...)
	allKnown = append(allKnown, knownNew...)
	allKnown = append(allKnown, knownInProgress...)

	stateGen := rapid.OneOf(
		rapid.SampledFrom(allKnown),
		rapid.StringMatching(`[A-Za-z ]{1,20}`),
	)

	// Resolved can be 0 (unresolved) or a positive unix-millis timestamp.
	resolvedGen := rapid.OneOf(
		rapid.Just(int64(0)),
		rapid.Int64Range(1, 4102444800000),
	)

	rapid.Check(t, func(t *rapid.T) {
		stateName := stateGen.Draw(t, "stateName")
		resolved := resolvedGen.Draw(t, "resolved")

		yt := model.YTIssue{
			ID:         "TEST-1",
			IDReadable: "TEST-1",
			Resolved:   resolved,
			CustomFields: []model.YTCustomField{
				{
					Name:  "State",
					Value: map[string]interface{}{"name": stateName, "id": "state-id"},
				},
			},
		}

		result := ConvertYTIssueToJira(yt, "http://test", nil)

		isDone := MapStateToCategory(stateName, nil).Key == "done"

		if isDone && resolved > 0 {
			// resolutiondate must be a valid ISO 8601 string
			if result.Fields.ResolutionDate == nil {
				t.Fatalf("resolutiondate is nil for done state %q with Resolved=%d; expected ISO 8601 value",
					stateName, resolved)
			}
			_, err := time.Parse("2006-01-02T15:04:05.000+0000", *result.Fields.ResolutionDate)
			if err != nil {
				t.Fatalf("resolutiondate %q is not valid ISO 8601: %v", *result.Fields.ResolutionDate, err)
			}
			// Verify the value matches the expected conversion of the Resolved timestamp
			expected := unixMillisToISO8601(resolved)
			if *result.Fields.ResolutionDate != expected {
				t.Fatalf("resolutiondate = %q, want %q", *result.Fields.ResolutionDate, expected)
			}
		} else {
			// resolutiondate must be nil
			if result.Fields.ResolutionDate != nil {
				t.Fatalf("resolutiondate should be nil for state %q (done=%v) with Resolved=%d, got %q",
					stateName, isDone, resolved, *result.Fields.ResolutionDate)
			}
		}
	})
}

// TestPropertyLabelsMapping validates Property 7: Labels Mapping Preserves Tags.
// For any list of YouTrack tag names on an issue, the fields.labels array in the
// converted Jira issue SHALL contain exactly those tag names in the same order,
// with no additions, removals, or modifications. Nil/empty tags produce an empty
// array (not nil).
//
// **Validates: Requirements 12.1, 12.2, 12.3**
func TestPropertyLabelsMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(0, 20).Draw(t, "tagCount")

		tags := make([]model.YTTag, count)
		for i := range count {
			tags[i] = model.YTTag{Name: rapid.String().Draw(t, fmt.Sprintf("tag%d", i))}
		}

		yt := model.YTIssue{
			ID:         "TEST-1",
			IDReadable: "TEST-1",
			Tags:       tags,
		}

		result := ConvertYTIssueToJira(yt, "http://test", nil)

		// Labels must never be nil
		if result.Fields.Labels == nil {
			t.Fatal("Labels is nil, expected empty slice")
		}

		// Length must match tag count
		if len(result.Fields.Labels) != count {
			t.Fatalf("Labels length = %d, want %d", len(result.Fields.Labels), count)
		}

		// Each label must match the corresponding tag name, in order
		for i, tag := range tags {
			if result.Fields.Labels[i] != tag.Name {
				t.Fatalf("Labels[%d] = %q, want %q", i, result.Fields.Labels[i], tag.Name)
			}
		}
	})
}

// TestPropertyLabelsMappingNilTags verifies that nil tags produce an empty labels array.
//
// **Validates: Requirements 12.3**
func TestPropertyLabelsMappingNilTags(t *testing.T) {
	yt := model.YTIssue{
		ID:         "TEST-1",
		IDReadable: "TEST-1",
		Tags:       nil,
	}

	result := ConvertYTIssueToJira(yt, "http://test", nil)

	if result.Fields.Labels == nil {
		t.Fatal("Labels is nil for nil Tags, expected empty slice")
	}
	if len(result.Fields.Labels) != 0 {
		t.Fatalf("Labels length = %d for nil Tags, want 0", len(result.Fields.Labels))
	}
}

// --- Bug Condition Exploration Test for resolutiondate-changelog-derivation spec ---
//
// This test encodes the EXPECTED behavior: DeriveResolutionDateFromActivities should
// return the timestamp of the last done-transition from activity items.
// On unfixed code, this test MUST FAIL (compile error or assertion failure) — failure
// confirms the bug exists.

// TestPropertyBugCondition_DoneTransitionOverridesResolved validates Property 1: Bug Condition.
// For any activity slice containing at least one State field change with an Added value
// whose name (case-insensitive) is in the doneStates map, DeriveResolutionDateFromActivities
// SHALL return non-nil and equal the timestamp of the last such done-transition.
//
// Test cases covered:
//   - Single done-transition ("Fixed")
//   - Multiple transitions (Open→Fixed→Reopened→Closed — last wins)
//   - Case-insensitive matching ("FIXED", "fixed", "Fixed")
//   - Mixed field types (Priority/Assignee changes interleaved with State changes)
//
// **Validates: Requirements 1.1, 1.2, 1.3, 2.1, 2.2, 2.3**
func TestPropertyBugCondition_DoneTransitionOverridesResolved(t *testing.T) {
	// The old hardcoded doneStates for backward-compat testing.
	legacyDoneStates := map[string]struct{}{
		"fixed": {}, "verified": {}, "obsolete": {}, "done": {},
		"closed": {}, "resolved": {}, "complete": {}, "completed": {},
	}
	// Collect all done-state names from the legacy set for generation.
	doneNames := make([]string, 0, len(legacyDoneStates))
	for name := range legacyDoneStates {
		doneNames = append(doneNames, name)
	}

	// Case variation helper: randomly produce upper, lower, or title-case versions
	// of a done-state name to test case-insensitive matching.
	caseVariant := func(t *rapid.T, base string, label string) string {
		variant := rapid.SampledFrom([]int{0, 1, 2}).Draw(t, label)
		switch variant {
		case 0:
			return strings.ToUpper(base)
		case 1:
			return strings.ToLower(base)
		default:
			return strings.ToUpper(base[:1]) + base[1:]
		}
	}

	// Non-done state names for generating noise activities (reopen, non-State fields).
	nonDoneStates := []string{"Open", "In Progress", "Reopened", "Submitted", "Waiting", "New"}
	nonStateFields := []string{"Priority", "Assignee", "Type", "Subsystem", "Fix versions"}

	rapid.Check(t, func(t *rapid.T) {
		// Decide how many done-transitions to include (1–3)
		numDoneTransitions := rapid.IntRange(1, 3).Draw(t, "numDoneTransitions")
		// Decide how many noise activities (non-State or non-done State changes)
		numNoise := rapid.IntRange(0, 5).Draw(t, "numNoise")

		activities := make([]model.YTActivityItem, 0, numDoneTransitions+numNoise)
		var expectedLastTimestamp int64

		// Generate done-transition activities with increasing timestamps to ensure
		// the last one has the highest timestamp.
		for i := 0; i < numDoneTransitions; i++ {
			// Use a base offset so timestamps are distinct and ordered.
			ts := int64((i+1)*1000000) + rapid.Int64Range(0, 999999).Draw(t, fmt.Sprintf("doneTs_%d", i))
			doneName := rapid.SampledFrom(doneNames).Draw(t, fmt.Sprintf("doneName_%d", i))
			displayName := caseVariant(t, doneName, fmt.Sprintf("doneCase_%d", i))

			activities = append(activities, model.YTActivityItem{
				ID:        fmt.Sprintf("done-%d", i),
				Timestamp: ts,
				Author:    &model.YTUser{Login: "user"},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: displayName, ID: fmt.Sprintf("state-%d", i)}},
				Removed:   []model.YTFieldDiff{{Name: "Previous", ID: "prev-state"}},
			})

			// Track the last (highest) done-transition timestamp
			if ts > expectedLastTimestamp {
				expectedLastTimestamp = ts
			}
		}

		// Generate noise activities: mix of non-State field changes and State→non-done changes.
		for i := 0; i < numNoise; i++ {
			ts := rapid.Int64Range(0, int64((numDoneTransitions+1)*1000000)).Draw(t, fmt.Sprintf("noiseTs_%d", i))
			isStateChange := rapid.Bool().Draw(t, fmt.Sprintf("noiseIsState_%d", i))

			if isStateChange {
				// State change to a non-done state (e.g., "Reopened", "In Progress")
				nonDone := rapid.SampledFrom(nonDoneStates).Draw(t, fmt.Sprintf("nonDone_%d", i))
				activities = append(activities, model.YTActivityItem{
					ID:        fmt.Sprintf("noise-state-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: "State"},
					Added:     []model.YTFieldDiff{{Name: nonDone, ID: fmt.Sprintf("nds-%d", i)}},
					Removed:   []model.YTFieldDiff{{Name: "Previous", ID: "prev"}},
				})
			} else {
				// Non-State field change (Priority, Assignee, etc.)
				fieldName := rapid.SampledFrom(nonStateFields).Draw(t, fmt.Sprintf("noiseField_%d", i))
				activities = append(activities, model.YTActivityItem{
					ID:        fmt.Sprintf("noise-field-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: fieldName},
					Added:     []model.YTFieldDiff{{Name: "SomeValue", ID: "sv"}},
				})
			}
		}

		// Call the function under test.
		// On unfixed code, this will not compile — confirming the function is missing.
		result := DeriveResolutionDateFromActivities(activities, legacyDoneStates)

		// Assert non-nil: a done-transition exists, so the function must find it.
		if result == nil {
			t.Fatal("DeriveResolutionDateFromActivities returned nil; expected non-nil for activities containing a done-transition")
		}

		// Assert the returned timestamp equals the last done-transition timestamp.
		if *result != expectedLastTimestamp {
			t.Fatalf("DeriveResolutionDateFromActivities = %d, want %d (last done-transition timestamp)",
				*result, expectedLastTimestamp)
		}
	})
}

// --- Preservation Property-Based Tests for resolutiondate-changelog-derivation spec ---

// TestPropertyPreservation_NoDoneTransitionReturnsNil validates Property 2: Preservation.
// For any activity slice where NO Added value's name (case-insensitive) is in the doneStates
// map, DeriveResolutionDateFromActivities SHALL return nil. This covers:
//   - Empty activity slices
//   - Activities with only non-State field changes (Priority, Assignee, Type, etc.)
//   - Activities with State changes to non-done states ("Open", "In Progress", "Reopened")
//   - Mixed combinations of the above
//
// **Validates: Requirements 2.4, 3.1, 3.2, 3.3**
func TestPropertyPreservation_NoDoneTransitionReturnsNil(t *testing.T) {
	// Non-done state names — none of these appear in the doneStates map.
	nonDoneStateNames := []string{
		"Open", "In Progress", "Reopened", "Submitted", "Waiting",
		"New", "To Do", "Backlog", "Incomplete", "In Review", "Testing",
	}
	// Non-State field names for generating activities that aren't State changes at all.
	nonStateFields := []string{
		"Priority", "Assignee", "Type", "Subsystem", "Fix versions",
		"Estimation", "Spent time", "Affected versions",
	}

	rapid.Check(t, func(t *rapid.T) {
		// Generate 0–8 activities, each guaranteed NOT to be a done-transition.
		numActivities := rapid.IntRange(0, 8).Draw(t, "numActivities")
		activities := make([]model.YTActivityItem, numActivities)

		for i := 0; i < numActivities; i++ {
			ts := rapid.Int64Range(0, 4102444800000).Draw(t, fmt.Sprintf("ts_%d", i))
			isStateChange := rapid.Bool().Draw(t, fmt.Sprintf("isState_%d", i))

			if isStateChange {
				// State change to a non-done state
				stateName := rapid.SampledFrom(nonDoneStateNames).Draw(t, fmt.Sprintf("nonDoneState_%d", i))
				activities[i] = model.YTActivityItem{
					ID:        fmt.Sprintf("act-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: "State"},
					Added:     []model.YTFieldDiff{{Name: stateName, ID: fmt.Sprintf("s-%d", i)}},
					Removed:   []model.YTFieldDiff{{Name: "Previous", ID: "prev"}},
				}
			} else {
				// Non-State field change (Priority, Assignee, etc.)
				fieldName := rapid.SampledFrom(nonStateFields).Draw(t, fmt.Sprintf("field_%d", i))
				activities[i] = model.YTActivityItem{
					ID:        fmt.Sprintf("act-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: fieldName},
					Added:     []model.YTFieldDiff{{Name: "SomeValue", ID: "val"}},
				}
			}
		}

		// DeriveResolutionDateFromActivities must return nil when no done-transition exists.
		result := DeriveResolutionDateFromActivities(activities, nil)
		if result != nil {
			t.Fatalf("DeriveResolutionDateFromActivities returned %d for activities with no done-transition; expected nil", *result)
		}
	})
}

// TestPropertyDynamic_ConverterStatusAndResolution validates Property 4: Converter Status and Resolution Correctness.
//
// For any YTIssue with a State custom field value named S and any resolved state set,
// ConvertYTIssueToJira(issue, baseURL, resolvedStates) SHALL produce a JiraIssue where:
//   - fields.status.statusCategory matches MapStateToCategory(S, resolvedStates)
//   - fields.resolutiondate is non-nil (valid ISO 8601) if and only if
//     MapStateToCategory(S, resolvedStates).Key == "done" AND issue.Resolved > 0
//   - fields.resolutiondate is nil otherwise
//
// Feature: dynamic-resolved-states, Property 4: Converter Status and Resolution Correctness
//
// **Validates: Requirements 4.1, 4.2, 4.3**
func TestPropertyDynamic_ConverterStatusAndResolution(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random state name
		stateName := rapid.StringMatching(`[A-Za-z ]{1,20}`).Draw(t, "stateName")

		// Generate a random resolved state set
		resolvedStates := genResolvedStateSet().Draw(t, "resolvedStates")

		// Generate Resolved: 0 (unresolved) or a positive unix-millis timestamp
		resolved := rapid.OneOf(
			rapid.Just(int64(0)),
			rapid.Int64Range(1, 4102444800000),
		).Draw(t, "resolved")

		yt := model.YTIssue{
			ID:         "2-100",
			IDReadable: "TEST-1",
			Summary:    "Test issue",
			Resolved:   resolved,
			CustomFields: []model.YTCustomField{
				{
					Name:  "State",
					Value: map[string]interface{}{"name": stateName, "id": "state-id"},
				},
			},
		}

		result := ConvertYTIssueToJira(yt, "http://test", resolvedStates)

		// Compute expected category
		expectedCat := MapStateToCategory(stateName, resolvedStates)

		// Verify statusCategory matches
		if result.Fields.Status == nil {
			t.Fatal("Status is nil, expected non-nil")
		}
		sc := result.Fields.Status.StatusCategory
		if sc.ID != expectedCat.ID {
			t.Fatalf("statusCategory.ID = %d, want %d (state=%q)", sc.ID, expectedCat.ID, stateName)
		}
		if sc.Name != expectedCat.Name {
			t.Fatalf("statusCategory.Name = %q, want %q (state=%q)", sc.Name, expectedCat.Name, stateName)
		}
		if sc.Key != expectedCat.Key {
			t.Fatalf("statusCategory.Key = %q, want %q (state=%q)", sc.Key, expectedCat.Key, stateName)
		}
		if sc.ColorName != expectedCat.ColorName {
			t.Fatalf("statusCategory.ColorName = %q, want %q (state=%q)", sc.ColorName, expectedCat.ColorName, stateName)
		}

		// Verify resolutiondate correctness
		isDone := expectedCat.Key == "done"
		if isDone && resolved > 0 {
			if result.Fields.ResolutionDate == nil {
				t.Fatalf("resolutiondate is nil for done state %q with Resolved=%d; expected ISO 8601", stateName, resolved)
			}
			expected := unixMillisToISO8601(resolved)
			if *result.Fields.ResolutionDate != expected {
				t.Fatalf("resolutiondate = %q, want %q", *result.Fields.ResolutionDate, expected)
			}
			// Verify it parses as valid ISO 8601
			_, err := time.Parse("2006-01-02T15:04:05.000+0000", *result.Fields.ResolutionDate)
			if err != nil {
				t.Fatalf("resolutiondate %q is not valid ISO 8601: %v", *result.Fields.ResolutionDate, err)
			}
		} else {
			if result.Fields.ResolutionDate != nil {
				t.Fatalf("resolutiondate should be nil for state %q (done=%v, Resolved=%d), got %q",
					stateName, isDone, resolved, *result.Fields.ResolutionDate)
			}
		}
	})
}

// --- Property Test for dynamic-resolved-states spec ---

// TestPropertyDynamic_ActivityResolutionUsesResolvedSet validates Property 6:
// Activity-Based Resolution Uses Dynamic Set.
//
// Feature: dynamic-resolved-states, Property 6: Activity-Based Resolution Uses Dynamic Set
//
// For any slice of YTActivityItem containing State field changes and any resolved state set,
// DeriveResolutionDateFromActivities(activities, resolvedStates) SHALL return:
//   - The timestamp of the latest activity whose Added contains a state name present
//     (case-insensitively) in resolvedStates, or
//   - nil if no such activity exists.
//
// **Validates: Requirements 6.1, 6.2**
func TestPropertyDynamic_ActivityResolutionUsesResolvedSet(t *testing.T) {
	// Generator for a random resolved state set with 1–5 lowercased keys.
	resolvedSetGen := rapid.Custom(func(t *rapid.T) ResolvedStateSet {
		n := rapid.IntRange(1, 5).Draw(t, "resolvedSetSize")
		set := make(ResolvedStateSet, n)
		for i := 0; i < n; i++ {
			name := rapid.StringMatching(`[a-z]{3,12}`).Draw(t, fmt.Sprintf("resolvedName_%d", i))
			set[name] = struct{}{}
		}
		return set
	})

	// Collect resolved names as a slice for sampling.
	resolvedNames := func(set ResolvedStateSet) []string {
		names := make([]string, 0, len(set))
		for k := range set {
			names = append(names, k)
		}
		return names
	}

	// Case variation helper.
	caseVariant := func(t *rapid.T, base string, label string) string {
		v := rapid.SampledFrom([]int{0, 1, 2}).Draw(t, label)
		switch v {
		case 0:
			return strings.ToUpper(base)
		case 1:
			return strings.ToLower(base)
		default:
			if len(base) == 0 {
				return base
			}
			return strings.ToUpper(base[:1]) + base[1:]
		}
	}

	// Non-State field names for noise activities.
	nonStateFields := []string{"Priority", "Assignee", "Type", "Subsystem", "Fix versions"}
	// Non-resolved state names guaranteed not to collide with the 3–12 lowercase alpha resolved names.
	nonResolvedStates := []string{"Zzz Open", "Zzz In Progress", "Zzz Reopened", "Zzz Waiting"}

	rapid.Check(t, func(t *rapid.T) {
		resolvedSet := resolvedSetGen.Draw(t, "resolvedSet")
		rNames := resolvedNames(resolvedSet)

		// Decide how many done-transitions (0–4) and noise activities (0–5).
		numDone := rapid.IntRange(0, 4).Draw(t, "numDone")
		numNoise := rapid.IntRange(0, 5).Draw(t, "numNoise")

		activities := make([]model.YTActivityItem, 0, numDone+numNoise)
		var expectedTs int64
		expectNonNil := false

		// Generate done-transition activities (State field, Added name is in resolvedSet).
		for i := 0; i < numDone; i++ {
			ts := rapid.Int64Range(1, 4102444800000).Draw(t, fmt.Sprintf("doneTs_%d", i))
			baseName := rapid.SampledFrom(rNames).Draw(t, fmt.Sprintf("doneBase_%d", i))
			displayName := caseVariant(t, baseName, fmt.Sprintf("doneCase_%d", i))

			activities = append(activities, model.YTActivityItem{
				ID:        fmt.Sprintf("done-%d", i),
				Timestamp: ts,
				Author:    &model.YTUser{Login: "user"},
				Field:     &model.YTFieldRef{Name: "State"},
				Added:     []model.YTFieldDiff{{Name: displayName, ID: fmt.Sprintf("s-%d", i)}},
				Removed:   []model.YTFieldDiff{{Name: "Previous", ID: "prev"}},
			})

			if ts > expectedTs {
				expectedTs = ts
				expectNonNil = true
			}
		}

		// Generate noise activities.
		for i := 0; i < numNoise; i++ {
			ts := rapid.Int64Range(0, 4102444800000).Draw(t, fmt.Sprintf("noiseTs_%d", i))
			isStateChange := rapid.Bool().Draw(t, fmt.Sprintf("noiseIsState_%d", i))

			if isStateChange {
				// State change to a non-resolved state.
				nonRes := rapid.SampledFrom(nonResolvedStates).Draw(t, fmt.Sprintf("nonRes_%d", i))
				activities = append(activities, model.YTActivityItem{
					ID:        fmt.Sprintf("noise-state-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: "State"},
					Added:     []model.YTFieldDiff{{Name: nonRes, ID: fmt.Sprintf("nrs-%d", i)}},
					Removed:   []model.YTFieldDiff{{Name: "Prev", ID: "p"}},
				})
			} else {
				// Non-State field change.
				fieldName := rapid.SampledFrom(nonStateFields).Draw(t, fmt.Sprintf("noiseField_%d", i))
				activities = append(activities, model.YTActivityItem{
					ID:        fmt.Sprintf("noise-field-%d", i),
					Timestamp: ts,
					Author:    &model.YTUser{Login: "user"},
					Field:     &model.YTFieldRef{Name: fieldName},
					Added:     []model.YTFieldDiff{{Name: "Val", ID: "v"}},
				})
			}
		}

		result := DeriveResolutionDateFromActivities(activities, resolvedSet)

		if expectNonNil {
			if result == nil {
				t.Fatal("DeriveResolutionDateFromActivities returned nil; expected non-nil for activities containing a done-transition")
			}
			if *result != expectedTs {
				t.Fatalf("DeriveResolutionDateFromActivities = %d, want %d (latest done-transition)", *result, expectedTs)
			}
		} else {
			if result != nil {
				t.Fatalf("DeriveResolutionDateFromActivities returned %d; expected nil when no done-transition exists", *result)
			}
		}
	})
}
