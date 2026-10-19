package sqlite

import (
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
)

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.3
func TestWorkspaceMaterializationBudgetPreservesConfiguredBounds(t *testing.T) {
	previousSetup, previousLaunch := constants.SetupScriptTimeout, constants.AgentLaunchTimeout
	t.Cleanup(func() {
		constants.SetupScriptTimeout, constants.AgentLaunchTimeout = previousSetup, previousLaunch
	})
	for _, tt := range []struct {
		name  string
		setup time.Duration
		want  time.Duration
	}{
		{name: "default", setup: 10 * time.Minute, want: 30 * time.Minute},
		{name: "large valid setting", setup: time.Duration(math.MaxInt64) / 2, want: time.Duration(math.MaxInt64)},
		{name: "maximum valid setting", setup: time.Duration(math.MaxInt64) - 5*time.Minute, want: time.Duration(math.MaxInt64)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			constants.ApplyPreparationTimeout(tt.setup)
			if got := workspaceMaterializationAbandonAfter(); got != tt.want {
				t.Fatalf("configured setup %s produced abandonment budget %s, want %s", tt.setup, got, tt.want)
			}
		})
	}
}
