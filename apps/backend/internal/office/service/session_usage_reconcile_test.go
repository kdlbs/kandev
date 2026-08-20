package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// fakeSessionUsageWriter accumulates the deltas handlePromptUsage sends to
// the task-session rollup, mirroring what the real SQLite repo's
// IncrementTaskSessionUsage does with UPDATE ... SET x = x + ?.
type fakeSessionUsageWriter struct {
	tokensIn       int64
	tokensCachedIn int64
	tokensOut      int64
	costSubcents   int64
}

func (f *fakeSessionUsageWriter) IncrementTaskSessionUsage(
	_ context.Context, _ string, tokensIn, tokensCachedIn, tokensOut, costSubcents int64,
) error {
	f.tokensIn += tokensIn
	f.tokensCachedIn += tokensCachedIn
	f.tokensOut += tokensOut
	f.costSubcents += costSubcents
	return nil
}

// TestPromptUsage_RollupReconcilesWithCostLedger asserts the actual
// invariant this rollup exists to maintain: task_sessions' cumulative
// totals must equal the sum of the office_cost_events rows they were
// derived from. Before the fix, tokensCachedIn silently never reaches the
// writer even though every event row carries it, which is exactly what let
// task_sessions.tokens_in under-report cache-heavy sessions by orders of
// magnitude in production.
func TestPromptUsage_RollupReconcilesWithCostLedger(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	writer := &fakeSessionUsageWriter{}
	svc.SetSessionUsageWriter(writer)

	createTestAgent(t, svc, "ws-1", "worker-reconcile")
	svc.ExecSQL(t, `INSERT INTO tasks (
			id, workspace_id, project_id, title, created_at, updated_at
		) VALUES (
			'task-reconcile', 'ws-1', 'project-1', 'Reconcile task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`)
	setTestTaskAssignee(t, svc, "task-reconcile", "worker-reconcile")

	const sessionID = "session-reconcile-1"

	// Cache-heavy shape mirroring the reported bug: cached read tokens
	// dominate input tokens by several orders of magnitude.
	usages := []map[string]interface{}{
		{
			"input_tokens":        100,
			"cached_read_tokens":  50_000_000,
			"cached_write_tokens": 1_000_000,
			"output_tokens":       200,
		},
		{
			"input_tokens":        10,
			"cached_read_tokens":  2_000,
			"cached_write_tokens": 0,
			"output_tokens":       5,
		},
	}
	for _, usage := range usages {
		event := bus.NewEvent(events.SessionPromptUsageUpdated, "test", map[string]interface{}{
			"task_id":    "task-reconcile",
			"session_id": sessionID,
			"agent_id":   "claude-acp",
			"model":      "default",
			"usage":      usage,
		})
		if err := eb.Publish(ctx, events.BuildSessionPromptUsageSubject(sessionID), event); err != nil {
			t.Fatalf("publish prompt usage: %v", err)
		}
	}

	costs, err := svc.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}

	var wantIn, wantCachedIn, wantOut, wantCost int64
	var ledgerRows int
	for _, c := range costs {
		if c.SessionID != sessionID {
			continue
		}
		ledgerRows++
		wantIn += c.TokensIn
		wantCachedIn += c.TokensCachedIn
		if c.TokensOut != nil {
			wantOut += *c.TokensOut
		}
		wantCost += c.CostSubcents
	}
	if ledgerRows != len(usages) {
		t.Fatalf("ledger rows for session = %d, want %d", ledgerRows, len(usages))
	}
	if wantCachedIn == 0 {
		t.Fatal("test setup bug: ledger recorded zero cached tokens, reconciliation would be vacuous")
	}

	if writer.tokensIn != wantIn {
		t.Errorf("rollup tokens_in = %d, want %d (sum of office_cost_events.tokens_in)", writer.tokensIn, wantIn)
	}
	if writer.tokensCachedIn != wantCachedIn {
		t.Errorf("rollup tokens_cached_in = %d, want %d (sum of office_cost_events.tokens_cached_in)",
			writer.tokensCachedIn, wantCachedIn)
	}
	if writer.tokensOut != wantOut {
		t.Errorf("rollup tokens_out = %d, want %d (sum of office_cost_events.tokens_out)", writer.tokensOut, wantOut)
	}
	if writer.costSubcents != wantCost {
		t.Errorf("rollup cost_subcents = %d, want %d (sum of office_cost_events.cost_subcents)", writer.costSubcents, wantCost)
	}
}
