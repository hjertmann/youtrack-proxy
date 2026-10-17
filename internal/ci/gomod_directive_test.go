// Package ci models the go.mod directive-floor contract for the x/sync bump so
// the accepted end state can be reproduced and reasoned about in a runnable test,
// without a live CI toolchain run.
//
// Spec: xsync-bump-go-directive-fix
package ci

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// goModPath is relative to this test file's package dir (internal/ci).
const goModPath = "../../go.mod"

// goDirectiveRe extracts the version body of the `go` directive line in go.mod,
// e.g. "1.26.0" from "go 1.26.0". The directive is at column 0 (a top-level
// go.mod statement), distinguishing it from an indented `go` inside a toolchain
// or godebug block.
var goDirectiveRe = regexp.MustCompile(`(?m)^go\s+(\S+)\s*$`)

// xsyncRequireRe extracts the version of the golang.org/x/sync require line,
// e.g. "v0.23.0" from "\tgolang.org/x/sync v0.23.0". It matches both the
// grouped-require form (indented) and a single-line `require golang.org/x/sync vX`.
var xsyncRequireRe = regexp.MustCompile(`(?m)^\s*(?:require\s+)?golang\.org/x/sync\s+(v\S+)`)

// readGoMod returns the extracted `go` directive version and x/sync require
// version from the repo's go.mod, failing the test if either cannot be found.
func readGoMod(t *testing.T) (goDirective, xsyncVersion string) {
	t.Helper()

	raw, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("reading go.mod %q: %v", goModPath, err)
	}
	content := string(raw)

	dm := goDirectiveRe.FindStringSubmatch(content)
	if dm == nil {
		t.Fatalf("go.mod has no top-level `go` directive; got:\n%s", content)
	}
	xm := xsyncRequireRe.FindStringSubmatch(content)
	if xm == nil {
		t.Fatalf("go.mod has no golang.org/x/sync require line; got:\n%s", content)
	}
	return dm[1], xm[1]
}

// Feature: xsync-bump-go-directive-fix, Property 1: Expected Behavior
//
// TestGoDirectiveAcceptsXsyncFloor encodes the ACCEPTED end state. golang.org/x/sync
// v0.23.0's own go.mod declares `go 1.26.0`, and Go forbids a module's `go` directive
// from being below any dependency's — so `go 1.25.0` + x/sync v0.23.0 cannot build on
// any toolchain. The fix therefore accepts the 1.26.0 floor (rather than trying to pin
// back to 1.25.0) and bumps the CI/build toolchain to Go 1.26. This asserts both: the
// `go` directive is 1.26.0 and x/sync is kept at or above the bumped version (v0.23.0
// at the time of the original fix; the October 2026 dependency upgrade moved it to
// v0.24.0, which declares the same `go 1.26.0` floor).
//
// **Validates: Requirements 2.1, 2.2**
func TestGoDirectiveAcceptsXsyncFloor(t *testing.T) {
	goDirective, xsyncVersion := readGoMod(t)

	// x/sync must be kept at or above the bumped v0.23.0 — the upgrade is kept,
	// not reverted. The floor is a minimum, not an exact pin, so later bumps
	// (e.g. v0.24.0) still satisfy the contract.
	const floor = "v0.23.0"
	if !semverAtLeast(xsyncVersion, floor) {
		t.Fatalf("golang.org/x/sync = %q, want >= %q (the bump must be kept)", xsyncVersion, floor)
	}

	// The `go` directive must be 1.26.0 — the floor x/sync v0.23.0 and v0.24.0
	// both require. A lower directive would be rejected by the toolchain given
	// this dependency.
	if goDirective != "1.26.0" {
		t.Fatalf("go directive = %q, want %q (x/sync %s requires go >= 1.26.0)",
			goDirective, "1.26.0", xsyncVersion)
	}
}

// semverAtLeast reports whether v (e.g. "v0.24.0") is at or above floor
// (e.g. "v0.23.0"), comparing the major/minor/patch components numerically.
// It returns false for versions it cannot parse.
func semverAtLeast(v, floor string) bool {
	parse := func(s string) ([3]int, bool) {
		var parts [3]int
		s = strings.TrimPrefix(s, "v")
		fields := strings.Split(s, ".")
		if len(fields) == 0 || len(fields) > 3 {
			return parts, false
		}
		for i, f := range fields {
			n, err := strconv.Atoi(f)
			if err != nil {
				return parts, false
			}
			parts[i] = n
		}
		return parts, true
	}
	vp, okV := parse(v)
	fp, okF := parse(floor)
	if !okV || !okF {
		return false
	}
	for i := 0; i < 3; i++ {
		if vp[i] != fp[i] {
			return vp[i] > fp[i]
		}
	}
	return true
}

// Feature: xsync-bump-go-directive-fix, Property 2: Preservation
//
// TestPreservation_XsyncBumpKept asserts the observed baseline that the fix must
// preserve: the x/sync require line stays at or above the bumped v0.23.0. Accepting
// the 1.26.0 floor must NOT revert the dependency upgrade (later bumps, such as the
// v0.24.0 from the October 2026 upgrade, are equally preserved).
//
// **Validates: Requirements 3.2**
func TestPreservation_XsyncBumpKept(t *testing.T) {
	_, xsyncVersion := readGoMod(t)
	if !semverAtLeast(xsyncVersion, "v0.23.0") {
		t.Fatalf("golang.org/x/sync = %q, want >= %q (the bump must be preserved, not reverted)", xsyncVersion, "v0.23.0")
	}
}

// Feature: xsync-bump-go-directive-fix, Property 2: Preservation
//
// TestPreservation_SemaphoreImportUnchanged asserts the sole x/sync consumer,
// internal/client/youtrack.go, still imports golang.org/x/sync/semaphore. The fix
// is a go.mod directive + CI toolchain change only — no application code changes
// (Req 3.1).
//
// **Validates: Requirements 3.1**
func TestPreservation_SemaphoreImportUnchanged(t *testing.T) {
	const youtrackPath = "../client/youtrack.go"

	raw, err := os.ReadFile(youtrackPath)
	if err != nil {
		t.Fatalf("reading %q: %v", youtrackPath, err)
	}
	if !regexp.MustCompile(`(?m)^\s*"golang\.org/x/sync/semaphore"`).Match(raw) {
		t.Fatalf("%s must still import golang.org/x/sync/semaphore (code unchanged); got:\n%s", youtrackPath, raw)
	}
}
