package sysprompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompletionInstructionsDistinguishAcceptanceFromCommit covers the
// source-specific outcome contract: acceptance is not transition proof, and a
// later turn in the same step can replace an unconsumed signal.
func TestCompletionInstructionsDistinguishAcceptanceFromCommit(t *testing.T) {
	cases := map[string]string{
		"task":   FormatKandevContext("task-outcome", "session-outcome", true),
		"office": FormatOfficeContextWithOptions("task-outcome", "session-outcome", true),
	}
	for name, context := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, context, "already_signaled returns the retained outcome")
			assert.Contains(t, context, "pending means only that the signal was recorded")
			assert.Contains(t, context, "not that a transition committed")
			assert.Contains(t, context, "reviewer received the handoff")
			assert.Contains(t, context, "A later turn in the same step can replace an unconsumed signal")
		})
	}
}

func TestCompletionOutcomeGuidanceOmittedWhenUngated(t *testing.T) {
	assert.NotContains(t, FormatKandevContext("task", "session", false), "pending means only that the signal was recorded")
	assert.NotContains(t, FormatOfficeContextWithOptions("task", "session", false), "pending means only that the signal was recorded")
}
