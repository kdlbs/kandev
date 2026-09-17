package orchestrator

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantCompletionTracksEveryBackgroundWorkKind(t *testing.T) {
	s := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	require.False(t, s.HasOutstandingSessionWork("session"))
	for _, kind := range []streams.BackgroundWorkKind{streams.BackgroundWorkKindShell, streams.BackgroundWorkKindMonitor, streams.BackgroundWorkKindSubagent} {
		s.registerBackgroundWorkKind("session", "tool", "execution", "work", kind)
		require.True(t, s.HasOutstandingSessionWork("session"))
		s.completeBackgroundTaskForExecution("session", "tool", "execution")
		require.False(t, s.HasOutstandingSessionWork("session"))
	}
}
