package acp

import (
	"encoding/json"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// notifWork is the item type carried on notifQueue. Exactly one of the two
// fields is populated: notif for a real SDK notification (the common case),
// sync for a barrier posted by syncNotifQueue. The worker closes sync when
// it pops the barrier off the queue, releasing the waiter once everything
// queued ahead of it has been processed.
type notifWork struct {
	notif acp.SessionNotification
	sync  chan struct{}
}

// enqueueACPUpdate is the SDK-facing notification handler. It pushes the
// notification onto notifQueue and returns immediately, so the ACP SDK's
// receive loop is never blocked by our per-item processing (json.Marshal,
// debug log I/O, monitor capture, downstream sendUpdate). The actual work
// runs on runUpdateWorker.
//
// The select honours lifetimeCtx so a Close in flight unblocks any sender
// that would otherwise stall on a full queue. Sending blocks (rather than
// dropping) when the queue is full: with the handleACPUpdate fast path the
// worker drains in nanoseconds-to-microseconds, so a full 4096-slot queue
// means we're in a catastrophic stall and slowing the SDK is preferable to
// silently losing notifications.
func (a *Adapter) enqueueACPUpdate(n acp.SessionNotification) {
	select {
	case <-a.lifetimeCtx.Done():
		return
	case a.notifQueue <- notifWork{notif: n}:
	}
}

// syncNotifQueue blocks until every notifWork enqueued before this call has
// been processed by the worker. It works by posting a barrier item onto the
// same FIFO queue and waiting for the worker to close it.
//
// Motivation: the worker is async, so after conn.Prompt() returns the final
// text chunks emitted by the agent right before the prompt response may
// still be sitting in notifQueue. If sendPrompt emits EventTypeComplete
// immediately, the downstream "turn complete" handler flushes the message
// buffer before those chunks land, the agent's text is dropped, and the
// turn persists as had_output=false. Calling this before the complete emit
// guarantees the chunks are delivered to updatesCh first.
//
// No caller-context honored: returning before the barrier closes would
// reintroduce the very race this primitive exists to prevent (sweeps and
// EventTypeComplete running while the worker still has queued frames).
// Only adapter shutdown via lifetimeCtx can release the wait early.
func (a *Adapter) syncNotifQueue() {
	ch := make(chan struct{})
	select {
	case <-a.lifetimeCtx.Done():
		return
	case a.notifQueue <- notifWork{sync: ch}:
	}
	select {
	case <-a.lifetimeCtx.Done():
	case <-ch:
	}
}

// startUpdateWorker spawns the goroutine that drains notifQueue and calls
// handleACPUpdate for each notification. Called exactly once from
// NewAdapter so the queue always has a consumer for the adapter's
// lifetime — do not call again (a second invocation would Add(1) to
// workerWg and spawn a second reader, breaking the single-worker FIFO
// guarantee). Close waits for the worker to exit via workerWg before
// closing updatesCh.
func (a *Adapter) startUpdateWorker() {
	a.workerWg.Add(1)
	go a.runUpdateWorker()
}

// runUpdateWorker is the worker loop. It exits when lifetimeCtx is cancelled
// (by Close). A single worker preserves FIFO ordering across notifications,
// matching the SDK's own serial delivery contract.
func (a *Adapter) runUpdateWorker() {
	defer a.workerWg.Done()
	for {
		select {
		case <-a.lifetimeCtx.Done():
			return
		case item := <-a.notifQueue:
			if item.sync != nil {
				close(item.sync)
				continue
			}
			a.handleACPUpdate(item.notif)
		}
	}
}

// handleACPUpdate converts ACP SessionNotification to protocol-agnostic AgentEvent.
// Runs synchronously on the update worker goroutine; do not call from the SDK's
// notification path (use enqueueACPUpdate instead). Unit tests invoke this
// directly to exercise the conversion logic without spinning up the worker.
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
		a.maybeScheduleAsyncTurnComplete(*event)
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
			return &AgentEvent{
				Type:           streams.EventTypeSessionModels,
				SessionID:      sessionID,
				CurrentModelID: currentModelFromConfig(configOptions),
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

// acpSessionInfoUpdate represents the ACP "session_info_update" notification.
// TODO: Replace with the SDK type when acp-go-sdk exposes it.
type acpSessionInfoUpdate struct {
	SessionUpdate string         `json:"sessionUpdate"`
	Title         string         `json:"title,omitempty"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
	Meta          map[string]any `json:"_meta,omitempty"`
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
	// maxSize is the largest context-window size reported for this session.
	// claude-acp's default model emits a 200K turn-start frame and a 1M
	// turn-end frame; sticky-max prevents the stale 200K from shrinking
	// the displayed window after the authoritative 1M frame lands.
	maxSize int64
}

// ensureUsageTracker returns the per-session usage tracker, creating it if
// needed. Callers must hold a.mu (write lock).
func (a *Adapter) ensureUsageTracker(sessionID string) *usageTracker {
	tr := a.usageBySession[sessionID]
	if tr == nil {
		tr = &usageTracker{}
		a.usageBySession[sessionID] = tr
	}
	return tr
}

// recordUsageAndMaxSize updates per-session usage/cost tracking and returns
// the sticky-max context window size for the session.
func (a *Adapter) recordUsageAndMaxSize(sessionID string, size, used, costSubcents int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	tr := a.ensureUsageTracker(sessionID)
	if size > tr.maxSize {
		tr.maxSize = size
	}
	if used > tr.lastUsed {
		tr.lastUsed = used
	}
	if costSubcents > 0 {
		tr.lastCostSubcents = costSubcents
	}
	return tr.maxSize
}

// resetContextWindowMaxSize clears the sticky-max window for a session so the
// next usage_update can establish the new model's window after SetModel.
func (a *Adapter) resetContextWindowMaxSize(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tr := a.usageBySession[sessionID]; tr != nil {
		tr.maxSize = 0
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

	var sessionInfo acpSessionInfoUpdate
	if err := json.Unmarshal(envelope.Update, &sessionInfo); err == nil &&
		sessionInfo.SessionUpdate == "session_info_update" {
		return &AgentEvent{
			Type:             streams.EventTypeSessionInfo,
			SessionID:        sessionID,
			SessionTitle:     sessionInfo.Title,
			SessionUpdatedAt: sessionInfo.UpdatedAt,
			SessionMeta:      sessionInfo.Meta,
		}
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
	effectiveSize := a.recordUsageAndMaxSize(sessionID, usage.Size, usage.Used, costSubcents)

	remaining := max(effectiveSize-usage.Used, 0)
	return &AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		ContextWindowSize:      effectiveSize,
		ContextWindowUsed:      usage.Used,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(usage.Used) / float64(effectiveSize) * 100,
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
		update := monitorEventEvent(sessionID, toolCallID, ev.Body, payload)
		a.sendUpdate(update)
		a.maybeScheduleAsyncTurnComplete(update)
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
