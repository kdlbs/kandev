package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsyncQuestionGuidanceOffDuringPromptReplay(t *testing.T) {
	service := &Service{}
	stored := startCreatedSessionOptions{
		asyncQuestionGuidanceResolved: true,
		asyncQuestionsSupported:       true,
	}

	off := service.gateAsyncQuestionGuidance(stored)
	require.True(t, off.asyncQuestionGuidanceResolved)
	require.False(t, off.asyncQuestionsSupported, "a persisted true value cannot enable async guidance while the flag is off")

	service.SetAsyncQuestionsEnabled(true)
	on := service.gateAsyncQuestionGuidance(stored)
	require.True(t, on.asyncQuestionsSupported, "enabled startup preserves the persisted capability decision")
}
