package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
// tick, mirroring maxRecoveryPerTick (scheduler_recovery.go).
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

	candidates, err := svc.repo.ListStuckParents(ctx, maxWakeReconcilePerTick)
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
// receipt already matches the current child set or its assignee cannot
// accept a run right now.
func (h *ParentWakeReconciler) reconcileOne(
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate,
) {
	childHash := hashChildSetKey(c.ChildSetKey)

	receipt, err := svc.repo.GetWakeReceipt(ctx, c.ParentTaskID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("wake sweep: get wake receipt failed",
			zap.String("parent_task_id", c.ParentTaskID), zap.Error(err))
		return
	}
	if receipt != nil && receipt.ChildSetHash == childHash {
		svc.recordWakeUnchangedSkip(c.ParentTaskID)
		return
	}
	svc.recordWakeReceiptStale(c.ParentTaskID)

	if c.AssigneeAgentProfileID == "" {
		svc.recordWakeAssigneeUnresolved(c.ParentTaskID, "no_runner")
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

	h.emit(ctx, svc, c, childHash, payload)
}

// childrenCompletedPayload mirrors the shape enrichChildrenContext
// (scheduler_integration.go) expects on a task_children_completed run's
// payload column. TaskID is declared first so it marshals first in the
// JSON text: ParseRunPayload decodes this payload into a
// map[string]string for its task_id lookup, and a later type mismatch on
// the children array must not stop task_id from having already been set.
type childrenCompletedPayload struct {
	TaskID    string                     `json:"task_id"`
	Children  []childSummaryPayloadEntry `json:"children"`
	Truncated bool                       `json:"truncated"`
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
	ctx context.Context, svc *Service, parentTaskID string,
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
		TaskID:    parentTaskID,
		Children:  entries,
		Truncated: truncated,
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
	ctx context.Context, svc *Service, c sqlite.StuckParentCandidate, childHash, payload string,
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
	if err := svc.repo.UpsertWakeReceiptTx(ctx, tx, c.ParentTaskID, childHash, runReq.ID, deliveredAt); err != nil {
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

// hashChildSetKey hashes the sorted "id:state,..." child-set key produced
// by ListStuckParents into the fixed-width value stored in
// parent_child_wake_receipts.child_set_hash.
func hashChildSetKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
