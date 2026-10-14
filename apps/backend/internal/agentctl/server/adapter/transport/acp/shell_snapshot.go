package acp

import (
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const shellSnapshotInterval = 250 * time.Millisecond

type shellSnapshotKey struct {
	session, tool, parent, title, status string
	generation                           uint64
	shell                                streams.ShellExecPayload
}

type shellSnapshotState struct {
	key     shellSnapshotKey
	last    time.Time
	pending *AgentEvent
}

func shellSnapshotEligible(n sdk.SessionNotification, event *AgentEvent) bool {
	update := n.Update.ToolCallUpdate
	if update == nil || !foregroundShellSnapshot(event) {
		return false
	}
	if update.Title != nil || update.Name != nil || update.Kind != nil || update.RawInput != nil ||
		update.RawOutput != nil || len(update.Locations) != 0 || len(n.Meta) != 0 {
		return false
	}
	return shellOutputMetadata(update.Meta, event.ToolCallID) && shellOutputContents(event.ToolCallContents) &&
		(len(update.Meta) != 0 || len(event.ToolCallContents) != 0)
}

func foregroundShellSnapshot(event *AgentEvent) bool {
	if event == nil || event.Type != streams.EventTypeToolUpdate || event.ToolStatus != toolStatusInProgress ||
		event.NormalizedPayload == nil || event.NormalizedPayload.BackgroundWork() != nil {
		return false
	}
	shell := event.NormalizedPayload.ShellExec()
	return shell != nil && !shell.Background && shell.Output != nil && shell.Output.ExitCode == nil
}

func shellOutputMetadata(meta map[string]any, toolID string) bool {
	for key, value := range meta {
		if key != "terminal_output" && key != "terminal_output_delta" {
			return false
		}
		if fields, ok := value.(map[string]any); ok && !shellOutputFields(fields, toolID) {
			return false
		}
		if _, ok := terminalOutputData(meta, key); !ok {
			return false
		}
	}
	return true
}

func shellOutputFields(fields map[string]any, toolID string) bool {
	for key, value := range fields {
		switch key {
		case "data":
		case "terminal_id":
			if terminalID, ok := value.(string); !ok || terminalID != toolID {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func shellOutputContents(contents []streams.ToolCallContentItem) bool {
	for _, item := range contents {
		if item.Type != toolContentType || item.Content == nil || item.Path != "" || item.OldText != nil ||
			item.NewText != "" || item.TerminalID != "" {
			return false
		}
		block := *item.Content
		block.Text = ""
		if block != (streams.ContentBlock{Type: contentTypeText}) {
			return false
		}
	}
	return true
}

// Only complete normalized snapshots can supersede output waiting for admission.
// Every source delta has already been merged before this boundary.
func (a *Adapter) sendShellSnapshot(event AgentEvent, generation uint64) {
	a.updateEmitMu.Lock()
	defer a.updateEmitMu.Unlock()
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()
	if closed {
		return
	}
	if event.PromptGeneration == 0 {
		event.PromptGeneration = generation
	}
	shell := *event.NormalizedPayload.ShellExec()
	shell.Output = nil
	key := shellSnapshotKey{session: event.SessionID, tool: event.ToolCallID,
		parent: event.ParentToolCallID, title: event.ToolTitle, status: event.ToolStatus,
		generation: event.PromptGeneration, shell: shell}
	now := time.Now()
	state := &a.shellSnapshots
	if state.key != key || state.last.IsZero() {
		a.flushShellSnapshotLocked()
		*state = shellSnapshotState{key: key, last: now}
		a.sendUpdateNow(event)
		return
	}
	event.NormalizedPayload = event.NormalizedPayload.Snapshot()
	state.pending = &event
	remaining := shellSnapshotInterval - now.Sub(state.last)
	if remaining <= 0 {
		a.flushShellSnapshotLocked()
		return
	}
	if a.shellSnapshotTimer != nil {
		a.shellSnapshotTimer.Reset(remaining)
	}
}

func (a *Adapter) flushShellSnapshotLocked() {
	if a.shellSnapshotTimer != nil {
		a.shellSnapshotTimer.Stop()
	}
	state := &a.shellSnapshots
	if state.pending != nil {
		event := *state.pending
		state.pending = nil
		a.sendUpdateNow(event)
		state.last = time.Now()
	}
}

func (a *Adapter) flushShellSnapshot() {
	a.updateEmitMu.Lock()
	defer a.updateEmitMu.Unlock()
	a.flushShellSnapshotLocked()
	a.shellSnapshots = shellSnapshotState{}
}

func (a *Adapter) flushDueShellSnapshot() {
	a.updateEmitMu.Lock()
	defer a.updateEmitMu.Unlock()
	if time.Since(a.shellSnapshots.last) >= shellSnapshotInterval {
		a.flushShellSnapshotLocked()
	}
}

func (a *Adapter) discardShellSnapshot() {
	a.updateEmitMu.Lock()
	defer a.updateEmitMu.Unlock()
	if a.shellSnapshotTimer != nil {
		a.shellSnapshotTimer.Stop()
	}
	a.shellSnapshots = shellSnapshotState{}
}
