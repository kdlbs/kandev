package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
func TestHandleSpawnSessionPreservesWorkflowSourceBinding(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, targetSession := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)

	tests := []struct {
		name       string
		senderTask string
		senderSess string
		wantOrigin bool
	}{
		{
			name:       "same-task sender session",
			senderTask: target.ID,
			senderSess: targetSession.ID,
			wantOrigin: true,
		},
		{
			name:       "cross-task sender session",
			senderTask: sender.ID,
			senderSess: "sender-sess-1",
			wantOrigin: true,
		},
		{
			name:       "absent sender session",
			senderTask: target.ID,
			wantOrigin: false,
		},
		{
			name:       "unknown sender session",
			senderTask: target.ID,
			senderSess: "sess-ghost",
			wantOrigin: false,
		},
		{
			name:       "mismatched sender session",
			senderTask: sender.ID,
			senderSess: targetSession.ID,
			wantOrigin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, orch := newMessageTaskHandler(t, svc)
			msg := makeWSMessage(t, ws.ActionMCPSpawnSession,
				spawnPayload(target.ID, "help with the review", tt.senderTask, tt.senderSess))

			resp, err := h.handleSpawnSession(context.Background(), msg)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)

			require.Len(t, orch.launchCalls, 1)
			launched := orch.launchCalls[0]
			assert.True(t, launched.PreserveWorkflowSourceBinding,
				"every spawn must preserve the source workflow binding, independently of attribution")
			assert.Equal(t, tt.wantOrigin, launched.SpawnOrigin != nil)
		})
	}
}
