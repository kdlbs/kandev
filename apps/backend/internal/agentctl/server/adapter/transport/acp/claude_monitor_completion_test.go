package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestClaudeMonitorCompletionSource(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "claude-0.107.0-2.1.323-monitor-completion.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(fixture))
	completion, ok := parseClaudeMonitorCompletionRecord(line, "native-session-1", claudeNativeVersion)
	if !ok {
		t.Fatal("qualified task notification was not parsed")
	}
	if completion.TaskID != "monitor-task-1" || completion.ToolUseID != "toolu_monitor_1" {
		t.Fatalf("completion identity = (%q, %q), want (monitor-task-1, toolu_monitor_1)", completion.TaskID, completion.ToolUseID)
	}
	if completion.State != streams.RunStateCompleted {
		t.Fatalf("completion state = %q, want completed", completion.State)
	}
	if completion.FinishedAt.IsZero() {
		t.Fatal("completion timestamp is missing")
	}

	for name, mutate := range map[string]func(map[string]any){
		"wrong session":     func(record map[string]any) { record["sessionId"] = "another-session" },
		"wrong version":     func(record map[string]any) { record["version"] = "2.1.322" },
		"wrong record type": func(record map[string]any) { record["type"] = "progress" },
		"wrong attachment":  func(record map[string]any) { record["attachment"].(map[string]any)["type"] = "other" },
		"wrong origin": func(record map[string]any) {
			record["attachment"].(map[string]any)["origin"].(map[string]any)["kind"] = "human"
		},
		"wrong producer": func(record map[string]any) {
			record["attachment"].(map[string]any)["origin"].(map[string]any)["producer"] = "assistant"
		},
	} {
		t.Run(name, func(t *testing.T) {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatal(err)
			}
			mutate(record)
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := parseClaudeMonitorCompletionRecord(string(encoded), "native-session-1", claudeNativeVersion); ok {
				t.Fatal("unqualified record was accepted")
			}
		})
	}

	for _, test := range []struct {
		status string
		state  streams.RunState
	}{
		{status: "failed", state: streams.RunStateFailed},
		{status: "stopped", state: streams.RunStateInterrupted},
	} {
		t.Run(test.status, func(t *testing.T) {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatal(err)
			}
			attachment := record["attachment"].(map[string]any)
			prompt := attachment["prompt"].(string)
			attachment["prompt"] = strings.Replace(
				prompt,
				"<status>completed</status>",
				"<status>"+test.status+"</status>",
				1,
			)
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			completion, ok := parseClaudeMonitorCompletionRecord(string(encoded), "native-session-1", claudeNativeVersion)
			if !ok || completion.State != test.state {
				t.Fatalf("completion = %+v, accepted = %t, want state %s", completion, ok, test.state)
			}
		})
	}

	if _, ok := parseClaudeMonitorCompletionRecord(strings.TrimSuffix(line, "}"), "native-session-1", claudeNativeVersion); ok {
		t.Fatal("incomplete JSONL record was accepted")
	}
}

func TestClaudeMonitorAttachmentAndUserCompletionDeduplicate(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "claude-0.107.0-2.1.323-monitor-completion.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	attachmentLine := strings.TrimSpace(string(fixture))
	var attachment map[string]any
	if err := json.Unmarshal([]byte(attachmentLine), &attachment); err != nil {
		t.Fatal(err)
	}
	userLine := claudeMonitorUserRecordLine(
		t, attachmentLine, "monitor-task-1", "toolu_monitor_1", time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	)
	attachmentCompletion, ok := parseClaudeMonitorCompletionRecord(
		attachmentLine, "native-session-1", claudeNativeVersion,
	)
	if !ok {
		t.Fatal("qualified queued-command attachment was not parsed")
	}
	userCompletion, ok := parseClaudeMonitorCompletionRecord(
		userLine, "native-session-1", claudeNativeVersion,
	)
	if !ok {
		t.Fatal("qualified user task notification was not parsed")
	}

	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = "native-session-1"
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(attachmentCompletion.TaskID, false)
	seedMonitor(t, a, "native-session-1", attachmentCompletion.TaskID, attachmentCompletion.ToolUseID)
	a.activeToolCalls[attachmentCompletion.ToolUseID] = payload
	a.registerClaudeMonitorCompletion(a.sessionID, attachmentCompletion)
	a.registerClaudeMonitorCompletion(a.sessionID, userCompletion)

	events := drainEvents(a)
	requireClaudeMonitorSettlementEvents(t, events, "monitor-task-1", "toolu_monitor_1", streams.RunStateCompleted)
}

func TestClaudeMonitorCompletionReaderDiscardsOversizedRecordAndContinues(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "claude-0.107.0-2.1.323-monitor-completion.jsonl"))
	require.NoError(t, err)
	attachmentLine := strings.TrimSpace(string(fixture))
	admittedAt := time.Date(2026, 11, 20, 11, 59, 0, 0, time.UTC)
	firstLine := claudeMonitorUserRecordLine(
		t, attachmentLine, "monitor-task-first", "toolu_monitor_first", admittedAt.Add(time.Minute),
	)
	secondLine := claudeMonitorUserRecordLine(
		t, attachmentLine, "monitor-task-second", "toolu_monitor_second", admittedAt.Add(2*time.Minute),
	)

	configDir, workDir := t.TempDir(), t.TempDir()
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, "native-session-1"))
	require.NoError(t, os.MkdirAll(filepath.Dir(project), 0o700))
	require.NoError(t, os.WriteFile(project, nil, 0o600))
	reader, err := newClaudeMonitorCompletionReader(
		configDir, workDir, "native-session-1", admittedAt,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	oversized := strings.Repeat("x", claudeMonitorMaxReadBytes+4096)
	require.NoError(t, os.WriteFile(project, []byte(firstLine+"\n"+oversized), 0o600))
	first, err := reader.ReadAvailable()
	require.NoError(t, err, "an oversized line must be discarded without retiring the reader")
	require.Len(t, first, 1)
	require.Equal(t, "monitor-task-first", first[0].TaskID)

	appendFile, err := os.OpenFile(project, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, writeErr := appendFile.WriteString(oversized[claudeMonitorMaxReadBytes-len(firstLine)-1:] + "\n" + secondLine + "\n")
	require.NoError(t, writeErr)
	require.NoError(t, appendFile.Close())
	second, err := reader.ReadAvailable()
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "monitor-task-second", second[0].TaskID)
}

func TestClaudeMonitorReaderAdmissionHonorsFeatureFlagAndWaitsForMonitorLaunch(t *testing.T) {
	const sessionID = "native-session-1"
	configDir, workDir := t.TempDir(), t.TempDir()
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, sessionID))
	require.NoError(t, os.MkdirAll(filepath.Dir(project), 0o700))
	require.NoError(t, os.WriteFile(project, nil, 0o600))

	disabled := newClaudeMonitorCompletionTestAdapter(t, configDir, workDir, sessionID, false)
	if state := disabled.beginClaudeMonitorPrompt(sessionID); state != nil {
		t.Fatal("disabled Background Work admitted a native transcript reader")
	}

	a := newClaudeMonitorCompletionTestAdapter(t, configDir, workDir, sessionID, true)
	state := a.beginClaudeMonitorPrompt(sessionID)
	require.NotNil(t, state, "enabled Background Work keeps the prompt-start cursor")
	admittedAt := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	state.admittedAt = admittedAt
	state.reader.admittedAt = admittedAt
	line := claudeMonitorUserRecordLine(
		t, claudeMonitorFixtureLine(t), "monitor-task-early", "toolu_monitor_early", admittedAt.Add(time.Second),
	)
	appendFile, err := os.OpenFile(project, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, writeErr := appendFile.WriteString(line + "\n")
	require.NoError(t, writeErr)
	require.NoError(t, appendFile.Close())

	// This named delay spans multiple native-reader polling intervals. A prompt
	// without a Monitor must not scan transcript records while it waits.
	timer := time.NewTimer(3 * claudeMonitorPollInterval)
	<-timer.C
	a.mu.RLock()
	pendingBeforeLaunch := len(a.pendingClaudeMonitorCompletions[sessionID])
	a.mu.RUnlock()
	require.Zero(t, pendingBeforeLaunch, "the cursor must not poll before a Monitor launch")

	toolCallID := "toolu_monitor_early"
	call := &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "gh pr checks --watch"},
	}
	require.NotNil(t, a.convertToolCallUpdate(sessionID, call), "Monitor launch must start the retained reader")
	require.Eventually(t, func() bool {
		a.mu.RLock()
		defer a.mu.RUnlock()
		return len(a.pendingClaudeMonitorCompletions[sessionID]) == 1
	}, time.Second, 10*time.Millisecond, "the early native completion should be held until registration")

	status := acp.ToolCallStatus(toolStatusCompleted)
	registration := a.convertToolCallResultUpdate(sessionID, &acp.SessionToolCallUpdate{
		ToolCallId: acp.ToolCallId(toolCallID),
		Status:     &status,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task monitor-task-early, timeout 60000ms).",
	})
	require.NotNil(t, registration)
	events := drainEvents(a)
	require.Len(t, events, 1)
	require.Equal(t, streams.EventTypeBackgroundWorkUpdated, events[0].Type)
	require.Equal(t, streams.RunStateCompleted, events[0].BackgroundWork.State)
	a.endClaudeMonitorPrompt(sessionID, state)
}

func newClaudeMonitorCompletionTestAdapter(
	t *testing.T,
	configDir, workDir, sessionID string,
	enabled bool,
) *Adapter {
	t.Helper()
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.cfg.ClaudeConfigDir = configDir
	a.cfg.WorkDir = workDir
	a.cfg.AgentBackgroundWorkEnabled = enabled
	a.backgroundWorkEnabled = enabled
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	return a
}

func claudeMonitorFixtureLine(t *testing.T) string {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", "claude-0.107.0-2.1.323-monitor-completion.jsonl"))
	require.NoError(t, err)
	return strings.TrimSpace(string(fixture))
}

func claudeMonitorUserRecordLine(t *testing.T, attachmentLine, taskID, toolUseID string, timestamp time.Time) string {
	t.Helper()
	var attachment map[string]any
	require.NoError(t, json.Unmarshal([]byte(attachmentLine), &attachment))
	attachmentFields := attachment["attachment"].(map[string]any)
	prompt := attachmentFields["prompt"].(string)
	prompt = strings.Replace(prompt, "<task-id>monitor-task-1</task-id>", "<task-id>"+taskID+"</task-id>", 1)
	prompt = strings.Replace(prompt, "<tool-use-id>toolu_monitor_1</tool-use-id>", "<tool-use-id>"+toolUseID+"</tool-use-id>", 1)
	userRecord := map[string]any{
		"type":      "user",
		"sessionId": attachment["sessionId"],
		"version":   attachment["version"],
		"timestamp": timestamp.UTC().Format(time.RFC3339Nano),
		"origin":    attachmentFields["origin"],
		"message": map[string]any{
			"role":    "user",
			"content": prompt,
		},
	}
	encoded, err := json.Marshal(userRecord)
	require.NoError(t, err)
	return string(encoded)
}

func TestClaudeMonitorCompletionObservationUsesTrackedIdentity(t *testing.T) {
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-1", "toolu_monitor_1"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	seedMonitor(t, a, sessionID, taskID, toolCallID)
	a.activeToolCalls[toolCallID] = payload
	for _, mismatch := range []claudeMonitorCompletion{
		{
			SessionID: sessionID, TaskID: "another-task", ToolUseID: toolCallID,
			State: streams.RunStateCompleted, FinishedAt: time.Now().UTC(),
		},
		{
			SessionID: sessionID, TaskID: taskID, ToolUseID: "another-tool-call",
			State: streams.RunStateCompleted, FinishedAt: time.Now().UTC(),
		},
	} {
		a.registerClaudeMonitorCompletion(sessionID, mismatch)
	}
	if events := drainEvents(a); len(events) != 0 || !payload.IsActiveMonitor() {
		t.Fatalf("mismatched completion changed the live Monitor: events=%+v payload=%+v", events, payload)
	}
	a.registerClaudeMonitorCompletion(sessionID, claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      streams.RunStateFailed,
		FinishedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	})
	a.registerClaudeMonitorCompletion(sessionID, claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      streams.RunStateFailed,
		FinishedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	})

	events := drainEvents(a)
	requireClaudeMonitorSettlementEvents(t, events, taskID, toolCallID, streams.RunStateFailed)
	if payload.IsActiveMonitor() || a.isTrackedMonitor(sessionID, toolCallID) {
		t.Fatal("completed Monitor remains active")
	}
	if _, ok := a.activeToolCalls[toolCallID]; ok {
		t.Fatal("terminal Monitor payload remains in active tool calls")
	}
}

func TestClaudeMonitorInterruptedCompletionMarksSourceCardCancelled(t *testing.T) {
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-interrupted", "toolu_monitor_interrupted"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	seedMonitor(t, a, sessionID, taskID, toolCallID)
	a.activeToolCalls[toolCallID] = payload

	a.registerClaudeMonitorCompletion(sessionID, claudeMonitorCompletion{
		SessionID: sessionID, TaskID: taskID, ToolUseID: toolCallID,
		State: streams.RunStateInterrupted, FinishedAt: time.Now().UTC(),
	})

	requireClaudeMonitorSettlementEvents(t, drainEvents(a), taskID, toolCallID, streams.RunStateInterrupted)
}

func TestClaudeMonitorCompletionBeforeRegistrationSettlesMatchingMonitor(t *testing.T) {
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-1", "toolu_monitor_1"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	readerState := &claudeMonitorReaderState{cancel: func() {}, activePrompts: 1, started: true}
	readerState.generation.Store(1)
	a.claudeMonitorReaders[sessionID] = readerState
	a.registerClaudeMonitorCompletion(sessionID, claudeMonitorCompletion{
		SessionID: sessionID, TaskID: taskID, ToolUseID: toolCallID,
		State: streams.RunStateCompleted, FinishedAt: time.Now().UTC(),
	})
	if len(a.pendingClaudeMonitorCompletions[sessionID]) != 1 {
		t.Fatal("completion arriving before registration was not held for the active prompt")
	}

	initial := &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(toolCallID),
		Title:      monitorToolName,
		Kind:       acp.ToolKind("other"),
		Meta:       monitorMeta(),
		RawInput:   map[string]any{"command": "gh pr checks --watch"},
	}
	if event := a.convertToolCallUpdate(sessionID, initial); event == nil {
		t.Fatal("initial Monitor tool call was not registered")
	}
	status := acp.ToolCallStatus(toolStatusCompleted)
	event := a.convertToolCallResultUpdate(sessionID, &acp.SessionToolCallUpdate{
		ToolCallId: toolCallID,
		Status:     &status,
		Meta:       monitorMeta(),
		RawOutput:  "Monitor started (task " + taskID + ", timeout 60000ms).",
	})
	if event == nil || event.ToolStatus != toolStatusComplete || event.BackgroundWork != nil {
		t.Fatalf("early completion registration event = %+v", event)
	}
	require.True(t, event.NormalizedPayload.Monitor().Ended)
	events := drainEvents(a)
	require.Len(t, events, 1)
	require.Equal(t, streams.EventTypeBackgroundWorkUpdated, events[0].Type)
	require.Equal(t, streams.RunStateCompleted, events[0].BackgroundWork.State)
	if a.isTrackedMonitor(sessionID, toolCallID) {
		t.Fatal("early terminal evidence left its Monitor active")
	}
}

func requireClaudeMonitorSettlementEvents(
	t *testing.T,
	events []AgentEvent,
	taskID, toolCallID string,
	state streams.RunState,
) {
	t.Helper()
	require.Len(t, events, 2, "native completion updates both the workload and its source Monitor card")
	var workloadUpdate, cardUpdate *AgentEvent
	for i := range events {
		event := &events[i]
		switch event.Type {
		case streams.EventTypeBackgroundWorkUpdated:
			workloadUpdate = event
		case streams.EventTypeToolUpdate:
			cardUpdate = event
		}
	}
	require.NotNil(t, workloadUpdate)
	require.NotNil(t, workloadUpdate.BackgroundWork)
	require.Equal(t, taskID, workloadUpdate.BackgroundWork.WorkID)
	require.Equal(t, toolCallID, workloadUpdate.BackgroundWork.SourceCallID)
	require.Equal(t, state, workloadUpdate.BackgroundWork.State)
	require.Empty(t, workloadUpdate.ToolCallID)
	require.Empty(t, workloadUpdate.ToolStatus)

	require.NotNil(t, cardUpdate)
	require.Equal(t, toolCallID, cardUpdate.ToolCallID)
	require.Nil(t, cardUpdate.BackgroundWork, "card presentation must not settle the workload projection")
	wantStatus, _ := claudeMonitorToolStatus(state)
	require.Equal(t, wantStatus, cardUpdate.ToolStatus)
	require.NotNil(t, cardUpdate.NormalizedPayload)
	monitor := cardUpdate.NormalizedPayload.Monitor()
	require.NotNil(t, monitor)
	require.Equal(t, taskID, monitor.TaskID)
	require.True(t, monitor.Ended)
}

func TestClaudeMonitorCompletionFromPreviousSessionGenerationIsIgnored(t *testing.T) {
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-1", "toolu_monitor_1"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	seedMonitor(t, a, sessionID, taskID, toolCallID)
	a.activeToolCalls[toolCallID] = payload

	a.mu.Lock()
	a.advanceNativeGenerationLocked()
	a.mu.Unlock()
	a.resetClaudeMonitorReadersForSessionTransition(sessionID, sessionID)
	a.registerClaudeMonitorCompletionForGeneration(sessionID, 1, claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      streams.RunStateCompleted,
		FinishedAt: time.Now().UTC(),
	})

	if events := drainEvents(a); len(events) != 0 || !payload.IsActiveMonitor() || !a.isTrackedMonitor(sessionID, toolCallID) {
		t.Fatalf("previous generation settled the current Monitor: events=%+v payload=%+v", events, payload)
	}
}

func TestClaudeMonitorReaderRebindsOnSameSessionResume(t *testing.T) {
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-1", "toolu_monitor_1"
	a := newTestAdapterForAgent(claudeAgentID)
	t.Cleanup(func() { _ = a.Close() })
	a.sessionID = sessionID
	a.nativeGeneration = 1
	a.agentInfo = &AgentInfo{Name: claudeMonitorEvidenceBridgeName, Version: claudeMonitorEvidenceBridgeBuild}
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	seedMonitor(t, a, sessionID, taskID, toolCallID)
	a.activeToolCalls[toolCallID] = payload
	readerState := &claudeMonitorReaderState{cancel: func() {}, started: true}
	readerState.generation.Store(1)
	a.claudeMonitorReaders[sessionID] = readerState
	parsedGeneration := readerState.generation.Load()

	a.mu.Lock()
	a.advanceNativeGenerationLocked()
	a.rebindClaudeMonitorReaderGenerationLocked(sessionID, a.nativeGeneration)
	a.mu.Unlock()
	require.Equal(t, uint64(1), parsedGeneration)
	a.registerClaudeMonitorCompletionFromReader(sessionID, readerState, claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      streams.RunStateCompleted,
		FinishedAt: time.Now().UTC(),
	})

	events := drainEvents(a)
	if len(events) != 2 || payload.IsActiveMonitor() {
		t.Fatalf("same-session resume did not retain the reader binding: events=%+v payload=%+v", events, payload)
	}
	requireClaudeMonitorSettlementEvents(t, events, taskID, toolCallID, streams.RunStateCompleted)
}

func TestClaudeMonitorCompletionReaderUsesAppendWindowAndStableFileIdentity(t *testing.T) {
	configDir := t.TempDir()
	workDir := t.TempDir()
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, "native-session-1"))
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("old history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	admittedAt := time.Date(2026, 11, 20, 11, 59, 0, 0, time.UTC)
	reader, err := newClaudeMonitorCompletionReader(
		configDir,
		workDir,
		"native-session-1",
		admittedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	fixture, err := os.ReadFile(filepath.Join("testdata", "claude-0.107.0-2.1.323-monitor-completion.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(fixture))
	file, err := os.OpenFile(project, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	partial := line[:len(line)/2]
	if _, err := file.WriteString(partial); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if completions, err := reader.ReadAvailable(); err != nil || len(completions) != 0 {
		t.Fatalf("partial record read = (%+v, %v), want no completion", completions, err)
	}
	file, err = os.OpenFile(project, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(line[len(partial):] + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	completions, err := reader.ReadAvailable()
	if err != nil || len(completions) != 1 {
		t.Fatalf("completed append read = (%+v, %v), want one completion", completions, err)
	}
	if completions[0].TaskID != "monitor-task-1" {
		t.Fatalf("completion task ID = %q, want monitor-task-1", completions[0].TaskID)
	}

	backup := project + ".old"
	if err := os.Rename(project, backup); err != nil {
		t.Fatal(err)
	}
	replacementLine := claudeMonitorUserRecordLine(
		t, line, "monitor-task-replacement", "toolu_monitor_replacement", admittedAt.Add(time.Minute),
	)
	if err := os.WriteFile(project, []byte(replacementLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadAvailable(); err == nil {
		t.Fatal("reader did not report the replacement boundary")
	}
	completions, err = reader.ReadAvailable()
	if err != nil || len(completions) != 1 || completions[0].TaskID != "monitor-task-replacement" {
		t.Fatalf("reader did not resume after a regular file replacement: completions=%+v err=%v", completions, err)
	}
}

func TestClaudeMonitorCompletionReaderRecoversAfterTranscriptTruncation(t *testing.T) {
	configDir, workDir := t.TempDir(), t.TempDir()
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, "native-session-1"))
	require.NoError(t, os.MkdirAll(filepath.Dir(project), 0o700))
	require.NoError(t, os.WriteFile(project, []byte("old history\n"), 0o600))
	admittedAt := time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC)
	reader, err := newClaudeMonitorCompletionReader(configDir, workDir, "native-session-1", admittedAt)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	fixture := claudeMonitorFixtureLine(t)
	firstLine := claudeMonitorUserRecordLine(
		t, fixture, "monitor-task-before-truncation", "toolu_monitor_before_truncation", admittedAt.Add(time.Second),
	)
	secondLine := claudeMonitorUserRecordLine(
		t, fixture, "monitor-task-after-truncation", "toolu_monitor_after_truncation", admittedAt.Add(2*time.Second),
	)
	appendFile, err := os.OpenFile(project, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = appendFile.WriteString(firstLine + "\n")
	require.NoError(t, err)
	require.NoError(t, appendFile.Close())
	first, err := reader.ReadAvailable()
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "monitor-task-before-truncation", first[0].TaskID)
	require.Less(t, len(secondLine), len(firstLine)+1, "the rewrite must reduce the file below the retained offset")
	require.NoError(t, os.WriteFile(project, []byte(secondLine+"\n"), 0o600))
	if _, err := reader.ReadAvailable(); err == nil {
		t.Fatal("reader did not report the truncation boundary")
	}
	second, err := reader.ReadAvailable()
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "monitor-task-after-truncation", second[0].TaskID)
}
