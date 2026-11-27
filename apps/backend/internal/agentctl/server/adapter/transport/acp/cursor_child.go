package acp

import (
	"context"
	"encoding/json"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type cursorSessionClient struct {
	*acpclient.Client
	adapter *Adapter
}

const cursorChildSpawned = "subagent_spawned"

var _ sdk.RawSessionUpdateHandler = cursorSessionClient{}

type cursorChildFrame struct {
	SessionID string
	Update    struct {
		SessionUpdate     string
		SubagentSessionID string
		Name              string
		Task              string
		State             string
		Meta              struct{ Cursor struct{ ToolCallID string } } `json:"_meta"`
	}
	background bool
}

func (c cursorSessionClient) HandleRawSessionUpdate(_ context.Context, raw json.RawMessage) (bool, error) {
	var frame cursorChildFrame
	if json.Unmarshal(raw, &frame) != nil {
		return false, nil
	}
	switch frame.Update.SessionUpdate {
	case cursorChildSpawned, "subagent_state_update":
		select {
		case c.adapter.notifQueue <- notifWork{cursorChild: &frame, promptGeneration: c.adapter.currentPromptGeneration()}:
		case <-c.adapter.lifetimeCtx.Done():
		}
		return true, nil
	default:
		return false, nil
	}
}

func (a *Adapter) ignoreCursorChildSession(session string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.isLoadingSession {
		return a.loadingSessionID != "" && session != a.loadingSessionID
	}
	return a.sessionID != "" && session != a.sessionID
}

type cursorChild struct {
	id, title, workID   string
	generation          uint64
	background, spawned bool
	terminal, reported  streams.RunState
}

func (a *Adapter) observeCursorBackgroundLaunch(session string, update *sdk.SessionToolCallUpdate, generation uint64) {
	if update == nil {
		return
	}
	output, _ := update.RawOutput.(map[string]any)
	if background, _ := output["isBackground"].(bool); !background {
		return
	}
	frame := cursorChildFrame{SessionID: session, background: true}
	frame.Update.Meta.Cursor.ToolCallID = string(update.ToolCallId)
	a.observeCursorChild(frame, generation)
}

func (a *Adapter) observeCursorChild(frame cursorChildFrame, generation uint64) {
	a.mu.Lock()
	event := a.cursorChildUpdateLocked(frame, generation)
	a.mu.Unlock()
	if event != nil {
		a.sendUpdate(*event)
	}
}

func (a *Adapter) cursorChildUpdateLocked(frame cursorChildFrame, generation uint64) *AgentEvent {
	call := frame.Update.Meta.Cursor.ToolCallID
	if !a.backgroundWorkEnabled || !a.cursorShellWatchSessionLocked(frame.SessionID) || a.isLoadingSession || call == "" {
		return nil
	}
	child := a.cursorChildren[call]
	if child.reported.IsTerminal() {
		return nil
	}
	if !child.apply(frame) {
		return nil
	}
	if child.workID == "" {
		child.workID = uuid.NewString()
		child.generation = generation
	}
	if a.cursorChildren == nil {
		a.cursorChildren = make(map[string]cursorChild)
	}
	a.cursorChildren[call] = child
	if !child.spawned || !child.background {
		return nil
	}
	state := child.terminal
	if state == "" {
		state = streams.RunStateRunning
	}
	if child.reported == state {
		return nil
	}
	child.reported = state
	a.cursorChildren[call] = child
	event := child.event(call, frame.SessionID, state)
	return &event
}

func (child *cursorChild) apply(frame cursorChildFrame) bool {
	if frame.background {
		child.background = true
		return true
	}
	u := frame.Update
	if u.SubagentSessionID == "" || (child.id != "" && child.id != u.SubagentSessionID) {
		return false
	}
	child.id = u.SubagentSessionID
	if u.SessionUpdate == cursorChildSpawned {
		child.spawned = true
		child.title = u.Task
		if child.title == "" {
			child.title = u.Name
		}
		if child.title == "" {
			child.title = child.id
		}
		return true
	}
	if child.terminal != "" {
		return false
	}
	switch u.State {
	case string(sdk.ToolCallStatusCompleted):
		child.terminal = streams.RunStateCompleted
	case string(sdk.ToolCallStatusFailed):
		child.terminal = streams.RunStateFailed
	case string(sdk.StopReasonCancelled):
		child.terminal = streams.RunStateInterrupted
	case "disconnected":
		child.terminal = streams.RunStateUnknown
	default:
		return false
	}
	return true
}

func (child cursorChild) event(call, session string, state streams.RunState) AgentEvent {
	obs := &streams.WorkloadRunObservation{
		SessionID: session, WorkID: child.workID, RunID: child.workID,
		Kind: streams.WorkloadKindSubagent, Title: child.title, State: state,
		SourceCallID: call, Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
	if state.IsTerminal() {
		now := time.Now().UTC()
		obs.FinishedAt = &now
	}
	return AgentEvent{Type: streams.EventTypeBackgroundWorkUpdated, SessionID: session, PromptGeneration: child.generation, BackgroundWork: obs}
}
