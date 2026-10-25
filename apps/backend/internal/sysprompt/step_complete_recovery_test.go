package sysprompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompletionInstructionsExplainStaleTurnRecovery covers AC-003.4: both
// signal-gated prompt surfaces preserve final-action and question barriers,
// distinguish stale rejection from already_signaled, and avoid move authority.
func TestCompletionInstructionsExplainStaleTurnRecovery(t *testing.T) {
	cases := map[string]string{
		"task":   FormatKandevContext("task-recovery", "session-recovery", true),
		"office": FormatOfficeContextWithOptions("task-recovery", "session-recovery", true),
	}
	for name, context := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "task" {
				assert.Contains(t, context, "Call it as the LAST action")
			} else {
				assert.Contains(t, context, "Call step_complete_kandev as the LAST action")
			}
			assert.Contains(t, context, "before a question")
			assert.Contains(t, context, "workflow step changed")
			assert.Contains(t, context, "retries in this turn cannot recover")
			assert.Contains(t, context, "End the turn and have the user resume the session")
			assert.Contains(t, context, "already_signaled")
			assert.Contains(t, context, "Do not move the task solely to bypass a stale-turn error")
		})
	}
}

func TestCompletionInstructionsOmitRecoveryGuidanceWhenUngated(t *testing.T) {
	assert.NotContains(t, FormatKandevContext("task", "session", false), "workflow step changed")
	assert.NotContains(t, FormatOfficeContextWithOptions("task", "session", false), "workflow step changed")
}
