package acp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type claudeMonitorReaderState struct {
	reader        *claudeMonitorCompletionReader
	ctx           context.Context
	cancel        context.CancelFunc
	generation    atomic.Uint64
	activePrompts int
	admittedAt    time.Time
	started       bool
}

type claudeMonitorSettlement struct {
	backgroundWork AgentEvent
	toolUpdate     AgentEvent
}

func (s *claudeMonitorSettlement) send(a *Adapter) {
	if s == nil {
		return
	}
	s.sendBackgroundWork(a)
	a.sendUpdate(s.toolUpdate)
}

func (s *claudeMonitorSettlement) sendBackgroundWork(a *Adapter) {
	if s == nil {
		return
	}
	a.sendUpdate(s.backgroundWork)
}

func (a *Adapter) beginClaudeMonitorPrompt(sessionID string) *claudeMonitorReaderState {
	generation, existing, eligible := a.claudeMonitorReaderAdmission(sessionID)
	if !eligible {
		return nil
	}
	state := a.reuseClaudeMonitorReader(sessionID, existing, generation)
	if state == nil {
		state = a.createClaudeMonitorReader(sessionID, generation)
	}
	if state != nil && a.hasActiveClaudeMonitors(sessionID) {
		a.startClaudeMonitorCompletionReader(sessionID, state)
	}
	return state
}

func (a *Adapter) claudeMonitorReaderAdmission(sessionID string) (uint64, *claudeMonitorReaderState, bool) {
	if a.agentID != claudeAgentID || !a.backgroundWorkEnabled || a.cfg == nil || a.cfg.ClaudeConfigDir == "" {
		return 0, nil, false
	}
	name, version := a.connectedAgentIdentity()
	if !verifiedClaudeMonitorEvidenceBridge(a.agentID, name, version) {
		return 0, nil, false
	}
	a.mu.RLock()
	if a.closed || a.sessionID != sessionID {
		a.mu.RUnlock()
		return 0, nil, false
	}
	generation := a.nativeGeneration
	state := a.claudeMonitorReaders[sessionID]
	a.mu.RUnlock()
	return generation, state, true
}

func (a *Adapter) reuseClaudeMonitorReader(
	sessionID string,
	state *claudeMonitorReaderState,
	generation uint64,
) *claudeMonitorReaderState {
	if state == nil || state.generation.Load() != generation {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.claudeMonitorReaders[sessionID] != state || a.closed || a.nativeGeneration != generation {
		return nil
	}
	state.activePrompts++
	return state
}

func (a *Adapter) createClaudeMonitorReader(sessionID string, generation uint64) *claudeMonitorReaderState {
	now := time.Now().UTC()
	reader, err := newClaudeMonitorCompletionReader(a.cfg.ClaudeConfigDir, a.cfg.WorkDir, sessionID, now)
	if err != nil {
		return nil
	}
	readerCtx, cancel := context.WithCancel(a.lifetimeCtx)
	state := &claudeMonitorReaderState{
		reader: reader, ctx: readerCtx, cancel: cancel, activePrompts: 1, admittedAt: now,
	}
	state.generation.Store(generation)
	a.mu.Lock()
	if a.closed || a.sessionID != sessionID || a.nativeGeneration != generation {
		a.mu.Unlock()
		cancel()
		_ = reader.Close()
		return nil
	}
	if previous := a.claudeMonitorReaders[sessionID]; previous != nil && previous.generation.Load() == generation {
		previous.activePrompts++
		a.mu.Unlock()
		cancel()
		_ = reader.Close()
		return previous
	}
	var replacedReader *claudeMonitorCompletionReader
	if previous := a.claudeMonitorReaders[sessionID]; previous != nil {
		previous.cancel()
		if !previous.started {
			replacedReader = previous.reader
		}
	}
	a.claudeMonitorReaders[sessionID] = state
	a.mu.Unlock()
	if replacedReader != nil {
		_ = replacedReader.Close()
	}
	return state
}

func (a *Adapter) hasActiveClaudeMonitors(sessionID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.activeMonitors[sessionID]) > 0
}

func (a *Adapter) startClaudeMonitorCompletionReader(sessionID string, expected *claudeMonitorReaderState) {
	a.mu.Lock()
	state := a.claudeMonitorReaders[sessionID]
	if state == nil || (expected != nil && state != expected) || state.started || a.closed ||
		state.generation.Load() != a.nativeGeneration ||
		(state.activePrompts == 0 && len(a.activeMonitors[sessionID]) == 0) {
		a.mu.Unlock()
		return
	}
	state.started = true
	readerCtx := state.ctx
	a.claudeMonitorReaderWg.Add(1)
	a.mu.Unlock()
	go a.runClaudeMonitorCompletionReader(readerCtx, sessionID, state)
}

func (a *Adapter) endClaudeMonitorPrompt(sessionID string, completedPrompt *claudeMonitorReaderState) {
	if completedPrompt == nil {
		return
	}
	a.mu.Lock()
	state := a.claudeMonitorReaders[sessionID]
	if state == nil || state != completedPrompt {
		a.mu.Unlock()
		return
	}
	if state.activePrompts > 0 {
		state.activePrompts--
	}
	closeReader := false
	if state.activePrompts == 0 && len(a.activeMonitors[sessionID]) == 0 {
		delete(a.claudeMonitorReaders, sessionID)
		delete(a.pendingClaudeMonitorCompletions, sessionID)
		state.cancel()
		closeReader = !state.started
	}
	a.mu.Unlock()
	if closeReader {
		_ = state.reader.Close()
	}
}

func (a *Adapter) runClaudeMonitorCompletionReader(ctx context.Context, sessionID string, state *claudeMonitorReaderState) {
	defer a.claudeMonitorReaderWg.Done()
	defer state.cancel()
	defer func() { _ = state.reader.Close() }()
	ticker := time.NewTicker(claudeMonitorPollInterval)
	defer ticker.Stop()
	for {
		completions, err := state.reader.ReadAvailable()
		if err != nil {
			if !a.claudeMonitorReaderNeeded(sessionID, state) || !waitForClaudeMonitorReaderRetry(ctx, ticker.C) {
				return
			}
			continue
		}
		for _, completion := range completions {
			a.registerClaudeMonitorCompletionFromReader(sessionID, state, completion)
		}
		if !a.claudeMonitorReaderNeeded(sessionID, state) {
			return
		}
		if !waitForClaudeMonitorReaderRetry(ctx, ticker.C) {
			return
		}
	}
}

func waitForClaudeMonitorReaderRetry(ctx context.Context, retry <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-retry:
		return true
	}
}

func (a *Adapter) claudeMonitorReaderNeeded(sessionID string, state *claudeMonitorReaderState) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.claudeMonitorReaders[sessionID] != state || a.nativeGeneration != state.generation.Load() || a.closed {
		return false
	}
	if state.activePrompts > 0 || len(a.activeMonitors[sessionID]) > 0 {
		return true
	}
	delete(a.claudeMonitorReaders, sessionID)
	delete(a.pendingClaudeMonitorCompletions, sessionID)
	return false
}

func (a *Adapter) cancelClaudeMonitorReadersLocked(sessionID string) []*claudeMonitorCompletionReader {
	var readers []*claudeMonitorCompletionReader
	if sessionID == "" {
		for id, state := range a.claudeMonitorReaders {
			state.cancel()
			if !state.started {
				readers = append(readers, state.reader)
			}
			delete(a.claudeMonitorReaders, id)
		}
		clear(a.pendingClaudeMonitorCompletions)
		clear(a.completedClaudeMonitorCompletions)
		return readers
	}
	if state := a.claudeMonitorReaders[sessionID]; state != nil {
		state.cancel()
		if !state.started {
			readers = append(readers, state.reader)
		}
		delete(a.claudeMonitorReaders, sessionID)
	}
	delete(a.pendingClaudeMonitorCompletions, sessionID)
	delete(a.completedClaudeMonitorCompletions, sessionID)
	return readers
}

func (a *Adapter) resetClaudeMonitorReadersForSessionTransition(previous, current string) {
	a.mu.Lock()
	readers := a.cancelClaudeMonitorReadersLocked(previous)
	if current != previous {
		readers = append(readers, a.cancelClaudeMonitorReadersLocked(current)...)
	}
	a.mu.Unlock()
	for _, reader := range readers {
		_ = reader.Close()
	}
}

func (a *Adapter) rebindClaudeMonitorReaderGenerationLocked(sessionID string, generation uint64) {
	if state := a.claudeMonitorReaders[sessionID]; state != nil {
		state.generation.Store(generation)
	}
}

func (a *Adapter) takePendingClaudeMonitorCompletionLocked(
	sessionID, taskID, toolCallID string,
) (claudeMonitorCompletion, bool) {
	key := claudeMonitorCompletionKey{taskID: taskID, toolUseID: toolCallID}
	pending := a.pendingClaudeMonitorCompletions[sessionID]
	completion, ok := pending[key]
	if ok {
		delete(pending, key)
		if len(pending) == 0 {
			delete(a.pendingClaudeMonitorCompletions, sessionID)
		}
	}
	return completion, ok
}

func (a *Adapter) registerClaudeMonitorCompletion(sessionID string, completion claudeMonitorCompletion) {
	a.mu.RLock()
	generation := a.nativeGeneration
	a.mu.RUnlock()
	a.registerClaudeMonitorCompletionForGeneration(sessionID, generation, completion)
}

func (a *Adapter) registerClaudeMonitorCompletionForGeneration(
	sessionID string,
	generation uint64,
	completion claudeMonitorCompletion,
) {
	if !a.validClaudeMonitorCompletion(sessionID, completion) {
		return
	}
	a.mu.Lock()
	if a.closed || a.sessionID != sessionID || a.nativeGeneration != generation {
		a.mu.Unlock()
		return
	}
	settlement := a.applyClaudeMonitorCompletionLocked(sessionID, completion)
	a.mu.Unlock()
	settlement.send(a)
}

func (a *Adapter) registerClaudeMonitorCompletionFromReader(
	sessionID string,
	state *claudeMonitorReaderState,
	completion claudeMonitorCompletion,
) {
	if state == nil || !a.validClaudeMonitorCompletion(sessionID, completion) {
		return
	}
	a.mu.Lock()
	if a.closed || a.sessionID != sessionID || a.claudeMonitorReaders[sessionID] != state ||
		a.nativeGeneration != state.generation.Load() {
		a.mu.Unlock()
		return
	}
	settlement := a.applyClaudeMonitorCompletionLocked(sessionID, completion)
	a.mu.Unlock()
	settlement.send(a)
}

func (a *Adapter) validClaudeMonitorCompletion(sessionID string, completion claudeMonitorCompletion) bool {
	name, version := a.connectedAgentIdentity()
	return verifiedClaudeMonitorEvidenceBridge(a.agentID, name, version) && completion.SessionID == sessionID &&
		completion.TaskID != "" && completion.ToolUseID != "" && completion.State.IsTerminal() &&
		!completion.FinishedAt.IsZero()
}

func (a *Adapter) applyClaudeMonitorCompletionLocked(
	sessionID string,
	completion claudeMonitorCompletion,
) *claudeMonitorSettlement {
	key := claudeMonitorCompletionKey{taskID: completion.TaskID, toolUseID: completion.ToolUseID}
	if _, done := a.completedClaudeMonitorCompletions[sessionID][key]; done {
		return nil
	}
	toolCallID, tracked := a.activeMonitors[sessionID][completion.TaskID]
	if !tracked {
		a.holdPendingClaudeMonitorCompletionLocked(sessionID, key, completion)
		return nil
	}
	if toolCallID != completion.ToolUseID {
		return nil
	}
	payload := a.activeToolCalls[toolCallID]
	if !payloadMatchesClaudeMonitorCompletion(payload, completion) {
		return nil
	}
	return a.settleClaudeMonitorLocked(sessionID, completion, payload)
}

func recognizeTaskStopCompletedTaskID(
	meta map[string]any,
	rawOutput, rawInput any,
	payload *streams.NormalizedPayload,
) (string, bool) {
	if !isTaskStopMeta(meta) {
		return "", false
	}
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	toolResponse, _ := claudeCode["toolResponse"].(map[string]any)
	responseTaskID := taskStopTaskID(toolResponse)
	outputTaskID := taskStopOutputTaskID(rawOutput)
	if responseTaskID != "" && outputTaskID != "" && responseTaskID != outputTaskID {
		return "", false
	}
	taskID := responseTaskID
	if taskID == "" {
		taskID = outputTaskID
	}
	if taskID == "" {
		return "", false
	}
	inputTaskID, consistentInput := taskStopInputTaskID(rawInput, payload)
	if !consistentInput || (inputTaskID != "" && inputTaskID != taskID) {
		return "", false
	}
	return taskID, true
}

func taskStopInputTaskID(rawInput any, payload *streams.NormalizedPayload) (string, bool) {
	frameTaskID := taskStopTaskID(rawInput)
	payloadTaskID := ""
	if payload != nil && payload.Generic() != nil {
		args, _ := payload.Generic().Input.(map[string]any)
		if args != nil {
			rawArgs, _ := args["raw_input"].(map[string]any)
			payloadTaskID = taskStopTaskID(rawArgs)
			if payloadTaskID == "" {
				payloadTaskID = taskStopTaskID(args)
			}
		}
	}
	if frameTaskID != "" && payloadTaskID != "" && frameTaskID != payloadTaskID {
		return "", false
	}
	if frameTaskID != "" {
		return frameTaskID, true
	}
	return payloadTaskID, true
}

func taskStopOutputTaskID(rawOutput any) string {
	switch output := rawOutput.(type) {
	case map[string]any, map[string]string:
		return taskStopTaskID(output)
	case string:
		var decoded map[string]any
		if json.Unmarshal([]byte(output), &decoded) == nil {
			return taskStopTaskID(decoded)
		}
	case json.RawMessage:
		var decoded map[string]any
		if json.Unmarshal(output, &decoded) == nil {
			return taskStopTaskID(decoded)
		}
	}
	return ""
}

func taskStopTaskID(value any) string {
	var taskID string
	switch object := value.(type) {
	case map[string]any:
		taskID, _ = object["task_id"].(string)
	case map[string]string:
		taskID = object["task_id"]
	}
	return strings.TrimSpace(taskID)
}

func (a *Adapter) settleTrackedMonitorFromTaskStopLocked(sessionID, taskID string) *claudeMonitorSettlement {
	if sessionID == "" || sessionID != a.sessionID || taskID == "" {
		return nil
	}
	toolCallID, tracked := a.activeMonitors[sessionID][taskID]
	if !tracked {
		return nil
	}
	payload := a.activeToolCalls[toolCallID]
	completion := claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      streams.RunStateInterrupted,
		FinishedAt: time.Now().UTC(),
	}
	if !payloadMatchesClaudeMonitorCompletion(payload, completion) {
		return nil
	}
	return a.settleClaudeMonitorLocked(sessionID, completion, payload)
}

func (a *Adapter) holdPendingClaudeMonitorCompletionLocked(
	sessionID string,
	key claudeMonitorCompletionKey,
	completion claudeMonitorCompletion,
) {
	state := a.claudeMonitorReaders[sessionID]
	if state == nil || state.generation.Load() != a.nativeGeneration || state.activePrompts == 0 {
		return
	}
	pending := a.pendingClaudeMonitorCompletions[sessionID]
	if len(pending) >= claudeMonitorMaxPendingRecords {
		return
	}
	if pending == nil {
		pending = make(map[claudeMonitorCompletionKey]claudeMonitorCompletion)
		a.pendingClaudeMonitorCompletions[sessionID] = pending
	}
	pending[key] = completion
}

func payloadMatchesClaudeMonitorCompletion(payload *streams.NormalizedPayload, completion claudeMonitorCompletion) bool {
	return payload != nil && payload.Monitor() != nil && payload.Monitor().TaskID == completion.TaskID &&
		payload.BackgroundWork() != nil && payload.BackgroundWork().WorkID == completion.TaskID && payload.IsActiveMonitor()
}

func verifiedClaudeMonitorEvidenceBridge(agentID, agentName, agentVersion string) bool {
	return agentID == claudeAgentID && agentName == claudeMonitorEvidenceBridgeName &&
		(agentVersion == verifiedClaudeTerminalBridgeManagedVersion ||
			agentVersion == verifiedClaudeTerminalBridgeCurrentVersion)
}

func (a *Adapter) settleClaudeMonitorLocked(
	sessionID string,
	completion claudeMonitorCompletion,
	payload *streams.NormalizedPayload,
) *claudeMonitorSettlement {
	key := claudeMonitorCompletionKey{taskID: completion.TaskID, toolUseID: completion.ToolUseID}
	completed := a.completedClaudeMonitorCompletions[sessionID]
	if completed == nil {
		completed = make(map[claudeMonitorCompletionKey]struct{})
		a.completedClaudeMonitorCompletions[sessionID] = completed
	}
	if len(completed) < claudeMonitorMaxCompletionMarks {
		completed[key] = struct{}{}
	}
	markMonitorEnded(payload, string(completion.State))
	delete(a.activeToolCalls, completion.ToolUseID)
	a.dropMonitorByToolCallIDLocked(sessionID, completion.ToolUseID)
	a.forgetPromptHandoffToolLocked(completion.ToolUseID)
	observation := claudeMonitorRunObservation(sessionID, completion, payload)
	toolStatus, _ := claudeMonitorToolStatus(completion.State)
	return &claudeMonitorSettlement{
		backgroundWork: AgentEvent{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			SessionID:      sessionID,
			BackgroundWork: observation,
		},
		toolUpdate: AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			ToolCallID:        completion.ToolUseID,
			ToolName:          monitorToolName,
			ToolStatus:        toolStatus,
			NormalizedPayload: payload,
		},
	}
}

func claudeMonitorToolStatus(state streams.RunState) (string, string) {
	switch state {
	case streams.RunStateCompleted:
		return toolStatusComplete, toolStatusCompleted
	case streams.RunStateFailed:
		return toolStatusError, "failed"
	case streams.RunStateInterrupted:
		return toolStatusCancelled, "interrupted"
	default:
		return toolStatusComplete, "ended"
	}
}

func claudeMonitorRunObservation(
	sessionID string,
	completion claudeMonitorCompletion,
	payload *streams.NormalizedPayload,
) *streams.WorkloadRunObservation {
	work := payload.BackgroundWork()
	if work == nil {
		return nil
	}
	title := monitorTitleFromPayload(payload)
	observation := &streams.WorkloadRunObservation{
		SessionID:    sessionID,
		WorkID:       completion.TaskID,
		RunID:        work.RunID,
		Kind:         streams.WorkloadKindMonitor,
		Title:        title,
		State:        completion.State,
		SourceCallID: completion.ToolUseID,
		FinishedAt:   &completion.FinishedAt,
	}
	if work.Capabilities != nil {
		observation.Capabilities = *work.Capabilities
	}
	return observation
}

func (a *Adapter) settleTrackedMonitorFromACPUpdateLocked(
	sessionID, toolCallID, status string,
	payload *streams.NormalizedPayload,
) *streams.WorkloadRunObservation {
	if !a.isTrackedMonitorLocked(sessionID, toolCallID) || payload == nil || payload.Monitor() == nil || !payload.IsActiveMonitor() {
		return nil
	}
	state := streams.RunStateCompleted
	switch status {
	case toolStatusError:
		state = streams.RunStateFailed
	case toolStatusCancelled:
		state = streams.RunStateInterrupted
	case toolStatusComplete:
	default:
		return nil
	}
	taskID := payload.Monitor().TaskID
	if taskID == "" {
		return nil
	}
	work := payload.BackgroundWork()
	if work == nil || work.Kind != streams.BackgroundWorkKindMonitor || work.WorkID != taskID {
		return nil
	}
	completion := claudeMonitorCompletion{
		SessionID:  sessionID,
		TaskID:     taskID,
		ToolUseID:  toolCallID,
		State:      state,
		FinishedAt: time.Now().UTC(),
	}
	markMonitorEnded(payload, string(state))
	key := claudeMonitorCompletionKey{taskID: taskID, toolUseID: toolCallID}
	completed := a.completedClaudeMonitorCompletions[sessionID]
	if completed == nil {
		completed = make(map[claudeMonitorCompletionKey]struct{})
		a.completedClaudeMonitorCompletions[sessionID] = completed
	}
	if len(completed) < claudeMonitorMaxCompletionMarks {
		completed[key] = struct{}{}
	}
	return claudeMonitorRunObservation(sessionID, completion, payload)
}

func claudeMonitorSessionPath(workDir, sessionID string) string {
	return filepath.Join("projects", encodeClaudeMonitorProjectName(filepath.Clean(workDir)), sessionID+".jsonl")
}

func (a *Adapter) claudeMonitorToolCallSettledLocked(sessionID, toolCallID string) bool {
	for key := range a.completedClaudeMonitorCompletions[sessionID] {
		if key.toolUseID == toolCallID {
			return true
		}
	}
	return false
}

func isTerminalToolUpdateStatus(status string) bool {
	return status == toolStatusComplete || status == toolStatusError || status == toolStatusCancelled
}

func selectWorkloadObservation(primary, secondary *streams.WorkloadRunObservation) *streams.WorkloadRunObservation {
	if secondary != nil {
		return secondary
	}
	return primary
}
