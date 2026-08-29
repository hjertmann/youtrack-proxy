package service

import (
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestProperty4_IssueTypeIDAlignment validates Property 4: Issue-type ID alignment.
// For any YouTrack issue with a Type custom field value that has a valid bundle value ID,
// the issuetype.id in the Jira-converted issue SHALL equal the id produced by
// idmap.Encode for the same bundle value ID, formatted via idmap.FormatID.
// This is the deterministic equivalent of the old IDMap consistency check.
//
// **Validates: Requirements 4.1, 4.2, 4.3**
func TestProperty4_IssueTypeIDAlignment(t *testing.T) {
	// Use valid YouTrack bundle value ID prefixes (69-N for types)
	rapid.Check(t, func(rt *rapid.T) {
		leaf := rapid.Int64Range(0, 10000).Draw(rt, "leaf")
		bundleValueID := "69-" + idmap.FormatID(leaf)
		typeName := rapid.StringMatching(`[A-Z][a-z]{2,10}`).Draw(rt, "typeName")

		issue := model.YTIssue{
			ID:         "2-100",
			IDReadable: "TEST-1",
			Summary:    "test issue",
			Created:    1700000000000,
			Updated:    1700000000000,
			CustomFields: []model.YTCustomField{
				{
					Name: "Type",
					Value: map[string]interface{}{
						"id":   bundleValueID,
						"name": typeName,
					},
				},
			},
		}

		jiraIssue := ConvertYTIssueToJira(issue, "http://test", nil)
		converterID := jiraIssue.Fields.IssueType.ID

		// Simulate what HandleListIssueTypes does: Encode the same bundle value ID.
		numID, err := idmap.Encode(bundleValueID)
		if err != nil {
			rt.Fatalf("idmap.Encode(%q) error: %v", bundleValueID, err)
		}
		pickerID := idmap.FormatID(numID)

		if converterID != pickerID {
			rt.Fatalf("ID mismatch for bundleValueID=%q: converter=%q, picker=%q",
				bundleValueID, converterID, pickerID)
		}
	})
}
