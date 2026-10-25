package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
)

const automationStartOwnershipReleaseTimeout = 10 * time.Second

// Release is conditional on the captured owner, so delayed cleanup cannot erase a successor.
func (s *Service) releaseAutomationStartOwnership(ctx context.Context, taskID, owner string) bool {
	remover, ok := s.repo.(interface {
		RemoveTaskMetadataKeyIfValue(context.Context, string, string, string) (bool, error)
	})
	if !ok || owner == "" {
		return false
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), automationStartOwnershipReleaseTimeout)
	defer cancel()
	removed, err := remover.RemoveTaskMetadataKeyIfValue(releaseCtx, taskID, models.MetaKeyAutomationStartOwned, owner)
	if err != nil {
		s.logger.Warn("failed to clear automation start ownership marker",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	if removed {
		s.publishTaskUpdatedByID(releaseCtx, taskID)
	}
	return removed
}

type automationStartOwnershipLister interface {
	ListTasksWithAutomationStartOwned(context.Context) ([]*models.Task, error)
}

// Recovery uses definitive ledger evidence, never process restart or lease age.
func (s *Service) releaseAbandonedAutomationStartOwnership(ctx context.Context) {
	lister, ok := s.repo.(automationStartOwnershipLister)
	if !ok {
		return
	}
	tasks, err := lister.ListTasksWithAutomationStartOwned(ctx)
	if err != nil {
		s.logger.Warn("failed to list automation start ownership markers", zap.Error(err))
		return
	}
	for _, task := range tasks {
		s.reconcileAutomationStartOwnership(ctx, task)
	}
}

func (s *Service) reconcileAutomationStartOwnership(ctx context.Context, task *models.Task) bool {
	if task == nil {
		return false
	}
	owner, _ := task.Metadata[models.MetaKeyAutomationStartOwned].(string)
	binding, ok := s.automationService.(automationRunBinding)
	if owner == "" || !ok {
		return false
	}
	run, err := binding.GetRun(ctx, owner)
	if err != nil || run == nil {
		return false
	}
	var launch *automation.RetryLaunch
	if run.RetryGroupID != "" {
		ledger, ok := s.automationService.(automationRetryLaunchOwner)
		if !ok {
			return false
		}
		launch, err = ledger.GetRetryLaunch(ctx, owner)
		if err != nil {
			return false
		}
	}
	if !automationInitialStartResolved(task, run, launch) {
		return false
	}
	return s.releaseAutomationStartOwnership(ctx, task.ID, owner)
}

// Dispatch completion can reconcile only its immutable run identity.
func (s *Service) reconcileAutomationStartOwnershipForRun(ctx context.Context, taskID, runID string) bool {
	return s.reconcileAutomationStartOwnership(ctx, &models.Task{
		ID: taskID, Metadata: map[string]interface{}{models.MetaKeyAutomationStartOwned: runID},
	})
}

// Legacy drop cleanup requires absence of persisted session evidence.
// A session row can precede run binding, and its state alone cannot prove termination.
func (s *Service) releaseDroppedAutomationStartOwnership(ctx context.Context, task *models.Task, runID string) {
	binding, ok := s.automationService.(automationRunBinding)
	owner, _ := task.Metadata[models.MetaKeyAutomationStartOwned].(string)
	if !ok || owner == "" || owner != runID {
		return
	}
	run, err := binding.GetRun(ctx, runID)
	if err != nil || run == nil || run.TaskID != task.ID {
		return
	}
	if run.RetryGroupID == "" && run.Status == automation.RunStatusFailed && run.SessionID == "" && run.TurnID == "" {
		sessions, err := s.repo.ListTaskSessions(ctx, task.ID)
		if err != nil || len(sessions) != 0 {
			return
		}
		s.releaseAutomationStartOwnership(ctx, task.ID, owner)
		return
	}
	s.reconcileAutomationStartOwnership(ctx, task)
}

func automationInitialStartResolved(task *models.Task, run *automation.AutomationRun, launch *automation.RetryLaunch) bool {
	owner, _ := task.Metadata[models.MetaKeyAutomationStartOwned].(string)
	if owner == "" || run == nil || run.ID != owner || run.TaskID != task.ID {
		return false
	}
	if run.RetryGroupID == "" {
		return run.Status == automation.RunStatusTaskCreated && run.SessionID != "" && run.TurnID != ""
	}
	return automationRetryInitialStartResolved(run, launch)
}

func automationRetryInitialStartResolved(run *automation.AutomationRun, launch *automation.RetryLaunch) bool {
	switch run.Status {
	case automation.RunStatusFailed, automation.RunStatusSucceeded, automation.RunStatusCancelled:
	default:
		return false
	}
	if launch == nil || launch.State != automation.RetryLaunchStopped ||
		launch.AutomationID != run.AutomationID || run.RetryGroupGeneration <= 0 {
		return false
	}
	expected := &automation.RetryLaunch{
		RunID: run.ID, Generation: run.RetryGroupGeneration, TaskID: run.TaskID,
		SessionID: run.SessionID, TurnID: run.TurnID, SubmissionID: launch.SubmissionID,
	}
	return launch.SessionID != "" && launch.TurnID != "" && launch.SubmissionID != "" &&
		sameRetryLaunchIdentity(launch, expected)
}
