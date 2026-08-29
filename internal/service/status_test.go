package service

import (
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

func TestBuildResolvedStateSet_HappyPath(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Open", IsResolved: false},
				{Name: "Fixed", IsResolved: true},
				{Name: "In Progress", IsResolved: false},
				{Name: "Verified", IsResolved: true},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)

	if len(set) != 2 {
		t.Fatalf("len = %d, want 2", len(set))
	}
	if _, ok := set["fixed"]; !ok {
		t.Error("missing 'fixed'")
	}
	if _, ok := set["verified"]; !ok {
		t.Error("missing 'verified'")
	}
	// Non-resolved should be absent.
	if _, ok := set["open"]; ok {
		t.Error("'open' should not be in resolved set")
	}
}

func TestBuildResolvedStateSet_NoStateField(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "Priority"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Critical", IsResolved: false},
			}},
		},
		{
			Field:  model.YTCustomFieldRef{Name: "Type"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Bug", IsResolved: false},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestBuildResolvedStateSet_NilBundle(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: nil,
		},
	}

	set := BuildResolvedStateSet(fields)
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestBuildResolvedStateSet_EmptyValues(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestBuildResolvedStateSet_NoneResolved(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Open", IsResolved: false},
				{Name: "In Progress", IsResolved: false},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(set))
	}
}

func TestBuildResolvedStateSet_AllResolved(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Done", IsResolved: true},
				{Name: "Closed", IsResolved: true},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if len(set) != 2 {
		t.Fatalf("len = %d, want 2", len(set))
	}
	if _, ok := set["done"]; !ok {
		t.Error("missing 'done'")
	}
	if _, ok := set["closed"]; !ok {
		t.Error("missing 'closed'")
	}
}

func TestBuildResolvedStateSet_LowercasesNames(t *testing.T) {
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "FIXED", IsResolved: true},
				{Name: "Won't Fix", IsResolved: true},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if _, ok := set["fixed"]; !ok {
		t.Error("expected lowercased 'fixed'")
	}
	if _, ok := set["won't fix"]; !ok {
		t.Error("expected lowercased \"won't fix\"")
	}
	// Original case should not be present.
	if _, ok := set["FIXED"]; ok {
		t.Error("uppercase 'FIXED' should not be a key")
	}
}

func TestBuildResolvedStateSet_MultipleFieldsUsesFirstState(t *testing.T) {
	// If there are multiple "State" fields (unusual), only the first is used.
	fields := []model.YTProjectCustomField{
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Alpha", IsResolved: true},
			}},
		},
		{
			Field:  model.YTCustomFieldRef{Name: "State"},
			Bundle: &model.YTFieldBundle{Values: []model.YTBundleValue{
				{Name: "Beta", IsResolved: true},
			}},
		},
	}

	set := BuildResolvedStateSet(fields)
	if _, ok := set["alpha"]; !ok {
		t.Error("expected 'alpha' from first State field")
	}
	if _, ok := set["beta"]; ok {
		t.Error("'beta' from second State field should not be present")
	}
}

func TestBuildResolvedStateSet_EmptyFieldSlice(t *testing.T) {
	set := BuildResolvedStateSet(nil)
	if len(set) != 0 {
		t.Fatalf("expected empty set for nil input, got %d entries", len(set))
	}

	set = BuildResolvedStateSet([]model.YTProjectCustomField{})
	if len(set) != 0 {
		t.Fatalf("expected empty set for empty slice, got %d entries", len(set))
	}
}

// TestNewStatesMap validates the newStates map contains exactly the 7 expected entries.
func TestNewStatesMap(t *testing.T) {
	expected := []string{"open", "submitted", "incomplete", "new", "reopened", "to do", "backlog"}

	if len(newStates) != len(expected) {
		t.Fatalf("newStates has %d entries, want %d", len(newStates), len(expected))
	}
	for _, s := range expected {
		if _, ok := newStates[s]; !ok {
			t.Errorf("newStates missing %q", s)
		}
	}
}
