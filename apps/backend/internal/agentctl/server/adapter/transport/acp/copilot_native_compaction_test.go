package acp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

const testCopilotNativeSession = "native-session-1"

func newCopilotNativeTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	a := NewAdapter(&shared.Config{
		AgentID:                    copilotAgentID,
		WorkDir:                    t.TempDir(),
		CopilotNativeSessionEvents: true,
	}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	a.sessionID = testCopilotNativeSession
	a.copilotNativeActive.Store(true)
	a.mu.Unlock()
	return a
}

func copilotUsageParams(t *testing.T, agentID *string, used, limit int64) json.RawMessage {
	t.Helper()
	params := map[string]any{
		"sessionId": testCopilotNativeSession,
		"type":      copilotNativeTypeUsage,
		"data":      map[string]any{"currentTokens": used, "tokenLimit": limit},
	}
	if agentID != nil {
		params["agentId"] = *agentID
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal usage params: %v", err)
	}
	return encoded
}

func copilotCompactionParams(t *testing.T, agentID *string, eventType string, data map[string]any) json.RawMessage {
	t.Helper()
	params := map[string]any{
		"sessionId": testCopilotNativeSession,
		"type":      eventType,
		"data":      data,
	}
	if agentID != nil {
		params["agentId"] = *agentID
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal compaction params: %v", err)
	}
	return encoded
}

func decodeTestCopilotEvent(t *testing.T, params json.RawMessage) decodedCopilotNativeEvent {
	t.Helper()
	event, ok := decodeCopilotNativeEvent(copilotSessionEventMethod, params)
	if !ok {
		t.Fatalf("decodeCopilotNativeEvent rejected %s", params)
	}
	return event
}

func nextUpdate(t *testing.T, a *Adapter) AgentEvent {
	t.Helper()
	select {
	case event := <-a.updatesCh:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for adapter update")
		return AgentEvent{}
	}
}

func TestCopilotNativeCompletionAcceptance(t *testing.T) {
	childID := "child-1"
	success := true
	failure := false

	tests := []struct {
		name        string
		agentID     *string
		data        map[string]any
		wantCount   bool
		wantPartial bool
	}{
		{
			name:      "applied root completion",
			data:      map[string]any{"compactionAttemptId": "attempt-1", "success": success, "outcome": "applied", "trigger": "manual"},
			wantCount: true,
		},
		{
			name:      "applied automatic completion uses the same rule",
			data:      map[string]any{"compactionAttemptId": "attempt-2", "success": success, "outcome": "applied", "trigger": "threshold"},
			wantCount: true,
		},
		{
			name:      "completion without a preceding start",
			data:      map[string]any{"compactionAttemptId": "attempt-3", "success": success, "outcome": "applied"},
			wantCount: true,
		},
		{
			name:        "child completion is ignored",
			agentID:     &childID,
			data:        map[string]any{"compactionAttemptId": "attempt-4", "success": success, "outcome": "applied"},
			wantCount:   false,
			wantPartial: false,
		},
		{
			name:        "failed completion is known evidence, not partial",
			data:        map[string]any{"compactionAttemptId": "attempt-5", "success": failure, "outcome": "failed"},
			wantPartial: false,
		},
		{
			name:        "non-applied outcome is known evidence, not partial",
			data:        map[string]any{"compactionAttemptId": "attempt-6", "success": success, "outcome": "cancelled"},
			wantPartial: false,
		},
		{
			name:        "missing attempt identity is not countable",
			data:        map[string]any{"success": success, "outcome": "applied"},
			wantPartial: true,
		},
		{
			name:        "missing success is not countable",
			data:        map[string]any{"compactionAttemptId": "attempt-7", "outcome": "applied"},
			wantPartial: true,
		},
		{
			name:        "omitted data is partial",
			data:        map[string]any{"omitted": "too-large", "bytes": float64(40000)},
			wantPartial: true,
		},
		{
			name:        "wrong-typed outcome is partial",
			data:        map[string]any{"compactionAttemptId": "attempt-8", "success": success, "outcome": float64(1)},
			wantPartial: true,
		},
		{
			name:        "wrong-typed success is partial",
			data:        map[string]any{"compactionAttemptId": "attempt-9", "success": "yes", "outcome": "applied"},
			wantPartial: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := decodeTestCopilotEvent(t, copilotCompactionParams(t, tc.agentID, copilotNativeTypeCompactionComplete, tc.data))
			observation, partial := copilotCompactionCompletion(event)
			if tc.wantCount {
				if observation == nil || !observation.ValidCompletion() {
					t.Fatalf("expected countable observation, got %+v partial=%v", observation, partial)
				}
			} else if observation != nil {
				t.Fatalf("unexpected countable observation: %+v", observation)
			}
			if partial != tc.wantPartial {
				t.Fatalf("partial = %v, want %v", partial, tc.wantPartial)
			}
		})
	}
}

func TestCopilotNativeRootUsage(t *testing.T) {
	childID := "child-1"
	root := decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 13210, 272000))
	if used, size, ok := copilotRootUsage(root); !ok || used != 13210 || size != 272000 {
		t.Fatalf("root usage = (%d, %d, %v), want (13210, 272000, true)", used, size, ok)
	}
	child := decodeTestCopilotEvent(t, copilotUsageParams(t, &childID, 2877, 272000))
	if _, _, ok := copilotRootUsage(child); ok {
		t.Fatal("child usage must not supply the parent gauge")
	}
	malformed := decodeTestCopilotEvent(t, []byte(`{"sessionId":"`+testCopilotNativeSession+`","type":"session.usage_info","agentId":null,"data":{"currentTokens":1,"tokenLimit":100}}`))
	if _, _, ok := copilotRootUsage(malformed); ok {
		t.Fatal("null agentId is malformed attribution, not root")
	}
}

func TestCopilotNativeAppliesRootGaugeAndCountsObservedCompletion(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 12171, 272000)))
	gauge := nextUpdate(t, a)
	if gauge.Type != streams.EventTypeContextWindow || gauge.ContextWindowUsed != 12171 || gauge.ContextWindowSize != 272000 {
		t.Fatalf("gauge = %+v, want root usage", gauge)
	}

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionStart, map[string]any{
		"compactionAttemptId": "attempt-1", "trigger": "manual",
	})))
	start := nextUpdate(t, a)
	if start.Type != streams.EventTypeToolCall || start.ToolCallID != compactionToolCallID(testCopilotNativeSession, "attempt-1") {
		t.Fatalf("start timeline = %+v", start)
	}

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionComplete, map[string]any{
		"compactionAttemptId": "attempt-1", "success": true, "outcome": "applied",
	})))
	var completion *AgentEvent
	for i := 0; i < 2; i++ {
		event := nextUpdate(t, a)
		if event.Type == streams.EventTypeContextCompactionObservation {
			completion = &event
		}
	}
	if completion == nil || completion.ContextCompaction == nil ||
		!completion.ContextCompaction.ValidCompletion() || completion.ContextCompaction.AttemptID != "attempt-1" {
		t.Fatalf("completion observation = %+v", completion)
	}
}

func TestCopilotNativeMixedUsageUpdatesAccountingWithoutCompetingGauge(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.handleACPUpdate(acp.SessionNotification{
		SessionId: acp.SessionId(testCopilotNativeSession),
		Update: acp.SessionUpdate{
			UsageUpdate: &acp.SessionUsageUpdate{Size: 200000, Used: 13000},
		},
	}, 0)
	if delta, _ := a.consumeUsageDelta(testCopilotNativeSession); delta != 13000 {
		t.Fatalf("mixed usage accounting delta = %d, want 13000", delta)
	}
	select {
	case event := <-a.updatesCh:
		t.Fatalf("mixed Copilot usage must not emit a competing gauge: %+v", event)
	default:
	}
}

func TestCopilotNativeSessionBindingBuffersUntilBound(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	a := NewAdapter(&shared.Config{
		AgentID:                    copilotAgentID,
		WorkDir:                    t.TempDir(),
		CopilotNativeSessionEvents: true,
	}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.copilotNativeActive.Store(true)

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 100, 1000)))
	a.mu.Lock()
	pending := len(a.pendingCopilotNative)
	a.sessionID = testCopilotNativeSession
	a.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending native events = %d, want 1", pending)
	}
	a.flushPendingCopilotNative()
	if event := nextUpdate(t, a); event.Type != streams.EventTypeContextWindow {
		t.Fatalf("flushed event = %+v, want context window", event)
	}
}

func TestCopilotNativeStaleSessionIsDropped(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	stale := decodeTestCopilotEvent(t, []byte(`{"sessionId":"other-session","type":"session.usage_info","data":{"currentTokens":1,"tokenLimit":100}}`))
	a.handleCopilotNativeWork(stale)
	select {
	case event := <-a.updatesCh:
		t.Fatalf("stale native event produced an update: %+v", event)
	default:
	}
}

func TestCopilotNativeOmittedCompletionEmitsPartialReporting(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	event := decodeTestCopilotEvent(t, []byte(`{
		"sessionId":"`+testCopilotNativeSession+`",
		"type":"session.compaction_complete",
		"dataOmitted":"too-large",
		"data":{"omitted":"too-large","bytes":40000}
	}`))
	a.handleCopilotNativeWork(event)
	update := nextUpdate(t, a)
	if update.Type != streams.EventTypeContextCompactionObservation || update.ContextCompaction == nil {
		t.Fatalf("reporting update = %+v", update)
	}
	if update.ContextCompaction.Kind != streams.ContextCompactionObservationKindReporting ||
		update.ContextCompaction.Coverage != streams.ContextCompactionCoveragePartial ||
		update.ContextCompaction.ValidCompletion() {
		t.Fatalf("reporting observation = %+v", update.ContextCompaction)
	}
}

func TestCopilotMixedUsageStillEmitsGaugeWhenNativeDisabled(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	a := NewAdapter(&shared.Config{AgentID: copilotAgentID, WorkDir: t.TempDir()}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	a.sessionID = testCopilotNativeSession
	a.mu.Unlock()

	a.handleACPUpdate(acp.SessionNotification{
		SessionId: acp.SessionId(testCopilotNativeSession),
		Update: acp.SessionUpdate{
			UsageUpdate: &acp.SessionUsageUpdate{Size: 272000, Used: 9695},
		},
	}, 0)
	event := nextUpdate(t, a)
	if event.Type != streams.EventTypeContextWindow || event.ContextWindowUsed != 9695 || event.ContextWindowSize != 272000 {
		t.Fatalf("gauge = %+v, want ordinary usage when native collection is off", event)
	}
}

func TestCopilotNativeBindingBuffersAcrossNewSessionReplacement(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	a := NewAdapter(&shared.Config{
		AgentID:                    copilotAgentID,
		WorkDir:                    t.TempDir(),
		CopilotNativeSessionEvents: true,
	}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	a.sessionID = "prior-session"
	a.copilotNativeActive.Store(true)
	a.bindingCopilotNative = true
	a.mu.Unlock()

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, []byte(`{
		"sessionId":"`+testCopilotNativeSession+`",
		"type":"session.usage_info",
		"data":{"currentTokens":12171,"tokenLimit":272000}
	}`)))
	a.mu.Lock()
	pending := len(a.pendingCopilotNative)
	a.sessionID = testCopilotNativeSession
	a.bindingCopilotNative = false
	a.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending native events = %d, want the event held across creation", pending)
	}
	a.flushPendingCopilotNative()
	if event := nextUpdate(t, a); event.Type != streams.EventTypeContextWindow || event.ContextWindowUsed != 12171 {
		t.Fatalf("flushed event = %+v, want the new conversation gauge", event)
	}
}

func TestCopilotNativeGaugeDoesNotChangePromptAccounting(t *testing.T) {
	t.Run("native sample between ordinary samples", func(t *testing.T) {
		a := newCopilotNativeTestAdapter(t)
		a.handleACPUpdate(acp.SessionNotification{
			SessionId: acp.SessionId(testCopilotNativeSession),
			Update: acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{
				Size: 200000, Used: 13000, Cost: &acp.Cost{Amount: 0.01, Currency: "USD"},
			}},
		}, 0)
		a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 12171, 272000)))
		delta, cost, present := a.consumeUsageDeltaWithPresence(testCopilotNativeSession)
		if delta != 13000 || cost != 100 || !present {
			t.Fatalf("accounting delta=%d cost=%d present=%v, want ordinary sample only", delta, cost, present)
		}
	})

	t.Run("native sample before ordinary sample", func(t *testing.T) {
		a := newCopilotNativeTestAdapter(t)
		a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 12171, 272000)))
		a.handleACPUpdate(acp.SessionNotification{
			SessionId: acp.SessionId(testCopilotNativeSession),
			Update: acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{
				Size: 200000, Used: 13000, Cost: &acp.Cost{Amount: 0.01, Currency: "USD"},
			}},
		}, 0)
		delta, cost, present := a.consumeUsageDeltaWithPresence(testCopilotNativeSession)
		if delta != 13000 || cost != 100 || !present {
			t.Fatalf("accounting delta=%d cost=%d present=%v, want ordinary sample only", delta, cost, present)
		}
	})

	t.Run("creation baseline excludes buffered native samples", func(t *testing.T) {
		a := newCopilotNativeTestAdapter(t)
		a.mu.Lock()
		a.ensureUsageTracker(testCopilotNativeSession).latestUsed = 5000
		a.retainConsumedUsageBaselineLocked(testCopilotNativeSession)
		a.mu.Unlock()
		a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 9000, 272000)))
		delta, cost, present := a.consumeUsageDeltaWithPresence(testCopilotNativeSession)
		if delta != 0 || cost != 0 || present {
			t.Fatalf("creation accounting delta=%d cost=%d present=%v, want no native charge", delta, cost, present)
		}
	})
}

func TestCopilotNativeInactiveRuntimeReportsUnavailable(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.copilotNativeActive.Store(false)
	a.emitCopilotNativeReporting(
		testCopilotNativeSession,
		streams.ContextCompactionAvailabilityAvailable,
		streams.ContextCompactionCoverageObserved,
		"",
	)
	update := nextUpdate(t, a)
	if update.Type != streams.EventTypeContextCompactionObservation || update.ContextCompaction == nil {
		t.Fatalf("reporting update = %+v", update)
	}
	if update.ContextCompaction.Availability != streams.ContextCompactionAvailabilityUnavailable ||
		update.ContextCompaction.Coverage != streams.ContextCompactionCoveragePartial {
		t.Fatalf("reporting = %+v, want unavailable/partial", update.ContextCompaction)
	}
}

func TestCopilotNativeNonObjectCompletionDataIsPartial(t *testing.T) {
	event, ok := decodeCopilotNativeEvent(copilotSessionEventMethod, []byte(`{
		"sessionId":"`+testCopilotNativeSession+`",
		"type":"session.compaction_complete",
		"data":[1,2,3]
	}`))
	if !ok {
		t.Fatal("envelope should decode so the unusable payload can be classified")
	}
	if observation, partial := copilotCompactionCompletion(event); observation != nil || !partial {
		t.Fatalf("non-object completion = %+v partial=%v, want partial only", observation, partial)
	}
}

func TestCopilotNativeBindingOverflowReportsPartial(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.mu.Lock()
	a.sessionID = "prior-session"
	a.bindingCopilotNative = true
	a.mu.Unlock()
	for i := 0; i < copilotPendingNativeLimit+1; i++ {
		a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 100+int64(i), 272000)))
	}
	a.mu.Lock()
	pending := len(a.pendingCopilotNative)
	dropped := a.pendingCopilotNativeDropped
	a.sessionID = testCopilotNativeSession
	a.bindingCopilotNative = false
	a.mu.Unlock()
	if pending != copilotPendingNativeLimit || !dropped {
		t.Fatalf("pending=%d dropped=%v, want a bounded overflow marker", pending, dropped)
	}
	a.flushPendingCopilotNative()

	gauges := 0
	var reporting *streams.ContextCompactionObservation
	for i := 0; i < copilotPendingNativeLimit+1; i++ {
		event := nextUpdate(t, a)
		if event.Type == streams.EventTypeContextWindow {
			gauges++
		}
		if event.Type == streams.EventTypeContextCompactionObservation {
			reporting = event.ContextCompaction
		}
	}
	if gauges != copilotPendingNativeLimit || reporting == nil ||
		reporting.Coverage != streams.ContextCompactionCoveragePartial ||
		reporting.Reason != "binding_buffer_overflow" ||
		reporting.Availability != streams.ContextCompactionAvailabilityAvailable {
		t.Fatalf("gauges=%d reporting=%+v, want bounded gauges and sticky partial", gauges, reporting)
	}
}

func TestCopilotNativeOverflowSurvivesDeferredFlush(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.mu.Lock()
	a.sessionID = "prior-session"
	a.bindingCopilotNative = true
	a.mu.Unlock()
	for i := 0; i < copilotPendingNativeLimit+1; i++ {
		a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 100+int64(i), 272000)))
	}

	a.mu.Lock()
	a.sessionID = testCopilotNativeSession
	a.bindingCopilotNative = false
	a.isLoadingSession = true
	a.mu.Unlock()
	a.flushPendingCopilotNative()

	a.mu.Lock()
	deferredPending := len(a.pendingCopilotNative)
	deferredDropped := a.pendingCopilotNativeDropped
	a.isLoadingSession = false
	a.mu.Unlock()
	if deferredPending != copilotPendingNativeLimit || !deferredDropped {
		t.Fatalf("deferred pending=%d dropped=%v, want the overflow marker restored", deferredPending, deferredDropped)
	}

	a.flushPendingCopilotNative()
	var reporting *streams.ContextCompactionObservation
	for i := 0; i < copilotPendingNativeLimit+1; i++ {
		event := nextUpdate(t, a)
		if event.Type == streams.EventTypeContextCompactionObservation {
			reporting = event.ContextCompaction
		}
	}
	if reporting == nil || reporting.Coverage != streams.ContextCompactionCoveragePartial ||
		reporting.Reason != "binding_buffer_overflow" {
		t.Fatalf("reporting=%+v, want deferred overflow to remain partial", reporting)
	}
}

func TestCopilotNativeBindingBarrierDrainsBeforeNewerWork(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.mu.Lock()
	a.sessionID = "prior-session"
	a.bindingCopilotNative = true
	a.mu.Unlock()

	a.handleCopilotNativeWork(decodeTestCopilotEvent(t, copilotUsageParams(t, nil, 100, 272000)))
	installed := make(chan struct{}, 1)
	a.onNativeBindingInstalled = func() {
		// Simulate a newer notification arriving after the binding is visible
		// but before the buffered drain.
		a.mu.Lock()
		a.sessionID = testCopilotNativeSession
		a.bindingCopilotNative = false
		a.mu.Unlock()
		a.enqueueCopilotNativeNotification(copilotSessionEventMethod, copilotUsageParams(t, nil, 200, 272000))
		installed <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		a.installNativeBindingBarrier(testCopilotNativeSession, false, false)
		close(done)
	}()
	select {
	case <-installed:
	case <-time.After(time.Second):
		t.Fatal("binding barrier did not install")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("binding barrier did not complete")
	}
	first := nextUpdate(t, a)
	second := nextUpdate(t, a)
	if first.ContextWindowUsed != 100 || second.ContextWindowUsed != 200 {
		t.Fatalf("drain order = (%d, %d), want buffered 100 before newer 200", first.ContextWindowUsed, second.ContextWindowUsed)
	}
}

func TestCopilotNativeUnknownOutcomeIsPartial(t *testing.T) {
	data := map[string]any{"compactionAttemptId": "attempt-future", "success": true, "outcome": "future_outcome"}
	event := decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionComplete, data))
	observation, partial := copilotCompactionCompletion(event)
	if observation != nil || !partial {
		t.Fatalf("unknown outcome = %+v partial=%v, want partial evidence only", observation, partial)
	}

	failed := map[string]any{"compactionAttemptId": "attempt-failed", "success": true, "outcome": "failed"}
	event = decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionComplete, failed))
	if observation, partial = copilotCompactionCompletion(event); observation != nil || partial {
		t.Fatalf("known non-applied outcome = %+v partial=%v, want complete non-counting evidence", observation, partial)
	}
}

func TestCopilotNativeTimelineCarriesTokenAndErrorDetails(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	complete := decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionComplete, map[string]any{
		"compactionAttemptId": "attempt-timeline", "trigger": "manual", "success": true, "outcome": "applied",
		"preCompactionTokens": 1200, "postCompactionTokens": 400,
	}))
	a.emitCopilotCompactionTimeline(complete, acp.CompactionStatusCompleted)
	event := nextUpdate(t, a)
	if event.NormalizedPayload == nil {
		t.Fatal("timeline event has no normalized payload")
	}
	compaction := event.NormalizedPayload.ContextCompaction()
	if compaction == nil || compaction.Trigger != "manual" || compaction.PreTokens == nil || *compaction.PreTokens != 1200 ||
		compaction.PostTokens == nil || *compaction.PostTokens != 400 {
		t.Fatalf("timeline compaction = %+v, want trigger and token details", compaction)
	}

	failed := decodeTestCopilotEvent(t, copilotCompactionParams(t, nil, copilotNativeTypeCompactionComplete, map[string]any{
		"compactionAttemptId": "attempt-failure-timeline", "success": false, "outcome": "failed",
		"failureReason": "empty_model_response",
	}))
	a.emitCopilotCompactionTimeline(failed, acp.CompactionStatusFailed)
	event = nextUpdate(t, a)
	compaction = event.NormalizedPayload.ContextCompaction()
	if compaction == nil || compaction.Error != "empty_model_response" {
		t.Fatalf("failed timeline compaction = %+v, want the failure reason", compaction)
	}
}

func TestCopilotNativeModelChangeResetsGaugeBaseline(t *testing.T) {
	a := newCopilotNativeTestAdapter(t)
	a.mu.Lock()
	a.nativeGaugeMaxSize[testCopilotNativeSession] = 272000
	a.contextSamples[testCopilotNativeSession] = contextWindowSample{used: 1000, size: 272000}
	a.mu.Unlock()

	a.finalizeSetModel(sessionmodel.MethodSetModel, testCopilotNativeSession, "model-b", nil, nil, configOptionIDModel, nil)

	a.mu.RLock()
	_, hasNative := a.nativeGaugeMaxSize[testCopilotNativeSession]
	_, hasSample := a.contextSamples[testCopilotNativeSession]
	a.mu.RUnlock()
	if hasNative || hasSample {
		t.Fatalf("model change retained native gauge=%v sample=%v; want both reset", hasNative, hasSample)
	}
}
