package sqlite_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWakeupRequestsJSONPatchAppearsOnce is a source-level pinning
// assertion for the coalesce-merge unification
// (docs/specs/office/system-design/routine-catch-up-01.md, "Delivering the
// gap to the agent"): wakeup_requests.go must contain exactly one SQL
// occurrence of json_patch, meaning exactly one place a coalescing wakeup
// request's payload is merged into a run's context_snapshot. Two SQL sites
// would mean only one of them carries the AC-OFFICE-ROUTINE-CATCHUP-002.10
// gap-key strip (json_remove of missed_ticks/missed_since/missed_truncated),
// silently reintroducing the bug this design fixes.
//
// The naive `grep -c json_patch` on this file returns THREE lines: the two
// SQL string literals this test counts, plus the helper's own doc comment,
// which names the function in prose. Parsing with go/parser and counting
// only *ast.BasicLit string literals excludes the comment by construction —
// an unqualified "exactly once" assertion would FAIL against the correct
// implementation.
func TestWakeupRequestsJSONPatchAppearsOnce(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	targetPath := filepath.Join(filepath.Dir(sourceFile), "wakeup_requests.go")

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, targetPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", targetPath, err)
	}

	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if containsJSONPatch(lit.Value) {
			count++
		}
		return true
	})
	if count != 1 {
		t.Fatalf("json_patch appears in %d SQL string literal(s) in wakeup_requests.go, want exactly 1 "+
			"(the single mergeWakeupPayloadIntoRunSnapshotWith call site)", count)
	}
}

func containsJSONPatch(s string) bool {
	const needle = "json_patch"
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
