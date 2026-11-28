package acpv2

import (
	"context"
	"strconv"
	"strings"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type projection struct {
	messages       map[messageKey][]wire.ContentBlock
	tools          map[string]wire.ToolCallUpdate
	terminals      map[string]terminalState
	children       map[wire.SessionID]childReference
	subagents      map[wire.SessionID]subagentProjection
	backgroundWork map[backgroundWorkKey]backgroundWorkRecord
}

type promptIdentity struct {
	generation   uint64
	submissionID string
}

func (i promptIdentity) available() bool { return i.generation != 0 }

func promptIdentityFromEvent(event *streams.AgentEvent) promptIdentity {
	if event == nil || event.PromptGeneration == 0 {
		return promptIdentity{}
	}
	return promptIdentity{generation: event.PromptGeneration, submissionID: event.DeliverySubmissionID}
}

type childReference struct {
	toolCallID         string
	origin             promptIdentity
	hasLaunchReference bool
	frozen             bool
}

type subagentProjection struct {
	update wire.SubagentUpdate
}

type messageKey struct {
	session wire.SessionID
	id      wire.MessageID
	kind    wire.SessionUpdateType
}

func (a *Adapter) handleUpdate(_ context.Context, notification wire.UpdateSessionNotification) error {
	a.project(func(batch *eventBatch) {
		if a.ctx.Err() != nil {
			return
		}
		if attach := a.attach; attach != nil {
			a.stageUpdate(attach, notification)
		} else {
			a.projectUpdate(notification, batch)
		}
	})
	return nil
}

func (a *Adapter) stageUpdate(attach *sessionAttachment, notification wire.UpdateSessionNotification) {
	if attach.session == "" || attach.replayed {
		attach.live = append(attach.live, notification)
		return
	}
	if notification.Update.Type() == wire.UpdateReplayComplete && notification.SessionID == attach.session {
		attach.replayed = true
		close(attach.replay)
		return
	}
	a.projectEntity(&attach.state, attach.session, notification, nil)
}

func (a *Adapter) projectUpdate(notification wire.UpdateSessionNotification, batch *eventBatch) {
	if notification.SessionID == a.session {
		switch update := notification.Update.Value.(type) {
		case *wire.StateUpdate:
			a.projectBackgroundSnapshot(&a.state, a.session, update, batch)
			a.observeForeground(update, batch)
			return
		case *wire.ConfigOptionUpdate:
			a.projectSettings(update.ConfigOptions, "", map[string]any{
				streams.ConfigOptionsSourceKey: streams.ConfigOptionsSourceProviderUpdate,
			}, batch)
			return
		case *wire.AvailableCommandsUpdate:
			a.projectCommands(update.AvailableCommands, batch)
			return
		case *wire.UsageUpdate:
			a.projectUsage(update, batch)
			return
		case *wire.PlanUpdate:
			a.projectPlan(update.Plan, batch)
			return
		}
	}
	a.projectEntity(&a.state, a.session, notification, batch)
}

func (a *Adapter) projectEntity(state *projection, root wire.SessionID, notification wire.UpdateSessionNotification, batch *eventBatch) {
	session := notification.SessionID
	parent := state.children[session].toolCallID
	if root == "" || (session != root && parent == "") {
		return
	}
	event := streams.AgentEvent{SessionID: string(root), ParentToolCallID: parent}
	live := batch != nil
	if live && a.turn != nil {
		event.PromptGeneration = a.turn.generation
		event.DeliverySubmissionID = a.turn.submissionID
	}
	var projected bool
	switch update := notification.Update.Value.(type) {
	case *wire.MessageChunk:
		key := makeMessageKey(session, update.MessageID, update.Kind)
		projected = a.projectMessage(state, key, []wire.ContentBlock{update.Content}, true, &event, batch)
	case *wire.MessageUpdate:
		key := makeMessageKey(session, update.MessageID, update.Kind)
		if update.Content.IsSet() {
			content, _ := update.Content.Get()
			projected = a.projectMessage(state, key, content, false, &event, batch)
		}
	case *wire.ToolCallUpdate:
		projected = a.projectTool(state, session, *update, nil, &event, live)
	case *wire.ToolCallContentChunk:
		projected = a.projectTool(state, session, wire.ToolCallUpdate{ToolCallID: update.ToolCallID}, &update.Content, &event, live)
	case *wire.TerminalUpdate:
		projected = state.projectTerminal(session, *update, "", &event, live)
	case *wire.TerminalOutputChunk:
		projected = state.projectTerminal(session, wire.TerminalUpdate{TerminalID: update.TerminalID}, update.Data, &event, live)
	case *wire.SubagentUpdate:
		projected = state.projectSubagent(session, update, &event, live)
	}
	if projected {
		batch.add(event)
		a.projectBackgroundEntityUpdate(state, session, notification, batch)
	}
}

func makeMessageKey(session wire.SessionID, id wire.MessageID, kind wire.SessionUpdateType) messageKey {
	return messageKey{session: session, id: id, kind: wire.SessionUpdateType(strings.TrimSuffix(string(kind), "_chunk"))}
}

func (a *Adapter) projectMessage(
	state *projection, key messageKey, content []wire.ContentBlock, appendChunk bool, event *streams.AgentEvent, batch *eventBatch,
) bool {
	if key.id == "" {
		return false
	}
	if state.messages == nil {
		state.messages = make(map[messageKey][]wire.ContentBlock)
	}
	oldBlocks, known := state.messages[key]
	if appendChunk {
		state.messages[key] = append(oldBlocks, content...)
	} else {
		state.messages[key] = content
	}
	if key.kind == wire.UpdateUserMessage {
		if batch != nil && !known && a.turn != nil && key.session == a.turn.session && a.turn.userMessage == "" {
			a.turn.userMessage = string(key.id)
			a.settleTurn(a.turn, batch)
		}
		return false
	}
	if batch == nil {
		return false
	}
	return setMessageEvent(key, oldBlocks, content, appendChunk, event)
}

func setMessageEvent(
	key messageKey, previous, content []wire.ContentBlock, appendChunk bool, event *streams.AgentEvent,
) bool {
	text := textContent(content)
	if !appendChunk {
		old := textContent(previous)
		if suffix, ok := strings.CutPrefix(text, old); ok {
			text = suffix
		} else {
			event.ReplaceMessageContent = true
		}
	}
	if text == "" && !event.ReplaceMessageContent {
		return false
	}
	event.ProtocolMessageID = entityID(key.session, string(key.id))
	if key.kind == wire.UpdateAgentThought {
		event.Type, event.ReasoningText = streams.EventTypeReasoning, text
	} else {
		event.Type, event.Text = streams.EventTypeMessageChunk, text
		event.Role = "assistant"
	}
	return true
}

func textContent(content []wire.ContentBlock) string {
	var text strings.Builder
	for _, block := range content {
		if block.Type == "text" {
			text.WriteString(valueOrZero(block.Text))
		}
	}
	return text.String()
}

func entityID[T ~string](session wire.SessionID, id T) string {
	return strconv.Quote(string(session)) + ":" + string(id)
}

func (a *Adapter) observeForeground(update *wire.StateUpdate, batch *eventBatch) {
	turn := a.turn
	if turn == nil {
		return
	}
	reason, _ := update.StopReason.Get()
	turn.observeState(string(update.State), string(reason))
	if turn.active {
		a.signalTurnStarted(turn)
	}
	a.settleTurn(turn, batch)
}

func (a *Adapter) projectUsage(update *wire.UsageUpdate, batch *eventBatch) {
	if update.Size == 0 {
		return
	}
	used := min(update.Used, update.Size)
	batch.add(streams.AgentEvent{
		Type: streams.EventTypeContextWindow, SessionID: string(a.session),
		ContextWindowUsed: int64(used), ContextWindowSize: int64(update.Size),
		ContextWindowRemaining: int64(update.Size - used),
		ContextEfficiency:      float64(used) / float64(update.Size) * 100,
	})
}

func (a *Adapter) projectPlan(plan wire.Plan, batch *eventBatch) {
	event := streams.AgentEvent{Type: streams.EventTypePlan, SessionID: string(a.session)}
	switch plan.Type {
	case "items":
		for _, entry := range valueOrZero(plan.Entries) {
			event.PlanEntries = append(event.PlanEntries, streams.PlanEntry{
				Description: entry.Content, Priority: entry.Priority, Status: entry.Status,
			})
		}
	case "markdown":
		event.Type, event.PlanContent = streams.EventTypeAgentPlan, valueOrZero(plan.Content)
	default:
		return
	}
	batch.add(event)
}
