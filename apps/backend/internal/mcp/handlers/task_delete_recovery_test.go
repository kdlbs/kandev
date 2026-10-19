package handlers

import (
	"context"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHandleDeleteTaskRejectsUnknownCleanupMode(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPDeleteTask, map[string]string{"task_id": "task", "workspace_cleanup": "purge"})
	response, err := h.handleDeleteTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
}
