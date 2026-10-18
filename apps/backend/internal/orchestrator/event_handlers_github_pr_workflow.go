package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	taskPRStateMerged = "merged"
	taskPRStateClosed = "closed"
)

// defaultTaskPRWorkflowRetryDelays bounds the retries of a PR workflow event
// that failed with an error. The poller publishes an event only when PR data
// changes, so without a retry one transient failure could leave the task on
// its step for good.
var defaultTaskPRWorkflowRetryDelays = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

// handleTaskPRWorkflowEvent runs processTaskPRWorkflowEvent and schedules
// bounded retries when it fails with an error. Intentional skips return no
// error and are not retried.
func (s *Service) handleTaskPRWorkflowEvent(ctx context.Context, pr *github.TaskPR) {
	if _, err := s.processTaskPRWorkflowEvent(ctx, pr); err != nil {
		s.scheduleTaskPRWorkflowRetry(pr, err)
	}
}

// processTaskPRWorkflowEvent runs the current step's on_pr_merged or
// on_pr_closed actions after the PR poller reports a linked pull request in
// that state. It never needs an agent turn.
//
// on_pr_merged fires once every linked PR is merged; on_pr_closed fires for
// the reported PR closed without merging. Either fires only when that outcome
// happened after the task entered its current step, so an outcome observed on
// an earlier step is never replayed by a later poller refresh. The operation
// id deduplicates redelivery of the same outcome.
//
// The stored PR row must still report the event's outcome: a delayed or
// retried snapshot of a PR that has since been reopened does not fire.
func (s *Service) processTaskPRWorkflowEvent(ctx context.Context, pr *github.TaskPR) (bool, error) {
	trigger, ok := taskPRWorkflowTrigger(pr)
	if !ok || !s.taskPRWorkflowWired() {
		return false, nil
	}
	task, err := s.taskPRWorkflowTask(ctx, pr.TaskID, trigger)
	if err != nil || task == nil {
		return false, err
	}
	operationID, payload, arrivalID, ok, err := s.taskPRWorkflowOutcome(ctx, task, pr, trigger)
	if err != nil || !ok {
		return false, err
	}
	session, live, err := s.taskPRWorkflowSession(ctx, task.ID)
	if err != nil || session == nil {
		return false, err
	}
	ctx = models.WithWorkflowStepTransitionGuard(ctx, task.ID, task.WorkflowStepID, arrivalID)

	unlock := s.lockChildCompletionOperation(operationID)
	defer unlock()
	applied, err := s.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		return false, err
	}

	state := s.buildMachineState(ctx, task, session)
	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:             task.ID,
		SessionID:          session.ID,
		Trigger:            trigger,
		OperationID:        operationID,
		EvaluateOnly:       true,
		DeferOperationMark: true,
		PreloadedState:     &state,
		Payload:            payload,
	})
	if err != nil {
		return false, fmt.Errorf("evaluate %s: %w", trigger, err)
	}
	transitioned := false
	if result.Transitioned {
		transitioned, err = s.applyTaskPRWorkflowTransition(ctx, task, session, live, result, trigger)
		if err != nil {
			return false, err
		}
	}
	return transitioned, s.workflowStore.MarkOperationApplied(ctx, operationID)
}

func (s *Service) taskPRWorkflowWired() bool {
	return s.workflowEngine != nil && s.workflowStore != nil &&
		s.workflowStepGetter != nil && s.githubService != nil
}

// taskPRWorkflowTrigger maps a PR state to its workflow trigger and applies
// the same payload gates as the github_pr_merged automation subscriber.
func taskPRWorkflowTrigger(pr *github.TaskPR) (engine.Trigger, bool) {
	if pr == nil || pr.TaskID == "" || pr.Owner == "" || pr.Repo == "" || pr.PRNumber <= 0 {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(pr.State)) {
	case taskPRStateMerged:
		return engine.TriggerOnPRMerged, true
	case taskPRStateClosed:
		return engine.TriggerOnPRClosed, true
	}
	return "", false
}

// taskPRWorkflowTask loads the task when its current step declares actions
// for trigger. A nil task with a nil error means the event does not apply.
func (s *Service) taskPRWorkflowTask(ctx context.Context, taskID string, trigger engine.Trigger) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task: %w", err)
	}
	if task == nil || task.WorkflowStepID == "" || task.IsEphemeral || task.ArchivedAt != nil {
		return nil, nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil {
		return nil, fmt.Errorf("load workflow step: %w", err)
	}
	if step == nil || !stepHasPRWorkflowActions(step, trigger) {
		return nil, nil
	}
	return task, nil
}

func stepHasPRWorkflowActions(step *wfmodels.WorkflowStep, trigger engine.Trigger) bool {
	switch trigger {
	case engine.TriggerOnPRMerged:
		return len(step.Events.OnPRMerged) > 0
	case engine.TriggerOnPRClosed:
		return len(step.Events.OnPRClosed) > 0
	}
	return false
}

// taskPRWorkflowOutcome decides whether the reported PR completes the
// trigger's outcome on the task's current step, and returns its operation id,
// engine payload, and immutable arrival identity for the commit fence.
func (s *Service) taskPRWorkflowOutcome(
	ctx context.Context, task *models.Task, pr *github.TaskPR, trigger engine.Trigger,
) (string, any, int64, bool, error) {
	linked, found, err := s.linkedTaskPRs(ctx, pr)
	if err != nil || !found {
		return "", nil, 0, false, err
	}
	operationID, payload, outcomeAt, ok := taskPRWorkflowOperation(trigger, task.ID, pr, linked)
	if !ok {
		return "", nil, 0, false, nil
	}
	arrivedAt, arrivalID, err := s.taskStepArrivedAt(ctx, task)
	if err != nil {
		return "", nil, 0, false, err
	}
	if outcomeAt.Before(arrivedAt) {
		return "", nil, 0, false, nil
	}
	return operationID, payload, arrivalID, true, nil
}

// linkedTaskPRs returns the task's attached PRs with the reported PR's state
// taken from the event. found is false when the reported PR is not attached
// to the task (for example, detached) or when the stored row no longer reports
// the event's outcome (for example, reopened).
func (s *Service) linkedTaskPRs(ctx context.Context, pr *github.TaskPR) ([]*github.TaskPR, bool, error) {
	prsByTask, err := s.githubService.ListTaskPRs(ctx, []string{pr.TaskID})
	if err != nil {
		return nil, false, fmt.Errorf("list task PRs: %w", err)
	}
	found := false
	linked := make([]*github.TaskPR, 0, len(prsByTask[pr.TaskID]))
	for _, candidate := range prsByTask[pr.TaskID] {
		if candidate == nil {
			continue
		}
		if sameTaskPR(candidate, pr) {
			if !sameTaskPROutcome(candidate, pr) {
				return nil, false, nil
			}
			candidate = pr
			found = true
		}
		linked = append(linked, candidate)
	}
	return linked, found, nil
}

// sameTaskPROutcome reports whether two rows of one PR report the same state
// and merge/close time, to the second, so storage precision does not matter.
func sameTaskPROutcome(a, b *github.TaskPR) bool {
	return strings.EqualFold(strings.TrimSpace(a.State), strings.TrimSpace(b.State)) &&
		sameOptionalTime(a.MergedAt, b.MergedAt) && sameOptionalTime(a.ClosedAt, b.ClosedAt)
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Truncate(time.Second).Equal(b.Truncate(time.Second))
}

func sameTaskPR(a, b *github.TaskPR) bool {
	return a.PRNumber == b.PRNumber &&
		strings.EqualFold(a.Owner, b.Owner) &&
		strings.EqualFold(a.Repo, b.Repo)
}

// taskPRWorkflowOperation returns the idempotency key, engine payload, and
// outcome time for a PR workflow event, or false when the event does not
// fire. The merged outcome time is the latest merge among linked PRs.
func taskPRWorkflowOperation(
	trigger engine.Trigger, taskID string, pr *github.TaskPR, linked []*github.TaskPR,
) (string, any, time.Time, bool) {
	if trigger == engine.TriggerOnPRClosed {
		closedAt := taskPROutcomeTime(pr.ClosedAt)
		operationID := fmt.Sprintf("%s:%s:%s:%s", trigger, taskID, taskPRKey(pr), closedAt.Format(time.RFC3339Nano))
		return operationID, engine.OnPRClosedPayload{PR: linkedPR(pr)}, closedAt, true
	}
	var mergedAt time.Time
	keys := make([]string, 0, len(linked))
	prs := make([]engine.LinkedPR, 0, len(linked))
	for _, candidate := range linked {
		if !strings.EqualFold(strings.TrimSpace(candidate.State), taskPRStateMerged) {
			return "", nil, time.Time{}, false
		}
		if at := taskPROutcomeTime(candidate.MergedAt); at.After(mergedAt) {
			mergedAt = at
		}
		keys = append(keys, taskPRKey(candidate))
		prs = append(prs, linkedPR(candidate))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "|")))
	operationID := fmt.Sprintf("%s:%s:%s", trigger, taskID, hex.EncodeToString(sum[:]))
	return operationID, engine.OnPRMergedPayload{PRs: prs}, mergedAt, true
}

// taskPROutcomeTime treats an unknown merge or close time as observed now.
func taskPROutcomeTime(at *time.Time) time.Time {
	if at == nil || at.IsZero() {
		return time.Now().UTC()
	}
	return at.UTC()
}

func taskPRKey(pr *github.TaskPR) string {
	return fmt.Sprintf("%s/%s#%d", strings.ToLower(pr.Owner), strings.ToLower(pr.Repo), pr.PRNumber)
}

func linkedPR(pr *github.TaskPR) engine.LinkedPR {
	return engine.LinkedPR{
		Owner:    pr.Owner,
		Repo:     pr.Repo,
		PRNumber: pr.PRNumber,
		PRURL:    pr.PRURL,
		State:    pr.State,
	}
}

type taskStepTransitionLister interface {
	ListTaskStepTransitions(ctx context.Context, taskID string, beforeID int64, limit int) ([]models.StepTransition, error)
}

// taskStepArrivedAt returns when the task entered its current step, from the
// task-step transition ledger, together with the immutable row identity used
// to fence its commit. The zero time means the arrival is unknown.
func (s *Service) taskStepArrivedAt(ctx context.Context, task *models.Task) (time.Time, int64, error) {
	lister, ok := s.repo.(taskStepTransitionLister)
	if !ok {
		return time.Time{}, 0, nil
	}
	rows, err := lister.ListTaskStepTransitions(ctx, task.ID, 0, 1)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("load latest step transition: %w", err)
	}
	if len(rows) == 0 || rows[0].ToWorkflowStepID == nil || *rows[0].ToWorkflowStepID != task.WorkflowStepID {
		return time.Time{}, 0, nil
	}
	return rows[0].OccurredAt.UTC(), rows[0].ID, nil
}

// taskPRWorkflowSession picks the session the transition runs against. live
// is true for a non-terminal session, which also receives the destination's
// on_enter actions. When every session is terminal, the latest one (the
// primary first) only records the move. A task without sessions is skipped.
func (s *Service) taskPRWorkflowSession(ctx context.Context, taskID string) (*models.TaskSession, bool, error) {
	session, activeErr := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if activeErr == nil && session != nil {
		return session, true, nil
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, false, fmt.Errorf("list task sessions: %w", err)
	}
	var live, terminal *models.TaskSession
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if !isTerminalSessionState(session.State) {
			live = preferPrimarySession(live, session)
		} else {
			terminal = preferPrimarySession(terminal, session)
		}
	}
	if live != nil {
		return live, true, nil
	}
	if terminal != nil {
		return terminal, false, nil
	}
	if activeErr != nil && !errors.Is(activeErr, models.ErrTaskSessionNotFound) {
		return nil, false, fmt.Errorf("load active task session: %w", activeErr)
	}
	s.logger.Debug("PR workflow event: task has no session", zap.String("task_id", taskID))
	return nil, false, nil
}

// preferPrimarySession keeps the first session seen (sessions arrive newest
// first) unless a later one is the primary session.
func preferPrimarySession(current, candidate *models.TaskSession) *models.TaskSession {
	if current == nil || (!current.IsPrimary && candidate.IsPrimary) {
		return candidate
	}
	return current
}

// applyTaskPRWorkflowTransition commits an evaluated transition only while
// the task is still on the evaluated step. With a live session it runs the
// full lifecycle, including the destination's on_enter. With only terminal
// sessions it uses the session-independent guarded lifecycle: the move,
// history, task completion, and engine-owned entry actions still apply.
// A task that already left the step reports false with no error.
func (s *Service) applyTaskPRWorkflowTransition(
	ctx context.Context, task *models.Task, session *models.TaskSession, live bool,
	result engine.HandleResult, trigger engine.Trigger,
) (bool, error) {
	if !live {
		return s.applyGuardedTransitionLifecycle(ctx, task.ID, session.ID, result.FromStepID, result.ToStepID, trigger)
	}
	casAttempted, casApplied := false, false
	var commitErr error
	applied := s.applyEngineTransitionWithCommitMode(ctx, task.ID, session, result, trigger,
		task.Description, transitionLifecycleWithOnEnter,
		func(commitCtx context.Context) (bool, error) {
			casAttempted = true
			casApplied, commitErr = s.commitTaskPRWorkflowTransition(commitCtx, task.ID, session.ID, result, trigger)
			return casApplied, commitErr
		})
	switch {
	case commitErr != nil:
		return false, fmt.Errorf("commit %s transition: %w", trigger, commitErr)
	case applied || casApplied:
		return true, nil
	case casAttempted:
		return false, nil
	}
	return false, errors.New("PR workflow transition did not apply")
}

// commitTaskPRWorkflowTransition is the compare-and-swap commit for a live
// session: it writes the move only if the task is still on result.FromStepID,
// then publishes the update and clears the session review status as
// ApplyTransition does.
func (s *Service) commitTaskPRWorkflowTransition(
	ctx context.Context, taskID, sessionID string, result engine.HandleResult, trigger engine.Trigger,
) (bool, error) {
	targetStep, err := s.workflowStepGetter.GetStep(ctx, result.ToStepID)
	if err != nil {
		return false, err
	}
	transitionCtx := transitionContext(ctx, sessionID, trigger, targetStep)
	task, oldWorkflowID, applied, err := s.workflowStore.applyTransitionIfAtStepRaw(
		transitionCtx, taskID, result.FromStepID, result.ToStepID,
	)
	if err != nil || !applied {
		return false, err
	}
	s.publishTaskUpdated(ctx, task, oldWorkflowID)
	if task.QueuedForStepID == "" {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
	s.workflowStore.pullNextTaskOnVacate(ctx, result.FromStepID, taskID)
	return true, nil
}

// scheduleTaskPRWorkflowRetry re-runs a failed PR workflow event with bounded
// backoff on the CI automation worker lifecycle, so service shutdown cancels
// pending retries. Each retry stops when the stored PR no longer reports the
// failed event's outcome.
func (s *Service) scheduleTaskPRWorkflowRetry(pr *github.TaskPR, cause error) {
	s.ciAutomationMu.Lock()
	if s.ciAutomationStopped {
		s.ciAutomationMu.Unlock()
		return
	}
	if s.ciAutomationCtx == nil {
		s.ciAutomationCtx, s.ciAutomationCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.ciAutomationCtx
	s.ciAutomationWorkers.Add(1)
	s.ciAutomationMu.Unlock()

	snapshot := *pr
	delays := s.taskPRWorkflowRetryDelays
	if delays == nil {
		delays = defaultTaskPRWorkflowRetryDelays
	}
	go func() {
		defer s.ciAutomationWorkers.Done()
		err := cause
		for _, delay := range delays {
			s.logger.Warn("PR workflow event failed; retrying",
				zap.String("task_id", snapshot.TaskID),
				zap.Int("pr_number", snapshot.PRNumber),
				zap.Duration("delay", delay),
				zap.Error(err))
			timer := time.NewTimer(delay)
			select {
			case <-workerCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, err = s.processTaskPRWorkflowEvent(workerCtx, &snapshot); err == nil {
				return
			}
		}
		s.logger.Error("PR workflow event failed after retries",
			zap.String("task_id", snapshot.TaskID),
			zap.Int("pr_number", snapshot.PRNumber),
			zap.Error(err))
	}()
}
