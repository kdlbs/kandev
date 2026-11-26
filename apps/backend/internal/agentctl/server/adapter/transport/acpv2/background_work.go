package acpv2

import (
	"reflect"
	"time"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type backgroundWorkKey struct {
	session wire.SessionID
	kind    streams.WorkloadKind
	id      string
}

type backgroundWorkRecord struct {
	listed      bool
	observation streams.WorkloadRunObservation
	origin      promptIdentity
}

func (a *Adapter) projectBackgroundSnapshot(
	state *projection, session wire.SessionID, update *wire.StateUpdate, batch *eventBatch,
) {
	if !a.cfg.AgentBackgroundWorkEnabled || update == nil {
		return
	}
	items, present := update.Background.Get()
	if !present {
		a.markBackgroundUnknown(state, session, batch)
		return
	}
	seen := make(map[backgroundWorkKey]struct{}, len(items))
	for _, item := range items {
		key, ok := backgroundWorkKeyFor(session, item)
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		a.projectBackgroundRecord(state, key, true, batch)
	}
	for key, record := range state.backgroundWork {
		if key.session != session || !record.listed {
			continue
		}
		if _, ok := seen[key]; !ok {
			a.projectBackgroundRecord(state, key, false, batch)
		}
	}
}

func backgroundWorkKeyFor(session wire.SessionID, item wire.BackgroundWork) (backgroundWorkKey, bool) {
	switch item.Type {
	case "terminal":
		if item.TerminalID != nil {
			return backgroundWorkKey{session: session, kind: streams.WorkloadKindShell, id: string(*item.TerminalID)}, true
		}
	case "subagent":
		if item.SessionID != nil {
			return backgroundWorkKey{session: session, kind: streams.WorkloadKindSubagent, id: string(*item.SessionID)}, true
		}
	}
	return backgroundWorkKey{}, false
}

func (a *Adapter) markBackgroundUnknown(state *projection, session wire.SessionID, batch *eventBatch) {
	for key, record := range state.backgroundWork {
		if key.session == session && record.listed {
			a.projectBackgroundRecord(state, key, false, batch)
		}
	}
}

func (a *Adapter) projectBackgroundRecord(
	state *projection, key backgroundWorkKey, listed bool, batch *eventBatch,
) {
	if state.backgroundWork == nil {
		state.backgroundWork = make(map[backgroundWorkKey]backgroundWorkRecord)
	}
	previous, exists := state.backgroundWork[key]
	next := a.backgroundObservation(state, key, previous, exists, listed)
	origin := previous.origin
	if !origin.available() {
		origin = state.backgroundWorkOrigin(key)
	}
	changed := !exists || !reflect.DeepEqual(previous.observation, next) || previous.origin != origin
	state.backgroundWork[key] = backgroundWorkRecord{listed: listed, observation: next, origin: origin}
	if changed && batch != nil {
		event := streams.AgentEvent{
			Type: streams.EventTypeBackgroundWorkUpdated, SessionID: string(key.session), BackgroundWork: &next,
		}
		if origin.available() {
			event.PromptGeneration = origin.generation
			event.DeliverySubmissionID = origin.submissionID
		}
		batch.add(event)
	}
}

func (p *projection) backgroundWorkOrigin(key backgroundWorkKey) promptIdentity {
	switch key.kind {
	case streams.WorkloadKindShell:
		return p.terminals[entityID(key.session, "terminal:"+key.id)].origin
	case streams.WorkloadKindSubagent:
		return p.children[wire.SessionID(key.id)].origin
	default:
		return promptIdentity{}
	}
}

func (a *Adapter) backgroundObservation(
	state *projection, key backgroundWorkKey, previous backgroundWorkRecord, exists, listed bool,
) streams.WorkloadRunObservation {
	observation := streams.WorkloadRunObservation{
		SessionID: string(key.session), WorkID: entityID(key.session, string(key.kind)+":"+key.id),
		Kind: key.kind, State: streams.RunStateUnknown, Capabilities: backgroundWorkCapabilities(key.kind),
	}
	if exists {
		observation.RunID = previous.observation.RunID
		observation.OriginTurnID = previous.observation.OriginTurnID
		observation.SourceMessageID = previous.observation.SourceMessageID
		observation.SourceCallID = previous.observation.SourceCallID
		observation.ParentWorkID = previous.observation.ParentWorkID
		observation.StartedAt = previous.observation.StartedAt
		observation.FinishedAt = previous.observation.FinishedAt
		observation.Revision = previous.observation.Revision
		observation.ExitCode = previous.observation.ExitCode
		observation.Output = previous.observation.Output
		observation.OutputOffset = previous.observation.OutputOffset
		observation.OutputTruncated = previous.observation.OutputTruncated
	}
	stateObserved := false
	terminalEvidence := false
	switch key.kind {
	case streams.WorkloadKindShell:
		stateObserved = projectTerminalObservation(state, key, &observation)
		terminalEvidence = stateObserved
	case streams.WorkloadKindSubagent:
		stateObserved = projectSubagentObservation(state, key, &observation)
	}
	switch {
	case exists && previous.observation.State.IsTerminal() && !terminalEvidence:
		observation.State = previous.observation.State
	case !listed && !terminalEvidence:
		observation.State = streams.RunStateUnknown
	case stateObserved:
		// Provider evidence is authoritative, including an explicit unknown
		// child state while it remains in the root's background list.
	default:
		observation.State = streams.RunStateRunning
	}
	if observation.State.IsTerminal() && observation.FinishedAt == nil {
		finished := time.Now().UTC()
		observation.FinishedAt = &finished
	}
	return observation
}

func projectTerminalObservation(state *projection, key backgroundWorkKey, observation *streams.WorkloadRunObservation) bool {
	terminal, ok := state.terminals[entityID(key.session, "terminal:"+key.id)]
	if !ok {
		observation.Title = "Terminal"
		observation.SourceCallID = entityID(key.session, "terminal:"+key.id)
		return false
	}
	if terminal.command != "" {
		observation.Title = terminal.command
	} else if observation.Title == "" {
		observation.Title = "Terminal"
	}
	if terminal.sourceCallID != "" {
		observation.SourceCallID = terminal.sourceCallID
	} else if observation.SourceCallID == "" {
		observation.SourceCallID = entityID(key.session, "terminal:"+key.id)
	}
	observation.Output = terminal.output
	observation.OutputTruncated = terminal.truncated
	if exit, ok := terminal.exit.Get(); ok {
		observation.State = terminalRunState(exit)
		if exit.ExitCode != nil {
			code := int(*exit.ExitCode)
			observation.ExitCode = &code
		}
		return true
	}
	return false
}

func terminalRunState(exit wire.TerminalExitStatus) streams.RunState {
	if exit.ExitCode != nil {
		if *exit.ExitCode == 0 {
			return streams.RunStateCompleted
		}
		return streams.RunStateFailed
	}
	if exit.Signal != nil {
		return streams.RunStateInterrupted
	}
	return streams.RunStateEnded
}

func projectSubagentObservation(state *projection, key backgroundWorkKey, observation *streams.WorkloadRunObservation) bool {
	childID := wire.SessionID(key.id)
	child, ok := state.subagents[childID]
	stateObserved := false
	if !ok {
		observation.Title = "Subagent"
	} else {
		if title, present := child.update.Title.Get(); present && title != "" {
			observation.Title = title
		} else if description, present := child.update.Description.Get(); present && description != "" {
			observation.Title = description
		}
		if childState, present := child.update.State.Get(); present {
			observation.State = subagentRunState(childState)
			stateObserved = true
		}
	}
	childReference := state.children[childID]
	if childReference.hasLaunchReference && childReference.toolCallID != "" {
		observation.SourceCallID = childReference.toolCallID
	}
	return stateObserved
}

func subagentRunState(state wire.StateUpdate) streams.RunState {
	switch state.State {
	case wire.StateRunning:
		return streams.RunStateRunning
	case wire.StateRequiresAction:
		return streams.RunStateWaiting
	case wire.StateIdle:
		background, present := state.Background.Get()
		if present && len(background) != 0 {
			return streams.RunStateRunning
		}
		return streams.RunStateUnknown
	default:
		return streams.RunStateUnknown
	}
}

func backgroundWorkCapabilities(kind streams.WorkloadKind) streams.WorkloadCapabilities {
	output := "none"
	if kind == streams.WorkloadKindShell {
		output = "snapshot"
	}
	return streams.WorkloadCapabilities{Discovery: "snapshot", Output: output, Transcript: true}
}

func (a *Adapter) projectBackgroundEntityUpdate(
	state *projection, session wire.SessionID, notification wire.UpdateSessionNotification, batch *eventBatch,
) {
	if !a.cfg.AgentBackgroundWorkEnabled {
		return
	}
	var key backgroundWorkKey
	switch update := notification.Update.Value.(type) {
	case *wire.TerminalUpdate:
		key = backgroundWorkKey{session: session, kind: streams.WorkloadKindShell, id: string(update.TerminalID)}
	case *wire.TerminalOutputChunk:
		key = backgroundWorkKey{session: session, kind: streams.WorkloadKindShell, id: string(update.TerminalID)}
	case *wire.SubagentUpdate:
		key = backgroundWorkKey{session: session, kind: streams.WorkloadKindSubagent, id: string(update.SessionID)}
	case *wire.ToolCallUpdate:
		if content, present := update.Content.Get(); present {
			a.projectBackgroundToolReferences(state, session, content, batch)
		}
		return
	case *wire.ToolCallContentChunk:
		a.projectBackgroundToolReferences(state, session, []wire.ToolCallContent{update.Content}, batch)
		return
	default:
		return
	}
	if record, ok := state.backgroundWork[key]; ok {
		a.projectBackgroundRecord(state, key, record.listed, batch)
	}
}

func (a *Adapter) projectBackgroundToolReferences(
	state *projection, session wire.SessionID, content []wire.ToolCallContent, batch *eventBatch,
) {
	seen := make(map[backgroundWorkKey]struct{}, len(content))
	for _, item := range content {
		var keys []backgroundWorkKey
		if item.TerminalID != nil {
			keys = append(keys, backgroundWorkKey{session: session, kind: streams.WorkloadKindShell, id: string(*item.TerminalID)})
		}
		if item.SessionID != nil {
			keys = append(keys, backgroundWorkKey{session: session, kind: streams.WorkloadKindSubagent, id: string(*item.SessionID)})
		}
		for _, key := range keys {
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			if record, ok := state.backgroundWork[key]; ok {
				a.projectBackgroundRecord(state, key, record.listed, batch)
			}
		}
	}
}

func (p *projection) mergeBackgroundWork(previous map[backgroundWorkKey]backgroundWorkRecord) {
	if len(previous) == 0 {
		return
	}
	if p.backgroundWork == nil {
		p.backgroundWork = make(map[backgroundWorkKey]backgroundWorkRecord, len(previous))
	}
	for key, record := range previous {
		current, exists := p.backgroundWork[key]
		if !exists {
			p.backgroundWork[key] = record
			continue
		}
		if !current.origin.available() && record.origin.available() {
			current.origin = record.origin
			p.backgroundWork[key] = current
		}
	}
}
