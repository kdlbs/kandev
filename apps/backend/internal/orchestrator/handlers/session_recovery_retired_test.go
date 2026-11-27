package handlers

import (
	"context"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestSessionRecoveryRejectsRetiredInstructionAndBatchActions(t *testing.T) {
	handlers := setupOrchestratorHandlers(t)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	for _, test := range []struct {
		name    string
		action  string
		payload map[string]any
	}{
		{
			name:   "legacy action",
			action: ws.ActionSessionRecover,
			payload: map[string]any{
				"task_id": "task", "session_id": "session", "action": "resume_interrupted",
			},
		},
		{
			name:   "legacy payload on generic resume",
			action: ws.ActionSessionRecover,
			payload: map[string]any{
				"task_id": "task", "session_id": "session", "action": "resume",
				"interrupted_resume": map[string]any{"instruction": "old client instruction"},
			},
		},
		{
			name:   "legacy batch action",
			action: ws.ActionSessionRecoverBatch,
			payload: map[string]any{
				"items": []any{map[string]any{"task_id": "task", "session_id": "session"}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := dispatcher.Dispatch(context.Background(), createTestMessage(t, test.action, test.payload))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Contains(t, parseError(t, response).Message, "no longer supported")
		})
	}
}
