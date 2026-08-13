package lifecycle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
)

// workspaceEventsHarness drives the workspace-stream callbacks the manager
// registers on the StreamManager and captures what reaches the event bus.
type workspaceEventsHarness struct {
	mgr  *Manager
	bus  *MockEventBusWithTracking
	exec *AgentExecution
}

func newWorkspaceEventsHarness(t *testing.T) *workspaceEventsHarness {
	t.Helper()
	mgr, eventBus := createTestManagerWithTracking()
	cleanupManagerStopCh(t, mgr)
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}
	require.NoError(t, mgr.executionStore.Add(execution))
	return &workspaceEventsHarness{mgr: mgr, bus: eventBus, exec: execution}
}

func (h *workspaceEventsHarness) only(t *testing.T) trackedEvent {
	t.Helper()
	h.bus.mu.Lock()
	defer h.bus.mu.Unlock()
	require.Len(t, h.bus.PublishedEvents, 1)
	return h.bus.PublishedEvents[0]
}

func (h *workspaceEventsHarness) count() int {
	h.bus.mu.Lock()
	defer h.bus.mu.Unlock()
	return len(h.bus.PublishedEvents)
}

// TestHandleGitCommitCreatedPublishesCommitPayload pins the projection the
// orchestrator's commit sync reads: every field of the agentctl notification
// must survive the translation, on the session-scoped git subject.
func TestHandleGitCommitCreatedPublishesCommitPayload(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	committedAt := time.Date(2026, 8, 12, 10, 30, 0, 0, time.UTC)

	h.mgr.handleGitCommitCreated(h.exec, &agentctl.GitCommitNotification{
		CommitSHA:      "abc123",
		ParentSHA:      "parent456",
		Message:        "fix: stop the leak",
		AuthorName:     "Kandev Agent",
		AuthorEmail:    "agent@kandev.ai",
		FilesChanged:   3,
		Insertions:     42,
		Deletions:      7,
		CommittedAt:    committedAt,
		RepositoryName: "backend",
	})

	got := h.only(t)
	require.Equal(t, events.BuildGitEventSubject("session-1"), got.Subject)
	require.Equal(t, events.GitEvent, got.Event.Type)
	payload, ok := got.Event.Data.(*GitEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, GitEventTypeCommitCreated, payload.Type)
	require.Equal(t, "task-1", payload.TaskID)
	require.Equal(t, "session-1", payload.SessionID)
	require.Equal(t, "exec-1", payload.AgentID)
	require.NotNil(t, payload.Commit)
	require.Equal(t, "abc123", payload.Commit.CommitSHA)
	require.Equal(t, "parent456", payload.Commit.ParentSHA)
	require.Equal(t, "fix: stop the leak", payload.Commit.Message)
	require.Equal(t, "Kandev Agent", payload.Commit.AuthorName)
	require.Equal(t, "agent@kandev.ai", payload.Commit.AuthorEmail)
	require.Equal(t, 3, payload.Commit.FilesChanged)
	require.Equal(t, 42, payload.Commit.Insertions)
	require.Equal(t, 7, payload.Commit.Deletions)
	require.Equal(t, committedAt.Format(time.RFC3339), payload.Commit.CommittedAt)
	require.Equal(t, "backend", payload.Commit.RepositoryName,
		"the per-repo tag must survive so multi-repo workspaces attribute the commit correctly")
}

// TestHandleGitResetDetectedPublishesResetPayload pins the HEAD-moved-backward
// projection the orchestrator uses to re-sync its commit list.
func TestHandleGitResetDetectedPublishesResetPayload(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	at := time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)

	h.mgr.handleGitResetDetected(h.exec, &agentctl.GitResetNotification{
		PreviousHead:   "head-after",
		CurrentHead:    "head-before",
		RepositoryName: "backend",
		Timestamp:      at,
	})

	payload, ok := h.only(t).Event.Data.(*GitEventPayload)
	require.True(t, ok)
	require.Equal(t, GitEventTypeCommitsReset, payload.Type)
	require.Equal(t, at.Format(time.RFC3339Nano), payload.Timestamp,
		"the reset timestamp comes from the notification, not from publish time")
	require.NotNil(t, payload.Reset)
	require.Equal(t, "head-after", payload.Reset.PreviousHead)
	require.Equal(t, "head-before", payload.Reset.CurrentHead)
	require.Equal(t, "backend", payload.Reset.RepositoryName)
}

// TestHandleBranchSwitchPublishesBaseCommit pins that the new base commit rides
// along; the orchestrator needs it to rebase its diff range.
func TestHandleBranchSwitchPublishesBaseCommit(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	h.mgr.handleBranchSwitch(h.exec, &agentctl.GitBranchSwitchNotification{
		PreviousBranch: "main",
		CurrentBranch:  "feature/cover",
		CurrentHead:    "head-sha",
		BaseCommit:     "base-sha",
		RepositoryName: "backend",
		Timestamp:      at,
	})

	payload, ok := h.only(t).Event.Data.(*GitEventPayload)
	require.True(t, ok)
	require.Equal(t, GitEventTypeBranchSwitched, payload.Type)
	require.NotNil(t, payload.BranchSwitch)
	require.Equal(t, "main", payload.BranchSwitch.PreviousBranch)
	require.Equal(t, "feature/cover", payload.BranchSwitch.CurrentBranch)
	require.Equal(t, "head-sha", payload.BranchSwitch.CurrentHead)
	require.Equal(t, "base-sha", payload.BranchSwitch.BaseCommit)
	require.Equal(t, "backend", payload.BranchSwitch.RepositoryName)
	require.Equal(t, at.Format(time.RFC3339Nano), payload.Timestamp)
}

func TestHandleGitStatusUpdateUsesGitSubject(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.handleGitStatusUpdate(h.exec, &agentctl.GitStatusUpdate{
		Timestamp: time.Now(),
		Branch:    "feature/cover",
		Modified:  []string{"main.go"},
		Ahead:     2,
	})

	got := h.only(t)
	require.Equal(t, events.BuildGitEventSubject("session-1"), got.Subject)
	payload, ok := got.Event.Data.(*GitEventPayload)
	require.True(t, ok)
	require.Equal(t, GitEventTypeStatusUpdate, payload.Type)
	require.NotNil(t, payload.Status)
	require.Equal(t, "feature/cover", payload.Status.Branch)
	require.Equal(t, []string{"main.go"}, payload.Status.Modified)
	require.Equal(t, 2, payload.Status.Ahead)
}

func TestHandleFileChangeNotificationPublishesPathAndOperation(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	at := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)

	h.mgr.handleFileChangeNotification(h.exec, &agentctl.FileChangeNotification{
		Path:           "src/main.go",
		Operation:      "modified",
		Timestamp:      at,
		RepositoryName: "backend",
	})

	got := h.only(t)
	require.Equal(t, events.BuildFileChangeSubject("session-1"), got.Subject)
	require.Equal(t, events.FileChangeNotified, got.Event.Type)
	payload, ok := got.Event.Data.(FileChangeEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, "task-1", payload.TaskID)
	require.Equal(t, "exec-1", payload.AgentID)
	require.Equal(t, "src/main.go", payload.Path)
	require.Equal(t, "modified", payload.Operation)
	require.Equal(t, at.Format(time.RFC3339Nano), payload.Timestamp)
	require.Equal(t, "backend", payload.RepositoryName)
}

func TestHandleShellOutputAndExitPublishTypedPayloads(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.handleShellOutput(h.exec, "build succeeded\n")
	h.mgr.handleShellExit(h.exec, 137)

	h.bus.mu.Lock()
	published := append([]trackedEvent(nil), h.bus.PublishedEvents...)
	h.bus.mu.Unlock()
	require.Len(t, published, 2)

	require.Equal(t, events.BuildShellOutputSubject("session-1"), published[0].Subject)
	output, ok := published[0].Event.Data.(ShellOutputEventPayload)
	require.True(t, ok, "payload type = %T", published[0].Event.Data)
	require.Equal(t, "output", output.Type)
	require.Equal(t, "build succeeded\n", output.Data)
	require.Equal(t, "session-1", output.SessionID)

	require.Equal(t, events.BuildShellExitSubject("session-1"), published[1].Subject)
	exit, ok := published[1].Event.Data.(ShellExitEventPayload)
	require.True(t, ok, "payload type = %T", published[1].Event.Data)
	require.Equal(t, "exit", exit.Type)
	require.Equal(t, 137, exit.Code, "the exit code is what the UI renders; it must not be dropped")
}

func TestHandleProcessOutputAndStatusIgnoreNilPayloads(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.handleProcessOutput(h.exec, nil)
	h.mgr.handleProcessStatus(h.exec, nil)

	require.Zero(t, h.count(), "a nil notification must be dropped, not published as an empty event")
}

func TestHandleProcessStatusPublishesScriptMetadata(t *testing.T) {
	h := newWorkspaceEventsHarness(t)
	exitCode := 2
	at := time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)

	h.mgr.handleProcessStatus(h.exec, &agentctl.ProcessStatusUpdate{
		SessionID:  "session-1",
		ProcessID:  "proc-9",
		Command:    "make test",
		ScriptName: "setup",
		WorkingDir: "/workspace",
		Status:     "failed",
		ExitCode:   &exitCode,
		Timestamp:  at,
	})

	got := h.only(t)
	require.Equal(t, events.ProcessStatus, got.Event.Type)
	payload, ok := got.Event.Data.(ProcessStatusEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, "proc-9", payload.ProcessID)
	require.Equal(t, "make test", payload.Command)
	require.Equal(t, "setup", payload.ScriptName)
	require.Equal(t, "/workspace", payload.WorkingDir)
	require.Equal(t, "failed", payload.Status)
	require.NotNil(t, payload.ExitCode)
	require.Equal(t, 2, *payload.ExitCode)
}

func TestPublishAvailableCommandsCarriesCommandList(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.eventPublisher.PublishAvailableCommands(h.exec, []streams.AvailableCommand{
		{Name: "review", Description: "Review the diff"},
	})

	got := h.only(t)
	require.Equal(t, events.BuildAvailableCommandsSubject("session-1"), got.Subject)
	require.Equal(t, events.AvailableCommandsUpdated, got.Event.Type)
	payload, ok := got.Event.Data.(AvailableCommandsEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Len(t, payload.AvailableCommands, 1)
	require.Equal(t, "review", payload.AvailableCommands[0].Name)
	require.Equal(t, "Review the diff", payload.AvailableCommands[0].Description)
}

func TestPublishPermissionRequestProjectsOptions(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.eventPublisher.PublishPermissionRequest(h.exec, agentctl.AgentEvent{
		PendingID:       "pending-1",
		ToolCallID:      "tool-1",
		PermissionTitle: "Run `rm -rf build`",
		ActionType:      "execute",
		ActionDetails:   map[string]any{"command": "rm -rf build"},
		PermissionOptions: []agentctl.PermissionOption{
			{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
			{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
		},
	})

	got := h.only(t)
	require.Equal(t, events.BuildPermissionRequestSubject("session-1"), got.Subject)
	payload, ok := got.Event.Data.(PermissionRequestEventPayload)
	require.True(t, ok, "payload type = %T", got.Event.Data)
	require.Equal(t, "permission_request", payload.Type)
	require.Equal(t, "pending-1", payload.PendingID)
	require.Equal(t, "tool-1", payload.ToolCallID)
	require.Equal(t, "Run `rm -rf build`", payload.Title)
	require.Equal(t, "execute", payload.ActionType)
	require.Equal(t, map[string]any{"command": "rm -rf build"}, payload.ActionDetails)
	require.Equal(t, []PermissionOption{
		{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
		{OptionID: "deny", Name: "Deny", Kind: "reject_once"},
	}, payload.Options, "every option must reach the UI or the dialog renders unusable buttons")
}

// TestEventPublisherWithoutBusIsInert pins that a publisher wired without an
// event bus (embedded / test configurations) is a silent no-op rather than a
// nil-map panic on every workspace notification.
func TestEventPublisherWithoutBusIsInert(t *testing.T) {
	publisher := NewEventPublisher(nil, newTestLogger())
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}

	publisher.PublishGitEvent(&GitEventPayload{Type: GitEventTypeStatusUpdate, SessionID: "session-1"})
	publisher.PublishFileChange(execution, &agentctl.FileChangeNotification{Path: "a.go"})
	publisher.PublishShellOutput(execution, "hi")
	publisher.PublishShellExit(execution, 0)
	publisher.PublishPermissionRequest(execution, agentctl.AgentEvent{})
	publisher.PublishAvailableCommands(execution, nil)
	publisher.PublishProcessOutput(execution, &agentctl.ProcessOutput{})
	publisher.PublishProcessStatus(execution, &agentctl.ProcessStatusUpdate{})
}

// TestPublishGitEventStampsTimestampWhenMissing pins the fallback: a payload
// built without a timestamp still reaches subscribers with one, so the frontend
// can order events.
func TestPublishGitEventStampsTimestampWhenMissing(t *testing.T) {
	h := newWorkspaceEventsHarness(t)

	h.mgr.eventPublisher.PublishGitEvent(&GitEventPayload{
		Type:      GitEventTypeStatusUpdate,
		SessionID: "session-1",
	})

	payload, ok := h.only(t).Event.Data.(*GitEventPayload)
	require.True(t, ok)
	require.NotEmpty(t, payload.Timestamp)
	_, err := time.Parse(time.RFC3339Nano, payload.Timestamp)
	require.NoError(t, err)
}
