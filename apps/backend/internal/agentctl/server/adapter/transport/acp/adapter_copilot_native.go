package acp

import (
	"encoding/json"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

const copilotPendingNativeLimit = 64

// enqueueCopilotNativeNotification copies one validated vendor event onto the
// adapter FIFO. It runs on the SDK receive goroutine, so it must never block on
// normalization, locks, or downstream delivery.
func (a *Adapter) enqueueCopilotNativeNotification(method string, params json.RawMessage) {
	if !a.copilotNativeActive.Load() || a.agentID != copilotAgentID {
		return
	}
	decoded, ok := decodeCopilotNativeEvent(method, params)
	if !ok {
		return
	}
	work := decoded
	select {
	case <-a.lifetimeCtx.Done():
		return
	case a.notifQueue <- notifWork{native: &work}:
	}
}

// handleCopilotNativeWork runs on the single update worker. Events that arrive
// before the native conversation binding is established are retained in a
// bounded FIFO and drained at the binding barrier.
func (a *Adapter) handleCopilotNativeWork(event decodedCopilotNativeEvent) {
	if !a.copilotNativeActive.Load() {
		return
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	if a.sessionID == "" || a.isLoadingSession || a.bindingCopilotNative {
		if len(a.pendingCopilotNative) < copilotPendingNativeLimit {
			a.pendingCopilotNative = append(a.pendingCopilotNative, event)
		} else {
			a.pendingCopilotNativeDropped = true
			a.logger.Warn("dropping Copilot native event: pending binding buffer full",
				zap.String("event_type", event.Type))
		}
		a.mu.Unlock()
		return
	}
	active := event.SessionID == a.sessionID
	a.mu.Unlock()
	if !active {
		a.logger.Debug("dropping Copilot native event for a stale conversation",
			zap.String("event_type", event.Type),
			zap.String("native_session_id", event.SessionID))
		return
	}
	a.applyCopilotNativeEvent(event)
}

// flushPendingCopilotNative drains buffered events in arrival order. The caller
// must run this at a FIFO barrier after committing the new session identity.
func (a *Adapter) flushPendingCopilotNative() {
	a.mu.Lock()
	pending := a.pendingCopilotNative
	a.pendingCopilotNative = nil
	dropped := a.pendingCopilotNativeDropped
	a.pendingCopilotNativeDropped = false
	activeSession := a.sessionID
	isLoading := a.isLoadingSession
	a.mu.Unlock()
	if isLoading {
		a.mu.Lock()
		// A new load has started; retain the older events ahead of anything
		// buffered since and preserve both overflow and truncation evidence.
		a.pendingCopilotNativeDropped = a.pendingCopilotNativeDropped || dropped
		pending = append(pending, a.pendingCopilotNative...)
		if len(pending) > copilotPendingNativeLimit {
			pending = pending[:copilotPendingNativeLimit]
			a.pendingCopilotNativeDropped = true
		}
		a.pendingCopilotNative = pending
		a.mu.Unlock()
		return
	}
	for _, event := range pending {
		if event.SessionID != activeSession {
			continue
		}
		a.applyCopilotNativeEvent(event)
	}
	if dropped && activeSession != "" {
		a.emitCopilotNativeReporting(activeSession, streams.ContextCompactionAvailabilityAvailable,
			streams.ContextCompactionCoveragePartial, "binding_buffer_overflow")
	}
}

func (a *Adapter) applyCopilotNativeEvent(event decodedCopilotNativeEvent) {
	switch event.Type {
	case copilotNativeTypeUsage:
		used, size, ok := copilotRootUsage(event)
		if !ok {
			return
		}
		effectiveSize := a.recordNativeGaugeMaxSize(event.SessionID, size)
		remaining := max(effectiveSize-used, 0)
		a.sendUpdate(AgentEvent{
			Type:                   streams.EventTypeContextWindow,
			SessionID:              event.SessionID,
			ContextWindowSize:      effectiveSize,
			ContextWindowUsed:      used,
			ContextWindowRemaining: remaining,
			ContextEfficiency:      float64(used) / float64(effectiveSize) * 100,
		})
	case copilotNativeTypeCompactionStart:
		a.emitCopilotCompactionTimeline(event, acp.CompactionStatusInProgress)
	case copilotNativeTypeCompactionComplete:
		status := acp.CompactionStatusCompleted
		if !copilotCompletionSucceeded(event) {
			status = acp.CompactionStatusFailed
		}
		a.emitCopilotCompactionTimeline(event, status)
		if observation, partial := copilotCompactionCompletion(event); observation != nil {
			a.sendUpdate(AgentEvent{
				Type:              streams.EventTypeContextCompactionObservation,
				SessionID:         event.SessionID,
				ContextCompaction: observation,
			})
		} else if partial {
			if reporting := copilotOmittedCompletionReporting(event); reporting != nil {
				a.sendUpdate(AgentEvent{
					Type:              streams.EventTypeContextCompactionObservation,
					SessionID:         event.SessionID,
					ContextCompaction: reporting,
				})
			}
		}
	}
}

func copilotCompletionSucceeded(event decodedCopilotNativeEvent) bool {
	if event.DataOmitted || event.Malformed || !event.Root {
		return false
	}
	var data copilotNativeCompactionData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return false
	}
	return data.Success != nil && *data.Success && data.Outcome == "applied"
}

// emitCopilotCompactionTimeline reuses the standard compaction timeline
// identity. The timeline is presentation only; counting happens through the
// typed observation above.
func (a *Adapter) emitCopilotCompactionTimeline(event decodedCopilotNativeEvent, status acp.CompactionStatus) {
	if event.DataOmitted || event.Malformed || !event.Root {
		return
	}
	var data copilotNativeCompactionData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}
	attemptID := data.CompactionAttemptID
	if attemptID == "" {
		return
	}
	update := &acp.SessionCompactionUpdate{
		CompactionId:  acp.CompactionId(attemptID),
		Status:        status,
		SessionUpdate: "compaction",
	}
	if event.Type == copilotNativeTypeCompactionComplete {
		applyCopilotCompactionTimelineDetails(update, data)
	}
	if timeline := a.convertCompactionUpdate(event.SessionID, update); timeline != nil {
		a.sendUpdate(*timeline)
	}
}

func applyCopilotCompactionTimelineDetails(
	update *acp.SessionCompactionUpdate,
	data copilotNativeCompactionData,
) {
	contextMeta := map[string]any{}
	if data.Trigger != "" {
		contextMeta["trigger"] = data.Trigger
	}
	if data.PreCompactionTokens != nil {
		contextMeta["preTokens"] = *data.PreCompactionTokens
	}
	if data.PostCompactionTokens != nil {
		contextMeta["postTokens"] = *data.PostCompactionTokens
	}
	if len(contextMeta) > 0 {
		update.Meta = acp.OptionalValue(map[string]any{"contextCompaction": contextMeta})
	}
	reason := data.FailureReason
	if reason == "" {
		reason = data.Error
	}
	if reason != "" {
		update.Error = acp.OptionalValue(reason)
	}
}

// emitCopilotNativeReporting makes collection availability explicit before any
// compaction occurs. Silence must never be presented as proof of zero provider
// activity.
func (a *Adapter) emitCopilotNativeReporting(
	sessionID, availability, coverage, reason string,
) {
	if !a.copilotNativeSessionEvents || sessionID == "" {
		return
	}
	if availability == streams.ContextCompactionAvailabilityAvailable && !a.copilotNativeActive.Load() {
		availability = streams.ContextCompactionAvailabilityUnavailable
		coverage = streams.ContextCompactionCoveragePartial
		reason = "runtime_unqualified"
	}
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeContextCompactionObservation,
		SessionID: sessionID,
		ContextCompaction: &streams.ContextCompactionObservation{
			Kind:            streams.ContextCompactionObservationKindReporting,
			Provider:        copilotAgentID,
			NativeSessionID: sessionID,
			Availability:    availability,
			Coverage:        coverage,
			Reason:          reason,
		},
	})
}

// CopilotNativeSessionEventsActive reports whether this adapter actually
// subscribed to the vendor stream for its qualified runtime. Recovery uses it
// instead of the launch request alone: an older peer ignored the opt-in.
func (a *Adapter) CopilotNativeSessionEventsActive() bool {
	return a != nil && a.copilotNativeActive.Load()
}

// recordNativeGaugeMaxSize keeps the native token-limit baseline independent of
// the ordinary usage/cost tracker. Native root readings own the gauge only.
func (a *Adapter) recordNativeGaugeMaxSize(sessionID string, size int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.nativeGaugeMaxSize == nil {
		a.nativeGaugeMaxSize = make(map[string]int64)
	}
	if size > a.nativeGaugeMaxSize[sessionID] {
		a.nativeGaugeMaxSize[sessionID] = size
	}
	return a.nativeGaugeMaxSize[sessionID]
}

// installNativeBindingBarrier atomically publishes the native conversation
// identity and drains events buffered during creation/load/resume. The callback
// runs on the single notification worker, so an event queued after the barrier
// cannot overtake the buffered events.
func (a *Adapter) installNativeBindingBarrier(
	sessionID string,
	clearLoading bool,
	emitAvailableReporting bool,
) bool {
	bind := func() {
		a.mu.Lock()
		a.sessionID = sessionID
		a.bindingCopilotNative = false
		if clearLoading {
			a.isLoadingSession = false
		}
		a.mu.Unlock()
		if a.onNativeBindingInstalled != nil {
			a.onNativeBindingInstalled()
		}
		if emitAvailableReporting {
			a.emitCopilotNativeReporting(sessionID,
				streams.ContextCompactionAvailabilityAvailable,
				streams.ContextCompactionCoverageObserved, "")
		}
		a.flushPendingCopilotNative()
	}
	if a.syncNotifQueueThen(bind) {
		return true
	}
	a.mu.Lock()
	a.sessionID = sessionID
	a.bindingCopilotNative = false
	if clearLoading {
		a.isLoadingSession = false
	}
	a.pendingCopilotNative = nil
	a.pendingCopilotNativeDropped = false
	a.mu.Unlock()
	return false
}
