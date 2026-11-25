package acp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

const cursorShellWatchSessionID = "33333333-3333-4333-8333-333333333333"

// @covers AC-AGENTS-BACKGROUND-WORK-011.1
// @covers AC-AGENTS-BACKGROUND-WORK-011.2
// @covers AC-AGENTS-BACKGROUND-WORK-011.4
func TestCursorShellWatchLifecycle(t *testing.T) {
	fixture, err := os.ReadFile("testdata/cursor-shell-watch.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	a := newCursorShellWatchTestAdapter(t, true, cursorShellWatchSessionID, 1)

	var observations []streams.WorkloadRunObservation
	var toolStatuses []string
	for index, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		var notification acp.SessionNotification
		if err := json.Unmarshal([]byte(line), &notification); err != nil {
			t.Fatalf("decode notification %d: %v", index, err)
		}
		a.handleACPUpdate(notification, 41)
		for _, event := range drainEvents(a) {
			if event.Type == streams.EventTypeBackgroundWorkUpdated && event.BackgroundWork != nil {
				observations = append(observations, *event.BackgroundWork)
			}
			if event.Type == streams.EventTypeToolCall || event.Type == streams.EventTypeToolUpdate {
				toolStatuses = append(toolStatuses, event.ToolStatus)
				if event.NormalizedPayload == nil || event.NormalizedPayload.Kind() != streams.ToolKindGeneric {
					t.Fatalf("transcript event %d payload = %+v, want generic tool payload", index, event.NormalizedPayload)
				}
				if event.NormalizedPayload.BackgroundWork() != nil {
					t.Fatalf("Cursor watch changed transcript attestation at frame %d", index)
				}
			}
		}
		if index == 0 && len(observations) != 0 {
			t.Fatal("pending registration created a running observation")
		}
	}

	if len(observations) != 4 {
		t.Fatalf("Background Work observations = %d, want one running and one completed per watch: %+v", len(observations), observations)
	}
	if got, want := strings.Join(toolStatuses, ","), "pending,in_progress,complete,pending,in_progress,complete"; got != want {
		t.Fatalf("tool statuses = %s, want %s", got, want)
	}
	for index, watch := range []struct {
		callID string
		title  string
	}{
		{callID: "watch-11111111-1111-4111-8111-111111111111", title: "Watch first example script — /tmp/example-watch.sh"},
		{callID: "watch-22222222-2222-4222-8222-222222222222", title: "Second example script finished — /tmp/example-watch.sh"},
	} {
		running, completed := observations[index*2], observations[index*2+1]
		if running.State != streams.RunStateRunning || completed.State != streams.RunStateCompleted {
			t.Fatalf("watch %s observations = %s then %s, want running then completed", watch.callID, running.State, completed.State)
		}
		if running.Title != watch.title || completed.Title != watch.title {
			t.Fatalf("watch %s titles = %q and %q, want %q", watch.callID, running.Title, completed.Title, watch.title)
		}
		if running.WorkID == "" || running.WorkID != running.RunID || completed.WorkID != running.WorkID {
			t.Fatalf("watch %s identity changed: running=%+v completed=%+v", watch.callID, running, completed)
		}
		if running.SourceCallID != watch.callID || completed.SourceCallID != watch.callID {
			t.Fatalf("watch %s source call ids = %q and %q", watch.callID, running.SourceCallID, completed.SourceCallID)
		}
		if running.SessionID != cursorShellWatchSessionID || running.OriginTurnID != "" {
			t.Fatalf("watch %s session/origin = %q/%q, want captured session and unresolved origin", watch.callID, running.SessionID, running.OriginTurnID)
		}
		if running.Capabilities.Discovery != "events_only" || running.Capabilities.Output != "none" || len(running.Capabilities.Actions) != 0 || running.ExitCode != nil {
			t.Fatalf("watch %s capabilities/exit = %+v/%v, want inspection only and no exit code", watch.callID, running.Capabilities, running.ExitCode)
		}
		if running.State != streams.RunStateRunning || completed.State != streams.RunStateCompleted {
			t.Fatalf("watch status = %s/%s", running.State, completed.State)
		}
		if completed.FinishedAt == nil {
			t.Fatalf("watch %s completion has no finish time", watch.callID)
		}
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.2
func TestCursorShellWatchLifecycleInterleavesAndKeepsTerminalState(t *testing.T) {
	const sessionID = "session-interleaved-watches"
	a := newCursorShellWatchTestAdapter(t, true, sessionID, 1)
	const firstCall, secondCall = "watch-first", "watch-second"

	for _, callID := range []string{firstCall, secondCall} {
		events := handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
			sessionID, callID, "shared-shell", "Wait for shared shell", "/tmp/shared.sh",
		))
		if got := backgroundObservations(events); len(got) != 0 {
			t.Fatalf("pending registration %s emitted observations: %+v", callID, got)
		}
	}

	var running []streams.WorkloadRunObservation
	for _, callID := range []string{firstCall, secondCall} {
		events := handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
			sessionID, callID, acp.ToolCallStatusInProgress,
		))
		running = append(running, backgroundObservations(events)...)
	}
	if len(running) != 2 || running[0].WorkID == running[1].WorkID {
		t.Fatalf("interleaved running observations = %+v, want distinct workloads for shared shell", running)
	}

	terminal := handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, firstCall, acp.ToolCallStatusCompleted,
	))
	terminalObservations := backgroundObservations(terminal)
	if len(terminalObservations) != 1 || terminalObservations[0].WorkID != running[0].WorkID ||
		terminalObservations[0].State != streams.RunStateCompleted {
		t.Fatalf("first watch completion = %+v, want only first watch terminal", terminalObservations)
	}
	for _, update := range []acp.SessionNotification{
		cursorShellWatchUpdateNotification(sessionID, firstCall, acp.ToolCallStatusInProgress),
		cursorShellWatchUpdateNotification(sessionID, firstCall, acp.ToolCallStatusCompleted),
		cursorShellWatchRegistrationNotification(sessionID, firstCall, "shared-shell", "Replacement title", "/tmp/replacement.sh"),
	} {
		if got := backgroundObservations(handleCursorShellWatchFrame(a, update)); len(got) != 0 {
			t.Fatalf("duplicate or delayed frame reopened a terminal watch: %+v", got)
		}
	}
	secondTerminal := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, secondCall, acp.ToolCallStatusCompleted,
	)))
	if len(secondTerminal) != 1 || secondTerminal[0].WorkID != running[1].WorkID ||
		secondTerminal[0].State != streams.RunStateCompleted {
		t.Fatalf("sibling completion = %+v, want only second watch terminal", secondTerminal)
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.6
func TestCursorShellWatchFailureKeepsSiblingRunningAndRejectsLateProgress(t *testing.T) {
	const sessionID = "session-failed-watches"
	a := newCursorShellWatchTestAdapter(t, true, sessionID, 1)
	const failedCall, siblingCall = "watch-failed", "watch-sibling"
	for _, callID := range []string{failedCall, siblingCall} {
		handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
			sessionID, callID, "shared-shell", "Wait for shared shell", "/tmp/shared.sh",
		))
	}
	failedRunning := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, failedCall, acp.ToolCallStatusInProgress,
	)))
	siblingRunning := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, siblingCall, acp.ToolCallStatusInProgress,
	)))
	if len(failedRunning) != 1 || len(siblingRunning) != 1 {
		t.Fatalf("running observations = %+v and %+v, want both watches running", failedRunning, siblingRunning)
	}

	failed := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, failedCall, acp.ToolCallStatusFailed,
	)))
	if len(failed) != 1 || failed[0].WorkID != failedRunning[0].WorkID || failed[0].State != streams.RunStateFailed {
		t.Fatalf("failed watch observation = %+v, want failed workload %q", failed, failedRunning[0].WorkID)
	}
	if failed[0].FinishedAt == nil || failed[0].ExitCode != nil {
		t.Fatalf("failed watch finish/exit = %v/%v, want finish time and no shell exit code", failed[0].FinishedAt, failed[0].ExitCode)
	}

	lateProgress := handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		sessionID, failedCall, acp.ToolCallStatusInProgress,
	))
	if observations := backgroundObservations(lateProgress); len(observations) != 0 {
		t.Fatalf("late progress reopened failed watch: %+v", observations)
	}
	a.mu.RLock()
	failedState := a.cursorShellWatches[failedCall].state
	siblingState := a.cursorShellWatches[siblingCall].state
	a.mu.RUnlock()
	if failedState != streams.RunStateFailed || siblingState != streams.RunStateRunning {
		t.Fatalf("watch states after failure = %s/%s, want failed/running", failedState, siblingState)
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.3
func TestCursorShellWatchRejectsUnsupportedFrames(t *testing.T) {
	validShell := []any{map[string]any{
		"shellId": "shell-valid", "reason": "Wait for the script", "title": "/tmp/wait.sh",
	}}
	missingShellID := []any{
		map[string]any{"shellId": "shell-valid", "title": "/tmp/valid.sh"},
		map[string]any{"reason": "missing reference", "title": "/tmp/missing.sh"},
	}
	for _, test := range []struct {
		name    string
		agentID string
		enabled bool
		callID  string
		title   string
		shells  []any
		unbound bool
	}{
		{name: "disabled feature", agentID: cursorAgentID, callID: "watch-disabled", title: "Watching: disabled", shells: validShell},
		{name: "foreign provider", agentID: claudeAgentID, enabled: true, callID: "watch-foreign", title: "Watching: foreign", shells: validShell},
		{name: "misleading title", agentID: cursorAgentID, enabled: true, callID: "watch-misleading", title: "Watching: title only"},
		{name: "one missing shell ID", agentID: cursorAgentID, enabled: true, callID: "watch-malformed", title: "Watching: malformed", shells: missingShellID},
		{name: "non-watch call ID", agentID: cursorAgentID, enabled: true, callID: "tool-ordinary", title: "Watching: ordinary", shells: validShell},
		{name: "unbound session", agentID: cursorAgentID, enabled: true, callID: "watch-unbound", title: "Watching: unbound", shells: validShell, unbound: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sessionID := "session-unsupported"
			if test.unbound {
				sessionID = ""
			}
			a := newShellWatchTestAdapter(t, test.agentID, test.enabled, sessionID, 1)
			registration := cursorShellWatchRegistrationNotification(
				sessionID, test.callID, "shell-unsupported", "", "",
			)
			registration.Update.ToolCall.Title = test.title
			if test.shells == nil {
				registration.Update.ToolCall.RawInput = map[string]any{"command": "ordinary call"}
			} else {
				registration.Update.ToolCall.RawInput = map[string]any{"shells": test.shells}
			}
			frames := []acp.SessionNotification{
				registration,
				cursorShellWatchUpdateNotification(sessionID, test.callID, acp.ToolCallStatusInProgress),
			}
			var transcriptCalls int
			for _, frame := range frames {
				for _, event := range handleCursorShellWatchFrame(a, frame) {
					if event.Type == streams.EventTypeBackgroundWorkUpdated {
						t.Fatalf("unsupported frame emitted a Background Work observation: %+v", event.BackgroundWork)
					}
					if event.Type == streams.EventTypeToolCall && event.NormalizedPayload != nil &&
						event.NormalizedPayload.Kind() == streams.ToolKindGeneric {
						transcriptCalls++
					}
				}
			}
			if transcriptCalls != 1 {
				t.Fatalf("generic transcript calls = %d, want 1", transcriptCalls)
			}
		})
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.2
func TestCursorShellWatchIdentity(t *testing.T) {
	const callID = "watch-reused-call-id"
	a := newShellWatchTestAdapter(t, cursorAgentID, true, "session-original", 1)
	handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
		"session-original", callID, "shell-1", "Original watch", "/tmp/original.sh",
	))
	first := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		"session-original", callID, acp.ToolCallStatusInProgress,
	)))
	if len(first) != 1 {
		t.Fatalf("original watch observations = %+v", first)
	}
	wrongSession := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		"session-other", callID, acp.ToolCallStatusCompleted,
	)))
	if len(wrongSession) != 0 {
		t.Fatalf("wrong-session update emitted observations: %+v", wrongSession)
	}

	a.mu.Lock()
	a.advanceNativeGenerationLocked("session-replacement")
	a.sessionID = "session-replacement"
	a.mu.Unlock()
	oldGeneration := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		"session-original", callID, acp.ToolCallStatusCompleted,
	)))
	if len(oldGeneration) != 0 {
		t.Fatalf("old-generation update emitted observations: %+v", oldGeneration)
	}
	handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
		"session-replacement", callID, "shell-1", "Replacement watch", "/tmp/original.sh",
	))
	replacement := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		"session-replacement", callID, acp.ToolCallStatusInProgress,
	)))
	if len(replacement) != 1 || replacement[0].WorkID == first[0].WorkID {
		t.Fatalf("replacement identity = %+v, original work ID %q", replacement, first[0].WorkID)
	}

	otherAdapter := newShellWatchTestAdapter(t, cursorAgentID, true, "session-replacement", 1)
	handleCursorShellWatchFrame(otherAdapter, cursorShellWatchRegistrationNotification(
		"session-replacement", callID, "shell-1", "Replacement watch", "/tmp/original.sh",
	))
	separateAdapter := backgroundObservations(handleCursorShellWatchFrame(otherAdapter, cursorShellWatchUpdateNotification(
		"session-replacement", callID, acp.ToolCallStatusInProgress,
	)))
	if len(separateAdapter) != 1 || separateAdapter[0].WorkID == replacement[0].WorkID {
		t.Fatalf("replacement adapter identity = %+v, prior work ID %q", separateAdapter, replacement[0].WorkID)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("close adapter: %v", err)
	}
	a.mu.RLock()
	remaining := len(a.cursorShellWatches)
	a.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("closed adapter retained %d Cursor shell watches", remaining)
	}
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.8
func TestCursorShellWatchNativeResetRetiresRunningBinding(t *testing.T) {
	a, fake := newSessionRequestCaptureAdapter(t, acp.McpCapabilities{})
	a.agentID = cursorAgentID
	a.dialect = newCursorACPDialect()
	a.normalizer = NewNormalizer(cursorAgentID)
	a.backgroundWorkEnabled = true

	priorSessionID, err := a.NewSession(t.Context(), nil)
	if err != nil {
		t.Fatalf("create prior native session: %v", err)
	}
	drainEvents(a)
	const callID = "watch-reused-across-reset"
	handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
		priorSessionID, callID, "shell-reset", "Wait through reset", "/tmp/reset.sh",
	))
	running := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		priorSessionID, callID, acp.ToolCallStatusInProgress,
	)))
	if len(running) != 1 {
		t.Fatalf("pre-reset watch observation = %+v, want running", running)
	}
	const completedCallID = "watch-completed-before-reset"
	handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
		priorSessionID, completedCallID, "shell-completed", "Already completed", "/tmp/done.sh",
	))
	completedRunning := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		priorSessionID, completedCallID, acp.ToolCallStatusInProgress,
	)))
	if len(completedRunning) != 1 {
		t.Fatalf("pre-reset terminal watch observation = %+v, want running", completedRunning)
	}
	completed := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		priorSessionID, completedCallID, acp.ToolCallStatusCompleted,
	)))
	if len(completed) != 1 || completed[0].State != streams.RunStateCompleted {
		t.Fatalf("terminal watch observation = %+v, want completed before reset", completed)
	}

	newSessionID, err := a.ResetSession(t.Context(), nil)
	if err != nil {
		t.Fatalf("reset native session: %v", err)
	}
	if newSessionID == priorSessionID || fake.sessionCounter != 2 || a.acpConn == nil {
		t.Fatalf("native reset = %q after %q, sessions created %d, connection live %t; want retained process and a new session",
			newSessionID, priorSessionID, fake.sessionCounter, a.acpConn != nil)
	}
	var retired *AgentEvent
	for _, event := range drainEvents(a) {
		if event.Type != streams.EventTypeBackgroundWorkUpdated || event.BackgroundWork == nil {
			continue
		}
		if event.BackgroundWork.WorkID == completedRunning[0].WorkID {
			t.Fatalf("reset replaced terminal evidence with another observation: %+v", event.BackgroundWork)
		}
		if event.BackgroundWork.WorkID == running[0].WorkID {
			copy := event
			retired = &copy
		}
	}
	if retired == nil || retired.BackgroundWork.State != streams.RunStateUnknown {
		t.Fatalf("retired watch event = %+v, want Unknown after committed reset", retired)
	}
	if retired.SessionID != newSessionID || retired.BackgroundWork.SourceCallID != callID || retired.BackgroundWork.ExitCode != nil {
		t.Fatalf("retired watch binding = session %q observation %+v, want reset session fence and no exit code",
			retired.SessionID, retired.BackgroundWork)
	}

	handleCursorShellWatchFrame(a, cursorShellWatchRegistrationNotification(
		newSessionID, callID, "shell-reset", "Successor watch", "/tmp/reset.sh",
	))
	successor := backgroundObservations(handleCursorShellWatchFrame(a, cursorShellWatchUpdateNotification(
		newSessionID, callID, acp.ToolCallStatusInProgress,
	)))
	if len(successor) != 1 || successor[0].State != streams.RunStateRunning || successor[0].WorkID == running[0].WorkID {
		t.Fatalf("successor watch = %+v, want independent running workload from %q", successor, running[0].WorkID)
	}
}

func newShellWatchTestAdapter(
	t *testing.T,
	agentID string,
	enabled bool,
	sessionID string,
	nativeGeneration uint64,
) *Adapter {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	a := NewAdapter(&shared.Config{
		AgentID:                    agentID,
		WorkDir:                    "/tmp/test",
		AgentBackgroundWorkEnabled: enabled,
	}, log)
	a.sessionID = sessionID
	a.nativeGeneration = nativeGeneration
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func cursorShellWatchRegistrationNotification(
	sessionID, callID string,
	shellID, reason, title string,
) acp.SessionNotification {
	return makeNotification(sessionID, acp.SessionUpdate{
		ToolCall: &acp.SessionUpdateToolCall{
			SessionUpdate: "tool_call", ToolCallId: acp.ToolCallId(callID), Title: "Watching: " + reason,
			Kind: acp.ToolKind("other"), Status: acp.ToolCallStatusPending,
			RawInput: map[string]any{"shells": []any{map[string]any{
				"shellId": shellID, "reason": reason, "title": title,
			}}},
		},
	})
}

func cursorShellWatchUpdateNotification(
	sessionID, callID string,
	status acp.ToolCallStatus,
) acp.SessionNotification {
	return makeNotification(sessionID, acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{
			SessionUpdate: acpSessionUpdateToolCallUpdate, ToolCallId: acp.ToolCallId(callID), Status: &status,
		},
	})
}

func handleCursorShellWatchFrame(a *Adapter, notification acp.SessionNotification) []AgentEvent {
	a.handleACPUpdate(notification, 41)
	return drainEvents(a)
}

func backgroundObservations(events []AgentEvent) []streams.WorkloadRunObservation {
	var observations []streams.WorkloadRunObservation
	for _, event := range events {
		if event.Type == streams.EventTypeBackgroundWorkUpdated && event.BackgroundWork != nil {
			observations = append(observations, *event.BackgroundWork)
		}
	}
	return observations
}

func newCursorShellWatchTestAdapter(t *testing.T, enabled bool, sessionID string, nativeGeneration uint64) *Adapter {
	return newShellWatchTestAdapter(t, cursorAgentID, enabled, sessionID, nativeGeneration)
}
