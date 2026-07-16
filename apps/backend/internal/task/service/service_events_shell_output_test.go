package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPublishMessageEventProjectsShellOutput(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	eventBus := NewMockEventBus()
	svc := NewService(Repos{}, eventBus, log, RepositoryDiscoveryConfig{})
	normalized := streams.NewShellExec("make test", "/workspace", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "live-output-sentinel", Stderr: "live-error"}
	message := &models.Message{ID: "message-1", TaskSessionID: "session-1", Metadata: map[string]any{
		"status":     "running",
		"normalized": normalized,
	}}

	svc.publishMessageEvent(context.Background(), events.MessageUpdated, message)

	require.Len(t, eventBus.GetPublishedEvents(), 1)
	raw, err := json.Marshal(eventBus.GetPublishedEvents()[0].Data)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "live-output-sentinel")
	require.NotContains(t, string(raw), "live-error")
	require.Contains(t, string(raw), `"stdout_bytes":20`)
	require.Equal(t, "live-output-sentinel", normalized.ShellExec().Output.Stdout)
}

func TestPublishMessageEventProjectsPersistedShellOutput(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	eventBus := NewMockEventBus()
	svc := NewService(Repos{}, eventBus, log, RepositoryDiscoveryConfig{})
	output := map[string]any{"stdout": "persisted-event-sentinel"}
	message := &models.Message{ID: "message-1", TaskSessionID: "session-1", Metadata: map[string]any{
		"status": "running",
		"normalized": map[string]any{
			"kind":       "shell_exec",
			"shell_exec": map[string]any{"command": "make test", "output": output},
		},
	}}

	svc.publishMessageEvent(context.Background(), events.MessageAdded, message)

	require.Len(t, eventBus.GetPublishedEvents(), 1)
	raw, err := json.Marshal(eventBus.GetPublishedEvents()[0].Data)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "persisted-event-sentinel")
	require.Contains(t, string(raw), `"stdout_bytes":24`)
	require.Equal(t, "persisted-event-sentinel", output["stdout"])
}
