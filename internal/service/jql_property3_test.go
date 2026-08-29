package service

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty3_SlashDateNormalizationRoundTrip validates Property 3:
// Slash date normalization round-trip.
//
// For any valid date string in YYYY/MM/DD HH:mm format, parsing the JQL
// with slash separators and converting to YouTrack query SHALL produce the
// same output as parsing the equivalent dash-separated YYYY-MM-DD HH:mm date.
//
// **Validates: Requirements 2.1, 2.2, 2.3, 2.4**
func TestProperty3_SlashDateNormalizationRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a valid datetime.
		year := rapid.IntRange(2020, 2030).Draw(rt, "year")
		month := rapid.IntRange(1, 12).Draw(rt, "month")
		day := rapid.IntRange(1, 28).Draw(rt, "day")
		hour := rapid.IntRange(0, 23).Draw(rt, "hour")
		min := rapid.IntRange(0, 59).Draw(rt, "minute")

		dashDate := fmt.Sprintf("%04d-%02d-%02d %02d:%02d", year, month, day, hour, min)
		slashDate := fmt.Sprintf("%04d/%02d/%02d %02d:%02d", year, month, day, hour, min)

		// Test with "updated >=" clause.
		updatedDash := fmt.Sprintf("updated >= '%s'", dashDate)
		updatedSlash := fmt.Sprintf("updated >= '%s'", slashDate)

		parsedDash, err := ParseJQL(updatedDash)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) error: %v", updatedDash, err)
		}
		parsedSlash, err := ParseJQL(updatedSlash)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) error: %v", updatedSlash, err)
		}

		if parsedDash.UpdatedGE != parsedSlash.UpdatedGE {
			rt.Fatalf("UpdatedGE mismatch: dash=%q slash=%q", parsedDash.UpdatedGE, parsedSlash.UpdatedGE)
		}
		if parsedDash.UpdatedGE != dashDate {
			rt.Fatalf("UpdatedGE = %q, want %q", parsedDash.UpdatedGE, dashDate)
		}

		ytDash := parsedDash.ToYouTrackQuery()
		ytSlash := parsedSlash.ToYouTrackQuery()
		if ytDash != ytSlash {
			rt.Fatalf("ToYouTrackQuery mismatch:\n  dash:  %q\n  slash: %q", ytDash, ytSlash)
		}

		// Test with "created >=" clause.
		createdDash := fmt.Sprintf("created >= '%s'", dashDate)
		createdSlash := fmt.Sprintf("created >= '%s'", slashDate)

		parsedCDash, err := ParseJQL(createdDash)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) error: %v", createdDash, err)
		}
		parsedCSlash, err := ParseJQL(createdSlash)
		if err != nil {
			rt.Fatalf("ParseJQL(%q) error: %v", createdSlash, err)
		}

		if parsedCDash.CreatedGE != parsedCSlash.CreatedGE {
			rt.Fatalf("CreatedGE mismatch: dash=%q slash=%q", parsedCDash.CreatedGE, parsedCSlash.CreatedGE)
		}
		if parsedCDash.CreatedGE != dashDate {
			rt.Fatalf("CreatedGE = %q, want %q", parsedCDash.CreatedGE, dashDate)
		}

		ytCDash := parsedCDash.ToYouTrackQuery()
		ytCSlash := parsedCSlash.ToYouTrackQuery()
		if ytCDash != ytCSlash {
			rt.Fatalf("ToYouTrackQuery (created) mismatch:\n  dash:  %q\n  slash: %q", ytCDash, ytCSlash)
		}
	})
}
