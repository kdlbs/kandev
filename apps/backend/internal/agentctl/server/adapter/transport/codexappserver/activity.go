package codexappserver

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const (
	childStatusCompleted   = "completed"
	childStatusComplete    = "complete"
	childStatusErrored     = "errored"
	childStatusRunning     = "running"
	childStatusCancelled   = "cancelled"
	childStatusError       = "error"
	childStatusFailed      = "failed"
	childStatusInterrupted = "interrupted"
	childStatusCanceled    = "canceled"
)

type bufferedChildActivity struct {
	threadID string
	binding  childBinding
	activity string
}

func hasActiveChild(statuses, early map[string]string) bool {
	for _, status := range statuses {
		if !isTerminalChildStatus(status) {
			return true
		}
	}
	for _, status := range early {
		if !isTerminalChildStatus(status) {
			return true
		}
	}
	return false
}

func isTerminalChildStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case childStatusCompleted, childStatusComplete, childStatusFailed, childStatusError, childStatusErrored, childStatusInterrupted, childStatusCancelled, childStatusCanceled:
		return true
	default:
		return false
	}
}

func (a *Adapter) emitCollabToolCall(item map[string]any, rootThreadID, turnID string, completed bool) {
	callID := stringField(item, "id")
	if callID == "" {
		return
	}
	tool := stringField(item, "tool")
	model := stringField(item, "model")
	prompt := stringField(item, "prompt")
	description := strings.TrimSpace(prompt)
	if description == "" {
		description = "Codex subagent"
	}
	status := stringField(item, "status")
	if status == "" && completed {
		status = childStatusCompleted
	}
	terminal := status == childStatusCompleted || status == childStatusFailed || status == childStatusInterrupted
	receivers := stringSliceField(item, "receiverThreadIds")
	generation := a.currentGeneration()

	recorded, buffered := a.recordCollabToolCall(callID, status, receivers, rootThreadID, description, tool, model, generation)
	if !recorded {
		return
	}

	payload := streams.NewSubagentTask(description, prompt, tool)
	child := payload.SubagentTask()
	child.Model = model
	if len(receivers) == 1 {
		child.ProviderThreadID = receivers[0]
		child.ChildSessionID = receivers[0]
	}
	workID := callID
	if len(receivers) == 1 {
		workID = receivers[0]
	}
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, workID, !terminal, terminal)
	eventType := streams.EventTypeToolCall
	toolStatus := childStatusRunning
	if terminal {
		eventType = streams.EventTypeToolUpdate
		switch status {
		case childStatusFailed:
			toolStatus = childStatusError
		case childStatusInterrupted:
			toolStatus = childStatusInterrupted
		default:
			toolStatus = childStatusCompleted
		}
	}
	a.emit(streams.AgentEvent{
		Type:              eventType,
		SessionID:         rootThreadID,
		OperationID:       turnID,
		PromptGeneration:  generation,
		ToolCallID:        callID,
		ToolName:          tool,
		ToolTitle:         description,
		ToolStatus:        toolStatus,
		NormalizedPayload: payload,
	})
	for _, activity := range buffered {
		a.emitSubagentToolUpdate(activity.binding, subagentActivityToolStatus(activity.activity), activity.threadID)
	}
}

func (a *Adapter) recordCollabToolCall(
	callID, status string,
	receivers []string,
	rootThreadID, description, tool, model string,
	generation uint64,
) (bool, []bufferedChildActivity) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if previous, ok := a.childStatuses[callID]; ok && previous == status {
		return false, nil
	}
	a.childStatuses[callID] = status
	var buffered []bufferedChildActivity
	for _, receiver := range receivers {
		if receiver == "" {
			continue
		}
		a.children[receiver] = childBinding{
			toolCallID:     callID,
			parentThreadID: rootThreadID,
			generation:     generation,
			description:    description,
			subagentType:   tool,
			model:          model,
		}
		if activity, ok := a.earlyChildActivities[receiver]; ok {
			delete(a.earlyChildActivities, receiver)
			a.childStatuses[receiver] = activity
			buffered = append(buffered, bufferedChildActivity{
				threadID: receiver, binding: a.children[receiver], activity: activity,
			})
		}
	}
	return true, buffered
}

func (a *Adapter) emitSubagentActivity(item map[string]any) {
	childThreadID := stringField(item, "agentThreadId")
	if childThreadID == "" {
		return
	}
	a.mu.Lock()
	binding, ok := a.children[childThreadID]
	activity := stringField(item, "kind")
	if !ok {
		a.earlyChildActivities[childThreadID] = activity
		a.mu.Unlock()
		return
	}
	previous := a.childStatuses[childThreadID]
	if previous == activity {
		a.mu.Unlock()
		return
	}
	a.childStatuses[childThreadID] = activity
	a.mu.Unlock()

	a.emitSubagentToolUpdate(binding, subagentActivityToolStatus(activity), childThreadID)
}

func subagentActivityToolStatus(activity string) string {
	switch strings.ToLower(strings.TrimSpace(activity)) {
	case childStatusCompleted, "complete":
		return childStatusCompleted
	case childStatusFailed, childStatusError, "errored":
		return childStatusError
	case childStatusInterrupted, childStatusCancelled, "canceled":
		return childStatusInterrupted
	default:
		return childStatusRunning
	}
}

func (a *Adapter) emitChildStatus(childThreadID, status string) {
	a.mu.Lock()
	binding, ok := a.children[childThreadID]
	if !ok {
		a.earlyChildActivities[childThreadID] = status
		a.mu.Unlock()
		return
	}
	previous := a.childStatuses[childThreadID]
	if previous == status {
		a.mu.Unlock()
		return
	}
	a.childStatuses[childThreadID] = status
	a.mu.Unlock()

	switch strings.ToLower(status) {
	case childStatusCompleted, "complete":
		status = childStatusCompleted
	case childStatusFailed, "errored":
		status = childStatusError
	case childStatusInterrupted, childStatusCancelled, "canceled":
		status = childStatusInterrupted
	case childStatusRunning, "inprogress", "started":
		status = childStatusRunning
	default:
		return
	}
	a.emitSubagentToolUpdate(binding, status, childThreadID)
}

func (a *Adapter) emitSubagentToolUpdate(binding childBinding, status, childThreadID string) {
	payload := streams.NewSubagentTask(binding.description, "", binding.subagentType)
	payload.SubagentTask().ProviderThreadID = childThreadID
	payload.SubagentTask().ChildSessionID = childThreadID
	ended := status == childStatusCompleted || status == childStatusError || status == childStatusInterrupted
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, childThreadID, !ended, ended)
	a.emit(streams.AgentEvent{
		Type:              streams.EventTypeToolUpdate,
		SessionID:         binding.parentThreadID,
		PromptGeneration:  binding.generation,
		ToolCallID:        binding.toolCallID,
		ToolName:          binding.subagentType,
		ToolTitle:         binding.description,
		ToolStatus:        status,
		NormalizedPayload: payload,
	})
}

func (a *Adapter) emitCommandExecution(item map[string]any, rootThreadID, parentToolCallID string, completed bool) {
	itemID := stringField(item, "id")
	command := stringField(item, "command")
	workDir := stringField(item, "cwd")
	status := stringField(item, "status")
	if itemID == "" {
		return
	}
	a.mu.RLock()
	_, background := a.backgrounds[itemID]
	a.mu.RUnlock()
	payload := streams.NewShellExec(command, workDir, "", 0, background)
	if completed {
		output := &streams.ShellExecOutput{Stdout: stringField(item, "aggregatedOutput")}
		if rawExitCode, ok := item["exitCode"].(float64); ok {
			exitCode := int(rawExitCode)
			output.ExitCode = &exitCode
		}
		if output.Stdout != "" || output.ExitCode != nil {
			payload.ShellExec().Output = output
		}
	}
	if background {
		payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, itemID, !completed, completed)
	}
	eventType := streams.EventTypeToolCall
	if completed {
		eventType = streams.EventTypeToolUpdate
		if status == "" {
			status = childStatusCompleted
		}
	} else {
		status = childStatusRunning
	}
	a.emit(streams.AgentEvent{
		Type:              eventType,
		SessionID:         rootThreadID,
		ParentToolCallID:  parentToolCallID,
		ToolCallID:        itemID,
		ToolName:          "commandExecution",
		ToolTitle:         command,
		ToolStatus:        status,
		NormalizedPayload: payload,
	})
}

func (a *Adapter) restoreThreadBindings(ctx context.Context, threadID string) {
	client := a.getClient()
	if client == nil {
		return
	}
	var response protocol.ThreadReadResponse
	if err := client.Call(ctx, protocol.MethodThreadRead, protocol.ThreadReadParams{ThreadID: threadID, IncludeTurns: true}, &response); err != nil {
		return
	}
	for _, turn := range response.Thread.Turns {
		for _, rawItem := range turn.Items {
			var item map[string]any
			if json.Unmarshal(rawItem, &item) != nil {
				continue
			}
			switch stringField(item, "type") {
			case "collabAgentToolCall":
				a.restoreCollabBinding(item, threadID)
			case "subAgentActivity":
				a.restoreSubagentActivity(item)
			}
		}
	}
}

func (a *Adapter) restoreCollabBinding(item map[string]any, rootThreadID string) {
	callID := stringField(item, "id")
	if callID == "" {
		return
	}
	description := strings.TrimSpace(stringField(item, "prompt"))
	if description == "" {
		description = "Codex subagent"
	}
	a.mu.Lock()
	a.childStatuses[callID] = stringField(item, "status")
	for _, childThreadID := range stringSliceField(item, "receiverThreadIds") {
		if childThreadID == "" {
			continue
		}
		a.children[childThreadID] = childBinding{
			toolCallID:     callID,
			parentThreadID: rootThreadID,
			generation:     0,
			description:    description,
			subagentType:   stringField(item, "tool"),
			model:          stringField(item, "model"),
		}
	}
	a.mu.Unlock()
}

func (a *Adapter) restoreSubagentActivity(item map[string]any) {
	childThreadID := stringField(item, "agentThreadId")
	if childThreadID == "" {
		return
	}
	a.mu.Lock()
	if _, ok := a.children[childThreadID]; !ok {
		a.earlyChildActivities[childThreadID] = stringField(item, "kind")
	}
	a.mu.Unlock()
}

func (a *Adapter) refreshBackgroundTerminals(ctx context.Context, threadID string) {
	client := a.getClient()
	if client == nil || threadID == "" {
		return
	}
	var response protocol.BackgroundTerminalsListResponse
	if err := client.Call(ctx, protocol.MethodBackgroundTerminalsList, protocol.BackgroundTerminalsListParams{ThreadID: threadID}, &response); err != nil {
		return
	}
	next := make(map[string]protocol.BackgroundTerminal, len(response.Data))
	for _, terminal := range response.Data {
		if terminal.ItemID != "" {
			next[terminal.ItemID] = terminal
		}
	}
	a.mu.Lock()
	previous := a.backgrounds
	initialized := a.backgroundSnapshotLoaded
	a.backgrounds = next
	a.backgroundSnapshotLoaded = true
	a.mu.Unlock()
	for itemID, terminal := range next {
		if _, existed := previous[itemID]; !existed {
			a.emitBackgroundStart(threadID, terminal)
		}
	}
	if initialized {
		for itemID := range previous {
			if _, stillRunning := next[itemID]; !stillRunning {
				a.emit(streams.AgentEvent{Type: streams.EventTypeBackgroundComplete, SessionID: threadID, ToolCallID: itemID})
			}
		}
	}
	if len(next) > 0 {
		a.ensureBackgroundPoller(threadID)
	}
}

func (a *Adapter) emitBackgroundStart(threadID string, terminal protocol.BackgroundTerminal) {
	payload := streams.NewShellExec(terminal.Command, terminal.CWD, "", 0, true)
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, terminal.ItemID, true, false)
	a.emit(streams.AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         threadID,
		ToolCallID:        terminal.ItemID,
		ToolName:          "commandExecution",
		ToolTitle:         terminal.Command,
		ToolStatus:        childStatusRunning,
		NormalizedPayload: payload,
	})
}

func (a *Adapter) ensureBackgroundPoller(threadID string) {
	a.mu.Lock()
	if a.backgroundCancel != nil || a.closed {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.backgroundCancel = cancel
	a.mu.Unlock()
	go func() {
		ticker := time.NewTicker(backgroundPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.refreshBackgroundTerminals(ctx, threadID)
			}
		}
	}()
}

func (a *Adapter) eventScope(threadID string) (rootThreadID, parentToolCallID string, isChild bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if binding, ok := a.children[threadID]; ok {
		return binding.parentThreadID, binding.toolCallID, true
	}
	return threadID, "", false
}

func (a *Adapter) currentGeneration() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeGeneration
}

func stringSliceField(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok {
			items = append(items, value)
		}
	}
	return items
}
