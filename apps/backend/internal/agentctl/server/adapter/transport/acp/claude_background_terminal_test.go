package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// claudeTerminalAdapter builds a Claude adapter whose initialize handshake
// reports the exact verified bridge tuple.
func claudeTerminalAdapter(t *testing.T) *Adapter {
	t.Helper()
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = "session-claude"
	a.agentInfo = &AgentInfo{
		Name:    verifiedClaudeTerminalBridgeName,
		Version: verifiedClaudeTerminalBridgeVersion,
	}
	return a
}

type claudeBackgroundLaunch struct {
	toolCallID string
	command    string
	title      string
}

func startClaudeBackgroundLaunch(t *testing.T, a *Adapter, sessionID string, launch claudeBackgroundLaunch) *AgentEvent {
	t.Helper()
	call := &acp.SessionUpdateToolCall{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Kind:          acp.ToolKindExecute,
		Status:        acp.ToolCallStatusPending,
		Title:         "Terminal",
		SessionUpdate: "tool_call",
		RawInput:      map[string]any{},
	}
	require.NotNil(t, a.convertToolCallUpdate(sessionID, call))

	completed := acp.ToolCallStatusCompleted
	event := a.convertToolCallResultUpdate(sessionID, &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command":           launch.command,
			"description":       launch.title,
			"run_in_background": true,
		},
	})
	require.NotNil(t, event)
	require.True(t, event.NormalizedPayload.IsDetachedBackgroundLaunch(),
		"the launch tool must be bound before terminal evidence can settle it")
	return event
}

func startClaudeBackgroundShell(t *testing.T, a *Adapter, sessionID string, launch claudeBackgroundLaunch) *AgentEvent {
	t.Helper()
	call := &acp.SessionUpdateToolCall{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Kind:          acp.ToolKindExecute,
		Status:        acp.ToolCallStatusPending,
		Title:         "Terminal",
		SessionUpdate: "tool_call",
		RawInput:      map[string]any{},
	}
	require.NotNil(t, a.convertToolCallUpdate(sessionID, call))
	status := acp.ToolCallStatus("in_progress")
	event := a.convertToolCallResultUpdate(sessionID, &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(launch.toolCallID),
		Status:        &status,
		SessionUpdate: "tool_call_update",
		Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command":           launch.command,
			"description":       launch.title,
			"run_in_background": true,
		},
	})
	require.NotNil(t, event)
	require.True(t, event.NormalizedPayload.IsDetachedBackgroundLaunch())
	require.Empty(t, event.NormalizedPayload.BackgroundWork().WorkID)
	return event
}

func claudeTerminalExitUpdate(toolCallID string, exitCode, signal any) *acp.SessionToolCallUpdate {
	return &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(toolCallID),
		SessionUpdate: "tool_call_update",
		Meta: map[string]any{
			"terminal_exit": map[string]any{
				"exit_code":   exitCode,
				"signal":      signal,
				"terminal_id": toolCallID,
			},
		},
	}
}

func activeClaudeToolCallCount(a *Adapter) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.activeToolCalls)
}

type claudeCancelTestAgent struct {
	sessionRequestCaptureAgent
	connection     *acp.AgentSideConnection
	cancelRequests chan acp.CancelNotification
}

func (a *claudeCancelTestAgent) Cancel(_ context.Context, request acp.CancelNotification) error {
	a.cancelRequests <- request
	return nil
}

func newClaudeCancelTestConnection(t *testing.T, a *Adapter, sessionID string) *claudeCancelTestAgent {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	client := acpclient.NewClient(acpclient.WithUpdateHandler(a.enqueueACPUpdate))
	connection := acp.NewClientSideConnection(client, clientToAgentWriter, agentToClientReader)
	peer := &claudeCancelTestAgent{cancelRequests: make(chan acp.CancelNotification, 1)}
	peer.connection = acp.NewAgentSideConnection(peer, agentToClientWriter, clientToAgentReader)
	a.acpClient = client
	a.acpConn = connection
	a.sessionID = sessionID
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})
	return peer
}

func waitForClaudeToolUpdate(t *testing.T, a *Adapter, toolCallID string) AgentEvent {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-a.updatesCh:
			if event.Type == streams.EventTypeToolUpdate && event.ToolCallID == toolCallID {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for tool update %q", toolCallID)
		}
	}
}

func cancelClaudeWithQueuedLaunchUpdate(
	t *testing.T,
	a *Adapter,
	peer *claudeCancelTestAgent,
	launch claudeBackgroundLaunch,
) []AgentEvent {
	t.Helper()
	workerEntered := make(chan struct{})
	workerRelease := make(chan struct{})
	workerDone := make(chan bool, 1)
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(workerRelease) }) }
	t.Cleanup(releaseWorker)
	go func() {
		workerDone <- a.syncNotifQueueThen(func() {
			close(workerEntered)
			<-workerRelease
		})
	}()
	select {
	case <-workerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("notification worker did not reach the test barrier")
	}

	completed := acp.ToolCallStatusCompleted
	require.NoError(t, peer.connection.SessionUpdate(t.Context(), acp.SessionNotification{
		SessionId: "session-claude",
		Update: acp.SessionUpdate{ToolCallUpdate: &acp.SessionToolCallUpdate{
			ToolCallId:    acp.ToolCallId(launch.toolCallID),
			Status:        &completed,
			SessionUpdate: "tool_call_update",
			Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
			RawInput: map[string]any{
				"command":           launch.command,
				"description":       launch.title,
				"run_in_background": true,
			},
		}},
	}))
	require.Eventually(t, func() bool { return len(a.notifQueue) == 1 }, 2*time.Second, time.Millisecond,
		"completed launch update must be queued behind the held worker")
	cancelDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	go func() { cancelDone <- a.Cancel(ctx) }()
	require.Eventually(t, func() bool { return len(a.notifQueue) == 2 }, 2*time.Second, time.Millisecond,
		"Stop must queue its sweep barrier after the launch update")
	select {
	case request := <-peer.cancelRequests:
		require.Equal(t, "session-claude", string(request.SessionId))
	case <-time.After(2 * time.Second):
		t.Fatal("session/cancel must be sent before waiting for the blocked notification worker")
	}
	select {
	case err := <-cancelDone:
		require.NoError(t, err, "Stop must return while notification output is blocked")
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel waited indefinitely for the blocked notification worker")
	}
	releaseWorker()
	require.True(t, <-workerDone)
	require.Eventually(t, func() bool { return activeClaudeToolCallCount(a) == 2 }, 2*time.Second, time.Millisecond,
		"queued completion must be processed before the cancellation sweep")
	var events []AgentEvent
	cancelled := 0
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for cancelled < 2 {
		select {
		case event := <-a.updatesCh:
			events = append(events, event)
			if event.ToolStatus == toolStatusCancelled {
				cancelled++
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for cancellation updates; received %d of 2", cancelled)
		}
	}
	return events
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.3
// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalExitAfterTurnCancel(t *testing.T) {
	a := claudeTerminalAdapter(t)
	peer := newClaudeCancelTestConnection(t, a, "session-claude")
	first := claudeBackgroundLaunch{toolCallID: "toolu_cancel_first", command: "sleep 10", title: "First"}
	second := claudeBackgroundLaunch{toolCallID: "toolu_cancel_second", command: "sleep 20", title: "Second"}
	startClaudeBackgroundShell(t, a, "session-claude", first)
	secondLaunch := startClaudeBackgroundLaunch(t, a, "session-claude", second)
	require.Equal(t, second.toolCallID, secondLaunch.NormalizedPayload.BackgroundWork().WorkID)
	pendingID := "toolu_cancel_pending"
	require.NotNil(t, a.convertToolCallUpdate("session-claude", &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(pendingID), Kind: acp.ToolKindExecute,
		Status: acp.ToolCallStatusPending, Title: "Terminal", SessionUpdate: "tool_call",
		Meta: map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command":           "sleep 30",
			"description":       "Pending background shell",
			"run_in_background": true,
		},
	}))
	foregroundID := "toolu_cancel_foreground"
	require.NotNil(t, a.convertToolCallUpdate("session-claude", &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(foregroundID), Kind: acp.ToolKindRead,
		Status: acp.ToolCallStatusPending, Title: "Read", SessionUpdate: "tool_call",
		RawInput: map[string]any{"path": "README.md"},
	}))
	drainEvents(a)

	allEvents := cancelClaudeWithQueuedLaunchUpdate(t, a, peer, first)
	var cancelEvents []AgentEvent
	for _, event := range allEvents {
		if event.ToolStatus == toolStatusCancelled {
			cancelEvents = append(cancelEvents, event)
		}
	}
	a.mu.RLock()
	retained := a.activeToolCalls[first.toolCallID]
	a.mu.RUnlock()
	require.NotNil(t, retained)
	require.Equal(t, first.toolCallID, retained.BackgroundWork().WorkID)

	require.NoError(t, peer.connection.SessionUpdate(t.Context(), acp.SessionNotification{
		SessionId: "session-claude",
		Update:    acp.SessionUpdate{ToolCallUpdate: claudeTerminalExitUpdate(first.toolCallID, nil, nil)},
	}))
	terminal := waitForClaudeToolUpdate(t, a, first.toolCallID)
	require.NotNil(t, terminal.BackgroundWork, "the matching exit must retain its workload observation after Stop")
	require.Equal(t, streams.RunStateEnded, terminal.BackgroundWork.State)
	require.Equal(t, first.toolCallID, terminal.BackgroundWork.WorkID)
	require.Nil(t, terminal.BackgroundWork.ExitCode)
	require.Equal(t, toolStatusComplete, terminal.ToolStatus)
	require.Equal(t, 1, activeClaudeToolCallCount(a), "the live sibling remains tracked")
	require.Len(t, cancelEvents, 2, "pending Bash and foreground tools receive ordinary cancellation")
	cancelledIDs := make([]string, 0, len(cancelEvents))
	for _, event := range cancelEvents {
		require.Equal(t, toolStatusCancelled, event.ToolStatus)
		cancelledIDs = append(cancelledIDs, event.ToolCallID)
		if event.ToolCallID == pendingID {
			work := event.NormalizedPayload.BackgroundWork()
			require.NotNil(t, work)
			require.True(t, work.Ended, "an unconfirmed launch cannot remain live after cancellation")
			require.False(t, event.NormalizedPayload.IsActiveBackgroundWork())
			require.NotNil(t, event.BackgroundWork, "cancellation must project the terminal workload state explicitly")
			require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State)
		}
	}
	require.ElementsMatch(t, []string{pendingID, foregroundID}, cancelledIDs)
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestCancelPendingClaudeShellEmitsEndedWorkObservation(t *testing.T) {
	a := claudeTerminalAdapter(t)
	launch := claudeBackgroundLaunch{toolCallID: "toolu_cancel_pending", command: "sleep 30", title: "Pending"}
	startClaudeBackgroundShell(t, a, "session-claude", launch)
	drainEvents(a)

	a.cancelActiveToolCalls("session-claude")
	events := drainEvents(a)
	require.Len(t, events, 1)
	require.Equal(t, toolStatusCancelled, events[0].ToolStatus)
	require.NotNil(t, events[0].BackgroundWork)
	require.Equal(t, launch.toolCallID, events[0].BackgroundWork.WorkID)
	require.Equal(t, streams.RunStateEnded, events[0].BackgroundWork.State)
	require.True(t, events[0].NormalizedPayload.BackgroundWork().Ended)
}

// @covers AC-AGENTS-BACKGROUND-WORK-002.2
func TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement(t *testing.T) {
	a := claudeTerminalAdapter(t)
	a.sessionID = "session-old"
	launch := claudeBackgroundLaunch{toolCallID: "toolu_reused", command: "sleep 10", title: "Old shell"}
	startClaudeBackgroundShell(t, a, "session-old", launch)
	drainEvents(a)

	require.True(t, a.installNativeBindingBarrier("session-new", false, false))
	require.Equal(t, 0, activeClaudeToolCallCount(a), "native session replacement releases old shell bindings")
	newLaunch := claudeBackgroundLaunch{toolCallID: launch.toolCallID, command: "sleep 20", title: "New shell"}
	startClaudeBackgroundLaunch(t, a, "session-new", newLaunch)
	drainEvents(a)
	a.mu.RLock()
	newBinding := a.activeToolCalls[newLaunch.toolCallID]
	a.mu.RUnlock()
	require.NotNil(t, newBinding)

	staleStart := a.convertToolCallUpdate("session-old", &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(launch.toolCallID), Kind: acp.ToolKind("unknown"),
		Status: acp.ToolCallStatusPending, Title: "Unclassified stale start", SessionUpdate: "tool_call",
	})
	require.Nil(t, staleStart, "a stale session cannot replace the current shell binding")
	a.mu.RLock()
	retainedBinding := a.activeToolCalls[newLaunch.toolCallID]
	a.mu.RUnlock()
	require.Same(t, newBinding, retainedBinding, "a stale start must leave the new session's binding untouched")
	staleLaunchResult := acp.ToolCallStatusCompleted
	require.Nil(t, a.convertToolCallResultUpdate("session-old", &acp.SessionToolCallUpdate{
		ToolCallId: acp.ToolCallId(launch.toolCallID), Status: &staleLaunchResult,
		SessionUpdate: "tool_call_update",
		Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command": "stale old shell", "run_in_background": true,
		},
	}), "a stale launch result cannot mutate the current shell")

	require.Nil(t, a.convertToolCallResultUpdate("session-old", claudeTerminalExitUpdate(launch.toolCallID, nil, nil)),
		"a late exit from the replaced native session cannot settle a reused launch ID")
	require.Equal(t, 1, activeClaudeToolCallCount(a))
	terminal := a.convertToolCallResultUpdate("session-new", claudeTerminalExitUpdate(newLaunch.toolCallID, nil, nil))
	require.NotNil(t, terminal)
	require.NotNil(t, terminal.BackgroundWork)
	require.Equal(t, newLaunch.title, terminal.BackgroundWork.Title)
	require.Equal(t, streams.RunStateEnded, terminal.BackgroundWork.State)
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.3
// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalExitManagedDefault(t *testing.T) {
	packageName := agents.NewClaudeACP().ManagedNPMRuntime().Package
	version, err := agents.DefaultManagedNPMRuntimeVersion(packageName)
	require.NoError(t, err)

	fixturePath := filepath.Join("testdata", fmt.Sprintf("claude-%s-detached-bash.jsonl", version))
	fixture, err := os.Open(fixturePath)
	require.NoError(t, err, "managed Claude bridge version %s needs a qualified detached Bash fixture", version)
	t.Cleanup(func() { require.NoError(t, fixture.Close()) })

	a := newTestAdapterForAgent(claudeAgentID)
	a.agentInfo = &AgentInfo{Name: verifiedClaudeTerminalBridgeName, Version: version}
	t.Cleanup(func() { _ = a.Close() })
	var events []AgentEvent
	scanner := bufio.NewScanner(fixture)
	for scanner.Scan() {
		var notification acp.SessionNotification
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &notification), "decode managed bridge fixture frame")
		a.sessionID = string(notification.SessionId)
		a.handleACPUpdate(notification, 0)
		events = append(events, drainEvents(a)...)
		if notification.Update.ToolCallUpdate != nil && notification.Update.ToolCallUpdate.Status != nil &&
			*notification.Update.ToolCallUpdate.Status == acp.ToolCallStatusCompleted {
			require.Equal(t, 1, activeClaudeToolCallCount(a), "the detached launch result must leave its shell tracked")
		}
	}
	require.NoError(t, scanner.Err())
	require.NotEmpty(t, events)

	var terminal *AgentEvent
	for i := range events {
		if events[i].BackgroundWork != nil && events[i].BackgroundWork.State == streams.RunStateEnded {
			terminal = &events[i]
			break
		}
	}
	require.NotNil(t, terminal, "the separate managed-bridge terminal frame must settle its detached shell")
	require.Equal(t, "toolu_managed_detached", terminal.ToolCallID)
	require.Equal(t, "toolu_managed_detached", terminal.BackgroundWork.WorkID)
	require.Equal(t, streams.RunStateEnded, terminal.BackgroundWork.State)
	require.Nil(t, terminal.BackgroundWork.ExitCode, "a null exit code remains unavailable")
	require.Equal(t, 0, activeClaudeToolCallCount(a))
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.3
// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalPromptCleanupScope(t *testing.T) {
	cleanups := []struct {
		name string
		run  func(*Adapter)
	}{
		{name: "prompt end", run: func(a *Adapter) { a.cancelPromptEndToolCalls("session-claude") }},
		{name: "explicit cancel", run: func(a *Adapter) { a.cancelActiveToolCalls("session-claude") }},
	}
	identities := []struct {
		name          string
		agentName     string
		agentVersion  string
		wantPreserved bool
	}{
		{
			name:          "verified bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  verifiedClaudeTerminalBridgeVersion,
			wantPreserved: true,
		},
		{
			name:          "managed bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  verifiedClaudeTerminalBridgeManagedVersion,
			wantPreserved: true,
		},
		{
			name:          "current managed bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  verifiedClaudeTerminalBridgeCurrentVersion,
			wantPreserved: true,
		},
		{
			name:          "legacy bridge",
			agentName:     verifiedClaudeTerminalBridgeName,
			agentVersion:  "0.81.2",
			wantPreserved: false,
		},
		{
			name:          "missing identity",
			agentName:     "",
			agentVersion:  "",
			wantPreserved: false,
		},
	}
	for _, identity := range identities {
		for _, cleanup := range cleanups {
			t.Run(identity.name+" "+cleanup.name, func(t *testing.T) {
				a := claudeTerminalAdapter(t)
				a.agentInfo = &AgentInfo{Name: identity.agentName, Version: identity.agentVersion}
				launch := claudeBackgroundLaunch{
					toolCallID: "toolu_cleanup", command: "sleep 30", title: "Sleep",
				}
				startClaudeBackgroundLaunch(t, a, "session-claude", launch)
				drainEvents(a)

				cleanup.run(a)

				events := drainEvents(a)
				if identity.wantPreserved {
					require.Equal(t, 1, activeClaudeToolCallCount(a),
						"the verified bridge can still report this shell's terminal exit")
					for _, event := range events {
						require.NotEqual(t, toolStatusCancelled, event.ToolStatus)
					}
					return
				}
				require.Equal(t, 0, activeClaudeToolCallCount(a),
					"unsupported cleanup paths keep the prompt-end/explicit-cancel fallback")
				require.Len(t, events, 1)
				require.Equal(t, launch.toolCallID, events[0].ToolCallID)
				require.Equal(t, toolStatusCancelled, events[0].ToolStatus)
			})
		}
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalExit(t *testing.T) {
	t.Run("null exit settles only its matching live sibling", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		first := claudeBackgroundLaunch{
			toolCallID: "toolu_first", command: "go test ./...", title: "Run the test suite",
		}
		second := claudeBackgroundLaunch{
			toolCallID: "toolu_second", command: "gh pr checks --watch", title: "Watch checks",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", first)
		startClaudeBackgroundLaunch(t, a, "session-claude", second)
		// The foreground turn can end while a detached command still runs; the
		// verified binding must survive prompt-end cleanup so its later terminal
		// frame can settle the exact job.
		a.cancelPromptEndToolCalls("session-claude")
		for _, event := range drainEvents(a) {
			require.NotEqual(t, toolStatusCancelled, event.ToolStatus,
				"a verified detached shell must not be cancelled at prompt end")
		}
		require.Equal(t, 2, activeClaudeToolCallCount(a))

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(first.toolCallID, nil, nil))
		require.NotNil(t, event)
		require.Equal(t, toolStatusComplete, event.ToolStatus)
		require.False(t, event.ToolStatusMissing,
			"the adapter synthesized a terminal status from exact evidence")
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, first.toolCallID, event.BackgroundWork.WorkID)
		require.Equal(t, streams.WorkloadKindShell, event.BackgroundWork.Kind)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State,
			"a null bridge exit keeps the outcome unavailable rather than completed")
		require.Nil(t, event.BackgroundWork.ExitCode, "null is not zero")
		require.Equal(t, first.toolCallID, event.BackgroundWork.SourceCallID)
		require.Equal(t, first.title, event.BackgroundWork.Title)
		require.NotNil(t, event.BackgroundWork.FinishedAt)
		require.Equal(t, "none", event.BackgroundWork.Capabilities.Output)
		require.False(t, event.NormalizedPayload.IsActiveBackgroundWork())
		require.Equal(t, 1, activeClaudeToolCallCount(a), "a sibling job must stay tracked")

		a.mu.RLock()
		_, firstTracked := a.activeToolCalls[first.toolCallID]
		_, secondTracked := a.activeToolCalls[second.toolCallID]
		a.mu.RUnlock()
		require.False(t, firstTracked)
		require.True(t, secondTracked)

		// Duplicate terminal evidence cannot recreate the retired launch.
		duplicate := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(first.toolCallID, nil, nil))
		if duplicate != nil {
			require.Nil(t, duplicate.BackgroundWork)
		}
		require.Equal(t, 1, activeClaudeToolCallCount(a))
	})

	t.Run("numeric exit keeps the existing terminal-code path", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		launch := claudeBackgroundLaunch{
			toolCallID: "toolu_numeric", command: "echo done", title: "Print done",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", launch)

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, float64(0), nil))
		require.NotNil(t, event)
		require.Equal(t, toolStatusComplete, event.ToolStatus)
		require.Nil(t, event.BackgroundWork,
			"numeric evidence keeps the generic normalized projection")
		require.NotNil(t, event.NormalizedPayload.ShellExec().Output)
		require.NotNil(t, event.NormalizedPayload.ShellExec().Output.ExitCode)
		require.Equal(t, 0, *event.NormalizedPayload.ShellExec().Output.ExitCode)
		require.False(t, event.NormalizedPayload.IsActiveBackgroundWork())
	})

	t.Run("fast completion is terminal evidence on the launch result", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		call := &acp.SessionUpdateToolCall{
			ToolCallId:    "toolu_fast",
			Kind:          acp.ToolKindExecute,
			Status:        acp.ToolCallStatusPending,
			Title:         "Terminal",
			SessionUpdate: "tool_call",
			RawInput:      map[string]any{},
		}
		require.NotNil(t, a.convertToolCallUpdate("session-claude", call))

		completed := acp.ToolCallStatusCompleted
		update := claudeTerminalExitUpdate("toolu_fast", nil, nil)
		update.Status = &completed
		update.RawInput = map[string]any{
			"command":           "true",
			"description":       "Fast command",
			"run_in_background": true,
		}
		update.RawOutput = map[string]any{"output": "backgrounded"}
		event := a.convertToolCallResultUpdate("session-claude", update)
		require.NotNil(t, event)
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State)
		require.Nil(t, event.BackgroundWork.ExitCode)
	})

	t.Run("signal termination settles with an unavailable exit code", func(t *testing.T) {
		a := claudeTerminalAdapter(t)
		launch := claudeBackgroundLaunch{
			toolCallID: "toolu_signaled", command: "sleep 300", title: "Long sleep",
		}
		startClaudeBackgroundLaunch(t, a, "session-claude", launch)

		event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, nil, "SIGTERM"))
		require.NotNil(t, event)
		require.False(t, event.ToolStatusMissing)
		require.NotNil(t, event.BackgroundWork)
		require.Equal(t, streams.RunStateEnded, event.BackgroundWork.State)
		require.Nil(t, event.BackgroundWork.ExitCode, "a signal carries no exit code")
	})

	for _, tc := range []struct {
		name         string
		agentID      string
		agentName    string
		agentVersion string
	}{
		{"legacy bridge launch exit", claudeAgentID, verifiedClaudeTerminalBridgeName, "0.81.2"},
		{"unverified bridge version", claudeAgentID, verifiedClaudeTerminalBridgeName, "0.98.0"},
		{"foreign provider", openCodeAgentID, verifiedClaudeTerminalBridgeName, verifiedClaudeTerminalBridgeVersion},
		{"missing connected identity", claudeAgentID, "", ""},
	} {
		t.Run(tc.name+" retains prior behavior", func(t *testing.T) {
			a := claudeTerminalAdapter(t)
			a.agentInfo = &AgentInfo{Name: tc.agentName, Version: tc.agentVersion}
			launch := claudeBackgroundLaunch{
				toolCallID: "toolu_unverified", command: "sleep 30", title: "Sleep",
			}
			startClaudeBackgroundLaunch(t, a, "session-claude", launch)
			// The launch is recognized under the Claude identity; switching the
			// configured provider afterwards isolates the dialect gate.
			a.agentID = tc.agentID

			event := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(launch.toolCallID, nil, nil))
			require.NotNil(t, event)
			require.Nil(t, event.BackgroundWork)
			require.True(t, event.NormalizedPayload.IsActiveBackgroundWork(),
				"unverified evidence must not end the workload")
			require.Equal(t, 1, activeClaudeToolCallCount(a))
		})
	}

	for _, tc := range []struct {
		name  string
		build func(string) *acp.SessionToolCallUpdate
	}{
		{
			name: "mismatched terminal id",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				update := claudeTerminalExitUpdate(toolCallID, nil, nil)
				update.Meta["terminal_exit"].(map[string]any)["terminal_id"] = "toolu_other"
				return update
			},
		},
		{
			name: "missing terminal id",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return &acp.SessionToolCallUpdate{
					ToolCallId:    acp.ToolCallId(toolCallID),
					SessionUpdate: "tool_call_update",
					Meta: map[string]any{"terminal_exit": map[string]any{
						"exit_code": nil, "signal": nil,
					}},
				}
			},
		},
		{
			name: "malformed exit code type",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return claudeTerminalExitUpdate(toolCallID, "zero", nil)
			},
		},
		{
			name: "malformed signal type",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return claudeTerminalExitUpdate(toolCallID, nil, true)
			},
		},
		{
			name: "missing nullable fields",
			build: func(toolCallID string) *acp.SessionToolCallUpdate {
				return &acp.SessionToolCallUpdate{
					ToolCallId:    acp.ToolCallId(toolCallID),
					SessionUpdate: "tool_call_update",
					Meta: map[string]any{"terminal_exit": map[string]any{
						"terminal_id": toolCallID,
					}},
				}
			},
		},
	} {
		t.Run(tc.name+" establishes no termination", func(t *testing.T) {
			a := claudeTerminalAdapter(t)
			launch := claudeBackgroundLaunch{
				toolCallID: "toolu_malformed", command: "sleep 30", title: "Sleep",
			}
			startClaudeBackgroundLaunch(t, a, "session-claude", launch)

			event := a.convertToolCallResultUpdate("session-claude", tc.build(launch.toolCallID))
			require.NotNil(t, event)
			require.Nil(t, event.BackgroundWork)
			a.mu.RLock()
			tracked := a.activeToolCalls[launch.toolCallID]
			a.mu.RUnlock()
			require.NotNil(t, tracked, "the launch binding must survive unrelated evidence")
			require.True(t, tracked.IsActiveBackgroundWork())
			require.Equal(t, 1, activeClaudeToolCallCount(a))
		})
	}
}
