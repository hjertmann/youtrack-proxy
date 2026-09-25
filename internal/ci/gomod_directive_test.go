// Package ci models the go.mod directive-floor contract for the x/sync bump so
// the accepted end state can be reproduced and reasoned about in a runnable test,
// without a live CI toolchain run.
//
// Spec: xsync-bump-go-directive-fix
package ci

import (
	"os"
	"regexp"
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
// `go` directive is 1.26.0 and x/sync is kept at v0.23.0.
//
// **Validates: Requirements 2.1, 2.2**
func TestGoDirectiveAcceptsXsyncFloor(t *testing.T) {
	goDirective, xsyncVersion := readGoMod(t)

	// x/sync must be the bumped v0.23.0 — the upgrade is kept, not reverted.
	if xsyncVersion != "v0.23.0" {
		t.Fatalf("golang.org/x/sync = %q, want %q (the bump must be kept)", xsyncVersion, "v0.23.0")
	}

	// The `go` directive must be 1.26.0 — the floor x/sync v0.23.0 requires. A lower
	// directive would be rejected by the toolchain given this dependency.
	if goDirective != "1.26.0" {
		t.Fatalf("go directive = %q, want %q (x/sync %s requires go >= 1.26.0)",
			goDirective, "1.26.0", xsyncVersion)
	}
}

// Feature: xsync-bump-go-directive-fix, Property 2: Preservation
//
// TestPreservation_XsyncBumpKept asserts the observed baseline that the fix must
// preserve: the x/sync require line is v0.23.0. Accepting the 1.26.0 floor must NOT
// revert the dependency upgrade.
//
// **Validates: Requirements 3.2**
func TestPreservation_XsyncBumpKept(t *testing.T) {
	_, xsyncVersion := readGoMod(t)
	if xsyncVersion != "v0.23.0" {
		t.Fatalf("golang.org/x/sync = %q, want %q (the bump must be preserved, not reverted)", xsyncVersion, "v0.23.0")
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
