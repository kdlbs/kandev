package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
)

func TestAgentctlReadyPublishesNativeWorkspaceBeneathPromotedRoot(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.WorkspacePath = "/work/task-1/master"
	h.exec.TaskEnvironmentID = "env-1"

	h.mgr.eventPublisher.PublishAgentctlEvent(context.Background(), events.AgentctlReady, h.exec, "")

	payload, ok := h.only(t).Event.Data.(AgentctlEventPayload)
	require.True(t, ok, "payload type = %T", h.only(t).Event.Data)
	require.Equal(t, "/work/task-1/master", payload.AgentWorkspacePath,
		"ready events carry the launched execution workspace, not the promoted root")
	require.Equal(t, "/work/task-1/master", payload.WorktreePath)
}

func TestAgentctlStartingDoesNotPublishUnconfirmedNativeWorkspace(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.WorkspacePath = "/work/task-1/master"

	h.mgr.eventPublisher.PublishAgentctlEvent(context.Background(), events.AgentctlStarting, h.exec, "")

	payload, ok := h.only(t).Event.Data.(AgentctlEventPayload)
	require.True(t, ok, "payload type = %T", h.only(t).Event.Data)
	require.Empty(t, payload.AgentWorkspacePath,
		"starting events carry a prepared path, not a confirmed native workspace")
}

func TestNotifyWorktreeMaterializedKeepsLaunchedExecutionWorkspace(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.WorkspacePath = "/work/task-1/master"
	h.exec.TaskEnvironmentID = "env-1"

	h.mgr.NotifyWorktreeMaterialized(context.Background(), MaterializedWorktree{
		TaskID:            "task-1",
		SessionID:         "session-1",
		WorktreeID:        "wt-2",
		WorktreePath:      "/work/task-1/secondary",
		WorktreeBranch:    "kandev/feature",
		RepositoryID:      "repo-secondary",
		TaskWorkspacePath: "/work/task-1",
	})

	payload, ok := h.only(t).Event.Data.(AgentctlEventPayload)
	require.True(t, ok, "payload type = %T", h.only(t).Event.Data)
	require.Equal(t, "/work/task-1", payload.TaskWorkspacePath)
	require.Equal(t, "repo-secondary", payload.WorktreeRepositoryID,
		"the sibling worktree must carry its own repository identity")
	require.Equal(t, "/work/task-1/secondary", payload.WorktreePath,
		"the materialized sibling remains the worktree identity")
	require.Equal(t, "/work/task-1/master", payload.AgentWorkspacePath,
		"the native workspace must not be replaced by the sibling's worktree path")
}
