package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestRejectedMCPStartupWarningDoesNotCancelClarificationWatchdog(t *testing.T) {
	const sessionID = "session-mcp-startup-cancellation-owner"
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	operation, owner := svc.claimCancellation(sessionID, cancellationKindSilent)
	require.True(t, owner)
	svc.setCancellationIdentity(sessionID, operation, cancellationIdentity{
		executionID: "recovery-execution", promptGeneration: 1,
	})
	watchCtx, cancelWatchdog := context.WithCancel(context.Background())
	defer cancelWatchdog()
	entry := &clarificationWatchdogEntry{cancel: cancelWatchdog}
	entry.beginRecoveryCancellation()
	key := svc.clarificationWatchdogKey(sessionID, "pending-mcp-startup")
	svc.clarificationWatchdogs.Store(key, entry)
	t.Cleanup(func() {
		svc.cancelAllClarificationWatchdogs()
		svc.finishCancellation(sessionID, operation, nil)
	})

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: "task-mcp-startup-cancellation-owner", SessionID: sessionID,
		ExecutionID: "different-execution",
		Data:        &lifecycle.AgentStreamEventData{Type: streams.EventTypeMCPStartupWarning},
	})

	select {
	case <-watchCtx.Done():
		t.Fatal("rejected startup warning cancelled clarification recovery watchdog")
	default:
	}
	stored, ok := svc.clarificationWatchdogs.Load(key)
	require.True(t, ok, "rejected startup warning removed the live recovery watchdog")
	require.Same(t, entry, stored)
}
