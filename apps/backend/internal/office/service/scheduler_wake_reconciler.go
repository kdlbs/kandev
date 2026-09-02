package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// maxWakeReconcilePerTick caps the number of stuck parents processed in one
// tick, mirroring maxRecoveryPerTick (scheduler_recovery.go). ListStuckParents
// filters out every sticky non-actionable candidate — stale-or-missing
// receipt, active/terminal run, unresolved or paused/stopped/pending-approval
// assignee — in SQL before this cap is applied, so it bounds real work, not
// resting or permanently-blocked parents.
const maxWakeReconcilePerTick = 5

// ParentWakeReconciler is a level-triggered backstop for the
// task_children_completed wake: queueChildrenCompletedRun
// (event_subscribers.go) fires it edge-triggered off the child-completion
// event, and that dispatch can be lost without re-delivery. This handler
// re-derives "is this parent stuck" from current task state every tick, then
// sends the trigger through the same workflow engine used by the edge path.
type ParentWakeReconciler struct {
	scheduler *SchedulerIntegration
	logger    *logger.Logger
}

// NewParentWakeReconciler constructs the parent-wake reconciliation handler
// for a scheduler integration.
func NewParentWakeReconciler(si *SchedulerIntegration) *ParentWakeReconciler {
	return &ParentWakeReconciler{
		scheduler: si,
		logger:    si.svc.logger.WithFields(zap.String("component", "parent-wake-reconciler")),
	}
}

// Name implements scheduler/cron.Handler.
func (h *ParentWakeReconciler) Name() string { return "parent_wake_reconciler" }

// Tick implements scheduler/cron.Handler. The adoption check prevents an
// Office-enabled but Kanban-only installation from scanning task rows.
func (h *ParentWakeReconciler) Tick(ctx context.Context) error {
	adopted, err := h.scheduler.svc.repo.HasOfficeAdoption(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("check Office adoption: %w", err)
	}
	if !adopted {
		return nil
	}
	h.reconcile(ctx)
	return nil
}

// reconcile sweeps for stuck parents and re-delivers a wake for any whose
// last-delivered receipt no longer matches their current child set.
func (h *ParentWakeReconciler) reconcile(ctx context.Context) {
	svc := h.scheduler.svc

	candidates, err := svc.repo.ListStuckParents(ctx, RunReasonTaskChildrenCompleted, maxWakeReconcilePerTick)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: list stuck parents failed", zap.Error(err))
		return
	}

	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		svc.recordWakeCandidate(c.ParentTaskID)
		h.reconcileOne(ctx, svc, c)
	}
}

// reconcileOne re-delivers the wake for a single stuck parent, unless its
// assignee cannot accept a run right now. It re-reads the child-set key before
// dispatch and again before recording the receipt. This keeps a child update
// from making an old candidate look delivered for the new generation.
func (h *ParentWakeReconciler) reconcileOne(
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate,
) {
	// No dispatcher wired means there is nothing to admit: bail out before
	// IncrementWakeDeliverySeq below, which otherwise bumps the counter for
	// an attempt that was never going anywhere.
	if svc.engineDispatcher == nil {
		return
	}
	if err := svc.guardAgentStatus(ctx, c.AssigneeAgentProfileID); err != nil {
		svc.recordWakeAssigneeUnresolved(c.ParentTaskID, err.Error())
		return
	}

	payload, err := h.buildPayload(ctx, svc, c.ParentTaskID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: build payload failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	currentKey, err := svc.repo.GetChildSetKey(ctx, c.ParentTaskID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: revalidate child set failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}
	if currentKey != c.ChildSetKey {
		return
	}

	deliverySeq, err := svc.repo.IncrementWakeDeliverySeq(ctx, c.ParentTaskID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: increment delivery sequence failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	operationID := wakeOperationID(c.ParentTaskID, c.ChildSetKey, deliverySeq)
	accepted, err := svc.dispatchEngineTriggerForRecovery(
		ctx,
		c.ParentTaskID,
		engine.TriggerOnChildrenCompleted,
		payload,
		operationID,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: engine dispatch failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}
	if !accepted {
		return
	}

	h.recordReceipt(ctx, svc, c, operationID)
}

// buildPayload assembles the typed payload expected by the workflow engine's
// on_children_completed trigger. GetChildSummaries counts all children, and
// the candidate query separately excludes archived children from readiness.
func (h *ParentWakeReconciler) buildPayload(
	ctx context.Context, svc *Service, parentTaskID string,
) (engine.OnChildrenCompletedPayload, error) {
	children, truncated, err := svc.repo.GetChildSummaries(ctx, parentTaskID)
	if err != nil {
		return engine.OnChildrenCompletedPayload{}, fmt.Errorf("get child summaries: %w", err)
	}

	prsByTask := svc.lookupChildPRLinks(ctx, children)
	summaries := make([]engine.ChildSummary, 0, len(children))
	for _, c := range children {
		summaries = append(summaries, engine.ChildSummary{
			TaskID:  c.TaskID,
			Status:  c.State,
			Summary: c.LastComment,
			PRLinks: prsByTask[c.TaskID],
		})
	}

	if truncated {
		// The engine payload has no truncation field. The list is already
		// capped by the repository, so the available summaries remain the
		// same as the normal edge-triggered path.
		h.logger.Debug("wake sweep: child summaries truncated",
			zap.String("parent_task_id", parentTaskID))
	}
	return engine.OnChildrenCompletedPayload{ChildSummaries: summaries}, nil
}

// recordReceipt stores the operation-backed receipt after the workflow engine
// accepts the trigger. The engine owns run admission and can fan out to more
// than one target, so a single delivered run id cannot represent this wake.
func (h *ParentWakeReconciler) recordReceipt(
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate, operationID string,
) {
	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: begin tx failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback() }()

	currentKey, err := svc.repo.GetChildSetKeyTx(ctx, tx, c.ParentTaskID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: revalidate child set in receipt tx failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}
	if currentKey != c.ChildSetKey {
		return
	}

	// c.NewestChildUpdatedAt is the sweep-time MAX(tasks.updated_at) across
	// this parent's children (ListStuckParents), not just the ones that
	// completed — any later edit to a terminal child (title, description,
	// labels, metadata) with no state change bumps that same column, so the
	// next tick sees newest_child_updated_at != child_generation again and
	// re-admits the parent for one extra wake even though nothing completed.
	// This is bounded and self-correcting (recordReceipt persists the same
	// value that triggered the re-admit, so the tick after that sees them
	// equal and stops), and follows the same duplicate-over-missed bias
	// already accepted for wakeOperationID below. A real fix needs a
	// completion-specific generation distinct from generic updated_at — see
	// follow-up task fc871ca9-bcb2-4db5-915d-52c92c7bd1ad.
	deliveredAt := time.Now().UTC()
	if err := svc.repo.UpsertWakeReceiptTx(
		ctx, tx, c.ParentTaskID, c.ChildSetKey, "", operationID, c.NewestChildUpdatedAt, deliveredAt,
	); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: upsert wake receipt failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	if err := tx.Commit(); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: commit failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	svc.recordWakeEmitted(c.ParentTaskID, operationID)
}

// wakeOperationID includes deliverySeq (IncrementWakeDeliverySeq), a
// per-parent counter bumped once for every admitted dispatch attempt.
// tasks.updated_at has only one-second resolution, so two genuinely distinct
// deliveries landing in the same wall-clock second would hash identically on
// child set + generation alone, and the engine's permanent operation ledger
// would treat the second as already applied. Trading a stable, replayable id
// for a counter that never repeats means a dispatch that is admitted but
// whose receipt then fails to persist before the next tick's sweep gets a
// fresh id (and therefore a fresh, possibly duplicate, engine dispatch) on
// retry rather than one that could collide with a different, later delivery
// — ListStuckParents' own admission predicate remains the dedup gate for the
// common case where nothing changed.
func wakeOperationID(parentTaskID, childSetKey string, deliverySeq int64) string {
	sum := sha256.Sum256([]byte(parentTaskID + "\x00" + childSetKey + "\x00" + strconv.FormatInt(deliverySeq, 10)))
	return fmt.Sprintf("task_children_completed:%s:%s", parentTaskID, hex.EncodeToString(sum[:]))
}
