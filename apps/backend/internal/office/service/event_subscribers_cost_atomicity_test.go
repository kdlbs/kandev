package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// errInjectedRollupFailure is returned by txSessionUsageWriter while armed,
// standing in for a real rollup-side failure (e.g. a constraint violation or
// a dropped connection) that happens after the cost-event insert has
// already been staged in the same transaction.
var errInjectedRollupFailure = errors.New("injected rollup failure")

// txSessionUsageWriter is a shared.SessionUsageWriterTx test double whose
// Tx-aware increment can be armed to fail a fixed number of times before
// succeeding. It exists to prove recordCostEventAndRollup
// (event_subscribers.go) rolls the cost-event insert back when the rollup
// increment fails inside the shared transaction, and that a redelivery of
// the same usage_event_id afterward is not treated as a duplicate — the
// recovery path the PR #2606 review asked to see covered, since neither
// half of the write had actually committed.
type txSessionUsageWriter struct {
	mu        sync.Mutex
	failNextN int
	calls     int

	tokensIn       int64
	tokensCachedIn int64
	tokensOut      int64
	costSubcents   int64
}

func (w *txSessionUsageWriter) IncrementTaskSessionUsage(
	ctx context.Context, sessionID string, tokensIn, tokensCachedIn, tokensOut, costSubcents int64,
) error {
	return w.IncrementTaskSessionUsageTx(ctx, nil, sessionID, tokensIn, tokensCachedIn, tokensOut, costSubcents)
}

func (w *txSessionUsageWriter) IncrementTaskSessionUsageTx(
	_ context.Context, _ *sqlx.Tx, _ string, tokensIn, tokensCachedIn, tokensOut, costSubcents int64,
) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	if w.failNextN > 0 {
		w.failNextN--
		return errInjectedRollupFailure
	}
	w.tokensIn += tokensIn
	w.tokensCachedIn += tokensCachedIn
	w.tokensOut += tokensOut
	w.costSubcents += costSubcents
	return nil
}

// TestPromptUsage_RollupFailureRollsBackLedgerInsertThenRetrySucceeds covers
// the recovery path the PR #2606 review specifically asked for: force the
// rollup update to fail, then retry the same usage_event_id. Before
// recordCostEventAndRollup wrapped both writes in one transaction, the
// ledger insert committed regardless of the rollup outcome; a redelivery
// carrying the same usage_event_id would then hit the unique index and be
// dropped as a duplicate, leaving task_sessions permanently behind
// office_cost_events with no way to catch up.
func TestPromptUsage_RollupFailureRollsBackLedgerInsertThenRetrySucceeds(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	writer := &txSessionUsageWriter{failNextN: 1}
	svc.SetSessionUsageWriter(writer)

	createTestAgent(t, svc, "ws-1", "worker-atomic")
	insertTestTask(t, svc, "task-atomic", "ws-1")
	setTestTaskAssignee(t, svc, "task-atomic", "worker-atomic")

	buildEvent := func() *bus.Event {
		return bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
			"task_id":        "task-atomic",
			"session_id":     "session-atomic",
			"agent_id":       "claude-acp",
			"model":          "default",
			"usage_event_id": "usage-evt-atomic-1",
			"usage": map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		})
	}
	subject := events.BuildSessionPromptUsageSubject("session-atomic")

	// First delivery: the rollup increment is armed to fail. If the ledger
	// insert and the rollup increment were not atomic, the insert would
	// already be committed here.
	if err := eb.Publish(ctx, subject, buildEvent()); err != nil {
		t.Fatalf("publish (failing rollup): %v", err)
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs after failed rollup: %v", err)
	}
	if len(costs) != 0 {
		t.Fatalf("cost events after failed rollup = %d, want 0 (ledger insert must roll back with the failed rollup)",
			len(costs))
	}
	if writer.tokensIn != 0 || writer.tokensOut != 0 {
		t.Fatalf("rollup totals after failed attempt = (in=%d out=%d), want (0, 0): a failed increment must not partially apply",
			writer.tokensIn, writer.tokensOut)
	}

	// Redelivery of the same usage_event_id, this time with the rollup
	// writer succeeding. Because nothing committed on the first attempt,
	// the unique index must not treat this as a duplicate.
	if err := eb.Publish(ctx, subject, buildEvent()); err != nil {
		t.Fatalf("publish (retry): %v", err)
	}

	costs, err = svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs after retry: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost events after retry = %d, want 1", len(costs))
	}
	if writer.tokensIn != 100 || writer.tokensOut != 50 {
		t.Errorf("rollup totals after retry = (in=%d out=%d), want (in=100 out=50)", writer.tokensIn, writer.tokensOut)
	}
	if writer.calls != 2 {
		t.Errorf("rollup writer called %d times, want 2 (one failed attempt, one successful retry)", writer.calls)
	}
}
