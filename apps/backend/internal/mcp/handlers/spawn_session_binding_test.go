package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func pinnedSpawnFixture(t *testing.T) spawnReuseFixture {
	t.Helper()
	f := newSpawnReuseFixture(t)
	f.task.WorkflowStepID = "spawn-work"
	require.NoError(t, f.repo.UpdateTask(context.Background(), f.task))
	return f
}

// Covers source-binding preservation for the pinned-step spawn contract.
func TestHandleSpawnSession_PreservesPinnedSourceBinding(t *testing.T) {
	for _, tc := range []struct {
		name, requested, expected, caller string
	}{
		{"different profile", "requested-profile", "requested-profile", "sess-1"},
		{"same profile", "agent-profile-1", "agent-profile-1", "sess-1"},
		{"caller inheritance", "", "agent-profile-1", "sess-1"},
		{"primary inheritance without verified caller", "", "agent-profile-1", "unknown-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := pinnedSpawnFixture(t)
			ctx := context.Background()
			before, err := f.repo.GetWorkflowSessionBinding(ctx, f.task.ID, "step:spawn-work")
			require.NoError(t, err)
			payload := spawnPayload(f.task.ID, "independent review", f.task.ID, tc.caller)
			payload["agent_profile_id"], payload["name"] = tc.requested, "reviewer"
			response, err := f.handler.handleSpawnSession(ctx, makeWSMessage(t, ws.ActionMCPSpawnSession, payload))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
			var out orchestrator.LaunchSessionResponse
			require.NoError(t, json.Unmarshal(response.Payload, &out))
			verifyPinnedSpawn(t, f, out, tc.expected)
			after, err := f.repo.GetWorkflowSessionBinding(ctx, f.task.ID, "step:spawn-work")
			require.NoError(t, err)
			require.Equal(t, before, after, "additional spawn must not own the workflow source binding")
		})
	}
}

func TestLaunchSession_SelectorStillRecordsPinnedSourceBinding(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "workflow", true: "selector"}[explicit], func(t *testing.T) {
			f := pinnedSpawnFixture(t)
			ctx := context.Background()
			response, err := f.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
				TaskID: f.task.ID, Intent: orchestrator.IntentStart, Prompt: "workflow launch",
				AgentProfileID: f.source.AgentProfileID, ProfileExplicit: explicit,
			})
			require.NoError(t, err)
			require.NotEqual(t, f.source.ID, response.SessionID)
			binding, err := f.repo.GetWorkflowSessionBinding(ctx, f.task.ID, "step:spawn-work")
			require.NoError(t, err)
			require.Equal(t, response.SessionID, binding.SessionID)
			require.Equal(t, response.AgentProfileID, binding.AgentProfileID)
		})
	}
}

func verifyPinnedSpawn(t *testing.T, f spawnReuseFixture, out orchestrator.LaunchSessionResponse, profile string) {
	t.Helper()
	ctx := context.Background()
	require.NotEqual(t, f.source.ID, out.SessionID)
	session, err := f.repo.GetTaskSession(ctx, out.SessionID)
	require.NoError(t, err)
	require.Equal(t, profile, session.AgentProfileID)
	require.Equal(t, profile, out.AgentProfileID)
	require.Equal(t, "reviewer", session.Name)
	launched := <-f.runtime.launchEntered
	require.Equal(t, out.SessionID, launched.SessionID)
	require.Equal(t, profile, launched.AgentProfileID)
	original, err := f.repo.GetTaskSession(ctx, f.source.ID)
	require.NoError(t, err)
	require.Equal(t, "original", original.Name)
	require.Equal(t, f.source.AgentProfileID, original.AgentProfileID)
	sessions, err := f.repo.ListTaskSessions(ctx, f.task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}
