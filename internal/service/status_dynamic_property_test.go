package service

import (
	"strings"
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// genResolvedStateSet generates a random ResolvedStateSet (map[string]struct{})
// with lowercased keys, as BuildResolvedStateSet would produce.
func genResolvedStateSet() *rapid.Generator[ResolvedStateSet] {
	return rapid.Custom(func(t *rapid.T) ResolvedStateSet {
		keys := rapid.SliceOf(rapid.String()).Draw(t, "resolvedKeys")
		set := make(ResolvedStateSet, len(keys))
		for _, k := range keys {
			set[strings.ToLower(k)] = struct{}{}
		}
		return set
	})
}

// TestPropertyDynamic_MapStateToCategory validates Property 1: Dynamic State-to-Category Mapping.
//
// For any state name string and any resolved state set, MapStateToCategory(stateName, resolvedStates) SHALL return:
//   - CatDone if strings.ToLower(stateName) is in resolvedStates,
//   - CatToDo if the lowercased name is NOT in resolvedStates but IS in newStates,
//   - CatInProgress otherwise.
//
// The returned StatusCategoryInfo SHALL always have a valid Key in {"new", "indeterminate", "done"}
// with consistent ID, Name, and ColorName.
//
// Feature: dynamic-resolved-states, Property 1: Dynamic State-to-Category Mapping
//
// **Validates: Requirements 3.1, 3.2, 3.3**
func TestPropertyDynamic_MapStateToCategory(t *testing.T) {
	validCategories := map[string]StatusCategoryInfo{
		"new":           CatToDo,
		"indeterminate": CatInProgress,
		"done":          CatDone,
	}

	rapid.Check(t, func(t *rapid.T) {
		stateName := rapid.String().Draw(t, "stateName")
		resolvedStates := genResolvedStateSet().Draw(t, "resolvedStates")

		result := MapStateToCategory(stateName, resolvedStates)
		lower := strings.ToLower(stateName)

		// Verify three-way mapping logic
		_, inResolved := resolvedStates[lower]
		_, inNew := newStates[lower]

		switch {
		case inResolved:
			if result != CatDone {
				t.Fatalf("state %q is in resolvedStates, got %+v, want CatDone %+v", stateName, result, CatDone)
			}
		case inNew:
			if result != CatToDo {
				t.Fatalf("state %q not in resolvedStates but in newStates, got %+v, want CatToDo %+v", stateName, result, CatToDo)
			}
		default:
			if result != CatInProgress {
				t.Fatalf("state %q not in resolvedStates or newStates, got %+v, want CatInProgress %+v", stateName, result, CatInProgress)
			}
		}

		// Verify structural validity: Key must be valid and fields consistent
		expected, ok := validCategories[result.Key]
		if !ok {
			t.Fatalf("Key %q is not one of {new, indeterminate, done}", result.Key)
		}
		if result.ID != expected.ID {
			t.Fatalf("Key=%q: ID = %d, want %d", result.Key, result.ID, expected.ID)
		}
		if result.Name != expected.Name {
			t.Fatalf("Key=%q: Name = %q, want %q", result.Key, result.Name, expected.Name)
		}
		if result.ColorName != expected.ColorName {
			t.Fatalf("Key=%q: ColorName = %q, want %q", result.Key, result.ColorName, expected.ColorName)
		}
	})
}

// TestPropertyDynamic_IsDoneStateConsistency validates Property 2: IsDoneState Consistency.
//
// For any state name string and any resolved state set, IsDoneState(name, resolvedStates)
// SHALL return true if and only if MapStateToCategory(name, resolvedStates).Key == "done".
//
// Feature: dynamic-resolved-states, Property 2: IsDoneState Consistency
//
// **Validates: Requirements 3.5**
func TestPropertyDynamic_IsDoneStateConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.String().Draw(t, "stateName")
		resolved := genResolvedStateSet().Draw(t, "resolvedStates")

		isDone := IsDoneState(name, resolved)
		catKey := MapStateToCategory(name, resolved).Key

		if isDone != (catKey == "done") {
			t.Fatalf("IsDoneState(%q, resolvedStates) = %v, but MapStateToCategory().Key = %q; want consistency",
				name, isDone, catKey)
		}
	})
}

// genBundleValue generates a random YTBundleValue with an arbitrary name and IsResolved flag.
func genBundleValue() *rapid.Generator[model.YTBundleValue] {
	return rapid.Custom(func(t *rapid.T) model.YTBundleValue {
		return model.YTBundleValue{
			ID:         rapid.String().Draw(t, "id"),
			Name:       rapid.StringMatching(`[A-Za-z ]{1,30}`).Draw(t, "name"),
			IsResolved: rapid.Bool().Draw(t, "isResolved"),
			Type:       "StateBundleElement",
		}
	})
}

// genProjectCustomFields generates a random []YTProjectCustomField slice that may
// or may not contain a State field, with or without a bundle, exercising all branches.
func genProjectCustomFields() *rapid.Generator[[]model.YTProjectCustomField] {
	return rapid.Custom(func(t *rapid.T) []model.YTProjectCustomField {
		includeState := rapid.Bool().Draw(t, "includeState")
		extraCount := rapid.IntRange(0, 3).Draw(t, "extraFieldCount")

		var fields []model.YTProjectCustomField

		// Add non-State fields
		for i := 0; i < extraCount; i++ {
			fields = append(fields, model.YTProjectCustomField{
				Field:  model.YTCustomFieldRef{Name: rapid.SampledFrom([]string{"Type", "Priority", "Subsystem", "Fix versions"}).Draw(t, "otherFieldName")},
				Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{{ID: "1", Name: "v1"}}},
				Type:   "CustomField",
			})
		}

		if includeState {
			nilBundle := rapid.Bool().Draw(t, "nilBundle")
			var bundle *model.YTFieldBundle
			if !nilBundle {
				values := rapid.SliceOf(genBundleValue()).Draw(t, "bundleValues")
				bundle = &model.YTFieldBundle{Values: values, Type: "StateBundle"}
			}
			stateField := model.YTProjectCustomField{
				Field:  model.YTCustomFieldRef{Name: "State"},
				Bundle: bundle,
				Type:   "CustomField",
			}
			// Insert state field at a random position
			pos := rapid.IntRange(0, len(fields)).Draw(t, "statePos")
			fields = append(fields, model.YTProjectCustomField{})
			copy(fields[pos+1:], fields[pos:])
			fields[pos] = stateField
		}

		return fields
	})
}

// TestPropertyDynamic_BuildResolvedStateSet validates Property 3: BuildResolvedStateSet Collects Exactly Resolved Names.
//
// For any slice of YTProjectCustomField containing a State field with a bundle of values,
// BuildResolvedStateSet SHALL return a set containing exactly the strings.ToLower(name) of
// each bundle value where IsResolved == true, and no other entries. If no State field exists,
// or the bundle is nil, or no values have IsResolved == true, the returned set SHALL be empty.
//
// Feature: dynamic-resolved-states, Property 3: BuildResolvedStateSet Collects Exactly Resolved Names
//
// **Validates: Requirements 2.1, 2.3**
func TestPropertyDynamic_BuildResolvedStateSet(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fields := genProjectCustomFields().Draw(t, "fields")
		result := BuildResolvedStateSet(fields)

		// Compute expected set independently
		expected := make(ResolvedStateSet)
		var foundState bool
		for _, f := range fields {
			if f.Field.Name == "State" {
				foundState = true
				if f.Bundle != nil {
					for _, bv := range f.Bundle.Values {
						if bv.IsResolved {
							expected[strings.ToLower(bv.Name)] = struct{}{}
						}
					}
				}
				break // BuildResolvedStateSet returns on the first State field
			}
		}

		// If no State field found → expect empty
		if !foundState {
			if len(result) != 0 {
				t.Fatalf("no State field, expected empty set, got %v", result)
			}
			return
		}

		// Verify exact match: result contains exactly expected entries
		if len(result) != len(expected) {
			t.Fatalf("set size mismatch: got %d entries %v, want %d entries %v", len(result), result, len(expected), expected)
		}
		for k := range expected {
			if _, ok := result[k]; !ok {
				t.Fatalf("expected key %q missing from result set %v", k, result)
			}
		}
		for k := range result {
			if _, ok := expected[k]; !ok {
				t.Fatalf("unexpected key %q in result set %v, expected %v", k, result, expected)
			}
		}
	})
}
