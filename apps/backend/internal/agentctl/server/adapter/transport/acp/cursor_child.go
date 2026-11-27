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

const (
	cursorChildSpawned     = "subagent_spawned"
	cursorChildStateUpdate = "subagent_state_update"
)

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
	case cursorChildSpawned, cursorChildStateUpdate:
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

type cursorLaunchKey struct {
	call       string
	generation uint64
}

type cursorLaunch struct {
	childID    string
	background bool
}

type cursorChild struct {
	call, title, workID string
	generation          uint64
	spawned, background bool
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
	key := cursorLaunchKey{call: call, generation: generation}
	id, background := a.matchCursorChildLaunchLocked(key, frame)
	if id == "" {
		return nil
	}
	child := a.cursorChildren[id]
	if child.call != "" && child.call != call {
		return nil
	}
	if child.workID == "" {
		child.call = call
		child.generation = generation
		child.workID = uuid.NewString()
	}
	child.background = child.background || background
	if !child.apply(frame) {
		return nil
	}
	if a.cursorChildren == nil {
		a.cursorChildren = make(map[string]cursorChild)
	}
	a.cursorChildren[id] = child
	state := child.observedState()
	if child.reported == state {
		return nil
	}
	child.reported = state
	a.cursorChildren[id] = child
	event := child.event(call, frame.SessionID, state)
	return &event
}

// Cursor reuses call IDs across prompts. Only dispatch evidence joins by turn and
// call ID; later state updates identify the child by its native session ID.
func (a *Adapter) matchCursorChildLaunchLocked(key cursorLaunchKey, frame cursorChildFrame) (string, bool) {
	id := frame.Update.SubagentSessionID
	if _, retired := a.retiredCursorChildren[id]; retired {
		return "", false
	}
	launch := a.cursorChildLaunches[key]
	switch {
	case frame.background:
		launch.background = true
	case frame.Update.SessionUpdate == cursorChildSpawned:
		if id == "" || (launch.childID != "" && launch.childID != id) || a.cursorChildren[id].spawned {
			return "", false
		}
		launch.childID = id
	default:
		return id, false
	}
	if a.cursorChildLaunches == nil {
		a.cursorChildLaunches = make(map[cursorLaunchKey]cursorLaunch)
	}
	a.cursorChildLaunches[key] = launch
	return launch.childID, launch.background
}

func (child cursorChild) observedState() streams.RunState {
	if !child.spawned || !child.background {
		return ""
	}
	if child.terminal != "" {
		return child.terminal
	}
	return streams.RunStateRunning
}

func (child *cursorChild) apply(frame cursorChildFrame) bool {
	if frame.background {
		return true
	}
	u := frame.Update
	if u.SessionUpdate == cursorChildSpawned {
		child.spawned = true
		child.title = u.Task
		if child.title == "" {
			child.title = u.Name
		}
		if child.title == "" {
			child.title = u.SubagentSessionID
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
