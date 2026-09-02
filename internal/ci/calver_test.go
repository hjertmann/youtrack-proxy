// Package ci models the CI workflow's CalVer version-computation contract so the
// tag-collision TOCTOU race can be reproduced and reasoned about in a runnable test,
// without a live GitHub Actions run.
//
// Spec: ci-calver-tag-collision-fix
package ci

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// calVerRe matches the CalVer version body YYYY.MM.DD.NN with zero-padded fields,
// mirroring the format the "Compute CalVer version" step produces (the leading "v"
// prefix is added by the tag/release step, not by nextVersion).
var calVerRe = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}\.\d{2}$`)

// zeroPad2 mirrors the shell `printf '%02d'` used by the "Compute CalVer version"
// step in .github/workflows/build-and-push.yml.
func zeroPad2(n int) string {
	return fmt.Sprintf("%02d", n)
}

// nextVersion models the CalVer shell contract exactly:
//
//	date="$(date -u +'%Y.%m.%d')"
//	count="$(git tag -l "v${date}.*" | wc -l)"
//	seq="$(printf '%02d' "$((count + 1))")"
//	version="${date}.${seq}"
//
// tagCount is the number of existing tags for `date` observed at the moment the
// run reads it. The bug is that this read and the later tag creation are not atomic
// across concurrent runs.
func nextVersion(date string, tagCount int) string {
	return date + "." + zeroPad2(tagCount+1)
}

// Feature: ci-calver-tag-collision-fix, Property 1: Bug Condition
//
// TestBugCondition_OverlappingReadsCollide demonstrates the TOCTOU collision on the
// UNFIXED (no concurrency: block) model. When two same-day default-branch runs both
// read the tag count BEFORE either creates its tag, they observe the same count and
// compute the SAME CalVer version. The second `gh release create` then fails with
// HTTP 422 Release.tag_name already exists.
//
// This test asserts the two computed tags are EQUAL — the equality IS the bug. A
// varying number of racing runs (all reading the same pre-creation count) and a
// varying date are explored with rapid to confirm the collision is not date-specific.
//
// EXPECTED OUTCOME: passes, documenting the collision (all racing runs collide on the
// same tag). This is the SUCCESS case for a bug-condition exploration test.
//
// **Validates: Requirements 1.1, 1.2, 1.3**
func TestBugCondition_OverlappingReadsCollide(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		date := drawDate(rt)
		// Number of runs racing on the same pre-creation count. >= 2 is required
		// for a collision to be possible.
		racers := rapid.IntRange(2, 8).Draw(rt, "racers")
		// The tag count all racers read before ANY of them creates a tag. Under the
		// bug, every racer reads this same value (overlapping reads).
		sharedCount := rapid.IntRange(0, 50).Draw(rt, "sharedCount")

		tags := make([]string, racers)
		for i := range tags {
			// Overlapping reads: every run reads the SAME count, so every run
			// computes the SAME NN.
			tags[i] = nextVersion(date, sharedCount)
		}

		// The bug: all racers collide on one tag. Assert equality to document it.
		first := tags[0]
		for i := 1; i < racers; i++ {
			if tags[i] != first {
				rt.Fatalf("expected overlapping-reads collision: run %d computed %q, run 0 computed %q (date=%q, sharedCount=%d)",
					i, tags[i], first, date, sharedCount)
			}
		}
	})
}

// Feature: ci-calver-tag-collision-fix, Property 1: Bug Condition (deterministic anchor)
//
// TestBugCondition_TwoRunsReadZero pins the concrete production scenario from the
// bugfix report: two same-day runs both read count = 0 before either creates a tag,
// so both compute v<date>.01 and the second release fails HTTP 422.
//
// **Validates: Requirements 1.1, 1.2, 1.3**
func TestBugCondition_TwoRunsReadZero(t *testing.T) {
	date := "2026.09.02"

	// Both runs read count = 0 (overlap: neither has created its tag yet).
	run1 := nextVersion(date, 0)
	run2 := nextVersion(date, 0)

	if run1 != "2026.09.02.01" {
		t.Fatalf("run1 = %q, want %q", run1, "2026.09.02.01")
	}
	// The collision: both runs compute the identical tag. `gh release create` for
	// run2 would fail with HTTP 422 Release.tag_name already exists.
	if run1 != run2 {
		t.Fatalf("expected collision: run1=%q run2=%q, want them EQUAL (the bug)", run1, run2)
	}
}

// Feature: ci-calver-tag-collision-fix, Property 1: Fix model (serialized reads)
//
// TestFixModel_SerializedReadsAreUnique demonstrates the fix behavior: once runs are
// serialized (by the concurrency: block added in task 3), run 2 reads the count only
// AFTER run 1's tag exists, so it reads count = 1 and computes v<date>.02 — distinct
// from run 1's v<date>.01. No task 3 change is needed for this assertion; it validates
// that the version-computation contract itself yields unique tags under serialized reads.
//
// **Validates: Requirements 1.1**
func TestFixModel_SerializedReadsAreUnique(t *testing.T) {
	date := "2026.09.02"

	run1 := nextVersion(date, 0) // no tags yet
	run2 := nextVersion(date, 1) // run 1's tag now exists

	if run1 != "2026.09.02.01" {
		t.Fatalf("run1 = %q, want %q", run1, "2026.09.02.01")
	}
	if run2 != "2026.09.02.02" {
		t.Fatalf("run2 = %q, want %q", run2, "2026.09.02.02")
	}
	if run1 == run2 {
		t.Fatalf("serialized reads must be unique: run1=%q run2=%q", run1, run2)
	}
}

// drawDate generates a valid YYYY.MM.DD date string to vary the collision across dates.
func drawDate(rt *rapid.T) string {
	year := rapid.IntRange(2024, 2099).Draw(rt, "year")
	month := rapid.IntRange(1, 12).Draw(rt, "month")
	day := rapid.IntRange(1, 28).Draw(rt, "day") // 28 keeps every month valid
	return fmt.Sprintf("%04d.%02d.%02d", year, month, day)
}

// Feature: ci-calver-tag-collision-fix, Property 2: Preservation
//
// TestPreservation_SerializedRunsAndCalVerFormat asserts the version-computation
// contract over the NON-bug input space (serialized / lone runs, NOT isBugCondition).
// The concurrency: block added by task 3 only changes run scheduling, never the
// version-computation logic, so this contract must hold IDENTICALLY before and after
// the fix — that is what makes it a preservation check.
//
// Two properties are checked together:
//
//  1. Serialized same-day runs (1..N): run i reads tagCount = i-1 (the prior run's tag
//     exists once it has been created), so the produced tags are exactly .01 .. .0N with
//     no duplicates. A lone run (N=1) reads count 0 and yields .01.
//  2. CalVer format: for random valid dates and counts, nextVersion always matches
//     ^\d{4}\.\d{2}\.\d{2}\.\d{2}$ (YYYY.MM.DD.NN, zero-padded).
//
// EXPECTED OUTCOME: passes on the unfixed model (confirms the contract to preserve).
//
// **Validates: Requirements 3.1, 3.2**
func TestPreservation_SerializedRunsAndCalVerFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		date := drawDate(rt)

		// Serialized same-day runs: run i (1-based) reads the count AFTER the prior
		// run's tag exists, i.e. tagCount = i-1. This is the NOT-isBugCondition case.
		// n up to 99 keeps NN within the two-digit zero-padded contract.
		n := rapid.IntRange(1, 99).Draw(rt, "n")

		tags := make([]string, n)
		seen := make(map[string]bool, n)
		for i := 1; i <= n; i++ {
			tag := nextVersion(date, i-1)

			// Property 1: run i produces exactly .0i (zero-padded), and .01 for a lone run.
			want := date + "." + zeroPad2(i)
			if tag != want {
				rt.Fatalf("serialized run %d: got %q, want %q (date=%q, n=%d)", i, tag, want, date, n)
			}

			// No duplicates across the serialized sequence.
			if seen[tag] {
				rt.Fatalf("duplicate tag %q at serialized run %d (date=%q, n=%d)", tag, i, date, n)
			}
			seen[tag] = true

			// Property 2: every produced version matches the CalVer format.
			if !calVerRe.MatchString(tag) {
				rt.Fatalf("tag %q does not match CalVer format YYYY.MM.DD.NN (date=%q, run=%d)", tag, date, i)
			}

			tags[i-1] = tag
		}
	})
}

// Feature: ci-calver-tag-collision-fix, Property 1: Expected Behavior (the actual fix)
//
// TestWorkflowHasSerializingConcurrencyBlock is the primary fix check: it statically
// asserts the real workflow YAML carries a top-level concurrency: block configured to
// serialize-and-queue default-branch runs. That block is the fix — it turns the
// overlapping "read count / create tag" windows into non-overlapping ones, so each
// queued run reads a fresh tag count after the prior run's tag exists and computes a
// distinct NN.
//
// Requirements:
//   - group: calver-release-${{ github.ref }} — default-branch pushes share one ref, so
//     they land in one group and serialize; PR / other-branch runs have different refs
//     and are unaffected.
//   - cancel-in-progress: false — queued runs must still execute (no dropped releases).
//
// ponytail: raw-line assertion rather than a YAML parser — no yaml dependency is present
// in go.mod and adding one for two lines isn't worth it. Upgrade path: if this grows to
// assert nested/structural workflow properties, parse with gopkg.in/yaml.v3.
//
// **Validates: Requirements 2.1, 2.2, 2.3**
func TestWorkflowHasSerializingConcurrencyBlock(t *testing.T) {
	// Path is relative to this test file's package dir (internal/ci).
	const workflowPath = "../../.github/workflows/build-and-push.yml"

	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("reading workflow %q: %v", workflowPath, err)
	}
	content := string(raw)

	// A top-level concurrency: block must exist. At column 0 it is a sibling of
	// on:/env:/jobs: (as opposed to a job-scoped, indented concurrency:).
	if !regexp.MustCompile(`(?m)^concurrency:`).MatchString(content) {
		t.Fatalf("workflow is missing a top-level concurrency: block; got:\n%s", content)
	}

	// group keys on github.ref so default-branch pushes serialize against each other.
	wantGroup := "group: calver-release-${{ github.ref }}"
	if !strings.Contains(content, wantGroup) {
		t.Fatalf("workflow concurrency group not found; want a line containing %q", wantGroup)
	}

	// cancel-in-progress: false so queued release runs are not dropped.
	if !regexp.MustCompile(`(?m)^\s*cancel-in-progress:\s*false\s*$`).MatchString(content) {
		t.Fatalf("workflow must set 'cancel-in-progress: false' so queued runs are not cancelled")
	}
}
