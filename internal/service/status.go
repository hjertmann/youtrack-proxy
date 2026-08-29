package service

import (
	"strings"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// ResolvedStateSet is a set of lowercased state names marked isResolved in YouTrack.
type ResolvedStateSet = map[string]struct{}

// StatusCategoryInfo holds Jira status category metadata.
type StatusCategoryInfo struct {
	ID        int
	Name      string
	Key       string
	ColorName string
}

var (
	CatToDo       = StatusCategoryInfo{ID: 1, Name: "To Do", Key: "new", ColorName: "blue-gray"}
	CatInProgress = StatusCategoryInfo{ID: 4, Name: "In Progress", Key: "indeterminate", ColorName: "yellow"}
	CatDone       = StatusCategoryInfo{ID: 3, Name: "Done", Key: "done", ColorName: "green"}
)

var newStates = map[string]struct{}{
	"open":       {},
	"submitted":  {},
	"incomplete": {},
	"new":        {},
	"reopened":   {},
	"to do":      {},
	"backlog":    {},
}

// IsDoneState reports whether the given state name maps to a done-category status.
// Matching is case-insensitive, consistent with MapStateToCategory.
func IsDoneState(name string, resolvedStates ResolvedStateSet) bool {
	_, ok := resolvedStates[strings.ToLower(name)]
	return ok
}

// MapStateToCategory returns the Jira status category for a YouTrack state name.
// Unknown states default to CatInProgress. Matching is case-insensitive.
func MapStateToCategory(stateName string, resolvedStates ResolvedStateSet) StatusCategoryInfo {
	lower := strings.ToLower(stateName)
	if _, ok := resolvedStates[lower]; ok {
		return CatDone
	}
	if _, ok := newStates[lower]; ok {
		return CatToDo
	}
	return CatInProgress
}

// BuildResolvedStateSet extracts lowercased resolved state names from a project's
// custom fields. Returns an empty set if no State field, nil bundle, or no resolved values.
func BuildResolvedStateSet(fields []model.YTProjectCustomField) ResolvedStateSet {
	for _, f := range fields {
		if f.Field.Name != "State" || f.Bundle == nil {
			continue
		}
		set := make(ResolvedStateSet, len(f.Bundle.Values))
		for _, bv := range f.Bundle.Values {
			if bv.IsResolved {
				set[strings.ToLower(bv.Name)] = struct{}{}
			}
		}
		return set
	}
	return ResolvedStateSet{}
}
