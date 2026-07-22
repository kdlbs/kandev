package acp

import (
	"encoding/json"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// notifWork is the item type carried on notifQueue. notif is populated for a
// real SDK notification (the common case); sync identifies a barrier and may
// carry afterBarrier state finalization. The worker closes sync after the
// callback, releasing the waiter once everything queued ahead has completed.
type notifWork struct {
	notif        acp.SessionNotification
	sync         chan struct{}
	afterBarrier func()
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
func (a *Adapter) syncNotifQueue() bool {
	return a.syncNotifQueueThen(nil)
}

// syncNotifQueueThen runs afterBarrier on the update worker at the FIFO
// barrier boundary. This is useful when state must be finalized after all
// preceding notifications but before the worker can process later ones.
func (a *Adapter) syncNotifQueueThen(afterBarrier func()) bool {
	ch := make(chan struct{})
	select {
	case <-a.lifetimeCtx.Done():
		return false
	case a.notifQueue <- notifWork{sync: ch, afterBarrier: afterBarrier}:
	}
	select {
	case <-a.lifetimeCtx.Done():
		return false
	case <-ch:
		return true
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
				if item.afterBarrier != nil && a.lifetimeCtx.Err() == nil {
					item.afterBarrier()
				}
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

	suppressed := a.dialect.suppresses(n)
	var event *AgentEvent
	if !suppressed {
		event = a.convertNotification(n)
	}
	if event != nil {
		shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, sessionID, event)
		shared.TraceProtocolEvent(a.getPromptTraceCtx(), shared.ProtocolACP, a.agentID,
			event.Type, rawData, event)
		a.sendUpdate(*event)
		a.maybeScheduleAsyncTurnComplete(*event)
	} else if !suppressed {
		if updateJSON, err := json.Marshal(n.Update); err == nil {
			a.logger.Warn("unhandled ACP session notification",
				zap.String("session_id", sessionID),
				zap.String("update_json", string(updateJSON)))
		}
	}

	if !isLoading {
		if supplemental := a.emitDialectContextWindow(sessionID, n.Meta); supplemental != nil {
			shared.LogNormalizedEvent(shared.ProtocolACP, a.agentID, sessionID, supplemental)
		}
	}
}

// emitDialectContextWindow derives and enqueues an agent-specific context
// sample while holding the same lock that protects the active session/model.
// This keeps a model switch from interleaving between size selection and event
// delivery.
func (a *Adapter) emitDialectContextWindow(sessionID string, meta map[string]any) *AgentEvent {
	a.mu.Lock()
	if a.closed || sessionID != a.sessionID {
		a.mu.Unlock()
		return nil
	}
	sample, ok := a.dialect.contextSample(meta, a.availableModels, a.availableConfigOptions)
	if !ok || a.contextSamples[sessionID] == sample {
		a.mu.Unlock()
		return nil
	}
	remaining := max(sample.size-sample.used, 0)
	event := AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		ContextWindowSize:      sample.size,
		ContextWindowUsed:      sample.used,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(sample.used) / float64(sample.size) * 100,
	}
	sent := a.sendUpdateLocked(event)
	if sent {
		a.contextSamples[sessionID] = sample
	}
	a.mu.Unlock()
	if !sent {
		a.logger.Warn("updates channel full, dropping event", zap.String("type", event.Type))
		return nil
	}
	return &event
}

// convertNotification converts an ACP SessionNotification to an AgentEvent.
func (a *Adapter) convertNotification(n acp.SessionNotification) *AgentEvent {
	u := n.Update
	sessionID := string(n.SessionId)

	switch {
	case u.AgentMessageChunk != nil:
		return a.convertMessageChunkWithProtocolID(
			sessionID,
			u.AgentMessageChunk.Content,
			"assistant",
			derefStr(u.AgentMessageChunk.MessageId),
		)

	case u.UserMessageChunk != nil:
		return a.convertMessageChunkWithProtocolID(
			sessionID,
			u.UserMessageChunk.Content,
			"user",
			derefStr(u.UserMessageChunk.MessageId),
		)

	case u.AgentThoughtChunk != nil:
		if u.AgentThoughtChunk.Content.Text != nil {
			return &AgentEvent{
				Type:              streams.EventTypeReasoning,
				SessionID:         sessionID,
				ProtocolMessageID: derefStr(u.AgentThoughtChunk.MessageId),
				ReasoningText:     u.AgentThoughtChunk.Content.Text.Text,
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
				Data:           map[string]any{"config_options_source": "provider_update"},
			}
		}

	case u.SessionInfoUpdate != nil:
		return &AgentEvent{
			Type:             streams.EventTypeSessionInfo,
			SessionID:        sessionID,
			SessionTitle:     derefStr(u.SessionInfoUpdate.Title),
			SessionUpdatedAt: derefStr(u.SessionInfoUpdate.UpdatedAt),
			SessionMeta:      u.SessionInfoUpdate.Meta,
		}

	case u.UsageUpdate != nil:
		return a.convertUsageUpdate(sessionID, u.UsageUpdate)
	}

	return nil
}

// usageTracker carries the latest cumulative ACP usage sample and the sample
// consumed at the previous prompt boundary. `used` is current context
// occupancy, so its nonnegative growth is only an estimate of turn input. Cost
// is cumulative session cost and is converted to a true per-turn delta.
type usageTracker struct {
	latestUsed           int64
	consumedUsed         int64
	latestCostSubcents   int64
	consumedCostSubcents int64
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

func (a *Adapter) clearUsageTrackers() {
	a.mu.Lock()
	clear(a.usageBySession)
	a.mu.Unlock()
}

// retainConsumedUsageBaselineLocked drops stale session trackers while
// retaining the new session's sticky context size and latest sample. The
// retained sample is marked consumed so session-creation telemetry is not
// attributed to the first prompt. The caller must hold a.mu for writing.
func (a *Adapter) retainConsumedUsageBaselineLocked(sessionID string) {
	tracker := a.usageBySession[sessionID]
	clear(a.usageBySession)
	if tracker == nil {
		return
	}
	a.usageBySession[sessionID] = tracker
	tracker.consumedUsed = tracker.latestUsed
	tracker.consumedCostSubcents = tracker.latestCostSubcents
}

// recordUsageAndMaxSize updates per-session usage/cost tracking and returns
// the sticky-max context window size for the session. A decreasing cumulative
// value means compaction or a provider-side reset; advance both baselines so a
// later consume never emits a negative or stale delta.
func (a *Adapter) recordUsageAndMaxSize(sessionID string, size, used int64, cost *acp.Cost) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	tr := a.ensureUsageTracker(sessionID)
	if size > tr.maxSize {
		tr.maxSize = size
	}
	used = max(used, 0)
	if used < tr.latestUsed {
		tr.consumedUsed = used
	}
	tr.latestUsed = used
	if cost != nil && strings.EqualFold(cost.Currency, "USD") {
		costSubcents := max(int64(cost.Amount*10000), 0)
		if costSubcents < tr.latestCostSubcents {
			tr.consumedCostSubcents = costSubcents
		}
		tr.latestCostSubcents = costSubcents
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
	delete(a.contextSamples, sessionID)
}

// consumeUsageDelta returns nonnegative growth in context occupancy since the
// previous prompt boundary and the delta in cumulative USD session cost. Both
// consumed baselines advance to the latest observed sample.
func (a *Adapter) consumeUsageDelta(sessionID string) (int64, int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	tr := a.usageBySession[sessionID]
	if tr == nil {
		return 0, 0
	}
	delta := max(tr.latestUsed-tr.consumedUsed, 0)
	cost := max(tr.latestCostSubcents-tr.consumedCostSubcents, 0)
	tr.consumedUsed = tr.latestUsed
	tr.consumedCostSubcents = tr.latestCostSubcents
	return delta, cost
}

// consumeUsageBaselineLocked marks replayed usage and cost as historical so
// the first prompt after session/load starts from the restored cumulative
// sample. The caller must hold a.mu for writing.
func (a *Adapter) consumeUsageBaselineLocked(sessionID string) {
	if tr := a.usageBySession[sessionID]; tr != nil {
		tr.consumedUsed = tr.latestUsed
		tr.consumedCostSubcents = tr.latestCostSubcents
	}
}

func (a *Adapter) convertUsageUpdate(sessionID string, usage *acp.SessionUsageUpdate) *AgentEvent {
	if usage == nil || usage.Size <= 0 {
		return nil
	}
	size := int64(usage.Size)
	used := max(int64(usage.Used), 0)
	effectiveSize := a.recordUsageAndMaxSize(sessionID, size, used, usage.Cost)
	remaining := max(effectiveSize-used, 0)
	return &AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		ContextWindowSize:      effectiveSize,
		ContextWindowUsed:      used,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(used) / float64(effectiveSize) * 100,
	}
}

// convertMessageChunk converts an ACP ContentBlock to an AgentEvent, handling multimodal content.
// For text-only messages, sets the Text field for backward compatibility.
// For non-text content, populates ContentBlocks.
//
//nolint:nestif // pre-existing complexity preserved from adapter.go file split
func (a *Adapter) convertMessageChunk(sessionID string, content acp.ContentBlock, role string) *AgentEvent {
	return a.convertMessageChunkWithProtocolID(sessionID, content, role, "")
}

func (a *Adapter) convertMessageChunkWithProtocolID(
	sessionID string,
	content acp.ContentBlock,
	role string,
	protocolMessageID string,
) *AgentEvent {
	event := &AgentEvent{
		Type:              streams.EventTypeMessageChunk,
		SessionID:         sessionID,
		ProtocolMessageID: protocolMessageID,
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
