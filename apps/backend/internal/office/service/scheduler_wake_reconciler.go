package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// maxWakeReconcilePerTick caps the number of stuck parents processed in one
// tick, mirroring maxRecoveryPerTick (scheduler_recovery.go). ListStuckParents
// filters out every sticky non-actionable candidate — stale-or-missing
// receipt, active/finished run, unresolved or paused/stopped/pending-approval
// assignee — in SQL before this cap is applied, so it bounds real work, not
// resting or permanently-blocked parents.
const maxWakeReconcilePerTick = 5

// ParentWakeReconciler is a level-triggered backstop for the
// task_children_completed wake: queueChildrenCompletedRun
// (event_subscribers.go) fires it edge-triggered off the child-completion
// event, and that insert can be silently lost (the permanent
// idx_run_idempotency unique index conflicting with the courtesy 24h
// dedup window, a crash between the check and the insert, ...) with no
// re-delivery. This handler instead re-derives "is this parent stuck"
// from current task state every tick, so a lost wake self-heals on the
// next sweep instead of staying lost until an operator notices.
//
// It writes runs directly via CreateRunTx (bypassing QueueRun/CoalesceRun
// entirely, with a NULL idempotency key) so CoalesceRun's empty-key
// coalescing can never swallow the re-delivery the way it would if this
// went through QueueRun.
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
// assignee cannot accept a run right now. ListStuckParents' SQL already
// guarantees every candidate it returns has a stale-or-missing receipt (its
// LEFT JOIN excludes an exact child-set-key match), no active/finished wake
// run covering it (its NOT EXISTS against runs), and a resolvable,
// non-paused/stopped/pending-approval assignee (its own filters plus a LEFT
// JOIN against agent_profiles) — a second, Go-side receipt fetch-and-compare,
// or a check for an empty AssigneeAgentProfileID, would both be structurally
// unreachable here: nothing but this reconciler ever writes
// parent_child_wake_receipts, ticks run sequentially, and the assignee ID is
// computed in the same SELECT that filtered it. Do not re-add either check
// without a reason the SQL guarantee no longer holds; see the doc comment on
// ListStuckParents. guardAgentStatus is the one exception, kept deliberately:
// unlike a receipt, an agent's status can change from any caller at any
// time, so this closes the narrow race window between that SELECT and this
// read rather than duplicating a guarantee SQL already gives.
func (h *ParentWakeReconciler) reconcileOne(
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate,
) {
	if err := svc.guardAgentStatus(ctx, c.AssigneeAgentProfileID); err != nil {
		svc.recordWakeAssigneeUnresolved(c.ParentTaskID, err.Error())
		return
	}

	payload, err := h.buildPayload(ctx, svc, c.ParentTaskID, c.WorkflowStepID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: build payload failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	h.emit(ctx, svc, c, payload)
}

// childrenCompletedPayload mirrors the shape enrichChildrenContext
// (scheduler_integration.go) expects on a task_children_completed run's
// payload column. TaskID and WorkflowStepID are declared first so they
// marshal first in the JSON text: ParseRunPayload decodes this payload
// into a map[string]string for its task_id/workflow_step_id lookups, and
// a later type mismatch on the children array must not stop either from
// having already been set. WorkflowStepID lets evaluateRunStaleness
// (scheduler_staleness.go) cancel a re-delivered wake if the parent has
// since moved to another workflow step — the exact situation this
// reconciler's re-delivered runs are most likely to hit, since it only
// exists to redeliver a wake after time has already passed.
type childrenCompletedPayload struct {
	TaskID         string                     `json:"task_id"`
	WorkflowStepID string                     `json:"workflow_step_id"`
	Children       []childSummaryPayloadEntry `json:"children"`
	Truncated      bool                       `json:"truncated"`
}

type childSummaryPayloadEntry struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	State       string `json:"state"`
	LastComment string `json:"last_comment"`
}

// buildPayload assembles the run payload independently of the workflow
// engine's own children-completed dispatch (event_subscribers.go), since
// this reconciler inserts directly via CreateRunTx and never reaches the
// engine. GetChildSummaries counts *all* children (it has no archived_at
// filter), so a child archived after this parent's receipt was last
// written can still appear here; that's fine, this is prompt context, not
// the stuck-parent predicate the sweep itself uses.
func (h *ParentWakeReconciler) buildPayload(
	ctx context.Context, svc *Service, parentTaskID, workflowStepID string,
) (string, error) {
	children, truncated, err := svc.repo.GetChildSummaries(ctx, parentTaskID)
	if err != nil {
		return "", fmt.Errorf("get child summaries: %w", err)
	}

	entries := make([]childSummaryPayloadEntry, 0, len(children))
	for _, c := range children {
		entries = append(entries, childSummaryPayloadEntry{
			Identifier:  c.Identifier,
			Title:       c.Title,
			State:       c.State,
			LastComment: c.LastComment,
		})
	}

	out, err := json.Marshal(childrenCompletedPayload{
		TaskID:         parentTaskID,
		WorkflowStepID: workflowStepID,
		Children:       entries,
		Truncated:      truncated,
	})
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	return string(out), nil
}

// emit inserts the re-delivered run and upserts the wake receipt in a
// single transaction, so a failure between the two never leaves a receipt
// claiming delivery of a wake that was never actually queued.
func (h *ParentWakeReconciler) emit(
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate, payload string,
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

	runReq := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: c.AssigneeAgentProfileID,
		Reason:         RunReasonTaskChildrenCompleted,
		Payload:        payload,
		Status:         RunStatusQueued,
		CoalescedCount: 1,
		IdempotencyKey: nil,
	}
	if err := svc.repo.CreateRunTx(ctx, tx, runReq); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: create run failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}

	deliveredAt := time.Now().UTC()
	if err := svc.repo.UpsertWakeReceiptTx(ctx, tx, c.ParentTaskID, c.ChildSetKey, runReq.ID, deliveredAt); err != nil {
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

	svc.recordWakeEmitted(c.ParentTaskID, runReq.ID)
}
