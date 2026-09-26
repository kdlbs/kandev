package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// ciAutomationFollowUpTimeout bounds best-effort work that must continue
// after the durable prompt side effect, while keeping a stalled store or
// event bus from holding the detached MR evaluation forever. It is a var so
// blocking fakes can exercise the timeout without waiting for the production
// interval.
var ciAutomationFollowUpTimeout = 5 * time.Second

func runTaskMRAutomationFollowUp(
	parent context.Context,
	timeout time.Duration,
	operation func(context.Context) error,
) error {
	followUpCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer cancel()
	return operation(followUpCtx)
}

// handleTaskMRLifecycleAutomation is the evaluation entrypoint invoked for
// every observed MR change. It fails closed (AC31): a transient error from
// the task lookup or options load is returned to the caller (who logs and
// lets the next poll retry) rather than swallowed as "nothing to do". Only a
// definitive ErrTaskNotFound, a nil task, or an archived task discards the
// event.
func (s *Service) handleTaskMRLifecycleAutomation(ctx context.Context, mr *gitlab.TaskMR) error {
	return s.handleTaskMRLifecycleAutomationWithObservation(ctx, mr, nil)
}

func (s *Service) handleTaskMRLifecycleAutomationWithObservation(
	ctx context.Context, mr *gitlab.TaskMR, observation *taskMRReviewerObservation,
) error {
	if s.gitlabMRAutomation == nil || mr == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, mr.TaskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return nil
		}
		return err
	}
	if task == nil || task.ArchivedAt != nil {
		return nil
	}
	evaluation, err := s.gitlabMRAutomation.GetTaskMRAutomationEvaluation(
		ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID,
	)
	if err != nil {
		return err
	}
	if evaluation == nil || evaluation.Options == nil {
		return nil
	}
	options := evaluation.Options
	if !options.AutoFixEnabled && !options.AutoMergeEnabled &&
		!options.PromptOnReviewRequested && !options.PromptOnMerged && !options.PromptOnClosed {
		return nil
	}
	// Auto-fix and auto-merge run first (mirrors GitHub's
	// handleTaskPRCIAutomationWithRefresh ordering): a fresh auto-fix
	// dispatch in this pass must be able to block the same pass's
	// auto-merge attempt (AC16), and both must complete before the
	// lifecycle notification switches (merged/closed) observe the MR's
	// state for this pass.
	s.handleTaskMRCIAutomation(ctx, mr, options)
	s.evalTaskMRLifecycleSwitches(ctx, mr, options, evaluation.Checkpoint, observation)
	return nil
}

// evalTaskMRLifecycleSwitches runs the #2125 lifecycle-notification
// evaluation when at least one of its three switches is on. Split out of
// handleTaskMRLifecycleAutomationWithObservation to keep that function
// under the cyclomatic complexity limit.
func (s *Service) evalTaskMRLifecycleSwitches(
	ctx context.Context, mr *gitlab.TaskMR, options *gitlab.TaskMRAutomationResponse,
	checkpoint *gitlab.TaskMRLifecycleState, observation *taskMRReviewerObservation,
) {
	if !options.PromptOnReviewRequested && !options.PromptOnMerged && !options.PromptOnClosed {
		return
	}
	delivered, err := s.evalTaskMRLifecycleAtCheckpoint(ctx, mr, options, checkpoint, observation, s.gitlabMRAutomation)
	if err != nil {
		s.logger.Debug("task MR lifecycle automation failed",
			zap.String("task_id", mr.TaskID),
			zap.String("repository_id", mr.RepositoryID),
			zap.String("project_path", mr.ProjectPath),
			zap.Int("mr_iid", mr.MRIID),
			zap.Error(err))
		s.recordMRAutomationError(ctx, mr, err)
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
		return
	}
	if delivered {
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
	}
}

func (s *Service) evalTaskMRLifecycle(
	ctx context.Context,
	mr *gitlab.TaskMR,
	options *gitlab.TaskMRAutomationResponse,
	automation taskMRAgentAutomationService,
) (bool, error) {
	checkpoint, err := automation.GetTaskMRLifecycleState(
		ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID,
	)
	if err != nil {
		return false, err
	}
	return s.evalTaskMRLifecycleAtCheckpoint(ctx, mr, options, checkpoint, nil, automation)
}

func (s *Service) evalTaskMRLifecycleAtCheckpoint(
	ctx context.Context,
	mr *gitlab.TaskMR,
	options *gitlab.TaskMRAutomationResponse,
	checkpoint *gitlab.TaskMRLifecycleState,
	observation *taskMRReviewerObservation,
	automation taskMRAgentAutomationService,
) (bool, error) {
	reviewRequested, err := currentTaskMRReviewRequest(ctx, automation, mr, options, observation)
	if err != nil {
		return false, err
	}
	decision := decideTaskMRAgentPrompt(mr.State, options, checkpoint, reviewRequested)
	if decision.Event == "" {
		return false, stampTaskMRAgentObservations(ctx, automation, mr, decision)
	}
	prompt, err := taskMRAgentLifecyclePrompt(decision.Event, mr)
	if err != nil {
		return false, fmt.Errorf("build %s prompt: %w", decision.Event, err)
	}
	sessionID, err := s.dispatchTaskMRAgentPrompt(ctx, mr, prompt, decision.Event)
	if errors.Is(err, errTaskMRAgentInactive) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dispatch %s prompt: %w", decision.Event, err)
	}
	// The durable prompt has already been queued/drained (the user-visible
	// side effect). A cancellation racing this checkpoint write must not leave
	// it unstamped, but a stalled store must not hold the detached MR
	// evaluation forever. The next poll may retry this at-least-once operation.
	err = runTaskMRAutomationFollowUp(ctx, ciAutomationFollowUpTimeout, func(followUpCtx context.Context) error {
		return automation.RecordTaskMRLifecyclePrompt(followUpCtx, gitlab.TaskMRLifecyclePrompt{
			TaskID:          mr.TaskID,
			RepositoryID:    mr.RepositoryID,
			ProjectPath:     mr.ProjectPath,
			MRIID:           mr.MRIID,
			Event:           decision.Event,
			SessionID:       sessionID,
			PromptedAt:      time.Now().UTC(),
			ReviewRequested: decision.ReviewRequested != nil && *decision.ReviewRequested,
			ObservedState:   decision.ObservedState,
		})
	})
	return err == nil, err
}

func (s *Service) recordMRAutomationError(ctx context.Context, mr *gitlab.TaskMR, cause error) {
	if s.gitlabMRAutomation == nil {
		return
	}
	if err := runTaskMRAutomationFollowUp(ctx, ciAutomationFollowUpTimeout, func(followUpCtx context.Context) error {
		return s.gitlabMRAutomation.RecordTaskMRAutomationError(
			followUpCtx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, cause.Error(),
		)
	}); err != nil {
		s.logger.Debug("record MR automation error failed", zap.String("task_id", mr.TaskID), zap.Error(err))
	}
}

func (s *Service) publishTaskMRAutomationState(ctx context.Context, taskID string) {
	if s.gitlabMRAutomation == nil || s.eventBus == nil || taskID == "" {
		return
	}
	var resp *gitlab.TaskMRAutomationResponse
	err := runTaskMRAutomationFollowUp(ctx, ciAutomationFollowUpTimeout, func(followUpCtx context.Context) error {
		var err error
		resp, err = s.gitlabMRAutomation.GetTaskMRAutomationResponse(followUpCtx, taskID)
		return err
	})
	if err != nil {
		s.logger.Debug("load task MR options for state publish failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	event := bus.NewEvent(events.GitLabTaskMROptionsUpdated, mrAutomationStateEventSource, resp)
	if err := runTaskMRAutomationFollowUp(ctx, ciAutomationFollowUpTimeout, func(followUpCtx context.Context) error {
		return s.eventBus.Publish(followUpCtx, events.GitLabTaskMROptionsUpdated, event)
	}); err != nil {
		s.logger.Debug("publish task MR automation state failed", zap.String("task_id", taskID), zap.Error(err))
	}
}
