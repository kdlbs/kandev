package process

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestProviderReplacementCannotEmitTurnCompletionOrFailure(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, logger.Default())
	defer func() { _ = manager.StopForTeardown(context.Background()) }()
	require.NoError(t, manager.ReserveProviderReplacement("operation", time.Now().Add(time.Minute)))
	for _, kind := range []string{adapter.EventTypeComplete, adapter.EventTypeError} {
		event, err := manager.persistDeliveryEventContext(context.Background(), adapter.AgentEvent{Type: kind})
		require.NoError(t, err)
		require.Empty(t, event.Type, "a provider refresh cannot complete or fail a task turn")
	}
	manager.ReleaseProviderReplacement("operation")
	event, err := manager.persistDeliveryEventContext(context.Background(), adapter.AgentEvent{Type: adapter.EventTypeError})
	require.NoError(t, err)
	require.Equal(t, adapter.EventTypeError, event.Type, "ordinary process failures retain their existing semantics")
}
