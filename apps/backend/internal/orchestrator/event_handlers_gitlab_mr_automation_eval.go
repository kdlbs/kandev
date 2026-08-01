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

// handleTaskMRLifecycleAutomation is the evaluation entrypoint invoked for
// every observed MR change. It fails closed (AC31): a transient error from
// the task lookup or options load is returned to the caller (who logs and
// lets the next poll retry) rather than swallowed as "nothing to do". Only a
// definitive ErrTaskNotFound, a nil task, or an archived task discards the
// event.
func (s *Service) handleTaskMRLifecycleAutomation(ctx context.Context, mr *gitlab.TaskMR) error {
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
	options, err := s.gitlabMRAutomation.GetTaskMRAutomationResponse(ctx, mr.TaskID)
	if err != nil {
		return err
	}
	if !options.PromptOnReviewRequested && !options.PromptOnMerged && !options.PromptOnClosed {
		return nil
	}
	delivered, err := s.evalTaskMRLifecycle(ctx, mr, options, s.gitlabMRAutomation)
	if err != nil {
		s.logger.Debug("task MR lifecycle automation failed",
			zap.String("task_id", mr.TaskID),
			zap.String("repository_id", mr.RepositoryID),
			zap.String("project_path", mr.ProjectPath),
			zap.Int("mr_iid", mr.MRIID),
			zap.Error(err))
		s.recordMRAutomationError(ctx, mr, err)
		return nil
	}
	if delivered {
		s.publishTaskMRAutomationState(ctx, mr.TaskID)
	}
	return nil
}

func (s *Service) evalTaskMRLifecycle(
	ctx context.Context,
	mr *gitlab.TaskMR,
	options *gitlab.TaskMRAutomationResponse,
	automation taskMRAgentAutomationService,
) (bool, error) {
	terminal := mr.State == gitlabMRStateMerged || mr.State == gitlabMRStateClosed
	if !terminal && options.PromptOnReviewRequested {
		username, _, err := automation.RebindTaskMRReviewer(ctx, mr.TaskID)
		if err != nil {
			return false, err
		}
		options.ReviewReviewerUsername = username
	}
	checkpoint, err := automation.GetTaskMRLifecycleState(ctx, mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
	if err != nil {
		return false, err
	}
	reviewRequested, err := currentTaskMRReviewRequest(ctx, automation, mr, options)
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
	err = automation.RecordTaskMRLifecyclePrompt(ctx, gitlab.TaskMRLifecyclePrompt{
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
	return err == nil, err
}

func (s *Service) recordMRAutomationError(ctx context.Context, mr *gitlab.TaskMR, cause error) {
	if s.gitlabMRAutomation == nil {
		return
	}
	if err := s.gitlabMRAutomation.RecordTaskMRAutomationError(
		context.WithoutCancel(ctx), mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID, cause.Error(),
	); err != nil {
		s.logger.Debug("record MR automation error failed", zap.String("task_id", mr.TaskID), zap.Error(err))
	}
}

func (s *Service) publishTaskMRAutomationState(ctx context.Context, taskID string) {
	if s.gitlabMRAutomation == nil || s.eventBus == nil || taskID == "" {
		return
	}
	resp, err := s.gitlabMRAutomation.GetTaskMRAutomationResponse(context.WithoutCancel(ctx), taskID)
	if err != nil {
		s.logger.Debug("load task MR options for state publish failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	event := bus.NewEvent(events.GitLabTaskMROptionsUpdated, mrAutomationStateEventSource, resp)
	if err := s.eventBus.Publish(context.WithoutCancel(ctx), events.GitLabTaskMROptionsUpdated, event); err != nil {
		s.logger.Debug("publish task MR automation state failed", zap.String("task_id", taskID), zap.Error(err))
	}
}
