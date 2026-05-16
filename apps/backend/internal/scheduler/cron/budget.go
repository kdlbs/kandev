package cron

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// budgetThresholds enumerates the thresholds the cron handler watches
// for. Crossing any boundary fires once per (workspace, scope, scope
// id, threshold, period) — anchored on the start of the period so
// thresholds reset at each new period boundary.
var budgetThresholds = []int{50, 80, 90, 100}

// BudgetCheckResult is a lightweight projection of a budget evaluation
// for one configured policy. Mirrors costs.BudgetCheckResult so the
// real cost service satisfies the BudgetEvaluator interface for free,
// but the cron package does not import the office costs package
// directly — keeping this dependency edge narrow eases testing.
// SpentSubcents / LimitSubcents are hundredths of a cent.
type BudgetCheckResult struct {
	WorkspaceID   string
	ScopeType     string // "workspace" | "project" | "agent"
	ScopeID       string
	SpentSubcents int64
	LimitSubcents int64
	Period        string // policy.Period as configured (e.g. "monthly")
}

// PercentSpent reports the percentage of the limit currently consumed.
// Returns 0 when the limit is non-positive (no policy / disabled).
func (r BudgetCheckResult) PercentSpent() int {
	if r.LimitSubcents <= 0 {
		return 0
	}
	return int(r.SpentSubcents * 100 / r.LimitSubcents)
}

// BudgetEvaluator returns the current spend / limit picture for every
// configured policy across all workspaces. The cron handler treats
// results as a snapshot — it does not assume monotonic spend, only
// "the latest known value at this tick".
type BudgetEvaluator interface {
	EvaluatePolicies(ctx context.Context) ([]BudgetCheckResult, error)
}

// BudgetTaskScope returns tasks affected by a budget alert for the
// given scope. The handler does not call this directly per scope-type
// — instead it relies on the workflow's on_budget_alert action to
// resolve the right target task (typically the workspace coordination
// task via task_id: "{coordination_task.id}"), so the firing surface
// is one engine invocation per crossing not per impacted task.
//
// Defining a single resolver keeps the door open to future "fan out
// per task" semantics without restructuring the handler.
type BudgetTaskScope interface {
	ResolveAlertTaskID(ctx context.Context, scopeType, scopeID, workspaceID string) (string, error)
}

// BudgetEngineDispatcher is the engine-firing surface. Aliased to the
// same shape as HeartbeatEngineDispatcher so a single dispatcher
// implementation satisfies both.
type BudgetEngineDispatcher = HeartbeatEngineDispatcher

// BudgetHandler implements Handler for engine.TriggerOnBudgetAlert.
type BudgetHandler struct {
	evaluator  BudgetEvaluator
	scope      BudgetTaskScope
	dispatcher BudgetEngineDispatcher
	now        func() time.Time
	log        *logger.Logger

	// firedKeys tracks which (scope, threshold, period-start) events
	// have already fired this process. Phase 5 keeps state in-memory
	// for simplicity; a process restart re-fires once per active
	// crossing which the engine's idempotency layer absorbs by
	// OperationID. A future iteration may persist this in the office
	// activity log, but the plan deliberately scopes Phase 5 to
	// in-memory.
	firedKeys map[string]struct{}
}

// NewBudgetHandler builds a BudgetHandler. nil collaborators turn
// Tick into a no-op for that path so the cron loop can start before
// every dependency is wired (similar to NewHeartbeatHandler).
func NewBudgetHandler(
	evaluator BudgetEvaluator,
	scope BudgetTaskScope,
	dispatcher BudgetEngineDispatcher,
	now func() time.Time,
	log *logger.Logger,
) *BudgetHandler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BudgetHandler{
		evaluator:  evaluator,
		scope:      scope,
		dispatcher: dispatcher,
		now:        now,
		log:        log.WithFields(zap.String("handler", "budget")),
		firedKeys:  make(map[string]struct{}),
	}
}

// Name implements Handler.
func (h *BudgetHandler) Name() string { return "budget" }

// Tick implements Handler. The flow:
//
//  1. Snapshot every configured budget policy via the evaluator.
//  2. For each policy, find the highest crossed threshold this tick.
//  3. Fire engine.TriggerOnBudgetAlert at most once per
//     (workspace, scope_type, scope_id, threshold, period_start).
//
// Per-task fan-out is the engine's job — the on_budget_alert action's
// task_id field decides whether to wake the coordination task once or
// the affected tasks individually.
func (h *BudgetHandler) Tick(ctx context.Context) error {
	if !h.ready() {
		return nil
	}
	results, err := h.evaluator.EvaluatePolicies(ctx)
	if err != nil {
		return fmt.Errorf("evaluate budget policies: %w", err)
	}
	now := h.now()
	for _, r := range results {
		if threshold := highestCrossed(r.PercentSpent()); threshold > 0 {
			h.tryFire(ctx, r, threshold, now)
		}
	}
	return nil
}

// ready reports whether every collaborator is wired.
func (h *BudgetHandler) ready() bool {
	return h.evaluator != nil && h.scope != nil && h.dispatcher != nil
}

// tryFire fires the engine trigger once per (scope, threshold,
// period_start). Repeat ticks within the same period suppress.
func (h *BudgetHandler) tryFire(
	ctx context.Context,
	r BudgetCheckResult,
	threshold int,
	now time.Time,
) {
	periodStart := periodStartFor(r.Period, now)
	key := fmt.Sprintf("%s|%s|%s|%d|%d",
		r.WorkspaceID, r.ScopeType, r.ScopeID, threshold, periodStart.Unix())
	if _, fired := h.firedKeys[key]; fired {
		return
	}
	taskID, err := h.scope.ResolveAlertTaskID(ctx, r.ScopeType, r.ScopeID, r.WorkspaceID)
	if err != nil {
		h.log.Warn("resolve alert task failed",
			zap.String("scope_type", r.ScopeType),
			zap.String("scope_id", r.ScopeID),
			zap.Error(err))
		return
	}
	if taskID == "" {
		// No coordination task wired for the workspace yet (Phase 6
		// onboards them). Mark fired so we don't churn the log.
		h.firedKeys[key] = struct{}{}
		return
	}
	opID := fmt.Sprintf("budget:%s:%s:%s:%d:%d",
		r.WorkspaceID, r.ScopeType, r.ScopeID, threshold, periodStart.Unix())
	err = h.dispatcher.HandleTrigger(ctx,
		taskID,
		engine.TriggerOnBudgetAlert,
		engine.OnBudgetAlertPayload{
			BudgetPct: threshold,
			Scope:     r.ScopeType,
		},
		opID,
	)
	if err != nil {
		h.log.Debug("budget trigger dispatch failed",
			zap.String("task_id", taskID),
			zap.String("scope_type", r.ScopeType),
			zap.Int("threshold", threshold),
			zap.Error(err))
	}
	h.firedKeys[key] = struct{}{}
}

// highestCrossed returns the largest threshold crossed by the spend
// percentage, or 0 if none is crossed. Used so a 95% spent reading
// only fires the 90 threshold (not 50/80 redundantly) — those would
// have already fired on previous ticks at lower percentages.
func highestCrossed(percent int) int {
	highest := 0
	for _, t := range budgetThresholds {
		if percent >= t {
			highest = t
		}
	}
	return highest
}

// periodStartFor anchors the dedup key to the start of the current
// period so thresholds reset on a new period boundary. Periods are
// open-ended in the schema; this handler interprets the common
// values and falls back to a daily reset for anything unknown so
// alerts never get permanently suppressed by a stale fire.
func periodStartFor(period string, now time.Time) time.Time {
	switch period {
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "weekly":
		// ISO week: Monday as the first day.
		offset := (int(now.Weekday()) + 6) % 7
		anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return anchor.AddDate(0, 0, -offset)
	case "daily", "":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	default:
		// Unknown period: use today's UTC date so misconfigured
		// policies still reset their dedup state every 24h.
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
}
