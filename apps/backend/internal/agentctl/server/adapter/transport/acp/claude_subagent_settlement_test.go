package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-003.1, AC-AGENTS-SUBAGENT-LIFECYCLE-003.2, AC-AGENTS-SUBAGENT-LIFECYCLE-003.3, AC-AGENTS-SUBAGENT-LIFECYCLE-003.4, AC-AGENTS-SUBAGENT-LIFECYCLE-003.5, AC-AGENTS-SUBAGENT-LIFECYCLE-003.6
func TestClaudeSubagentSettlementReadsReportAndPreservesLiveness(t *testing.T) {
	adapter := newClaudeSubagentTestAdapter(t)
	reportPath := filepath.Join(t.TempDir(), "agent-123.output")
	contents := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"earlier report"}]}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"continue"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Final answer."},{"type":"text","text":"Second paragraph."},{"type":"tool_use","name":"Read"}]}}`,
	}, "\n")
	if err := os.WriteFile(reportPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", reportPath, true, 7)
	backgroundBefore := payload.BackgroundWork()
	if backgroundBefore == nil || !payload.IsActiveBackgroundWork() {
		t.Fatal("expected the launch to retain its existing active background identity")
	}

	adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
	update := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if update == nil || update.Type != streams.EventTypeToolUpdate {
		t.Fatalf("settlement did not emit a tool update: %+v", update)
	}
	subagent := update.NormalizedPayload.SubagentTask()
	if subagent.Status != "completed" || subagent.ResultText != "Final answer.\nSecond paragraph." {
		t.Fatalf("settled payload = %+v", subagent)
	}
	if !reflect.DeepEqual(backgroundBefore, update.NormalizedPayload.BackgroundWork()) ||
		!payload.IsActiveBackgroundWork() {
		t.Fatal("settlement changed background-work identity or activity")
	}
}

func TestClaudeSubagentReportReaderUsesBoundedTailAndLastAssistant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-123.output")
	contents := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"older"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"latest"},{"type":"text","text":"answer"}]}}`,
		`{"type":"result","result":"done"}`,
	}, "\n")
	oversized := strings.Repeat("x", maxClaudeSubagentOutputBytes+4096) + "\n" + contents
	if err := os.WriteFile(path, []byte(oversized), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeSubagentReport("agent-123", path, true); got != "latest\nanswer" {
		t.Fatalf("report = %q, want the latest assistant text blocks", got)
	}
}

func TestClaudeSubagentUnreadableReportsCompleteWithoutResult(t *testing.T) {
	existingReport := filepath.Join(t.TempDir(), "other-agent.output")
	if err := os.WriteFile(existingReport, []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"do not read"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	malformedReport := filepath.Join(t.TempDir(), "agent-123.output")
	if err := os.WriteFile(malformedReport, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		path       string
		canRead    bool
		wantResult string
	}{
		{name: "unavailable file", path: filepath.Join(t.TempDir(), "agent-123.output"), canRead: true},
		{name: "reported path has another basename", path: existingReport, canRead: true},
		{name: "provider did not permit reading", path: filepath.Join(t.TempDir(), "agent-123.output"), canRead: false},
		{name: "malformed report", path: malformedReport, canRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newClaudeSubagentTestAdapter(t)
			startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", tc.path, tc.canRead, 7)
			adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
			update := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
			if update == nil || update.NormalizedPayload.SubagentTask().Status != "completed" ||
				update.NormalizedPayload.SubagentTask().ResultText != tc.wantResult {
				t.Fatalf("settled payload = %+v", update)
			}
		})
	}
}

func TestClaudeSubagentSettlementRequiresMatchingSuccessfulUnhandedPrompt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		generation uint64
		stopReason string
		handedOff  bool
	}{
		{name: "different generation", generation: 8, stopReason: "end_turn"},
		{name: "cancelled", generation: 7, stopReason: "cancelled"},
		{name: "failed", generation: 7, stopReason: "failed"},
		{name: "handed off", generation: 7, stopReason: "end_turn", handedOff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newClaudeSubagentTestAdapter(t)
			payload := startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", "", false, 7)
			before := payload.SubagentTask().Status
			adapter.settleClaudeSubagents("session-1", tc.generation, tc.stopReason, tc.handedOff)
			if events := drainEvents(adapter); len(events) != 0 {
				t.Fatalf("ineligible prompt settlement emitted updates: %+v", events)
			}
			if payload.SubagentTask().Status != before {
				t.Fatalf("payload status changed from %q to %q", before, payload.SubagentTask().Status)
			}
		})
	}
}

func TestClaudeSubagentResumeIsIdempotentAndDoesNotChangeBackgroundWork(t *testing.T) {
	adapter := newClaudeSubagentTestAdapter(t)
	payload := startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", "", false, 7)
	adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
	drainEvents(adapter)
	backgroundBefore := payload.BackgroundWork()
	activeBackgroundCount := countActiveBackgroundWork(adapter)
	if activeBackgroundCount != 1 {
		t.Fatalf("active background work count before resume = %d, want 1", activeBackgroundCount)
	}

	sendMessage := claudeSendMessageNotification("session-1", "send-1", "agent-123")
	adapter.handleACPUpdate(sendMessage, 9)
	events := drainEvents(adapter)
	update := findClaudeSubagentEvent(events, "task-1")
	if update == nil {
		t.Fatalf("known child resume did not update its card: %+v", events)
	}
	if got := update.NormalizedPayload.SubagentTask(); got.Status != "running" || got.ResumeCount != 1 {
		t.Fatalf("resumed payload = %+v", got)
	}
	if !reflect.DeepEqual(backgroundBefore, update.NormalizedPayload.BackgroundWork()) {
		t.Fatal("resume changed the background-work identity")
	}
	if got := countActiveBackgroundWork(adapter); got != activeBackgroundCount {
		t.Fatalf("resume changed active background work count from %d to %d", activeBackgroundCount, got)
	}

	duplicate := acpNotification("session-1", &sdk.SessionToolCallUpdate{
		ToolCallId: "send-1",
		Status:     statusPointer(sdk.ToolCallStatusCompleted),
		Meta:       claudeSendMessageMeta(),
		RawInput:   map[string]any{"to": "agent-123"},
	})
	adapter.handleACPUpdate(duplicate, 9)
	for _, event := range drainEvents(adapter) {
		if event.ToolCallID == "task-1" {
			t.Fatalf("duplicate SendMessage update emitted a second card update: %+v", event)
		}
	}

	unknown := claudeSendMessageNotification("session-1", "send-unknown", "unknown-agent")
	adapter.handleACPUpdate(unknown, 10)
	for _, event := range drainEvents(adapter) {
		if event.ToolCallID == "task-1" {
			t.Fatalf("unknown child changed a card: %+v", event)
		}
	}
	if got := payload.SubagentTask(); got.ResumeCount != 0 || got.Status != subagentAsyncLaunchedStatus {
		t.Fatalf("resume mutated the earlier launch snapshot: %+v", got)
	}

	adapter.settleClaudeSubagents("session-1", 9, "end_turn", false)
	settled := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if settled == nil || settled.NormalizedPayload.SubagentTask().Status != "completed" ||
		settled.NormalizedPayload.SubagentTask().ResumeCount != 1 {
		t.Fatalf("resumed card did not settle with its resume count: %+v", settled)
	}
	if !reflect.DeepEqual(backgroundBefore, settled.NormalizedPayload.BackgroundWork()) {
		t.Fatal("settlement after resume changed background-work identity")
	}
	if got := countActiveBackgroundWork(adapter); got != activeBackgroundCount {
		t.Fatalf("settlement changed active background work count from %d to %d", activeBackgroundCount, got)
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-003.4
func TestClaudeForegroundSubagentCanResumeFromCapturedFrames(t *testing.T) {
	adapter := newClaudeSubagentTestAdapter(t)
	file, err := os.Open("testdata/claude-resume-foreground.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var events []AgentEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var frame sdk.SessionNotification
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatalf("decode sanitized Claude resume frame: %v", err)
		}
		toolCallID := claudeFixtureToolCallID(frame)
		if toolCallID == "send-message" {
			adapter.settleClaudeSubagents("session-resume", 7, "end_turn", false)
			for _, event := range drainEvents(adapter) {
				if event.ToolCallID == "task-agent" {
					t.Fatalf("unresumed foreground card settled at its original prompt end: %+v", event)
				}
			}
		}
		generation := uint64(7)
		if toolCallID == "send-message" || toolCallID == "read-after-resume" {
			generation = 8
		}
		adapter.handleACPUpdate(frame, generation)
		frameEvents := drainEvents(adapter)
		events = append(events, frameEvents...)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	var resumed *AgentEvent
	for i := range events {
		if events[i].ToolCallID == "task-agent" && events[i].NormalizedPayload != nil &&
			events[i].NormalizedPayload.SubagentTask() != nil &&
			events[i].NormalizedPayload.SubagentTask().Status == claudeSubagentRunningStatus {
			resumed = &events[i]
		}
	}
	if resumed == nil || resumed.NormalizedPayload.SubagentTask().ResumeCount != 1 {
		t.Fatalf("captured foreground child was not marked resumed: %+v", events)
	}
	for _, toolCallID := range []string{"read-before-resume", "read-after-resume"} {
		var child *AgentEvent
		for i := range events {
			if events[i].ToolCallID == toolCallID {
				child = &events[i]
				break
			}
		}
		if child == nil || child.ParentToolCallID != "task-agent" {
			t.Fatalf("captured child call %q lost its Claude parent link: %+v", toolCallID, child)
		}
	}

	adapter.settleClaudeSubagents("session-resume", 8, "end_turn", false)
	settled := findClaudeSubagentEvent(drainEvents(adapter), "task-agent")
	if settled == nil || settled.NormalizedPayload.SubagentTask().Status != "completed" {
		t.Fatalf("resumed foreground card did not settle at its follow-up prompt end: %+v", settled)
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-003.3, AC-AGENTS-SUBAGENT-LIFECYCLE-003.4
func TestClaudeResumePreservesPriorResultWhenNoNewReportIsAvailable(t *testing.T) {
	adapter := newClaudeSubagentTestAdapter(t)
	reportPath := filepath.Join(t.TempDir(), "agent-123.output")
	if err := os.WriteFile(reportPath, []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Earlier result."}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", reportPath, true, 7)
	adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
	settled := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if settled == nil || settled.NormalizedPayload.SubagentTask().ResultText != "Earlier result." {
		t.Fatalf("initial report missing from settled card: %+v", settled)
	}

	adapter.handleACPUpdate(claudeSendMessageNotification("session-1", "send-1", "agent-123"), 9)
	resumed := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if resumed == nil || resumed.NormalizedPayload.SubagentTask().ResultText != "Earlier result." {
		t.Fatalf("resume discarded the prior result: %+v", resumed)
	}
	if err := os.WriteFile(reportPath, []byte("malformed report"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter.settleClaudeSubagents("session-1", 9, "end_turn", false)
	settled = findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if settled == nil || settled.NormalizedPayload.SubagentTask().ResultText != "Earlier result." {
		t.Fatalf("settlement without a new report discarded the prior result: %+v", settled)
	}
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-003.6
func TestClaudeResumeAndSettlementPreserveParentToolCallID(t *testing.T) {
	t.Run("settlement", func(t *testing.T) {
		adapter := newClaudeSubagentTestAdapter(t)
		startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", "", false, 7)
		adapter.mu.Lock()
		adapter.toolCallParents["task-1"] = "parent-agent"
		adapter.mu.Unlock()
		adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
		update := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
		if update == nil || update.ParentToolCallID != "parent-agent" {
			t.Fatalf("settlement lost the parent tool call: %+v", update)
		}
	})

	t.Run("resume", func(t *testing.T) {
		adapter := newClaudeSubagentTestAdapter(t)
		startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", "", false, 7)
		adapter.mu.Lock()
		adapter.toolCallParents["task-1"] = "parent-agent"
		adapter.mu.Unlock()
		adapter.handleACPUpdate(claudeSendMessageNotification("session-1", "send-1", "agent-123"), 9)
		update := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
		if update == nil || update.ParentToolCallID != "parent-agent" {
			t.Fatalf("resume lost the parent tool call: %+v", update)
		}
	})
}

func claudeFixtureToolCallID(frame sdk.SessionNotification) string {
	if frame.Update.ToolCall != nil {
		return string(frame.Update.ToolCall.ToolCallId)
	}
	if frame.Update.ToolCallUpdate != nil {
		return string(frame.Update.ToolCallUpdate.ToolCallId)
	}
	return ""
}

func countActiveBackgroundWork(adapter *Adapter) int {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	count := 0
	for _, payload := range adapter.activeToolCalls {
		if payload != nil && payload.IsActiveBackgroundWork() {
			count++
		}
	}
	return count
}

func newClaudeSubagentTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	adapter := newTestAdapter()
	adapter.agentID = mockAgentID
	adapter.dialect = newACPDialect(mockAgentID)
	adapter.normalizer = NewNormalizer(mockAgentID)
	t.Cleanup(func() { _ = adapter.Close() })
	return adapter
}

func startClaudeAsyncSubagent(
	t *testing.T,
	adapter *Adapter,
	sessionID, toolCallID, agentID, outputFile string,
	canRead bool,
	generation uint64,
) *streams.NormalizedPayload {
	t.Helper()
	start := &sdk.SessionUpdateToolCall{
		ToolCallId: sdk.ToolCallId(toolCallID),
		Title:      "Task",
		Kind:       sdk.ToolKindOther,
		Status:     sdk.ToolCallStatusPending,
		Meta:       map[string]any{"claudeCode": map[string]any{"toolName": "Agent"}},
		RawInput:   map[string]any{"description": "Resume a child", "prompt": "Continue work"},
	}
	adapter.handleACPUpdate(acpNotification(sessionID, start), generation)
	drainEvents(adapter)
	response := map[string]any{
		"agentId":           agentID,
		"status":            subagentAsyncLaunchedStatus,
		"isAsync":           true,
		"outputFile":        outputFile,
		"canReadOutputFile": canRead,
	}
	update := &sdk.SessionToolCallUpdate{
		ToolCallId: sdk.ToolCallId(toolCallID),
		Meta:       map[string]any{"claudeCode": map[string]any{"toolResponse": response}},
	}
	adapter.handleACPUpdate(acpNotification(sessionID, update), generation)
	for _, event := range drainEvents(adapter) {
		if event.ToolCallID == toolCallID && event.NormalizedPayload != nil {
			return event.NormalizedPayload
		}
	}
	t.Fatalf("async subagent update missing for %q", toolCallID)
	return nil
}

func claudeSendMessageNotification(sessionID, toolCallID, agentID string) sdk.SessionNotification {
	return acpNotification(sessionID, &sdk.SessionUpdateToolCall{
		ToolCallId: sdk.ToolCallId(toolCallID),
		Title:      "SendMessage",
		Kind:       sdk.ToolKindOther,
		Status:     sdk.ToolCallStatusPending,
		Meta:       claudeSendMessageMeta(),
		RawInput:   map[string]any{"to": agentID, "message": "Continue"},
	})
}

func claudeSendMessageMeta() map[string]any {
	return map[string]any{"claudeCode": map[string]any{"toolName": "SendMessage"}}
}

func acpNotification(sessionID string, update any) sdk.SessionNotification {
	var sessionUpdate sdk.SessionUpdate
	switch value := update.(type) {
	case *sdk.SessionUpdateToolCall:
		sessionUpdate.ToolCall = value
	case *sdk.SessionToolCallUpdate:
		sessionUpdate.ToolCallUpdate = value
	default:
		panic(fmt.Sprintf("unsupported test update %T", update))
	}
	return sdk.SessionNotification{SessionId: sdk.SessionId(sessionID), Update: sessionUpdate}
}

func statusPointer(status sdk.ToolCallStatus) *sdk.ToolCallStatus {
	return &status
}

func findClaudeSubagentEvent(events []AgentEvent, toolCallID string) *AgentEvent {
	for i := range events {
		if events[i].ToolCallID == toolCallID && events[i].NormalizedPayload != nil &&
			events[i].NormalizedPayload.SubagentTask() != nil {
			return &events[i]
		}
	}
	return nil
}

func TestClaudeSubagentReportReaderRejectsMismatchedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-456.output")
	if err := os.WriteFile(path, []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"secret"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeSubagentReport("agent-123", path, true); got != "" {
		t.Fatalf("mismatched basename report = %q", got)
	}
}
