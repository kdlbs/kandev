package orchestrator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// schedulingOrphanEvent names the startup cause in logs and launch dispatch.
const schedulingOrphanEvent = "startup.scheduling_orphan"

// schedulingTaskLister is the narrow repository capability the startup
// SCHEDULING-orphan pass needs. It is reached through a type assertion on
// s.repo rather than growing sessionExecutorStore for one startup pass.
type schedulingTaskLister interface {
	ListSchedulingTasks(ctx context.Context) ([]*models.Task, error)
}

// schedulingOrphanTurnChecker is a narrow interface to check if a session has
// a turn for a specific step. It is reached through a type assertion on
// s.repo rather than growing sessionExecutorStore for one startup pass.
type schedulingOrphanTurnChecker interface {
	ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error)
}

// schedulingOrphanTransitionLister reads the newest step-transition ledger row,
// which names the accepted entry into the current step and the step it left.
// It is reached through a type assertion on s.repo like the other narrow
// capabilities of this pass.
type schedulingOrphanTransitionLister interface {
	ListTaskStepTransitions(ctx context.Context, taskID string, beforeID int64, limit int) ([]models.StepTransition, error)
}

// schedulingOrphanSessionLister is the narrow repository capability this pass
// needs to see an IDLE session, which the shared active-session lookup
// excludes. It is reached through a type assertion on s.repo rather than
// widening GetActiveTaskSessionByTaskID.
type schedulingOrphanSessionLister interface {
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
}

type schedulingOrphanStateRepairer interface {
	UpdateTaskStateIfCurrentEntry(context.Context, *models.Task, v1.TaskState) (bool, time.Time, error)
}

// reconcileSchedulingOrphansOnStartup restarts automatic step starts that a
// backend restart interrupted before they produced a running session.
// Such a task is in SCHEDULING with no
// deferred_launch record, no queue destination, and no lifecycle token, so no
// other startup pass lists it: the lifecycle sweep lists durable tokens, the
// WIP queue reconciliation lists queued destinations, the ceiling sweep lists
// deferred records, and the dependency pass lists pending dependency launches.
// Recovery reuses the existing automatic start path, so the session ceiling,
// workflow WIP, step prompt, and session serialization rules all still apply.
//
// It runs before the durable-token sweep so a task that a token owns is still
// token-bearing here and is skipped, rather than being seen as an orphan after
// its token was cleared but before its detached launch created a session.
func (s *Service) reconcileSchedulingOrphansOnStartup(ctx context.Context) {
	lister, ok := s.repo.(schedulingTaskLister)
	if !ok {
		return
	}
	tasks, err := lister.ListSchedulingTasks(ctx)
	if err != nil {
		s.logger.Warn("startup: failed to list SCHEDULING tasks for orphan recovery", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if s.schedulingOrphanRecoverable(ctx, task) {
			s.recoverSchedulingOrphan(ctx, task)
		}
	}
}

// schedulingOrphanRecoverable reports whether a SCHEDULING task is an accepted
// automatic start that a restart left without any owner. Every legitimate wait
// is excluded by an existing field or contract, so recovery never forces a
// launch or bypasses the ceiling or WIP limits.
func (s *Service) schedulingOrphanRecoverable(ctx context.Context, task *models.Task) bool {
	if task == nil || task.WorkflowStepID == "" || s.workflowStepGetter == nil {
		return false
	}
	// Waiting for workflow WIP admission: the queue path owns it.
	if task.QueuedForStepID != "" {
		return false
	}
	// A deferred_launch record carries a dependency, WIP overflow, or session
	// ceiling intent. Each has its own retry owner.
	if schedulingOrphanHasDeferredIntent(task) {
		return false
	}
	// A durable lifecycle token already names a recovery owner.
	if schedulingOrphanHasLifecycleToken(task) {
		return false
	}
	// A recorded launch error needs a user decision (model, credential,
	// branch); the task launch recovery action owns it.
	if _, found := models.LoadTaskLaunchError(task.Metadata); found {
		return false
	}
	// Office tasks queue runs, not sessions; their scheduler recovers them.
	if owned, err := s.lookupOfficeTask(ctx, task.ID); err != nil || owned {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return false
	}
	// A step without on_enter auto-start is a manual or hold destination.
	if !workflowmove.ShouldAutoStartAgent(step, nil) {
		return false
	}
	// A dependency-blocked task must not start; the dependency sweep launches
	// it once its predecessor resolves.
	if blocked, _ := s.dependencyBlocksAutoStart(ctx, task.ID, schedulingOrphanEvent); blocked {
		return false
	}
	return true
}

// schedulingOrphanHasDeferredIntent reports whether the task still carries a
// deferred_launch record. Key presence is the ownership signal, matching
// launchDeferredTask: a malformed, empty, or profile-less record is kept by that
// reader and stops the ordinary automatic start, so it is not absence here.
func schedulingOrphanHasDeferredIntent(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, present := task.Metadata[models.MetaKeyDeferredLaunch]
	return present
}

// schedulingOrphanHasLifecycleToken reports whether another startup pass
// already owns this task through a durable token.
func schedulingOrphanHasLifecycleToken(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	if queuedMoveExitPending(task) || manualMoveLifecyclePending(task) ||
		manualMoveLifecycleCompleted(task) || hasQueuePromotionPending(task) {
		return true
	}
	return autoStartOnCreateActionable(task)
}

// recoverSchedulingOrphan restarts the current step for one orphaned
// SCHEDULING task. A task without an active session goes through the ordinary
// automatic start. A resumable session without a turn for the current step
// receives the entry in place, so no duplicate session is created. A session
// that already has a turn for the current step only has its stale task state
// repaired. Any other active session state means a launch is already live and
// is left alone.
func (s *Service) recoverSchedulingOrphan(ctx context.Context, task *models.Task) {
	session, err := s.schedulingOrphanRecoverableSession(ctx, task.ID)
	if err != nil {
		s.logger.Warn(schedulingOrphanEvent+": failed to read the recoverable session",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	if session == nil {
		s.logger.Info(schedulingOrphanEvent+": restarting SCHEDULING task with no session",
			zap.String("task_id", task.ID),
			zap.String("step_id", task.WorkflowStepID))
		s.autoStartTaskForStep(ctx, task.ID, task.WorkflowStepID, schedulingOrphanEvent, 0, false)
		return
	}
	if !schedulingOrphanResumableSession(session) {
		return
	}
	hasTurnForStep, err := s.sessionHasTurnForStep(ctx, session.ID, task)
	if err != nil {
		s.logger.Warn(schedulingOrphanEvent+": failed to check session turns for step",
			zap.String("task_id", task.ID), zap.String("session_id", session.ID), zap.Error(err))
		return
	}
	if hasTurnForStep {
		// The step entry already delivered a turn; only the stale SCHEDULING
		// state is left. Repair it without sending another prompt.
		s.repairSchedulingOrphanTaskState(ctx, task)
		return
	}
	s.reenterSchedulingOrphanStep(ctx, task, session)
}

// schedulingOrphanRecoverableSession returns the session this pass should act
// on: the task's active session when one exists, otherwise the most recent
// resumable session. GetActiveTaskSessionByTaskID deliberately excludes IDLE
// (an office session between turns is not a task's active session), but this
// pass treats IDLE as resumable, so it resolves that state separately rather
// than widening the shared active-session lookup.
func (s *Service) schedulingOrphanRecoverableSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	active, err := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err == nil && active != nil {
		return active, nil
	}
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, err
	}
	lister, ok := s.repo.(schedulingOrphanSessionLister)
	if !ok {
		return nil, nil
	}
	sessions, err := lister.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range sessions {
		if candidate != nil && candidate.State == models.TaskSessionStateIdle {
			return candidate, nil
		}
	}
	return nil, nil
}

// sessionHasTurnForStep accepts conversational evidence only for the current
// entry. Older turn rows without an entry stamp use the ledger's time boundary.
func (s *Service) sessionHasTurnForStep(ctx context.Context, sessionID string, task *models.Task) (bool, error) {
	checker, ok := s.repo.(schedulingOrphanTurnChecker)
	if !ok {
		return false, nil
	}
	entry, err := s.schedulingOrphanAcceptedEntry(ctx, task)
	if err != nil {
		return false, err
	}
	turns, err := checker.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	for _, turn := range turns {
		if turn == nil || turn.Metadata == nil {
			continue
		}
		if lifecycleOnly, _ := turn.Metadata[models.TurnMetaKeyLifecycleOnly].(bool); lifecycleOnly {
			continue
		}
		if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != task.WorkflowStepID {
			continue
		}
		if stampedEntry := int64Field(turn.Metadata, models.TurnMetaKeyWorkflowStepTransitionIDAtStart); stampedEntry > 0 {
			if stampedEntry == entry.ID {
				return true, nil
			}
			continue
		}
		if !turn.StartedAt.Before(entry.OccurredAt) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) schedulingOrphanAcceptedEntry(ctx context.Context, task *models.Task) (models.StepTransition, error) {
	lister, ok := s.repo.(schedulingOrphanTransitionLister)
	if !ok {
		return models.StepTransition{}, errors.New("orphan recovery cannot read the accepted workflow entry")
	}
	rows, err := lister.ListTaskStepTransitions(ctx, task.ID, 0, 1)
	if err != nil {
		return models.StepTransition{}, err
	}
	if len(rows) == 0 && task.WorkflowStepTransitionID == 0 {
		return models.StepTransition{OccurredAt: task.CreatedAt}, nil
	}
	if len(rows) == 0 || rows[0].ToWorkflowStepID == nil || *rows[0].ToWorkflowStepID != task.WorkflowStepID ||
		(task.WorkflowStepTransitionID > 0 && rows[0].ID != task.WorkflowStepTransitionID) {
		return models.StepTransition{}, errors.New("orphan recovery workflow entry changed")
	}
	return rows[0], nil
}

// schedulingOrphanResumableSession reports whether an existing session is a
// stopped session that can receive the current step's entry turn. CREATED,
// STARTING, and RUNNING sessions are not resumed: a launch is already live.
func schedulingOrphanResumableSession(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	return session.State == models.TaskSessionStateWaitingForInput ||
		session.State == models.TaskSessionStateIdle
}

// reenterSchedulingOrphanStep runs the current step's on_enter on the existing
// session, like an ordinary same-step re-entry. on_exit does not run: the task
// never left the step. The entry-dispatch guard inside processOnEnter still
// rejects a stale route, so a task whose committed route no longer names this
// step is left to its owning path.
//
// The entry runs detached, like the launch of a task without a session: it
// resumes the agent and waits for readiness, and doing that inline would hold
// the rest of the startup sweep, including the bounded lifecycle-token pass,
// for up to one launch timeout per task. The goroutine is tracked in
// orphanReentryWorkers. Service.Stop waits for it for at most
// sendNowClaimRecoveryTimeout (10 seconds); detached work may outlive that bound.
func (s *Service) reenterSchedulingOrphanStep(ctx context.Context, task *models.Task, session *models.TaskSession) {
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return
	}
	transitionID, sourceStep := s.schedulingOrphanEntryContext(ctx, task, session)
	s.logger.Info(schedulingOrphanEvent+": re-entering the current step on a resumable session",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("step_id", step.ID),
		zap.Int64("transition_id", transitionID))
	s.orphanReentryWorkers.Add(1)
	go func() {
		defer s.orphanReentryWorkers.Done()
		entryCtx := context.WithoutCancel(ctx)
		s.processOnEnter(entryCtx, task.ID, session, step, task.Description, transitionID, sourceStep)
	}()
}

// schedulingOrphanEntryContext recovers the accepted entry into the current
// step when the resumable session is not yet proven to be its recipient. Only a
// committed route that names this step and this very session shows a completed
// same-step entry. A missing route, a route still naming another step, or a
// prepared route (the handoff was interrupted between persisting the target and
// promoting it, so the target may be a different session than the one found
// here) all mean the entry is unfinished. The newest ledger row, when it
// entered the current step, then supplies the entry id and the step that was
// left, so processOnEnter's entry guard and profile-switch preparation see the
// same context as the original transition and still enforce their own route and
// recipient checks. When the ledger cannot confirm the entry, the session
// re-enters with no entry context.
func (s *Service) schedulingOrphanEntryContext(
	ctx context.Context, task *models.Task, session *models.TaskSession,
) (int64, *wfmodels.WorkflowStep) {
	if route, ok := models.LoadWorkflowSessionRoute(task.Metadata); ok &&
		route.Phase == workflowSessionRouteCommitted &&
		route.DestinationStepID == task.WorkflowStepID &&
		route.DestinationID == session.ID &&
		workflowEntryDispatchIdentityMatches(route, task.WorkflowStepTransitionID) {
		return 0, nil
	}
	lister, ok := s.repo.(schedulingOrphanTransitionLister)
	if !ok {
		return 0, nil
	}
	rows, err := lister.ListTaskStepTransitions(ctx, task.ID, 0, 1)
	if err != nil || len(rows) == 0 {
		if err != nil {
			s.logger.Warn(schedulingOrphanEvent+": failed to read the current step entry",
				zap.String("task_id", task.ID), zap.String("session_id", session.ID), zap.Error(err))
		}
		return 0, nil
	}
	entry := rows[0]
	if entry.ToWorkflowStepID == nil || *entry.ToWorkflowStepID != task.WorkflowStepID {
		return 0, nil
	}
	var sourceStep *wfmodels.WorkflowStep
	if entry.FromWorkflowStepID != nil && *entry.FromWorkflowStepID != "" {
		if from, err := s.workflowStepGetter.GetStep(ctx, *entry.FromWorkflowStepID); err == nil {
			sourceStep = from
		}
	}
	return entry.ID, sourceStep
}

// repairSchedulingOrphanTaskState clears a stale SCHEDULING state for a task
// whose session already accepted a prompt. The turn-start projection that
// would have moved the task to IN_PROGRESS was lost to the same restart, so the
// state is repaired by a compare-and-set over its exact workflow entry and
// observed task revision, without running the step again.
func (s *Service) repairSchedulingOrphanTaskState(ctx context.Context, task *models.Task) {
	writer, ok := s.repo.(schedulingOrphanStateRepairer)
	if !ok {
		s.logger.Warn(schedulingOrphanEvent + ": repository cannot guard task state by accepted entry")
		return
	}
	updated, updatedAt, err := writer.UpdateTaskStateIfCurrentEntry(ctx, task, v1.TaskStateInProgress)
	if err != nil {
		s.logger.Warn(schedulingOrphanEvent+": failed to repair stale SCHEDULING task state",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	if updated {
		repaired := *task
		repaired.State = v1.TaskStateInProgress
		repaired.UpdatedAt = updatedAt
		s.publishTaskStateChanged(ctx, &repaired, task.State)
		s.logger.Info(schedulingOrphanEvent+": repaired stale SCHEDULING task state for a session with prompt history",
			zap.String("task_id", task.ID))
	}
}
