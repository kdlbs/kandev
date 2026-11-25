package lifecycle

import (
	"context"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPiThinkingResetAppliesOneCanonicalValue(t *testing.T) {
	mgr := newTestManager(t)
	mock := newRestartMockAgentctlServer(t, false, false)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))
	execution := runtimeConfigResetExecution(client, true)
	execution.AgentID = "pi-acp"
	execution.SetModelState(&CachedModelState{ConfigOptions: []streams.ConfigOption{{ID: "thought_level", Category: "thought_level", Type: "select", Options: []streams.ConfigOptionValue{{Value: "off"}}, CurrentValue: "off"}}})
	err := mgr.restoreSessionRuntimeConfig(ctx, execution, "reset-session", models.SessionRuntimeConfig{Mode: "medium", ConfigOptions: map[string]string{"thought_level": "off"}})
	require.NoError(t, err)
	require.Equal(t, []string{"off"}, mock.getSetModeIDs())
	require.Empty(t, mock.getSetOptions())
}
