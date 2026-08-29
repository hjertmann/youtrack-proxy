package idmap

import (
	"fmt"
	"math"
	"testing"

	"pgregory.net/rapid"
)

// Feature: activity-id-overflow-fix, Property 2: Base entity round-trip preservation
//
// TestPropertyBidirectionalRoundTrip validates that for any base entity YouTrack ID
// with TypeId in 0–255 and SeqId in 0–18,014,398,509,481,983 (54-bit baseSeqMask),
// Encode followed by Decode produces the original ID, and the encoded value is non-negative.
//
// **Validates: Requirements 3.1, 3.2**
func TestPropertyBidirectionalRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		typeId := rapid.Int64Range(0, baseTypeMask).Draw(rt, "typeId")
		seqId := rapid.Int64Range(0, baseSeqMask).Draw(rt, "seqId")

		ytID := fmt.Sprintf("%d-%d", typeId, seqId)

		encoded, err := Encode(ytID)
		if err != nil {
			rt.Fatalf("Encode(%q) failed: %v", ytID, err)
		}
		// ponytail: "0-0" legitimately encodes to 0 (all fields zero).
		// All other base entity IDs encode to > 0.
		if encoded < 0 {
			rt.Fatalf("Encode(%q) = %d, want >= 0", ytID, encoded)
		}
		if encoded == 0 && (typeId != 0 || seqId != 0) {
			rt.Fatalf("Encode(%q) = 0, want > 0 for non-zero fields", ytID)
		}

		decoded, ok := Decode(encoded)
		if !ok {
			rt.Fatalf("Decode(%d) returned false for %q", encoded, ytID)
		}
		if decoded != ytID {
			rt.Fatalf("round-trip failed: %q -> %d -> %q", ytID, encoded, decoded)
		}
	})
}

// Feature: activity-id-overflow-fix, Property 2: Base entity no-collisions
//
// TestPropertyNoCollisions validates that different base entity YouTrack IDs never
// encode to the same numeric value. Uses Mode A ranges: TypeId 0–255, SeqId 0–baseSeqMask.
//
// **Validates: Requirements 3.1, 3.2**
func TestPropertyNoCollisions(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		typeId1 := rapid.Int64Range(0, baseTypeMask).Draw(rt, "typeId1")
		seqId1 := rapid.Int64Range(0, baseSeqMask).Draw(rt, "seqId1")
		typeId2 := rapid.Int64Range(0, baseTypeMask).Draw(rt, "typeId2")
		seqId2 := rapid.Int64Range(0, baseSeqMask).Draw(rt, "seqId2")

		id1 := fmt.Sprintf("%d-%d", typeId1, seqId1)
		id2 := fmt.Sprintf("%d-%d", typeId2, seqId2)

		if id1 == id2 {
			return // same input, skip
		}

		enc1, err1 := Encode(id1)
		enc2, err2 := Encode(id2)
		if err1 != nil || err2 != nil {
			return // invalid IDs, skip
		}

		if enc1 == enc2 {
			rt.Fatalf("collision: %q and %q both encode to %d", id1, id2, enc1)
		}
	})
}

// Feature: activity-id-overflow-fix, Property 4: Negative decode failure
//
// TestDecodeNegativeReturnsFalse verifies that Decode returns ("", false)
// for negative int64 values.
//
// **Validates: Requirements 3.4**
func TestDecodeNegativeReturnsFalse(t *testing.T) {
	negatives := []int64{-1, -42, -1000, -9223372036854775808} // includes math.MinInt64
	for _, n := range negatives {
		decoded, ok := Decode(n)
		if ok {
			t.Fatalf("Decode(%d) returned ok=true, decoded=%q; want (\"\", false)", n, decoded)
		}
		if decoded != "" {
			t.Fatalf("Decode(%d) returned %q, want \"\"", n, decoded)
		}
	}
}

// Feature: activity-id-overflow-fix, Property 1: Activity stream round-trip preservation
//
// TestPropertyActivityStreamRoundTrip validates that for any activity stream YouTrack ID
// with TypeId in 0–127, SeqId in 0–16,777,215, CatId in 0–63, and EventOffset
// in 0–33,554,431, Encode followed by Decode produces the original ID, and the encoded
// value is strictly positive (flag bit is always set for activity IDs).
//
// **Validates: Requirements 2.1, 2.2, 3.6**
func TestPropertyActivityStreamRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		typeId := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId")
		seqId := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId")
		catId := rapid.Int64Range(0, actCatMask).Draw(rt, "catId")
		eventOffset := rapid.Int64Range(0, actEventMask).Draw(rt, "eventOffset")

		ytID := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

		encoded, err := Encode(ytID)
		if err != nil {
			rt.Fatalf("Encode(%q) failed: %v", ytID, err)
		}
		if encoded <= 0 {
			rt.Fatalf("Encode(%q) = %d, want > 0 for activity stream ID", ytID, encoded)
		}

		decoded, ok := Decode(encoded)
		if !ok {
			rt.Fatalf("Decode(%d) returned false for %q", encoded, ytID)
		}
		if decoded != ytID {
			rt.Fatalf("round-trip failed: %q -> %d -> %q", ytID, encoded, decoded)
		}
	})
}

// Feature: activity-id-overflow-fix, Property 4: Negative decode failure
//
// TestPropertyNegativeDecodeFailure validates that for any negative int64 value,
// Decode returns ("", false).
//
// **Validates: Requirements 3.4**
func TestPropertyNegativeDecodeFailure(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.Int64Range(math.MinInt64, -1).Draw(rt, "negativeID")

		decoded, ok := Decode(n)
		if ok {
			rt.Fatalf("Decode(%d) returned ok=true, want false", n)
		}
		if decoded != "" {
			rt.Fatalf("Decode(%d) returned %q, want \"\"", n, decoded)
		}
	})
}

// Feature: activity-id-overflow-fix, Property 3: Invalid input rejection
//
// TestPropertyInvalidInputRejection validates that Encode returns (0, non-nil error)
// for every category of invalid input: malformed strings, non-numeric components,
// field overflows (using Mode A and Mode B boundaries), negative components, and
// malformed activity suffixes.
//
// **Validates: Requirements 2.4, 3.5**
func TestPropertyInvalidInputRejection(t *testing.T) {
	// Table-driven subtests for deterministic invalid-input categories.
	cases := []struct {
		name  string
		input string
	}{
		// 1. no_hyphen — strings without any hyphen
		{"no_hyphen/alpha", "abc"},
		{"no_hyphen/numeric", "123"},
		{"no_hyphen/empty", ""},

		// 2. non_numeric — hyphen present but non-numeric components
		{"non_numeric/both", "a-b"},
		{"non_numeric/second", "1-xyz"},
		{"non_numeric/first", "foo-0"},

		// 3. base entity typeId overflow — TypeId > baseTypeMask (255)
		{"base_typeId_overflow/256", "256-0"},
		{"base_typeId_overflow/1000", "1000-5"},

		// 4. base entity seqId overflow — SeqId > baseSeqMask
		{"base_seqId_overflow/max_plus_1", fmt.Sprintf("0-%d", int64(baseSeqMask)+1)},

		// 5. activity typeId overflow — TypeId > actTypeMask (127)
		{"act_typeId_overflow/128", "128-0.0-0"},
		{"act_typeId_overflow/255", "255-0.0-0"},

		// 6. activity seqId overflow — SeqId > actSeqMask (16,777,215)
		{"act_seqId_overflow/max_plus_1", fmt.Sprintf("0-%d.0-0", int64(actSeqMask)+1)},

		// 7. activity catId overflow — CatId > actCatMask (63)
		{"act_catId_overflow/max_plus_1", fmt.Sprintf("0-0.%d-0", int64(actCatMask)+1)},

		// 8. activity eventOffset overflow — EventOffset > actEventMask (33,554,431)
		{"act_eventOffset_overflow/max_plus_1", fmt.Sprintf("0-0.0-%d", int64(actEventMask)+1)},

		// 9. malformed_activity_suffix — dot present but bad suffix
		{"malformed_activity_suffix/no_hyphen_after_dot", "0-0.9"},
		{"malformed_activity_suffix/trailing_dot", "0-0."},

		// 10. negative_component — negative numbers in the string
		{"negative_component/negative_type", "-1-0"},
		{"negative_component/negative_seq", "0--1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Encode(tc.input)
			if err == nil {
				t.Fatalf("Encode(%q) = (%d, nil), want (0, non-nil error)", tc.input, got)
			}
			if got != 0 {
				t.Fatalf("Encode(%q) = (%d, err), want (0, err)", tc.input, got)
			}
		})
	}

	// Property-based: Base entity TypeId overflow — random values above baseTypeMask.
	t.Run("property/base_typeId_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(int64(baseTypeMask)+1, 9999).Draw(rt, "typeId")
			seqId := rapid.Int64Range(0, baseSeqMask).Draw(rt, "seqId")
			id := fmt.Sprintf("%d-%d", typeId, seqId)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for base typeId overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})

	// Property-based: Base entity SeqId overflow — random values above baseSeqMask.
	t.Run("property/base_seqId_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(0, baseTypeMask).Draw(rt, "typeId")
			seqId := rapid.Int64Range(int64(baseSeqMask)+1, int64(baseSeqMask)+100000).Draw(rt, "seqId")
			id := fmt.Sprintf("%d-%d", typeId, seqId)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for base seqId overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})

	// Property-based: Activity TypeId overflow — random values above actTypeMask (127).
	t.Run("property/act_typeId_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(int64(actTypeMask)+1, 9999).Draw(rt, "typeId")
			seqId := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId")
			catId := rapid.Int64Range(0, actCatMask).Draw(rt, "catId")
			eventOffset := rapid.Int64Range(0, actEventMask).Draw(rt, "eventOffset")
			id := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for activity typeId overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})

	// Property-based: Activity SeqId overflow — random values above actSeqMask (16,777,215).
	t.Run("property/act_seqId_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId")
			seqId := rapid.Int64Range(int64(actSeqMask)+1, int64(actSeqMask)+100000).Draw(rt, "seqId")
			catId := rapid.Int64Range(0, actCatMask).Draw(rt, "catId")
			eventOffset := rapid.Int64Range(0, actEventMask).Draw(rt, "eventOffset")
			id := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for activity seqId overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})

	// Property-based: Activity CatId overflow — random values above actCatMask (63).
	t.Run("property/act_catId_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId")
			seqId := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId")
			catId := rapid.Int64Range(int64(actCatMask)+1, int64(actCatMask)+10000).Draw(rt, "catId")
			eventOffset := rapid.Int64Range(0, actEventMask).Draw(rt, "eventOffset")
			id := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for activity catId overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})

	// Property-based: Activity EventOffset overflow — random values above actEventMask (33,554,431).
	t.Run("property/act_eventOffset_overflow", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			typeId := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId")
			seqId := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId")
			catId := rapid.Int64Range(0, actCatMask).Draw(rt, "catId")
			eventOffset := rapid.Int64Range(int64(actEventMask)+1, int64(actEventMask)+100000).Draw(rt, "eventOffset")
			id := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

			got, err := Encode(id)
			if err == nil {
				rt.Fatalf("Encode(%q) = (%d, nil), want (0, error) for activity eventOffset overflow", id, got)
			}
			if got != 0 {
				rt.Fatalf("Encode(%q) = (%d, err), want (0, err)", id, got)
			}
		})
	})
}

// Feature: activity-id-overflow-fix, Property 1: Bug Condition
// Large EventOffset Activity IDs Fail to Encode
//
// This test targets the bug condition: activity stream IDs with eventOffset > 16,383
// (the current 14-bit maximum). On unfixed code, Encode rejects these IDs.
// On fixed code, Encode succeeds and Decode reproduces the original ID.
//
// **Validates: Requirements 1.1, 1.2, 2.1, 2.2**
func TestPropertyBugCondition_LargeEventOffsetRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		typeId := rapid.Int64Range(0, 127).Draw(rt, "typeId")
		seqId := rapid.Int64Range(0, 16_777_215).Draw(rt, "seqId")
		catId := rapid.Int64Range(0, 63).Draw(rt, "catId")
		eventOffset := rapid.Int64Range(16_384, 33_554_431).Draw(rt, "eventOffset")

		ytID := fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset)

		encoded, err := Encode(ytID)
		if err != nil {
			rt.Fatalf("Encode(%q) failed: %v", ytID, err)
		}
		if encoded <= 0 {
			rt.Fatalf("Encode(%q) = %d, want > 0 for activity stream ID", ytID, encoded)
		}

		decoded, ok := Decode(encoded)
		if !ok {
			rt.Fatalf("Decode(%d) returned false for %q", encoded, ytID)
		}
		if decoded != ytID {
			rt.Fatalf("round-trip failed: %q -> %d -> %q", ytID, encoded, decoded)
		}
	})
}

// Feature: activity-id-overflow-fix, Property 1: Bug Condition (deterministic)
// Production activity IDs that trigger the 14-bit eventOffset overflow.
//
// **Validates: Requirements 1.1, 1.2, 2.1, 2.2**
func TestBugCondition_ProductionEventOffsetValues(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"production_value_1191151", "0-0.9-1191151"},
		{"production_max_3737912", "0-0.9-3737912"},
		{"boundary_16384", "0-0.9-16384"},
		{"new_max_33554431", "0-0.9-33554431"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := Encode(tc.id)
			if err != nil {
				t.Fatalf("Encode(%q) failed: %v", tc.id, err)
			}
			if encoded <= 0 {
				t.Fatalf("Encode(%q) = %d, want > 0", tc.id, encoded)
			}

			decoded, ok := Decode(encoded)
			if !ok {
				t.Fatalf("Decode(%d) returned false for %q", encoded, tc.id)
			}
			if decoded != tc.id {
				t.Fatalf("round-trip failed: %q -> %d -> %q", tc.id, encoded, decoded)
			}
		})
	}
}

// Feature: activity-id-overflow-fix, Property 1: Activity stream no-collisions
//
// TestPropertyActivityNoCollisions validates that different activity stream YouTrack IDs
// never encode to the same numeric value. Uses Mode B ranges.
//
// **Validates: Requirements 2.1, 2.2**
func TestPropertyActivityNoCollisions(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		typeId1 := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId1")
		seqId1 := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId1")
		catId1 := rapid.Int64Range(0, actCatMask).Draw(rt, "catId1")
		event1 := rapid.Int64Range(0, actEventMask).Draw(rt, "event1")
		typeId2 := rapid.Int64Range(0, actTypeMask).Draw(rt, "typeId2")
		seqId2 := rapid.Int64Range(0, actSeqMask).Draw(rt, "seqId2")
		catId2 := rapid.Int64Range(0, actCatMask).Draw(rt, "catId2")
		event2 := rapid.Int64Range(0, actEventMask).Draw(rt, "event2")

		id1 := fmt.Sprintf("%d-%d.%d-%d", typeId1, seqId1, catId1, event1)
		id2 := fmt.Sprintf("%d-%d.%d-%d", typeId2, seqId2, catId2, event2)

		if id1 == id2 {
			return // same input, skip
		}

		enc1, err1 := Encode(id1)
		enc2, err2 := Encode(id2)
		if err1 != nil || err2 != nil {
			return // invalid IDs, skip
		}

		if enc1 == enc2 {
			rt.Fatalf("collision: %q and %q both encode to %d", id1, id2, enc1)
		}
	})
}
