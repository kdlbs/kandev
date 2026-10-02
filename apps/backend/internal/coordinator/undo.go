package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// Undo refusal codes and reasons (the 409 bodies).
const (
	UndoAlreadyUndone = "already_undone"
	UndoNotUndoable   = "not_undoable"
	UndoConflict      = "undo_conflict"

	UndoReasonMoved             = "moved"
	UndoReasonArchived          = "archived"
	UndoReasonAgentActive       = "agent_running"
	UndoReasonStepGone          = "step_deleted"
	UndoReasonStepDone          = "step_done"
	UndoReasonStepFull          = "step_full"
	UndoReasonFeederStartsAgent = "feeder_starts_agent"
)

// UndoRefusal is a 409 refusal of an undo. It carries no row; the client
// refetches.
type UndoRefusal struct {
	Code   string
	Reason string
}

func (e *UndoRefusal) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("undo refused: %s (%s)", e.Code, e.Reason)
	}
	return "undo refused: " + e.Code
}

func conflict(reason string) error { return &UndoRefusal{Code: UndoConflict, Reason: reason} }

// keyedLock serialises holders of one key in-process. Waiting honours the
// context, and an entry is dropped when its last holder or waiter leaves.
type keyedLock struct {
	mu      sync.Mutex
	entries map[string]*keyedEntry
}

type keyedEntry struct {
	token chan struct{}
	refs  int
}

func (k *keyedLock) acquire(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.entries == nil {
		k.entries = map[string]*keyedEntry{}
	}
	e, ok := k.entries[key]
	if !ok {
		e = &keyedEntry{token: make(chan struct{}, 1)}
		k.entries[key] = e
	}
	e.refs++
	k.mu.Unlock()

	select {
	case e.token <- struct{}{}:
		return func() {
			<-e.token
			k.leave(key, e)
		}, nil
	case <-ctx.Done():
		k.leave(key, e)
		return nil, ctx.Err()
	}
}

func (k *keyedLock) leave(key string, e *keyedEntry) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if e.refs--; e.refs == 0 {
		delete(k.entries, key)
	}
}

func (k *keyedLock) size() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.entries)
}

// UndoActivity reverses an approved create_task or move row and returns the
// original row as it reads afterwards. The reversal and the marker are two
// commits: the task service owns its own transaction.
func (s *Service) UndoActivity(ctx context.Context, workspaceID, coordinatorID, rowID string) (*ActivityItem, error) {
	if _, err := s.authorizedCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	if s.undoTasks == nil {
		return nil, errors.New("coordinator: undo is not wired")
	}
	release, err := s.undoLocks.acquire(ctx, rowID)
	if err != nil {
		return nil, err
	}
	defer release()

	row, err := s.store.GetActivityRow(ctx, s.store.db, coordinatorID, rowID)
	if err != nil {
		return nil, err
	}
	outcomes, err := s.undoOutcomes(ctx, row)
	if err != nil {
		return nil, err
	}
	probe := *row
	probe.UndoneAt = nil
	if !undoReadable(&probe, outcomes) {
		return nil, &UndoRefusal{Code: UndoNotUndoable}
	}
	if row.UndoneAt != nil {
		return nil, &UndoRefusal{Code: UndoAlreadyUndone}
	}
	if row.ActionClass == ActionCreateTask {
		err = s.reverseCreate(ctx, *row.TargetTaskID)
	} else {
		o, _ := parseMoveOutcome(outcomes[*row.ProposalID])
		err = s.reverseMove(ctx, *row.TargetTaskID, o)
	}
	if err != nil {
		return nil, err
	}
	if err := s.markUndone(ctx, row, decidingUserID(ctx)); err != nil {
		return nil, err
	}
	s.afterUndo(ctx, row)
	s.notifyUndone(ctx, row)
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	fresh, err := s.store.GetActivityRow(ctx, s.store.db, coordinatorID, rowID)
	if err != nil {
		return nil, err
	}
	return &s.enrichActivity(ctx, coordinatorID, []ActivityRow{*fresh})[0], nil
}

func (s *Service) undoOutcomes(ctx context.Context, row *ActivityRow) (map[string]*string, error) {
	if row.ActionClass != ActionMove || row.ProposalID == nil {
		return map[string]*string{}, nil
	}
	return s.store.MoveOutcomes(ctx, row.CoordinatorID, []string{*row.ProposalID})
}

func (s *Service) reverseCreate(ctx context.Context, taskID string) error {
	err := s.undoTasks.ArchiveTask(ctx, taskID)
	if err == nil || errors.Is(err, ErrTaskAlreadyArchived) || errors.Is(err, ErrTaskNotFound) {
		return nil
	}
	return fmt.Errorf("archive task for undo: %w", err)
}

// reverseMove moves the task back to the step it came from, refusing when the
// task is no longer where the move left it.
func (s *Service) reverseMove(ctx context.Context, taskID string, o moveOutcome) error {
	task, err := s.undoTasks.GetTask(ctx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return conflict(UndoReasonArchived)
	}
	if err != nil {
		return fmt.Errorf("read task for undo: %w", err)
	}
	if task.ArchivedAt != nil {
		return conflict(UndoReasonArchived)
	}
	s.logger.Info("undoing move", zap.String("task_id", taskID), zap.String("found_step_id", task.WorkflowStepID),
		zap.String("from_step_id", o.FromStepID), zap.String("to_step_id", o.ToStepID))
	switch task.WorkflowStepID {
	case o.FromStepID:
		return nil
	case o.ToStepID:
		return s.moveBack(ctx, taskID, task.WorkflowID, o.FromStepID)
	default:
		return conflict(UndoReasonMoved)
	}
}

func (s *Service) moveBack(ctx context.Context, taskID, workflowID, fromStepID string) error {
	step, err := s.undoTasks.GetStep(ctx, fromStepID)
	if errors.Is(err, ErrStepNotFound) {
		return conflict(UndoReasonStepGone)
	}
	if err != nil {
		return fmt.Errorf("read step for undo: %w", err)
	}
	if step.CompletesOnEnter {
		return conflict(UndoReasonStepDone)
	}
	active, err := s.undoTasks.HasActiveSession(ctx, taskID)
	if err != nil {
		return fmt.Errorf("read sessions for undo: %w", err)
	}
	if active {
		return conflict(UndoReasonAgentActive)
	}
	steps, err := s.undoTasks.ListSteps(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("read workflow steps for undo: %w", err)
	}
	stepListed := false
	for _, candidate := range steps {
		if candidate.ID == fromStepID {
			stepListed = true
			break
		}
	}
	if !stepListed {
		return conflict(UndoReasonStepGone)
	}
	if feedsAutoStartStep(steps, fromStepID) {
		return conflict(UndoReasonFeederStartsAgent)
	}
	_, err = s.undoTasks.MoveTaskWithOptions(ctx, taskID, workflowID, fromStepID, 0,
		UndoMoveOptions{ExpectedWorkflowID: workflowID, SkipStepPrompt: step.AutoStart})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrWIPLimitExceeded):
		return conflict(UndoReasonStepFull)
	case errors.Is(err, ErrMoveConflict):
		return conflict(UndoReasonMoved)
	default:
		return fmt.Errorf("move task back for undo: %w", err)
	}
}

// markUndone stamps the original row and writes the undone row in one
// transaction. Zero rows matched is already_undone when the row still exists
// and not found when retention removed it meanwhile.
func (s *Service) markUndone(ctx context.Context, row *ActivityRow, undoneBy string) error {
	now := s.store.now().UTC()
	return s.store.withCoordinatorLock(ctx, row.CoordinatorID, func(tx coordinatorExec) error {
		changed, err := s.store.MarkUndone(ctx, tx, row.ID, undoneBy, now)
		if err != nil {
			return err
		}
		if !changed {
			if _, err := s.store.GetActivityRow(ctx, tx, row.CoordinatorID, row.ID); err != nil {
				return err
			}
			return &UndoRefusal{Code: UndoAlreadyUndone}
		}
		return s.store.InsertActivity(ctx, tx, ActivityRow{
			ID:            uuid.NewString(),
			CoordinatorID: row.CoordinatorID,
			WorkspaceID:   row.WorkspaceID,
			ActionClass:   row.ActionClass,
			Outcome:       ActivityUndone,
			Authorization: AuthRequiresApproval,
			TargetTaskID:  row.TargetTaskID,
			ProposalID:    row.ProposalID,
			ActorUserID:   optString(undoneBy),
			Detail:        row.Detail,
			UndoOfID:      &row.ID,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	})
}

// tryAcquire takes key without waiting; ok is false when it is held.
func (k *keyedLock) tryAcquire(key string) (release func(), ok bool) {
	k.mu.Lock()
	if k.entries == nil {
		k.entries = map[string]*keyedEntry{}
	}
	e, exists := k.entries[key]
	if !exists {
		e = &keyedEntry{token: make(chan struct{}, 1)}
		k.entries[key] = e
	}
	e.refs++
	k.mu.Unlock()
	select {
	case e.token <- struct{}{}:
		return func() {
			<-e.token
			k.leave(key, e)
		}, true
	default:
		k.leave(key, e)
		return nil, false
	}
}
