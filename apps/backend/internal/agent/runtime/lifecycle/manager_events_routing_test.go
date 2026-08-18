package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleAvailableCommandsEventCachesAndPublishes pins both halves of the
// slash-command update: the execution caches the list for later reads, and the
// same list is published so the composer refreshes without a reload.
func TestHandleAvailableCommandsEventCachesAndPublishes(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	commands := []streams.AvailableCommand{
		{Name: "review", Description: "Review the diff"},
		{Name: "commit"},
	}

	h.mgr.handleAvailableCommandsEvent(h.exec, agentctl.AgentEvent{AvailableCommands: commands})

	require.Equal(t, commands, h.mgr.GetAvailableCommandsForSession("session-1"),
		"the cached list is what a late-joining subscriber reads back")
	got := h.only(t)
	require.Equal(t, events.AvailableCommandsUpdated, got.Event.Type)
	payload, ok := got.Event.Data.(AvailableCommandsEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, commands, payload.AvailableCommands)
}

func TestHandleAvailableCommandsEventIgnoresEmptyList(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.SetAvailableCommands([]streams.AvailableCommand{{Name: "review"}})

	h.mgr.handleAvailableCommandsEvent(h.exec, agentctl.AgentEvent{})

	require.Zero(t, h.count(), "an empty list must not be published")
	require.Len(t, h.mgr.GetAvailableCommandsForSession("session-1"), 1,
		"an empty update must not wipe the commands the agent already reported")
}

// TestHandleAgentEventDropsStaleMCPAttachment pins the identity check: a
// diagnostic from a replaced execution must not be attributed to the live one.
func TestHandleAgentEventDropsStaleMCPAttachment(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type: streams.EventTypeMCPAttachment,
		MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{
			AttemptID:   "attempt-1",
			ExecutionID: "exec-replaced",
		},
	})

	require.Zero(t, h.count(), "an attachment attempt from a replaced execution is dropped")
}

// TestHandleAgentEventStampsMCPAttachmentIdentity pins that the attempt is
// re-stamped with the live execution's identity before publication, and that a
// copy is stamped rather than the caller's struct.
func TestHandleAgentEventStampsMCPAttachmentIdentity(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.AgentID = "claude-acp"
	h.exec.Status = v1.AgentStatusReady
	attempt := &streams.MCPAttachmentAttempt{AttemptID: "attempt-1"}

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:                 streams.EventTypeMCPAttachment,
		MCPAttachmentAttempt: attempt,
	})

	got := h.only(t)
	payload, ok := got.Event.Data.(AgentStreamEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.NotNil(t, payload.Data)
	published := payload.Data.MCPAttachmentAttempt
	require.NotNil(t, published)
	require.Equal(t, "task-1", published.TaskID)
	require.Equal(t, "session-1", published.SessionID)
	require.Equal(t, "exec-1", published.ExecutionID)
	require.Equal(t, "claude-acp", published.AgentID)

	require.Empty(t, attempt.ExecutionID,
		"the caller's attempt must not be stamped in place; a copy is published")
	require.Equal(t, v1.AgentStatusReady, h.exec.Status,
		"attachment diagnostics are not model activity and must not flip Ready into Running")
}

// TestHandleStreamDisconnectIgnoresSupersededGeneration pins the guard that
// keeps an old prompt's disconnect from failing the execution that replaced it.
func TestHandleStreamDisconnectIgnoresSupersededGeneration(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.Status = v1.AgentStatusRunning
	h.exec.promptDoneCh = make(chan PromptCompletionSignal, 1)
	generation, err := h.mgr.BeginPrompt("exec-1")
	require.NoError(t, err)

	h.mgr.handleStreamDisconnect(h.exec, errors.New("connection reset"), generation+1)

	require.NotEqual(t, v1.AgentStatusFailed, h.exec.Status,
		"a disconnect from a superseded prompt generation must not fail the live execution")
}

func TestHandleStreamDisconnectFailsOwningGeneration(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.Status = v1.AgentStatusRunning
	h.exec.promptDoneCh = make(chan PromptCompletionSignal, 1)
	generation, err := h.mgr.BeginPrompt("exec-1")
	require.NoError(t, err)

	h.mgr.handleStreamDisconnect(h.exec, errors.New("connection reset"), generation)

	require.Equal(t, v1.AgentStatusFailed, h.exec.Status,
		"the owning prompt's disconnect fails the execution so the orchestrator can reconcile")
}

func TestNotifyWorktreeMaterializedPublishesAgentctlReady(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	h.exec.TaskEnvironmentID = "env-1"

	h.mgr.NotifyWorktreeMaterialized(context.Background(), MaterializedWorktree{
		TaskID:            "task-1",
		SessionID:         "session-1",
		WorktreeID:        "wt-1",
		WorktreePath:      "/work/task-1/backend",
		WorktreeBranch:    "kandev/feature",
		TaskWorkspacePath: "/work/task-1",
	})

	got := h.only(t)
	require.Equal(t, events.AgentctlReady, got.Subject)
	payload, ok := got.Event.Data.(AgentctlEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, "task-1", payload.TaskID)
	require.Equal(t, "session-1", payload.SessionID)
	require.Equal(t, "exec-1", payload.AgentExecutionID,
		"the live execution is resolved so the frontend can key the update")
	require.Equal(t, "env-1", payload.TaskEnvironmentID)
	require.Equal(t, "wt-1", payload.WorktreeID)
	require.Equal(t, "/work/task-1/backend", payload.WorktreePath)
	require.Equal(t, "kandev/feature", payload.WorktreeBranch)
	require.Equal(t, "/work/task-1", payload.TaskWorkspacePath)
}

func TestNotifyWorktreeMaterializedWithoutExecutionStillPublishes(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.NotifyWorktreeMaterialized(context.Background(), MaterializedWorktree{
		TaskID: "task-2", SessionID: "session-unknown", WorktreeID: "wt-2",
	})

	payload, ok := h.only(t).Event.Data.(AgentctlEventPayload)
	require.True(t, ok)
	require.Empty(t, payload.AgentExecutionID,
		"a worktree materialized before its execution exists still notifies the frontend")
	require.Equal(t, "wt-2", payload.WorktreeID)
}

func TestNotifyWorktreeMaterializedWithoutEventBusIsNoOp(t *testing.T) {
	mgr := newTestManager(t)
	mgr.eventPublisher = NewEventPublisher(nil, newTestLogger())

	mgr.NotifyWorktreeMaterialized(context.Background(), MaterializedWorktree{SessionID: "session-1"})
}

func TestBranchSnapshotErrorWrapsCause(t *testing.T) {
	cause := errors.New("write executors_running: disk full")
	err := &BranchSnapshotError{Cause: cause}

	require.Equal(t, "branch renamed but branch snapshot persistence failed: write executors_running: disk full",
		err.Error())
	require.ErrorIs(t, err, cause)
	require.False(t, err.Retryable(),
		"the branch already has its new name; retrying the Git op would target the wrong branch")

	var nilErr *BranchSnapshotError
	require.Equal(t, "branch renamed but branch snapshot persistence failed", nilErr.Error())
	require.NoError(t, nilErr.Unwrap())
	require.Equal(t, "branch renamed but branch snapshot persistence failed",
		(&BranchSnapshotError{}).Error())
}

func TestMetadataIntAcceptsJSONRoundTrippedShapes(t *testing.T) {
	require.Zero(t, metadataInt(nil, "port"))
	require.Equal(t, 8080, metadataInt(map[string]interface{}{"port": 8080}, "port"))
	require.Equal(t, 8080, metadataInt(map[string]interface{}{"port": int64(8080)}, "port"))
	require.Equal(t, 8080, metadataInt(map[string]interface{}{"port": float64(8080)}, "port"),
		"a JSON round-trip turns integers into float64")
	require.Equal(t, 8080, metadataInt(map[string]interface{}{"port": "8080"}, "port"))
	require.Zero(t, metadataInt(map[string]interface{}{"port": "not-a-number"}, "port"))
	require.Zero(t, metadataInt(map[string]interface{}{"port": true}, "port"))
	require.Zero(t, metadataInt(map[string]interface{}{}, "port"))
}

func TestAgentctlPIDFromExecution(t *testing.T) {
	require.Zero(t, agentctlPIDFromExecution(nil))

	execution := &AgentExecution{ID: "exec-1"}
	require.Zero(t, agentctlPIDFromExecution(execution))

	execution.setMetadataValue(MetadataKeySSHRemoteAgentctlPID, float64(4242))
	require.Equal(t, 4242, agentctlPIDFromExecution(execution),
		"the remote PID survives a persistence round-trip as a float")
}
