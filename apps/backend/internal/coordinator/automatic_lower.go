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
		from, to := effectiveSetting(stored, class), effectiveSetting(final, class)
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

// LowerGuard restricts a lowering to a raise it was decided against. After
// writes nothing unless the newest raise of the class changed strictly before
// it; RaiseID writes nothing unless the newest raise is that change row. The
// zero value is unguarded.
type LowerGuard struct {
	After   *time.Time
	RaiseID string
}

// permits reports whether the newest raise change satisfies the guard.
func (g LowerGuard) permits(raise *ClassChange) bool {
	if g.After == nil && g.RaiseID == "" {
		return true
	}
	if raise == nil {
		return false
	}
	if g.After != nil && !raise.ChangedAt.Before(*g.After) {
		return false
	}
	return g.RaiseID == "" || raise.ID == g.RaiseID
}

// LowerClass writes class to requires_approval as a system change and
// records reason. It writes nothing, and reports false, when the class is not
// automatic or the guard rejects the newest raise, so a retry is idempotent.
// No eligibility check applies.
func (s *Service) LowerClass(ctx context.Context, coordinatorID string, class Action, reason string, guard LowerGuard) (bool, error) {
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
		raise, err := s.store.NewestClassChangeTx(ctx, tx, coordinatorID, class, SettingAutomatic)
		if err != nil {
			return err
		}
		if !guard.permits(raise) {
			return nil
		}
		next := Policy{Version: policyVersion, Actions: make(map[Action]Setting, len(stored.Actions)), Eligibility: stored.Eligibility}
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
		return false, err
	}
	automaticLoweredTotal.Add(reason, 1)
	s.logger.Info("coordinator class lowered", zap.String("coordinator_id", coordinatorID),
		zap.String("class", string(class)), zap.String("reason", reason))
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	return true, nil
}

// afterUndo is the post-commit call of markUndone: when the undone task was
// created by an automatic approval it lowers create_task, and start_agent too
// when the approval started an agent. A failure or panic is logged and never
// fails the undo; the backstop retry covers it.
func (s *Service) afterUndo(ctx context.Context, row *ActivityRow) {
	if !s.phase3 || row.ActionClass != ActionCreateTask || row.TargetTaskID == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("coordinator undo lowering panicked", zap.String("coordinator_id", row.CoordinatorID), zap.Any("panic", r))
		}
	}()
	automatic, startsAgent, err := s.store.taskCreatedAutomatically(ctx, row.CoordinatorID, *row.TargetTaskID)
	if err != nil {
		s.logger.Error("coordinator undo lowering: read failed", zap.String("coordinator_id", row.CoordinatorID), zap.Error(err))
		return
	}
	if !automatic {
		return
	}
	classes := []Action{ActionCreateTask}
	if startsAgent {
		classes = append(classes, ActionStartAgent)
	}
	for _, class := range classes {
		if _, err := s.actionSettings().Lower(ctx, row.CoordinatorID, string(class), undoLoweredReason, LowerGuard{After: row.UndoneAt}); err != nil {
			automaticLowerFailedTotal.Add(lowerTriggerUndo, 1)
			s.logger.Error("coordinator undo lowering failed", zap.String("coordinator_id", row.CoordinatorID), zap.String("class", string(class)), zap.Error(err))
		}
	}
}

// taskCreatedAutomatically reports whether taskID is the task of one of the
// coordinator's proposals whose current claim is the automatic path's, and
// whether that proposal started an agent.
func (s *Store) taskCreatedAutomatically(ctx context.Context, coordinatorID, taskID string) (automatic, startsAgent bool, err error) {
	var n, agents int
	err = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN starts_agent THEN 1 ELSE 0 END), 0) FROM coordinator_proposals
		WHERE coordinator_id = ? AND task_id = ? AND claimed_automatically = 1`), coordinatorID, taskID).Scan(&n, &agents)
	if err != nil {
		return false, false, fmt.Errorf("read automatic claim of task: %w", err)
	}
	return n > 0, agents > 0, nil
}

// LoweringDuty is the backstop's per-coordinator lowering retry. For each
// raisable class that still reads automatic it looks for evidence dated
// strictly after the class's newest raise and within the last 24 hours, and
// lowers the class it finds some for. A read error skips that class.
func (s *Service) LoweringDuty() BackstopDuty {
	return func(ctx context.Context, coordinatorID string) error {
		if !s.phase3 {
			return nil
		}
		var first error
		for _, class := range []Action{ActionCreateTask, ActionResume, ActionStartAgent} {
			if err := s.retryLowering(ctx, coordinatorID, class); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
}

func (s *Service) retryLowering(ctx context.Context, coordinatorID string, class Action) error {
	setting, err := s.actionSettings().Setting(ctx, coordinatorID, string(class))
	if err != nil || setting.Value != SettingAutomatic {
		return err
	}
	now := s.store.now().UTC()
	after := now.Add(-automaticLimitWindow)
	if setting.ChangedAt != nil && setting.ChangedAt.After(after) {
		after = *setting.ChangedAt
	}
	reason, err := s.loweringEvidence(ctx, coordinatorID, class, after, now)
	if err != nil || reason == "" {
		return err
	}
	_, err = s.actionSettings().Lower(ctx, coordinatorID, string(class), reason, LowerGuard{RaiseID: setting.ChangeID})
	return err
}

// loweringEvidence returns the lowering reason for the first kind of evidence
// found for class dated strictly after after, or "" when there is none.
func (s *Service) loweringEvidence(ctx context.Context, coordinatorID string, class Action, after, now time.Time) (string, error) {
	if class != ActionCreateTask {
		if found, err := s.store.AutomaticFailureAfter(ctx, coordinatorID, class, after); err != nil || found {
			return failureLoweredReason, err
		}
		if found, err := s.store.ManagerOverrideAfter(ctx, coordinatorID, class, after); err != nil || found {
			return overrideLoweredReason, err
		}
	}
	if class == ActionResume {
		return "", nil
	}
	undone, err := s.decisionLog().UndoneTaskIDs(ctx, coordinatorID, after.Add(time.Microsecond), now.Add(time.Second))
	if err != nil || len(undone) == 0 {
		return "", err
	}
	claims := s.store.AutomaticClaimTaskIDs
	if class == ActionStartAgent {
		claims = s.store.AutomaticStartAgentTaskIDs
	}
	automatic, err := claims(ctx, coordinatorID)
	if err != nil {
		return "", err
	}
	for _, id := range undone {
		if _, ok := automatic[id]; ok {
			return undoLoweredReason, nil
		}
	}
	return "", nil
}
