package service

import (
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

// legacyDoneStates mirrors all states that were (or should have been) in the old
// hardcoded doneStates map, for backward-compat tests.
var legacyDoneStates = ResolvedStateSet{
	"fixed": {}, "verified": {}, "obsolete": {}, "done": {},
	"closed": {}, "resolved": {}, "complete": {}, "completed": {},
	"won't fix": {}, "duplicate": {}, "rejected": {}, "can't reproduce": {},
}

// randomCaseVariant returns a rapid generator that produces random casing
// variants of the given string (e.g. "won't fix" → "WON'T FIX", "Won't Fix", etc.)
func randomCaseVariant(base string) *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		runes := []rune(base)
		var b strings.Builder
		for _, r := range runes {
			if rapid.Bool().Draw(t, "upper") {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		}
		return b.String()
	})
}

// TestPropertyBugCondition_MissingTerminalStatesMapToDone validates that states
// previously missing from the hardcoded doneStates map ("won't fix", "duplicate",
// "rejected", "can't reproduce") now correctly map to CatDone when present in the
// dynamic resolved state set.
//
// **Validates: Requirements 3.1**
func TestPropertyBugCondition_MissingTerminalStatesMapToDone(t *testing.T) {
	missingStates := []string{"won't fix", "duplicate", "rejected", "can't reproduce"}

	for _, base := range missingStates {
		t.Run(base, func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				input := randomCaseVariant(base).Draw(t, "stateName")

				result := MapStateToCategory(input, legacyDoneStates)

				if result.Key != CatDone.Key {
					t.Fatalf("MapStateToCategory(%q) = {ID:%d, Name:%q, Key:%q, ColorName:%q}, want CatDone {ID:%d, Name:%q, Key:%q, ColorName:%q}",
						input, result.ID, result.Name, result.Key, result.ColorName,
						CatDone.ID, CatDone.Name, CatDone.Key, CatDone.ColorName)
				}
				if result.ID != CatDone.ID {
					t.Fatalf("MapStateToCategory(%q).ID = %d, want %d", input, result.ID, CatDone.ID)
				}
				if result.Name != CatDone.Name {
					t.Fatalf("MapStateToCategory(%q).Name = %q, want %q", input, result.Name, CatDone.Name)
				}
				if result.ColorName != CatDone.ColorName {
					t.Fatalf("MapStateToCategory(%q).ColorName = %q, want %q", input, result.ColorName, CatDone.ColorName)
				}
			})
		})
	}
}

// TestPreservation_ExistingDoneStatesReturnCatDone validates that all 8 existing doneStates
// continue to return CatDone with correct fields, using random casing variants.
//
// **Validates: Requirements 3.1**
func TestPreservation_ExistingDoneStatesReturnCatDone(t *testing.T) {
	existingDoneStates := []string{"fixed", "verified", "obsolete", "done", "closed", "resolved", "complete", "completed"}

	for _, base := range existingDoneStates {
		t.Run(base, func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				input := randomCaseVariant(base).Draw(t, "stateName")
				result := MapStateToCategory(input, legacyDoneStates)
				if result != CatDone {
					t.Fatalf("MapStateToCategory(%q) = %+v, want CatDone %+v", input, result, CatDone)
				}
			})
		})
	}
}

// TestPreservation_ExistingNewStatesReturnCatToDo validates that all 7 existing newStates
// continue to return CatToDo with correct fields, using random casing variants.
//
// **Validates: Requirements 3.2**
func TestPreservation_ExistingNewStatesReturnCatToDo(t *testing.T) {
	existingNewStates := []string{"open", "submitted", "incomplete", "new", "reopened", "to do", "backlog"}

	for _, base := range existingNewStates {
		t.Run(base, func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				input := randomCaseVariant(base).Draw(t, "stateName")
				result := MapStateToCategory(input, legacyDoneStates)
				if result != CatToDo {
					t.Fatalf("MapStateToCategory(%q) = %+v, want CatToDo %+v", input, result, CatToDo)
				}
			})
		})
	}
}

// TestPreservation_UnknownStatesReturnCatInProgress validates that arbitrary strings
// not in doneStates or newStates return CatInProgress. The four bug-condition states
// are filtered out so this test passes on both unfixed and fixed code.
//
// **Validates: Requirements 3.3, 3.4**
func TestPreservation_UnknownStatesReturnCatInProgress(t *testing.T) {
	knownStates := map[string]struct{}{
		// existing doneStates
		"fixed": {}, "verified": {}, "obsolete": {}, "done": {},
		"closed": {}, "resolved": {}, "complete": {}, "completed": {},
		// newStates
		"open": {}, "submitted": {}, "incomplete": {}, "new": {},
		"reopened": {}, "to do": {}, "backlog": {},
		// bug-condition states (filter out so test works pre- and post-fix)
		"won't fix": {}, "duplicate": {}, "rejected": {}, "can't reproduce": {},
	}

	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "stateName")
		lower := strings.ToLower(input)
		if _, known := knownStates[lower]; known {
			t.Skip("known state, not testing default fallback")
		}
		result := MapStateToCategory(input, legacyDoneStates)
		if result != CatInProgress {
			t.Fatalf("MapStateToCategory(%q) = %+v, want CatInProgress %+v", input, result, CatInProgress)
		}
	})
}

// TestPropertyStatusCategoryValidity validates Property 5: Status Category Validity.
// For any string passed to MapStateToCategory, the returned StatusCategoryInfo SHALL have
// a Key that is exactly one of "new", "indeterminate", or "done", and ID, Name, and ColorName
// SHALL be non-empty and consistent with the Key value.
//
// **Validates: Requirements 4.1, 4.2, 4.3, 4.4**
func TestPropertyStatusCategoryValidity(t *testing.T) {
	validCategories := map[string]StatusCategoryInfo{
		"new":           CatToDo,
		"indeterminate": CatInProgress,
		"done":          CatDone,
	}

	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "stateName")

		result := MapStateToCategory(input, legacyDoneStates)

		// Key must be one of the three valid values
		expected, ok := validCategories[result.Key]
		if !ok {
			t.Fatalf("Key %q is not one of {new, indeterminate, done}", result.Key)
		}

		// ID must be positive
		if result.ID <= 0 {
			t.Fatalf("ID = %d, want > 0", result.ID)
		}

		// Name must be non-empty
		if result.Name == "" {
			t.Fatal("Name is empty")
		}

		// ColorName must be non-empty
		if result.ColorName == "" {
			t.Fatal("ColorName is empty")
		}

		// All fields must be consistent with the Key
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
