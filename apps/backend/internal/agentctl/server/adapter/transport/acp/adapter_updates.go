package acp

import (
	"encoding/json"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic AgentEvent.
func (a *Adapter) handleACPUpdate(n acp.SessionNotification) {
	// Fast path during session/load: history-replay notifications can arrive as
	// a burst large enough to overflow the ACP SDK's 1024-deep notification
	// queue if the per-item handler is slow. Check the loading flag first and
	// skip json.Marshal + LogRawEvent for replay notifications we'd suppress
	// anyway. We still capture the Plan + Monitor state needed to reconcile
	// after replay.
	a.mu.RLock()
	isLoading := a.isLoadingSession
	a.mu.RUnlock()

	if isLoading {
		u := n.Update
		// Capture the last Plan from replay so we can re-emit it after load completes.
		a.mu.Lock()
		if u.Plan != nil {
			a.loadReplayPlan = u.Plan
		}
		a.mu.Unlock()

		// Even though we suppress replay notifications from reaching clients,
		// reconstruct any in-progress Monitor registrations so the post-replay
		// sweep can mark them ended-on-restart. Without this, a session where
		// Monitor was running before agentctl died would keep showing a
		// "watching" card forever after resume.
		a.captureReplayMonitor(string(n.SessionId), u)

		// Suppress conversation history events during load.
		// AvailableCommandsUpdate is intentionally NOT suppressed — it may arrive
		// after the replay completes as a "ready" signal, and the frontend treats
		// the last one as authoritative (last-write-wins).
		if u.AgentMessageChunk != nil || u.UserMessageChunk != nil || u.AgentThoughtChunk != nil ||
			u.ToolCall != nil || u.ToolCallUpdate != nil ||
			u.Plan != nil || u.CurrentModeUpdate != nil || u.ConfigOptionUpdate != nil {
			return
		}
	}

	// Marshal once for both debug logging and tracing.
	rawData, _ := json.Marshal(n)
	if len(rawData) > 0 {
		shared.LogRawEvent(shared.ProtocolACP, a.agentID, string(n.SessionId), "session_notification", rawData)
	}

	sessionID := string(n.SessionId)

	event := a.convertNotification(n)
	if event == nil {
		// Try untyped updates not yet supported by the ACP SDK.
		event = a.tryConvertUntypedUpdate(rawData, sessionID)
	}
	if event != nil {
		shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, sessionID, event)
		shared.TraceProtocolEvent(a.getPromptTraceCtx(), shared.ProtocolACP, a.agentID,
			event.Type, rawData, event)
		a.sendUpdate(*event)
	} else if updateJSON, err := json.Marshal(n.Update); err == nil {
		a.logger.Warn("unhandled ACP session notification",
			zap.String("session_id", sessionID),
			zap.String("update_json", string(updateJSON)))
	}
}

// convertNotification converts an ACP SessionNotification to an AgentEvent.
func (a *Adapter) convertNotification(n acp.SessionNotification) *AgentEvent {
	u := n.Update
	sessionID := string(n.SessionId)

	switch {
	case u.AgentMessageChunk != nil:
		return a.convertMessageChunk(sessionID, u.AgentMessageChunk.Content, "assistant")

	case u.UserMessageChunk != nil:
		return a.convertMessageChunk(sessionID, u.UserMessageChunk.Content, "user")

	case u.AgentThoughtChunk != nil:
		if u.AgentThoughtChunk.Content.Text != nil {
			return &AgentEvent{
				Type:          streams.EventTypeReasoning,
				SessionID:     sessionID,
				ReasoningText: u.AgentThoughtChunk.Content.Text.Text,
			}
		}

	case u.ToolCall != nil:
		return a.convertToolCallUpdate(sessionID, u.ToolCall)

	case u.ToolCallUpdate != nil:
		return a.convertToolCallResultUpdate(sessionID, u.ToolCallUpdate)

	case u.Plan != nil:
		entries := make([]PlanEntry, len(u.Plan.Entries))
		for i, e := range u.Plan.Entries {
			entries[i] = PlanEntry{
				Description: e.Content,
				Status:      string(e.Status),
				Priority:    string(e.Priority),
			}
		}
		return &AgentEvent{
			Type:        streams.EventTypePlan,
			SessionID:   sessionID,
			PlanEntries: entries,
		}

	case u.AvailableCommandsUpdate != nil:
		return a.convertAvailableCommands(sessionID, u.AvailableCommandsUpdate)

	case u.CurrentModeUpdate != nil:
		return &AgentEvent{
			Type:          streams.EventTypeSessionMode,
			SessionID:     sessionID,
			CurrentModeID: string(u.CurrentModeUpdate.CurrentModeId),
		}

	case u.ConfigOptionUpdate != nil:
		configOptions := convertACPConfigOptions(u.ConfigOptionUpdate.ConfigOptions)
		if len(configOptions) > 0 {
			// Refresh the cached config options so emitSetModelEvent
			// (called from SetModel) doesn't reuse the stale snapshot from
			// session/new. Include the cached available models so the event
			// doesn't overwrite the model list set during session init.
			a.mu.Lock()
			cachedModels := a.availableModels
			a.availableConfigOptions = configOptions
			a.mu.Unlock()
			currentModelID := resolveCurrentModelFromConfig(configOptions)
			return &AgentEvent{
				Type:           streams.EventTypeSessionModels,
				SessionID:      sessionID,
				CurrentModelID: currentModelID,
				SessionModels:  convertSessionModels(cachedModels),
				ConfigOptions:  configOptions,
			}
		}
	}

	return nil
}

// acpUsageUpdate represents the ACP "usage_update" session notification.
// TODO: Replace with acp.SessionUsageUpdate when the ACP SDK adds native support.
type acpUsageUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`
	Size          int64  `json:"size"`
	Used          int64  `json:"used"`
	Cost          *struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"cost,omitempty"`
}

// usageTracker carries the running cumulative usage state for one ACP
// session — used to infer codex-acp's per-turn deltas. Reset when the
// prompt-complete handler consumes it; lastUsed sticks so subsequent
// updates compute deltas from the new baseline.
//
// lastCostSubcents is the most recent USD cost (Layer A from spec) in
// hundredths of a cent; pumped through to PromptUsage.
// ProviderReportedCostSubcents at consume time. The actual claude-acp
// frames carry a cumulative number, but we treat the most-recent value
// as the per-turn cost because claude-code already emits one cost
// snapshot per session/prompt cycle.
type usageTracker struct {
	lastUsed         int64
	lastCostSubcents int64
}

// recordUsageDelta updates the per-session tracker. Returns the
// integer delta against the previous `used` snapshot — the next prompt
// complete call consumes this via consumeUsageDelta. Cost is forwarded
// as the latest value (claude-acp emits a per-turn cumulative cost; we
// treat the most recent value as the turn's cost).
func (a *Adapter) recordUsageDelta(sessionID string, used int64, costSubcents int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tr := a.usageBySession[sessionID]
	if tr == nil {
		tr = &usageTracker{}
		a.usageBySession[sessionID] = tr
	}
	if used > tr.lastUsed {
		tr.lastUsed = used
	}
	if costSubcents > 0 {
		tr.lastCostSubcents = costSubcents
	}
}

// consumeUsageDelta returns the delta of cumulative `used` since the
// last consumption + the most recent USD cost (subcents). Both fields
// are reset to zero after read so the next turn starts fresh.
func (a *Adapter) consumeUsageDelta(sessionID string) (int64, int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tr := a.usageBySession[sessionID]
	if tr == nil {
		return 0, 0
	}
	delta := tr.lastUsed
	cost := tr.lastCostSubcents
	tr.lastUsed = 0
	tr.lastCostSubcents = 0
	return delta, cost
}

// tryConvertUntypedUpdate handles ACP session update types not yet supported by the SDK.
// When the SDK adds native support, move the handling into convertNotification and delete this.
func (a *Adapter) tryConvertUntypedUpdate(rawNotification []byte, sessionID string) *AgentEvent {
	var envelope struct {
		Update json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(rawNotification, &envelope); err != nil {
		return nil
	}

	var usage acpUsageUpdate
	if err := json.Unmarshal(envelope.Update, &usage); err != nil {
		return nil
	}
	if usage.SessionUpdate != "usage_update" || usage.Size <= 0 {
		return nil
	}

	// Forward usage_update.cost to the prompt-complete handler via the
	// per-session tracker. claude-acp emits a USD float; we convert to
	// hundredths-of-a-cent (subcents). The actual cost frame might be
	// "cumulative-this-session"; the prompt-complete handler currently
	// treats the value as the turn's cost — close enough for the
	// budget+display surface today and revisited if claude-acp's
	// semantics change.
	var costSubcents int64
	if usage.Cost != nil {
		costSubcents = int64(usage.Cost.Amount * 10000)
	}
	a.recordUsageDelta(sessionID, usage.Used, costSubcents)

	remaining := max(usage.Size-usage.Used, 0)
	return &AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		ContextWindowSize:      usage.Size,
		ContextWindowUsed:      usage.Used,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(usage.Used) / float64(usage.Size) * 100,
	}
}

// convertMessageChunk converts an ACP ContentBlock to an AgentEvent, handling multimodal content.
// For text-only messages, sets the Text field for backward compatibility.
// For non-text content, populates ContentBlocks.
//
//nolint:nestif // pre-existing complexity preserved from adapter.go file split
func (a *Adapter) convertMessageChunk(sessionID string, content acp.ContentBlock, role string) *AgentEvent {
	event := &AgentEvent{
		Type:      streams.EventTypeMessageChunk,
		SessionID: sessionID,
	}

	// Only set Role for user messages (assistant is the default)
	if role == "user" {
		event.Role = role
	}

	// Text content goes directly into the Text field for backward compatibility
	if content.Text != nil {
		text := content.Text.Text
		// Claude-acp's Monitor tool injects each script line back to the model
		// as a `<task-notification>` user turn. The wrapper suppresses the
		// user_message_chunk so the model often "echoes" the envelope into its
		// own assistant text. Parse those out into proper Monitor events and
		// strip them from the chat text. Assistant role only — genuine user
		// messages don't carry these.
		if role == "assistant" {
			cleaned := a.routeMonitorEvents(sessionID, text)
			monitorTextRemoved := cleaned != text
			text = cleaned
			if monitorTextRemoved && strings.TrimSpace(text) == "" {
				return nil
			}
			if isMonitorHumanEcho(text) {
				return nil
			}
		}
		event.Text = text
		return event
	}

	// Non-text content uses the shared converter
	cb := a.convertContentBlockToStreams(content)
	if cb == nil {
		return nil
	}
	event.ContentBlocks = []streams.ContentBlock{*cb}
	return event
}

// routeMonitorEvents extracts Monitor `<task-notification>` envelopes from an
// agent_message_chunk text, emits a synthetic tool_call_update for each event
// against the originating Monitor's toolCallID, and returns the cleaned text.
// Returns the original text unchanged when no envelope is present (the common
// case for non-Monitor sessions).
func (a *Adapter) routeMonitorEvents(sessionID, text string) string {
	cleaned, events := extractMonitorEvents(text)
	if len(events) == 0 {
		return text
	}
	for _, ev := range events {
		toolCallID, ok := a.lookupMonitorByTaskID(sessionID, ev.TaskID)
		if !ok {
			a.logger.Debug("monitor event for unknown task, dropping envelope and event body",
				zap.String("session_id", sessionID),
				zap.String("task_id", ev.TaskID))
			continue
		}
		a.mu.Lock()
		payload := a.activeToolCalls[toolCallID]
		appendMonitorEvent(payload, ev.TaskID, monitorCommandFromPayload(payload), ev.Body)
		a.mu.Unlock()
		a.sendUpdate(monitorEventEvent(sessionID, toolCallID, ev.Body, payload))
		a.logger.Debug("monitor event routed",
			zap.String("session_id", sessionID),
			zap.String("task_id", ev.TaskID),
			zap.String("tool_call_id", toolCallID),
			zap.Int("body_len", len(ev.Body)))
	}
	return cleaned
}

// convertAvailableCommands converts an ACP AvailableCommandsUpdate to an AgentEvent,
// including input hints when available.
func (a *Adapter) convertAvailableCommands(sessionID string, update *acp.SessionAvailableCommandsUpdate) *AgentEvent {
	seen := make(map[string]struct{}, len(update.AvailableCommands))
	commands := make([]streams.AvailableCommand, 0, len(update.AvailableCommands))
	for _, cmd := range update.AvailableCommands {
		if _, dup := seen[cmd.Name]; dup {
			continue
		}
		seen[cmd.Name] = struct{}{}
		ac := streams.AvailableCommand{
			Name:        cmd.Name,
			Description: cmd.Description,
		}
		if cmd.Input != nil && cmd.Input.Unstructured != nil {
			ac.InputHint = cmd.Input.Unstructured.Hint
		}
		commands = append(commands, ac)
	}
	return &AgentEvent{
		Type:              streams.EventTypeAvailableCommands,
		SessionID:         sessionID,
		AvailableCommands: commands,
	}
}
