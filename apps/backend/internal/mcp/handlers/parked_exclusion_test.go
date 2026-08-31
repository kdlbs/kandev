package handlers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mcpClarificationGuardedSourceFiles are the files that implement the MCP
// clarification settle path AC-40b excludes: Handlers.setSessionWaitingForInput
// and its callees (updateSessionWaitingForClarification,
// updateClarificationSessionState), which write session state via the raw
// repository interface and never enter package orchestrator's
// updateTaskSessionStateWithHook — the one seam the parked-on-background-work
// settle hook hangs off (docs/specs/parked-board-mvp/spec.md §7.2). Both of
// spec §3's named callers of setSessionWaitingForInput are listed
// (handlers.go:3296 and parent_question.go:158) — Review round 2 should-fix:
// the guard originally scanned only handlers.go, leaving the second caller
// unchecked.
var mcpClarificationGuardedSourceFiles = []string{
	"handlers.go",
	"parent_question.go",
}

// TestMCPClarificationPathNeverReferencesParkedProjection is AC-40b's
// structural guard, mirroring the pattern
// notifications/service/parked_independence_test.go already uses for AC-76:
// the MCP clarification settle path is a named exclusion from the parked
// projection ("asserted so it is a decision rather than an omission" — spec's
// own words), so this asserts at the source level that nothing in it
// references the parked-projection machinery. handlers.go does legitimately
// import package orchestrator for unrelated types (LaunchSessionRequest,
// TitleBranchRenameResult, …), so the guard is scoped to the word "parked"
// rather than the import line.
func TestMCPClarificationPathNeverReferencesParkedProjection(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(sourceFile)

	var violations []string
	for _, rel := range mcpClarificationGuardedSourceFiles {
		path := filepath.Join(dir, rel)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "read %s", rel)
		if strings.Contains(strings.ToLower(string(data)), "parked") {
			violations = append(violations, rel+" references \"parked\"")
		}
	}
	require.Empty(t, violations, "MCP clarification settle path must stay untouched by the parked-projection slice (AC-40b):\n%s", strings.Join(violations, "\n"))
}
