package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyBugCondition_ScalarRemovedUnmarshal tests that scalar number values
// in the "removed" field of YTActivityItem JSON can be unmarshaled successfully.
//
// **Validates: Requirements 1.1, 1.3**
//
// EXPECTED: This test FAILS on unfixed code, confirming the bug exists.
// The bug is: json.Unmarshal cannot decode a scalar number into []YTFieldDiff.
func TestPropertyBugCondition_ScalarRemovedUnmarshal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random scalar number (integer or float)
		scalar := rapid.OneOf(
			rapid.Map(rapid.IntRange(-1000, 1000), func(n int) string {
				return strconv.Itoa(n)
			}),
			rapid.Map(rapid.Float64Range(-1000.0, 1000.0), func(f float64) string {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}),
		).Draw(t, "scalar")

		// Construct JSON with scalar in "removed" field
		jsonStr := fmt.Sprintf(`{"id":"test-1","timestamp":1000,"removed":%s,"added":[],"field":{"id":"f1","name":"Story Points"}}`, scalar)

		var item YTActivityItem
		err := json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed for scalar removed=%s: %v", scalar, err)
		}

		// Assert the scalar was converted to a single-element slice
		if len(item.Removed) != 1 {
			t.Fatalf("expected Removed to have 1 element, got %d for scalar %s", len(item.Removed), scalar)
		}
		if item.Removed[0].Name != scalar {
			t.Fatalf("expected Removed[0].Name=%q, got %q", scalar, item.Removed[0].Name)
		}
	})
}

// TestPropertyBugCondition_ScalarAddedUnmarshal tests that scalar number values
// in the "added" field of YTActivityItem JSON can be unmarshaled successfully.
//
// **Validates: Requirements 1.2, 1.3**
//
// EXPECTED: This test FAILS on unfixed code, confirming the bug exists.
func TestPropertyBugCondition_ScalarAddedUnmarshal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random scalar number (integer or float)
		scalar := rapid.OneOf(
			rapid.Map(rapid.IntRange(-1000, 1000), func(n int) string {
				return strconv.Itoa(n)
			}),
			rapid.Map(rapid.Float64Range(-1000.0, 1000.0), func(f float64) string {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}),
		).Draw(t, "scalar")

		// Construct JSON with scalar in "added" field
		jsonStr := fmt.Sprintf(`{"id":"test-2","timestamp":1000,"removed":[],"added":%s,"field":{"id":"f1","name":"Estimation"}}`, scalar)

		var item YTActivityItem
		err := json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed for scalar added=%s: %v", scalar, err)
		}

		// Assert the scalar was converted to a single-element slice
		if len(item.Added) != 1 {
			t.Fatalf("expected Added to have 1 element, got %d for scalar %s", len(item.Added), scalar)
		}
		if item.Added[0].Name != scalar {
			t.Fatalf("expected Added[0].Name=%q, got %q", scalar, item.Added[0].Name)
		}
	})
}

// TestPropertyBugCondition_BothScalarsUnmarshal tests that scalar number values
// in BOTH "added" and "removed" fields simultaneously can be unmarshaled.
//
// **Validates: Requirements 1.1, 1.2, 1.3**
//
// EXPECTED: This test FAILS on unfixed code, confirming the bug exists.
func TestPropertyBugCondition_BothScalarsUnmarshal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate two random scalar numbers
		removedScalar := rapid.OneOf(
			rapid.Map(rapid.IntRange(-1000, 1000), func(n int) string {
				return strconv.Itoa(n)
			}),
			rapid.Map(rapid.Float64Range(-1000.0, 1000.0), func(f float64) string {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}),
		).Draw(t, "removedScalar")

		addedScalar := rapid.OneOf(
			rapid.Map(rapid.IntRange(-1000, 1000), func(n int) string {
				return strconv.Itoa(n)
			}),
			rapid.Map(rapid.Float64Range(-1000.0, 1000.0), func(f float64) string {
				return strconv.FormatFloat(f, 'f', -1, 64)
			}),
		).Draw(t, "addedScalar")

		// Construct JSON with scalars in both fields
		jsonStr := fmt.Sprintf(`{"id":"test-3","timestamp":1000,"removed":%s,"added":%s,"field":{"id":"f1","name":"Priority"}}`, removedScalar, addedScalar)

		var item YTActivityItem
		err := json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed for removed=%s, added=%s: %v", removedScalar, addedScalar, err)
		}

		// Assert both scalars were converted
		if len(item.Removed) != 1 {
			t.Fatalf("expected Removed to have 1 element, got %d", len(item.Removed))
		}
		if item.Removed[0].Name != removedScalar {
			t.Fatalf("expected Removed[0].Name=%q, got %q", removedScalar, item.Removed[0].Name)
		}
		if len(item.Added) != 1 {
			t.Fatalf("expected Added to have 1 element, got %d", len(item.Added))
		}
		if item.Added[0].Name != addedScalar {
			t.Fatalf("expected Added[0].Name=%q, got %q", addedScalar, item.Added[0].Name)
		}
	})
}

// ============================================================================
// PRESERVATION PROPERTY TESTS
// These tests verify that array-typed and null/absent added/removed fields
// continue to unmarshal correctly. They should PASS on unfixed code.
// ============================================================================

// TestPropertyPreservation_ArrayFieldsRoundTrip tests that randomly generated
// []YTFieldDiff arrays serialize to JSON and unmarshal back into YTActivityItem
// producing identical slices. This ensures the fix does not regress array behavior.
//
// **Validates: Requirements 3.1, 3.2, 3.4**
func TestPropertyPreservation_ArrayFieldsRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random []YTFieldDiff for "added" (length 0-10)
		addedLen := rapid.IntRange(0, 10).Draw(t, "addedLen")
		added := make([]YTFieldDiff, addedLen)
		for i := range added {
			added[i] = YTFieldDiff{
				Name: rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, fmt.Sprintf("added[%d].Name", i)),
				Type: rapid.StringMatching(`[A-Za-z]{1,15}`).Draw(t, fmt.Sprintf("added[%d].Type", i)),
			}
		}

		// Generate random []YTFieldDiff for "removed" (length 0-10)
		removedLen := rapid.IntRange(0, 10).Draw(t, "removedLen")
		removed := make([]YTFieldDiff, removedLen)
		for i := range removed {
			removed[i] = YTFieldDiff{
				Name: rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, fmt.Sprintf("removed[%d].Name", i)),
				Type: rapid.StringMatching(`[A-Za-z]{1,15}`).Draw(t, fmt.Sprintf("removed[%d].Type", i)),
			}
		}

		// Serialize the added and removed arrays to JSON
		addedJSON, err := json.Marshal(added)
		if err != nil {
			t.Fatalf("failed to marshal added: %v", err)
		}
		removedJSON, err := json.Marshal(removed)
		if err != nil {
			t.Fatalf("failed to marshal removed: %v", err)
		}

		// Construct full YTActivityItem JSON with array-typed added/removed
		jsonStr := fmt.Sprintf(`{"id":"preserve-1","timestamp":2000,"added":%s,"removed":%s,"field":{"id":"f1","name":"State"}}`,
			string(addedJSON), string(removedJSON))

		var item YTActivityItem
		err = json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed for array inputs: %v\nJSON: %s", err, jsonStr)
		}

		// Assert Added slice matches
		if len(item.Added) != addedLen {
			t.Fatalf("expected Added length %d, got %d", addedLen, len(item.Added))
		}
		for i, fd := range item.Added {
			if fd.Name != added[i].Name {
				t.Fatalf("Added[%d].Name: expected %q, got %q", i, added[i].Name, fd.Name)
			}
			if fd.Type != added[i].Type {
				t.Fatalf("Added[%d].Type: expected %q, got %q", i, added[i].Type, fd.Type)
			}
		}

		// Assert Removed slice matches
		if len(item.Removed) != removedLen {
			t.Fatalf("expected Removed length %d, got %d", removedLen, len(item.Removed))
		}
		for i, fd := range item.Removed {
			if fd.Name != removed[i].Name {
				t.Fatalf("Removed[%d].Name: expected %q, got %q", i, removed[i].Name, fd.Name)
			}
			if fd.Type != removed[i].Type {
				t.Fatalf("Removed[%d].Type: expected %q, got %q", i, removed[i].Type, fd.Type)
			}
		}
	})
}

// TestPropertyPreservation_NullAndAbsentFields tests that null and absent
// added/removed fields produce nil slices, which is the current correct behavior.
//
// **Validates: Requirements 3.3**
func TestPropertyPreservation_NullAndAbsentFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Choose a scenario: null fields, absent fields, or mix
		scenario := rapid.IntRange(0, 4).Draw(t, "scenario")

		var jsonStr string
		switch scenario {
		case 0:
			// Both null
			jsonStr = `{"id":"null-1","timestamp":3000,"added":null,"removed":null,"field":{"id":"f1","name":"State"}}`
		case 1:
			// Both absent
			jsonStr = `{"id":"absent-1","timestamp":3000,"field":{"id":"f1","name":"State"}}`
		case 2:
			// Added null, removed absent
			jsonStr = `{"id":"mix-1","timestamp":3000,"added":null,"field":{"id":"f1","name":"State"}}`
		case 3:
			// Added absent, removed null
			jsonStr = `{"id":"mix-2","timestamp":3000,"removed":null,"field":{"id":"f1","name":"State"}}`
		case 4:
			// Both empty arrays (edge case - should produce empty slices)
			jsonStr = `{"id":"empty-1","timestamp":3000,"added":[],"removed":[],"field":{"id":"f1","name":"State"}}`
		}

		var item YTActivityItem
		err := json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed for scenario %d: %v\nJSON: %s", scenario, err, jsonStr)
		}

		if scenario == 4 {
			// Empty arrays: Go's json.Unmarshal produces a non-nil empty slice for `[]`
			// but we just need to verify no error and length == 0
			if len(item.Added) != 0 {
				t.Fatalf("scenario %d: expected Added length 0, got %d", scenario, len(item.Added))
			}
			if len(item.Removed) != 0 {
				t.Fatalf("scenario %d: expected Removed length 0, got %d", scenario, len(item.Removed))
			}
		} else {
			// Null or absent fields produce nil slices
			if item.Added != nil {
				t.Fatalf("scenario %d: expected Added to be nil, got %v", scenario, item.Added)
			}
			if item.Removed != nil {
				t.Fatalf("scenario %d: expected Removed to be nil, got %v", scenario, item.Removed)
			}
		}
	})
}

// TestPropertyPreservation_OtherFieldsUnaffected tests that non-added/removed
// fields (ID, Timestamp, Author, Field, Type) are correctly unmarshaled
// regardless of what added/removed contain. This ensures the fix preserves
// overall struct integrity.
//
// **Validates: Requirements 3.4**
func TestPropertyPreservation_OtherFieldsUnaffected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random field values
		id := rapid.StringMatching(`[a-z0-9\-]{1,20}`).Draw(t, "id")
		timestamp := rapid.Int64Range(0, 9999999999999).Draw(t, "timestamp")
		fieldName := rapid.StringMatching(`[A-Za-z ]{1,20}`).Draw(t, "fieldName")
		fieldID := rapid.StringMatching(`[a-z0-9\-]{1,10}`).Draw(t, "fieldID")

		// Use a valid array for added/removed (current working behavior)
		jsonStr := fmt.Sprintf(`{"id":%q,"timestamp":%d,"added":[{"name":"Val1","$type":"T1"}],"removed":[{"name":"Val2","$type":"T2"}],"field":{"id":%q,"name":%q}}`,
			id, timestamp, fieldID, fieldName)

		var item YTActivityItem
		err := json.Unmarshal([]byte(jsonStr), &item)
		if err != nil {
			t.Fatalf("UnmarshalJSON failed: %v\nJSON: %s", err, jsonStr)
		}

		// Assert non-added/removed fields preserved
		if item.ID != id {
			t.Fatalf("ID: expected %q, got %q", id, item.ID)
		}
		if item.Timestamp != timestamp {
			t.Fatalf("Timestamp: expected %d, got %d", timestamp, item.Timestamp)
		}
		if item.Field == nil {
			t.Fatal("Field should not be nil")
		}
		if item.Field.ID != fieldID {
			t.Fatalf("Field.ID: expected %q, got %q", fieldID, item.Field.ID)
		}
		if item.Field.Name != fieldName {
			t.Fatalf("Field.Name: expected %q, got %q", fieldName, item.Field.Name)
		}
	})
}
