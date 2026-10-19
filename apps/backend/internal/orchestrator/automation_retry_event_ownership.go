package orchestrator

import (
	"context"
	"errors"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
)

func hydrateRetryAutomationEvent(ctx context.Context, evt *automation.AutomationTriggeredEvent, binding automationRunBinding) (*automation.Automation, bool, error) {
	run, err := binding.GetRun(ctx, evt.RunID)
	if err != nil {
		return nil, true, err
	}
	if run == nil || run.RetryGroupID == "" {
		return nil, false, nil
	}
	snapshot, err := automation.DecodeRetryLaunchConfigSnapshot(run.RetryLaunchConfigSnapshot, run.RetryLaunchConfigVersion)
	if err != nil {
		return nil, true, err
	}
	if run.ID != evt.RunID || snapshot.AutomationID != run.AutomationID {
		return nil, true, errors.New("retry launch snapshot does not match its run owner")
	}
	evt.AutomationID = snapshot.AutomationID
	evt.TriggerID = snapshot.TriggerID
	evt.TriggerType = snapshot.TriggerType
	evt.DedupKey = snapshot.DedupKey
	evt.RetryGroupGeneration = run.RetryGroupGeneration
	evt.RetryExternalID = automation.RetryTaskExternalID(run.ID, run.RetryGroupGeneration)
	evt.SnapshotVersion = snapshot.Version
	return automationFromRetrySnapshot(snapshot), true, nil
}

func (s *Service) retryTaskCleanupOwned(ctx context.Context, taskID string) bool {
	if s.repo == nil {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("task cleanup ownership read failed", zap.String("task_id", taskID), zap.Error(err))
		return true
	}
	if task == nil || task.Metadata == nil {
		return false
	}
	runID, _ := task.Metadata[retryAutomationRunIDMetadataKey].(string)
	if runID == "" {
		return false
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return false
	}
	run, readErr := binding.GetRun(ctx, runID)
	if readErr != nil || (run != nil && run.RetryGroupID != "") {
		s.logger.Warn("retry task remains owned by its durable ledger", zap.String("task_id", taskID), zap.String("run_id", runID))
		return true
	}
	return false
}
