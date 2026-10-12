package sysprompt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PLAN-READ-003.2 AC-TASKS-PLAN-READ-003.3
func TestPlanFragmentGuidance(t *testing.T) {
	for name, prompt := range map[string]string{
		"task":             FormatKandevContext("task-range", "session-range", true),
		"office":           FormatOfficeContextWithOptions("task-range", "session-range", true),
		"plan mode":        PlanMode(),
		"default planning": DefaultPlanPrefix(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, instruction := range []string{
				"get_task_plan_kandev", "offset", "limit", "Unicode", "expected_version",
				"first page's version as expected_version",
				"edit_task_plan_kandev", `mode="append"`, "full", "replacement",
			} {
				require.Contains(t, prompt, instruction)
			}
		})
	}
	require.Contains(t, PlanMode(), "After saving, STOP and wait for the user to review.")
	require.Contains(t, DefaultPlanPrefix(), "Do not create any other files during this phase")
	require.NotContains(t, ConfigContext(), "get_task_plan_kandev")
}
