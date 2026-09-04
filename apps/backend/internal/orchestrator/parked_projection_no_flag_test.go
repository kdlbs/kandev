package orchestrator

import (
	"os"
	"strings"
	"testing"
)

// TestParkedProjectionDoesNotReferenceBackgroundPromptHandoffFlag is the
// round-5 F2 disposition for AC-35's architecture-test clause. Package-level
// import-direction is unsatisfiable here: the parked projection necessarily
// lives in internal/orchestrator, the same package that defines
// claudeBackgroundPromptHandoffEnabled. This narrows the assertion to the
// specific files this feature adds, checked by source text rather than by
// package graph.
//
// New files task-06 adds to the parked/task-level projection must be
// appended to parkedProjectionSourceFiles below rather than exempted.
func TestParkedProjectionDoesNotReferenceBackgroundPromptHandoffFlag(t *testing.T) {
	forbidden := []string{"claudeBackgroundPromptHandoffEnabled", "claudeBackgroundPromptHandoff"}

	for _, path := range parkedProjectionSourceFiles {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				t.Errorf("%s references %q; the parked projection must not read the "+
					"claudeBackgroundPromptHandoff flag or its accessor (AC-35)", path, needle)
			}
		}
	}
}

// parkedProjectionSourceFiles lists every non-test file this task (task-05)
// and task-06 add for the parked/task-level projection. Listed explicitly —
// do not glob the package — per F2's disposition in
// docs/plans/disambiguate-waiting/plan.md.
var parkedProjectionSourceFiles = []string{
	"parked_projection.go",
}
