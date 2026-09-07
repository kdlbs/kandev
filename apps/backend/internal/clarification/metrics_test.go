package clarification

import (
	"context"
	"expvar"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func clarificationTimeoutMetricValue(t *testing.T, phase string) int64 {
	t.Helper()
	value := clarificationResponseTimeoutTotal.Get(phase)
	if value == nil {
		return 0
	}
	counter, ok := value.(*expvar.Int)
	if !ok {
		t.Fatalf("timeout metric phase %q has type %T, want *expvar.Int", phase, value)
	}
	return counter.Value()
}

func TestPreClaimTimeoutPublishesPhaseMetricAndStructuredLog(t *testing.T) {
	shortenPreClaimTimeout(t)
	core, logs := observer.New(zapcore.InfoLevel)
	testLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	repo := &preClaimBlockingMessageStore{
		stubMessageStore: stubMessageStore{},
		entered:          make(chan struct{}),
	}
	resolver := NewResolver(
		NewStore(time.Minute),
		repo,
		nil,
		&stubAuthorizer{},
		nil,
		nil,
		nil,
		testLogger,
	)

	before := clarificationTimeoutMetricValue(t, clarificationResponsePhaseIdentity)
	_, _, err = resolver.ResolveBundle(context.Background(), "pending-metric-log", Outcome{})
	if !IsPreClaimTimeoutError(err) {
		t.Fatalf("ResolveBundle error = %v, want pre-claim timeout", err)
	}
	if got := clarificationTimeoutMetricValue(t, clarificationResponsePhaseIdentity); got != before+1 {
		t.Fatalf("identity timeout metric = %d, want %d", got, before+1)
	}
	if clarificationResponseTimeoutTotal.Get("pending-metric-log") != nil {
		t.Fatal("timeout metric is labelled by pending ID")
	}

	entries := logs.FilterMessage("clarification.response.phase").All()
	if len(entries) != 1 {
		t.Fatalf("phase log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["phase"] != clarificationResponsePhaseIdentity {
		t.Fatalf("phase field = %v, want %q", fields["phase"], clarificationResponsePhaseIdentity)
	}
	if fields["outcome"] != "timeout" {
		t.Fatalf("outcome field = %v, want timeout", fields["outcome"])
	}
	if fields["pending_id"] != "pending-metric-log" {
		t.Fatalf("pending_id field = %v, want pending-metric-log", fields["pending_id"])
	}
	if duration, ok := fields["duration_ms"].(int64); !ok || duration < 0 {
		t.Fatalf("duration_ms field = %v, want non-negative int64", fields["duration_ms"])
	}
}
