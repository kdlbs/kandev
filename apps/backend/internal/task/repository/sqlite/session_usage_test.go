package sqlite

import (
	"context"
	"testing"
)

// TestIncrementTaskSessionUsage_AccumulatesCachedInAcrossCalls confirms
// tokens_cached_in compounds the same way tokens_in/tokens_out/cost_subcents
// already do. Before the fix this column didn't exist and the parameter
// couldn't be threaded at all.
func TestIncrementTaskSessionUsage_AccumulatesCachedInAcrossCalls(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-cached-usage", "sess-cached-usage", "turn-cached-usage")

	if err := repo.IncrementTaskSessionUsageTx(ctx, nil, "sess-cached-usage", 100, 50_000_000, 200, 50); err != nil {
		t.Fatalf("first increment: %v", err)
	}
	if err := repo.IncrementTaskSessionUsageTx(ctx, nil, "sess-cached-usage", 10, 2_000, 20, 5); err != nil {
		t.Fatalf("second increment: %v", err)
	}

	var tokensIn, tokensCachedIn, tokensOut, costSubcents int64
	err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_in, tokens_cached_in, tokens_out, cost_subcents FROM task_sessions WHERE id = ?`),
		"sess-cached-usage").Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if tokensIn != 110 || tokensCachedIn != 50_002_000 || tokensOut != 220 || costSubcents != 55 {
		t.Errorf("totals = (%d,%d,%d,%d), want (110,50002000,220,55)",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// TestGetTaskSessionAndListTaskSessions_SurfaceRollupColumns pins
// docs/specs/task-cost-ledger/spec.md AC-28/AC-29: both session read paths
// (GetTaskSession's single-row scan and ListTaskSessions' multi-row scan)
// must return the four usage/cost rollup columns IncrementTaskSessionUsageTx
// maintains, not just the underlying SQL row.
func TestGetTaskSessionAndListTaskSessions_SurfaceRollupColumns(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-rollup-surface", "sess-rollup-surface", "turn-rollup-surface")

	if err := repo.IncrementTaskSessionUsageTx(ctx, nil, "sess-rollup-surface", 80, 8_203_943, 44979, 79118); err != nil {
		t.Fatalf("increment: %v", err)
	}

	got, err := repo.GetTaskSession(ctx, "sess-rollup-surface")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if got.TokensIn != 80 || got.TokensCachedIn != 8_203_943 || got.TokensOut != 44979 || got.CostSubcents != 79118 {
		t.Errorf("GetTaskSession rollup = %+v, want (80, 8203943, 44979, 79118)", got)
	}

	list, err := repo.ListTaskSessions(ctx, "task-rollup-surface")
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListTaskSessions returned %d sessions, want 1", len(list))
	}
	if list[0].TokensIn != 80 || list[0].TokensCachedIn != 8_203_943 || list[0].TokensOut != 44979 || list[0].CostSubcents != 79118 {
		t.Errorf("ListTaskSessions rollup = %+v, want (80, 8203943, 44979, 79118)", list[0])
	}
}
