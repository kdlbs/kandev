package routingerr

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestClaudeRefreshContentionIsTransient(t *testing.T) {
	err := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: "Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh"})
	require.Equal(t, DecisionShortRetry, Decide(ContextKanban, err, time.Now()))
}
