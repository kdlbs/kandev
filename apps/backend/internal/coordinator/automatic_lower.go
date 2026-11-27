package coordinator

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"time"

	"go.uber.org/zap"
)

var automaticLoweredTotal = expvar.NewMap("coordinator_automatic_lowered_total")

// recordClassChanges appends one class change per action whose value differs
// between stored and final, in the locked transaction that writes the policy.
// The manager is the caller.
func (s *Service) recordClassChanges(ctx context.Context, tx coordinatorExec, coordinatorID string, stored, final Policy) error {
	if !s.phase3 {
		return nil
	}
	for _, class := range AllActions {
		from, to := stored.Actions[class], final.Actions[class]
		if from == to {
			continue
		}
		if err := s.store.InsertClassChangeTx(ctx, tx, ClassChange{
			CoordinatorID: coordinatorID, Class: class, FromValue: from, ToValue: to, ChangedBy: decidingUserID(ctx),
		}); err != nil {
			return err
		}
	}
	return nil
}

// LowerClass writes class to requires_approval as a system change and records
// reason. It writes nothing when the class is not automatic, so a retry is
// idempotent. No eligibility check applies.
func (s *Service) LowerClass(ctx context.Context, coordinatorID string, class Action, reason string) error {
	var (
		lowered     bool
		workspaceID string
	)
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		var raw *string
		var revision int
		if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT policy_json, policy_revision, workspace_id FROM coordinators WHERE id = ?`), coordinatorID).
			Scan(&raw, &revision, &workspaceID); err != nil {
			return fmt.Errorf("read coordinator policy: %w", err)
		}
		stored, _ := ParsePolicy(raw)
		if stored.Actions[class] != SettingAutomatic {
			return nil
		}
		next := Policy{Version: policyVersion, Actions: make(map[Action]Setting, len(stored.Actions))}
		for a, v := range stored.Actions {
			next.Actions[a] = v
		}
		next.Actions[class] = SettingRequiresApproval
		encoded, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("encode policy: %w", err)
		}
		if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`UPDATE coordinators SET policy_json = ?, policy_revision = policy_revision + 1, updated_at = ? WHERE id = ?`),
			string(encoded), s.store.now().UTC(), coordinatorID); err != nil {
			return fmt.Errorf("lower class: %w", err)
		}
		lowered = true
		return s.store.InsertClassChangeTx(ctx, tx, ClassChange{
			CoordinatorID: coordinatorID, Class: class, FromValue: SettingAutomatic, ToValue: SettingRequiresApproval, Reason: reason,
		})
	})
	if err != nil || !lowered {
		return err
	}
	automaticLoweredTotal.Add(reason, 1)
	s.logger.Info("coordinator class lowered", zap.String("coordinator_id", coordinatorID),
		zap.String("class", string(class)), zap.String("reason", reason))
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	return nil
}

// afterUndo is the post-commit call of markUndone: when the undone task was
// created by an automatic approval it lowers create_task. A failure or panic
// is logged and never fails the undo; the backstop retry covers it.
func (s *Service) afterUndo(ctx context.Context, row *ActivityRow) {
	if !s.phase3 || row.ActionClass != ActionCreateTask || row.TargetTaskID == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("coordinator undo lowering panicked", zap.String("coordinator_id", row.CoordinatorID), zap.Any("panic", r))
		}
	}()
	automatic, err := s.store.taskCreatedAutomatically(ctx, row.CoordinatorID, *row.TargetTaskID)
	if err != nil {
		s.logger.Error("coordinator undo lowering: read failed", zap.String("coordinator_id", row.CoordinatorID), zap.Error(err))
		return
	}
	if !automatic {
		return
	}
	if err := s.actionSettings().Lower(ctx, row.CoordinatorID, string(ActionCreateTask), undoLoweredReason); err != nil {
		s.logger.Error("coordinator undo lowering failed", zap.String("coordinator_id", row.CoordinatorID), zap.Error(err))
	}
}

// taskCreatedAutomatically reports whether taskID is the task of one of the
// coordinator's proposals whose current claim is the automatic path's.
func (s *Store) taskCreatedAutomatically(ctx context.Context, coordinatorID, taskID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*) FROM coordinator_proposals
		WHERE coordinator_id = ? AND task_id = ? AND claimed_automatically = 1`), coordinatorID, taskID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("read automatic claim of task: %w", err)
	}
	return n > 0, nil
}

// LoweringDuty is the backstop's per-coordinator lowering retry: while
// create_task reads automatic, an undo in the last 24 hours of a task an
// automatic approval created lowers it. A read error skips the tick.
func (s *Service) LoweringDuty() BackstopDuty {
	return func(ctx context.Context, coordinatorID string) error {
		if !s.phase3 {
			return nil
		}
		setting, err := s.actionSettings().Setting(ctx, coordinatorID, string(ActionCreateTask))
		if err != nil || setting.Value != SettingAutomatic {
			return err
		}
		now := s.store.now().UTC()
		undone, err := s.decisionLog().UndoneTaskIDs(ctx, coordinatorID, now.Add(-automaticLimitWindow), now.Add(time.Second))
		if err != nil || len(undone) == 0 {
			return err
		}
		automatic, err := s.store.AutomaticClaimTaskIDs(ctx, coordinatorID)
		if err != nil {
			return err
		}
		for _, id := range undone {
			if _, ok := automatic[id]; ok {
				return s.actionSettings().Lower(ctx, coordinatorID, string(ActionCreateTask), undoLoweredReason)
			}
		}
		return nil
	}
}
