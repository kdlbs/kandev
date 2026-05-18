package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type fakeEvaluator struct {
	results []BudgetCheckResult
	err     error
}

func (f *fakeEvaluator) EvaluatePolicies(_ context.Context) ([]BudgetCheckResult, error) {
	return f.results, f.err
}

type fakeScope struct {
	taskID string
	err    error
	calls  []scopeCall
}

type scopeCall struct {
	scopeType, scopeID, workspaceID string
}

func (f *fakeScope) ResolveAlertTaskID(_ context.Context, scopeType, scopeID, workspaceID string) (string, error) {
	f.calls = append(f.calls, scopeCall{scopeType, scopeID, workspaceID})
	return f.taskID, f.err
}

func TestBudgetHandler_FiresOnceAtThresholdCrossing(t *testing.T) {
	eval := &fakeEvaluator{
		results: []BudgetCheckResult{{
			WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a1",
			SpentSubcents: 95, LimitSubcents: 100, Period: "monthly",
		}},
	}
	scope := &fakeScope{taskID: "coordination"}
	disp := &fakeDispatcher{}

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	h := NewBudgetHandler(eval, scope, disp,
		func() time.Time { return now }, logger.Default())

	// First tick fires.
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("first tick fires = %d, want 1", len(disp.calls))
	}
	if disp.calls[0].trigger != engine.TriggerOnBudgetAlert {
		t.Errorf("trigger = %q", disp.calls[0].trigger)
	}
	pl, ok := disp.calls[0].payload.(engine.OnBudgetAlertPayload)
	if !ok {
		t.Fatalf("payload type = %T", disp.calls[0].payload)
	}
	if pl.BudgetPct != 90 {
		t.Errorf("budget pct = %d, want 90 (highest crossed)", pl.BudgetPct)
	}
	if pl.Scope != "agent" {
		t.Errorf("scope = %q, want agent", pl.Scope)
	}

	// Second tick within the same period: no fire.
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("second tick within period fires = %d, want 1", len(disp.calls))
	}
}

func TestBudgetHandler_CoalescesPerScopeThresholdPeriod(t *testing.T) {
	eval := &fakeEvaluator{
		results: []BudgetCheckResult{
			{WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a1", SpentSubcents: 95, LimitSubcents: 100, Period: "monthly"},
			{WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a1", SpentSubcents: 95, LimitSubcents: 100, Period: "monthly"},
			// Same scope, same threshold — must coalesce.
		},
	}
	scope := &fakeScope{taskID: "coordination"}
	disp := &fakeDispatcher{}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	h := NewBudgetHandler(eval, scope, disp,
		func() time.Time { return now }, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("coalesce failed: got %d fires, want 1", len(disp.calls))
	}
}

func TestBudgetHandler_DistinctScopesEachFire(t *testing.T) {
	eval := &fakeEvaluator{
		results: []BudgetCheckResult{
			{WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a1", SpentSubcents: 95, LimitSubcents: 100, Period: "monthly"},
			{WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a2", SpentSubcents: 95, LimitSubcents: 100, Period: "monthly"},
			{WorkspaceID: "ws", ScopeType: "project", ScopeID: "p1", SpentSubcents: 95, LimitSubcents: 100, Period: "monthly"},
		},
	}
	scope := &fakeScope{taskID: "coordination"}
	disp := &fakeDispatcher{}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	h := NewBudgetHandler(eval, scope, disp,
		func() time.Time { return now }, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 3 {
		t.Fatalf("distinct scopes fires = %d, want 3", len(disp.calls))
	}
}

func TestBudgetHandler_HighestThresholdOnly(t *testing.T) {
	cases := []struct {
		name       string
		spent      int
		limit      int
		wantFire   bool
		wantThresh int
	}{
		{"under 50%", 40, 100, false, 0},
		{"50% exactly", 50, 100, true, 50},
		{"75%", 75, 100, true, 50},
		{"80%", 80, 100, true, 80},
		{"90%", 90, 100, true, 90},
		{"100%", 100, 100, true, 100},
		{"over 100%", 150, 100, true, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eval := &fakeEvaluator{
				results: []BudgetCheckResult{{
					WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a",
					SpentSubcents: int64(tc.spent), LimitSubcents: int64(tc.limit), Period: "monthly",
				}},
			}
			disp := &fakeDispatcher{}
			h := NewBudgetHandler(eval, &fakeScope{taskID: "c"}, disp,
				nil, logger.Default())
			if err := h.Tick(context.Background()); err != nil {
				t.Fatalf("Tick: %v", err)
			}
			if tc.wantFire && len(disp.calls) != 1 {
				t.Fatalf("expected fire, got %d calls", len(disp.calls))
			}
			if !tc.wantFire && len(disp.calls) != 0 {
				t.Fatalf("expected no fire, got %d calls", len(disp.calls))
			}
			if tc.wantFire {
				pl := disp.calls[0].payload.(engine.OnBudgetAlertPayload)
				if pl.BudgetPct != tc.wantThresh {
					t.Errorf("threshold = %d, want %d", pl.BudgetPct, tc.wantThresh)
				}
			}
		})
	}
}

func TestBudgetHandler_NoFireWhenScopeReturnsEmpty(t *testing.T) {
	eval := &fakeEvaluator{
		results: []BudgetCheckResult{{
			WorkspaceID: "ws", ScopeType: "workspace", ScopeID: "ws",
			SpentSubcents: 95, LimitSubcents: 100, Period: "monthly",
		}},
	}
	scope := &fakeScope{taskID: ""} // no coordination task wired
	disp := &fakeDispatcher{}

	h := NewBudgetHandler(eval, scope, disp, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(disp.calls) != 0 {
		t.Fatalf("expected 0 fires when no task resolved, got %d", len(disp.calls))
	}
	// And subsequent ticks must continue to skip without churning the
	// log. Mark-fired-on-empty is the contract.
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
}

func TestBudgetHandler_ResetsAtNewPeriod(t *testing.T) {
	eval := &fakeEvaluator{
		results: []BudgetCheckResult{{
			WorkspaceID: "ws", ScopeType: "agent", ScopeID: "a1",
			SpentSubcents: 95, LimitSubcents: 100, Period: "monthly",
		}},
	}
	scope := &fakeScope{taskID: "c"}
	disp := &fakeDispatcher{}

	clock := time.Date(2026, 5, 31, 23, 59, 0, 0, time.UTC)
	h := NewBudgetHandler(eval, scope, disp,
		func() time.Time { return clock }, logger.Default())

	// First tick at end of May fires.
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(disp.calls) != 1 {
		t.Fatalf("first fires = %d, want 1", len(disp.calls))
	}

	// Roll into June: same scope, same threshold should fire again
	// because the period_start anchor changed.
	clock = time.Date(2026, 6, 1, 0, 5, 0, 0, time.UTC)
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("june tick: %v", err)
	}
	if len(disp.calls) != 2 {
		t.Fatalf("new-period fires = %d, want 2", len(disp.calls))
	}
}

func TestBudgetHandler_EvaluatorErrorIsReturned(t *testing.T) {
	eval := &fakeEvaluator{err: errors.New("db down")}
	h := NewBudgetHandler(eval, &fakeScope{}, &fakeDispatcher{}, nil, logger.Default())
	if err := h.Tick(context.Background()); err == nil {
		t.Fatal("expected evaluator error to surface")
	}
}

func TestBudgetHandler_NilDependenciesAreNoOp(t *testing.T) {
	h := NewBudgetHandler(nil, nil, nil, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

func TestPeriodStartFor(t *testing.T) {
	now := time.Date(2026, 5, 14, 10, 30, 0, 0, time.UTC) // Thursday
	cases := []struct {
		period string
		want   time.Time
	}{
		{"monthly", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
		{"weekly", time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)}, // Monday of that week
		{"daily", time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)},
		{"", time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)},
		{"unknown", time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.period, func(t *testing.T) {
			if got := periodStartFor(tc.period, now); !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
