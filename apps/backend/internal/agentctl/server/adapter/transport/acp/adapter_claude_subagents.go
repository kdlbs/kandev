package acp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const maxClaudeSubagentOutputBytes = 256 << 10

const claudeSubagentRunningStatus = "running"

type claudeSubagentSessionState struct {
	byToolCallID      map[string]*claudeSubagentRecord
	byAgentID         map[string]map[string]*claudeSubagentRecord
	resumeToolCallIDs map[string]struct{}
}

type claudeSubagentRecord struct {
	toolCallID        string
	agentID           string
	outputFile        string
	canReadOutputFile bool
	wasAsync          bool
	activeGeneration  uint64
	resumedGeneration uint64
	payload           *streams.NormalizedPayload
}

type claudeSubagentSettlement struct {
	record *claudeSubagentRecord
	report string
	toolID string
}

func isClaudeSubagentLifecycleAgent(agentID string) bool {
	return agentID == claudeAgentID || agentID == mockAgentID
}

func (a *Adapter) claudeSubagentSessionLocked(sessionID string) *claudeSubagentSessionState {
	if a.claudeSubagents == nil {
		a.claudeSubagents = make(map[string]*claudeSubagentSessionState)
	}
	state := a.claudeSubagents[sessionID]
	if state == nil {
		state = &claudeSubagentSessionState{
			byToolCallID:      make(map[string]*claudeSubagentRecord),
			byAgentID:         make(map[string]map[string]*claudeSubagentRecord),
			resumeToolCallIDs: make(map[string]struct{}),
		}
		a.claudeSubagents[sessionID] = state
	}
	return state
}

func (a *Adapter) trackClaudeSubagentCallLocked(
	sessionID, toolCallID string,
	promptGeneration uint64,
	payload *streams.NormalizedPayload,
) {
	if !isClaudeSubagentLifecycleAgent(a.agentID) || sessionID == "" || toolCallID == "" ||
		promptGeneration == 0 {
		return
	}
	subagent := claudeSubagentPayloadWithIdentity(payload)
	if subagent == nil {
		return
	}
	state := a.claudeSubagentSessionLocked(sessionID)
	if existing := state.byToolCallID[toolCallID]; existing != nil {
		updateClaudeSubagentRecord(existing, subagent, promptGeneration, payload)
		return
	}
	record := &claudeSubagentRecord{
		toolCallID:        toolCallID,
		agentID:           subagent.AgentID,
		outputFile:        subagent.OutputFile,
		canReadOutputFile: subagent.CanReadOutputFile,
		wasAsync:          subagent.IsAsync,
		activeGeneration:  promptGeneration,
		payload:           payload.Snapshot(),
	}
	state.byToolCallID[toolCallID] = record
	byToolCall := state.byAgentID[subagent.AgentID]
	if byToolCall == nil {
		byToolCall = make(map[string]*claudeSubagentRecord)
		state.byAgentID[subagent.AgentID] = byToolCall
	}
	byToolCall[toolCallID] = record
}

func claudeSubagentPayloadWithIdentity(payload *streams.NormalizedPayload) *streams.SubagentTaskPayload {
	if payload == nil || payload.SubagentTask() == nil {
		return nil
	}
	subagent := payload.SubagentTask()
	if subagent.AgentID == "" {
		return nil
	}
	return subagent
}

func updateClaudeSubagentRecord(
	record *claudeSubagentRecord,
	subagent *streams.SubagentTaskPayload,
	promptGeneration uint64,
	payload *streams.NormalizedPayload,
) {
	if record.agentID != subagent.AgentID || promptGeneration < record.activeGeneration ||
		(record.resumedGeneration != 0 && promptGeneration != record.resumedGeneration) {
		return
	}
	record.wasAsync = record.wasAsync || subagent.IsAsync
	record.payload = payload.Snapshot()
	if subagent.OutputFile != "" {
		record.outputFile = subagent.OutputFile
	}
	record.canReadOutputFile = record.canReadOutputFile || subagent.CanReadOutputFile
}

func (a *Adapter) claudeSubagentResumeUpdate(
	sessionID, toolCallID, title string,
	meta map[string]any,
	rawInput any,
	promptGeneration uint64,
) *AgentEvent {
	if !isClaudeSubagentLifecycleAgent(a.agentID) || sessionID == "" || toolCallID == "" ||
		promptGeneration == 0 || a.dialect.parseSubagentControl(title, meta, rawInput) != streams.SubagentControlMessage {
		return nil
	}
	input, ok := rawInput.(map[string]any)
	if !ok {
		return nil
	}
	agentID, _ := input["to"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.claudeSubagents[sessionID]
	if state == nil {
		return nil
	}
	if _, seen := state.resumeToolCallIDs[toolCallID]; seen {
		return nil
	}
	byToolCall := state.byAgentID[agentID]
	if len(byToolCall) != 1 {
		return nil
	}
	var record *claudeSubagentRecord
	for _, candidate := range byToolCall {
		record = candidate
	}
	if record == nil || record.payload == nil || record.payload.SubagentTask() == nil {
		return nil
	}
	state.resumeToolCallIDs[toolCallID] = struct{}{}
	subagent := record.payload.SubagentTask()
	subagent.ResumeCount++
	subagent.Status = claudeSubagentRunningStatus
	record.activeGeneration = promptGeneration
	record.resumedGeneration = promptGeneration
	return &AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         sessionID,
		ToolCallID:        record.toolCallID,
		ParentToolCallID:  a.toolCallParents[record.toolCallID],
		ToolStatus:        toolStatusComplete,
		PromptGeneration:  promptGeneration,
		NormalizedPayload: record.payload.Snapshot(),
	}
}

func (a *Adapter) claudeSubagentResumeEvent(
	n acp.SessionNotification,
	promptGeneration uint64,
) *AgentEvent {
	sessionID := string(n.SessionId)
	switch {
	case n.Update.ToolCall != nil:
		call := n.Update.ToolCall
		return a.claudeSubagentResumeUpdate(
			sessionID,
			string(call.ToolCallId),
			call.Title,
			call.Meta,
			call.RawInput,
			promptGeneration,
		)
	case n.Update.ToolCallUpdate != nil:
		update := n.Update.ToolCallUpdate
		title := ""
		if update.Title != nil {
			title = *update.Title
		}
		return a.claudeSubagentResumeUpdate(
			sessionID,
			string(update.ToolCallId),
			title,
			update.Meta,
			update.RawInput,
			promptGeneration,
		)
	default:
		return nil
	}
}

func (a *Adapter) isClaudeSubagentResumedLocked(sessionID, toolCallID string, promptGeneration uint64) bool {
	if !isClaudeSubagentLifecycleAgent(a.agentID) || promptGeneration == 0 {
		return false
	}
	state := a.claudeSubagents[sessionID]
	if state == nil {
		return false
	}
	record := state.byToolCallID[toolCallID]
	return record != nil && record.resumedGeneration == promptGeneration
}

func (a *Adapter) settleClaudeSubagents(
	sessionID string,
	promptGeneration uint64,
	stopReason string,
	handedOff bool,
) {
	if !isClaudeSubagentLifecycleAgent(a.agentID) || sessionID == "" || promptGeneration == 0 ||
		stopReason != "end_turn" || handedOff {
		return
	}

	a.mu.RLock()
	candidates := collectClaudeSubagentSettlements(a.claudeSubagents[sessionID], promptGeneration)
	a.mu.RUnlock()
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].toolID < candidates[j].toolID })
	for i := range candidates {
		record := candidates[i].record
		candidates[i].report = readClaudeSubagentReport(record.agentID, record.outputFile, record.canReadOutputFile)
	}

	a.mu.Lock()
	events := a.completeClaudeSubagentSettlementsLocked(sessionID, promptGeneration, candidates)
	a.mu.Unlock()

	for i := range events {
		if events[i].NormalizedPayload.SubagentTask().ResultText == "" {
			a.logger.Debug("Claude subagent completed without a readable report",
				zap.String("session_id", sessionID),
				zap.String("tool_call_id", events[i].ToolCallID))
		}
		shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, sessionID, &events[i])
		a.sendUpdate(events[i])
	}
}

func collectClaudeSubagentSettlements(
	state *claudeSubagentSessionState,
	promptGeneration uint64,
) []claudeSubagentSettlement {
	if state == nil {
		return nil
	}
	candidates := make([]claudeSubagentSettlement, 0, len(state.byToolCallID))
	for _, record := range state.byToolCallID {
		if record == nil || (!record.wasAsync && record.resumedGeneration != promptGeneration) ||
			record.activeGeneration != promptGeneration || record.payload == nil ||
			record.payload.SubagentTask() == nil {
			continue
		}
		candidates = append(candidates, claudeSubagentSettlement{record: record, toolID: record.toolCallID})
	}
	return candidates
}

func (a *Adapter) completeClaudeSubagentSettlementsLocked(
	sessionID string,
	promptGeneration uint64,
	candidates []claudeSubagentSettlement,
) []AgentEvent {
	state := a.claudeSubagents[sessionID]
	if state == nil {
		return nil
	}
	events := make([]AgentEvent, 0, len(candidates))
	for _, candidate := range candidates {
		record := candidate.record
		if !claudeSubagentSettlementIsCurrent(state, record, promptGeneration) {
			continue
		}
		subagent := record.payload.SubagentTask()
		subagent.Status = "completed"
		if candidate.report != "" {
			subagent.ResultText = candidate.report
		}
		record.activeGeneration = 0
		record.resumedGeneration = 0
		events = append(events, AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			ToolCallID:        record.toolCallID,
			ParentToolCallID:  a.toolCallParents[record.toolCallID],
			ToolStatus:        toolStatusComplete,
			PromptGeneration:  promptGeneration,
			NormalizedPayload: record.payload.Snapshot(),
		})
	}
	return events
}

func claudeSubagentSettlementIsCurrent(
	state *claudeSubagentSessionState,
	record *claudeSubagentRecord,
	promptGeneration uint64,
) bool {
	return record != nil && state.byToolCallID[record.toolCallID] == record &&
		record.activeGeneration == promptGeneration && record.payload != nil && record.payload.SubagentTask() != nil
}

func (a *Adapter) clearClaudeSubagentsLocked(sessionID string) {
	if sessionID == "" {
		a.claudeSubagents = make(map[string]*claudeSubagentSessionState)
		return
	}
	delete(a.claudeSubagents, sessionID)
}

func readClaudeSubagentReport(agentID, outputFile string, canReadOutputFile bool) string {
	if !canReadOutputFile || agentID == "" || filepath.Base(outputFile) != agentID+".output" {
		return ""
	}
	file, err := os.Open(outputFile)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - int64(maxClaudeSubagentOutputBytes)
	if start < 0 {
		start = 0
	}
	reader := io.NewSectionReader(file, start, int64(maxClaudeSubagentOutputBytes))
	data, err := io.ReadAll(io.LimitReader(reader, int64(maxClaudeSubagentOutputBytes)))
	if err != nil {
		return ""
	}
	if start > 0 {
		newline := bytes.IndexByte(data, '\n')
		if newline < 0 {
			return ""
		}
		data = data[newline+1:]
	}
	return claudeSubagentAssistantText(data)
}

func claudeSubagentAssistantText(data []byte) string {
	lines := bytes.Split(data, []byte{'\n'})
	for index := len(lines) - 1; index >= 0; index-- {
		if len(bytes.TrimSpace(lines[index])) == 0 {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(lines[index], &entry); err != nil || entry.Type != "assistant" {
			continue
		}
		var textBlocks []string
		for _, block := range entry.Message.Content {
			if block.Type == "text" && block.Text != "" {
				textBlocks = append(textBlocks, block.Text)
			}
		}
		return strings.TrimSpace(strings.Join(textBlocks, "\n"))
	}
	return ""
}
