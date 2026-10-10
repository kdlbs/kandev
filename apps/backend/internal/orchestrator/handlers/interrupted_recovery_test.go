package handlers

import (
	"context"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInterruptedRecoverRequiresAcknowledgedIdentityBeforeServiceAccess(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	response, err := handlers.wsRecoverSession(context.Background(), createTestMessage(t, ws.ActionSessionRecover, map[string]any{"task_id": "task", "session_id": "session", "action": "resume_interrupted"}))
	require.NoError(t, err)
	require.Equal(t, "interrupted_resume is required", parseError(t, response).Message)
	response, err = handlers.wsRecoverSession(context.Background(), createTestMessage(t, ws.ActionSessionRecover, map[string]any{"task_id": "task", "session_id": "session", "action": "resume", "interrupted_resume": map[string]any{"acknowledge_interruption": false}}))
	require.NoError(t, err)
	require.Equal(t, "acknowledgment, instruction, identity, revision, and idempotency key are required", parseError(t, response).Message)
}

func TestInterruptedRecoveryRejectsProviderRestoredPolicy(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	response, err := handlers.wsRecoverSession(context.Background(), createTestMessage(t, ws.ActionSessionRecover, map[string]any{
		"task_id": "task", "session_id": "session", "action": "resume", "settings_policy": "provider_restored", "interrupted_resume": map[string]any{},
	}))
	require.NoError(t, err)
	require.Equal(t, "interrupted recovery requires strict settings policy", parseError(t, response).Message)
}
