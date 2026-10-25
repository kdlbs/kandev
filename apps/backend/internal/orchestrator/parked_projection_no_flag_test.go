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

// parkedProjectionSourceFiles lists every non-test file this feature touches
// on the parked-projection/probe path. Listed explicitly — do not glob the
// package — per F2's disposition in docs/plans/disambiguate-waiting/plan.md.
// A new file this feature adds on this path must be appended here rather
// than exempted (round-3 code review found this list incomplete twice).
var parkedProjectionSourceFiles = []string{
	"parked_projection.go",
	"background_probe.go",
	"background_probe_config.go",
	"background_work_attestation.go",
	"event_handlers_streaming.go",
	"service.go",
	"executor/executor.go",
	"../task/dto/dto.go",
	"../task/dto/parked_projection.go",
	"../task/service/service.go",
	"../task/service/service_events.go",
	"../task/service/parked_projection.go",
	"../task/handlers/message_handlers.go",
	"../task/handlers/task_handlers.go",
	"../task/handlers/task_http_handlers.go",
	"../task/handlers/task_ws_handlers.go",
	"../task/handlers/workflow_handlers.go",
	"../agentctl/server/process/probe/probe.go",
	"../agentctl/server/process/probe/processtable_darwin.go",
	"../agentctl/server/process/probe/processtable_linux.go",
	"../agentctl/server/process/probe/processtable_other.go",
	"../agent/runtime/agentctl/background_probe.go",
	"../agentctl/server/api/agent.go",
}
