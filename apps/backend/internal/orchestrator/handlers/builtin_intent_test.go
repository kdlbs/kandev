package handlers

import (
	"context"
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type builtinIntentQueueChecker struct {
	recordingQueueAdmissionReadinessChecker
	allow   bool
	checked []string
}

func (c *builtinIntentQueueChecker) ValidateBuiltinIntent(_ context.Context, taskID, sessionID, intent string) error {
	c.checked = []string{taskID, sessionID, intent}
	if !c.allow {
		return errors.New("built-in tools unavailable")
	}
	return nil
}

func TestBuiltinQueueChecksAuthorityAndPersistsTypedIntent(t *testing.T) {
	for _, allow := range []bool{false, true} {
		checker := &builtinIntentQueueChecker{allow: allow}
		h, queue := setupQueueHandlersWithDrainer(t, checker)
		request := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]any{"task_id": "task", "session_id": "session", "content": "user supplied text", "builtin_intent": "walkthrough"})
		response, err := h.wsQueueMessage(context.Background(), request)
		require.NoError(t, err)
		require.Equal(t, []string{"task", "session", "walkthrough"}, checker.checked)
		entries := queue.GetStatus(context.Background(), "session").Entries
		if allow {
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			require.Len(t, entries, 1)
			require.EqualValues(t, "walkthrough", entries[0].Metadata["builtin_intent"])
		} else {
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Empty(t, entries)
		}
	}
}

func TestBuiltinQueueRejectsUnknownIntentBeforeAdmission(t *testing.T) {
	h, queue := setupQueueHandlers(t)
	request := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]any{"task_id": "task", "session_id": "session", "content": "request", "builtin_intent": "arbitrary-tool"})
	response, err := h.wsQueueMessage(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, queue.GetStatus(context.Background(), "session").Entries)
}
