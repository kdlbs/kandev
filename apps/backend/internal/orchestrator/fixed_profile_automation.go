package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const fixedProfileAutomationWaitKey = "fixed_profile_automation_wait"

type fixedProfileWaitIdentityKey struct{}

func (s *Service) retainFixedProfileAutomation(
	ctx context.Context, task *models.Task, session *models.TaskSession, prompt string,
	metadata map[string]interface{}, runID string, action automation.ThreadAction, reason string, initialStart bool,
) bool {
	if runID == "" || s.messageQueue == nil {
		return false
	}
	ctx = context.WithoutCancel(ctx)
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, task.ID, session.ID)
	if err != nil {
		return false
	}
	wait := map[string]interface{}{
		"run_id": runID, "prompt": prompt, "metadata": metadata,
		"action": string(action), "reason": reason,
		"incarnation_id": identity.SessionIncarnationID, "since": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if initialStart {
		wait["initial_start"] = true
	}
	return s.repo.SetSessionMetadataKey(ctx, session.ID, fixedProfileAutomationWaitKey, wait) == nil
}

func (s *Service) validateFixedProfileWaitIdentity(ctx context.Context, sessionID string) error {
	identity, ok := ctx.Value(fixedProfileWaitIdentityKey{}).(messagequeue.QueueSessionIdentity)
	if !ok {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != identity.TaskID ||
		session.ID != identity.SessionID || session.QueueIncarnationID != identity.SessionIncarnationID {
		return &sessionRecoveryRequiredError{Block: &models.SessionRecoveryBlock{Reason: "fixed_profile_wait_owner_changed"}}
	}
	return nil
}

// Existing durable run/launch ownership decides replay; this path never
// invents an accepted turn, a new run, or a new retry attempt.
func (s *Service) resumeFixedProfileAutomation(ctx context.Context, session *models.TaskSession) (owned, done bool) {
	wait, ok := session.Metadata[fixedProfileAutomationWaitKey].(map[string]interface{})
	if !ok || wait["incarnation_id"] != session.QueueIncarnationID {
		return false, false
	}
	binding, ok := s.automationService.(automationRunBinding)
	if !ok {
		return true, false
	}
	run, err := binding.GetRun(ctx, models.StringFromAny(wait["run_id"]))
	if err != nil {
		return true, false
	}
	if run == nil || (run.Status != automation.RunStatusTriggered && run.Status != automation.RunStatusTaskCreated) {
		s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
		return true, true
	}
	task, eligible := s.fixedProfileAutomationTask(ctx, session, run)
	if !eligible {
		return true, false
	}
	identity := messagequeue.QueueSessionIdentity{TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID}
	ctx = context.WithValue(ctx, fixedProfileWaitIdentityKey{}, identity)
	if run.RetryGroupID != "" {
		return true, s.resumeFixedProfileRetryRun(ctx, run, session, wait)
	}
	if run.Status == automation.RunStatusTaskCreated {
		s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
		return true, true
	}
	return true, s.resumeFixedProfileContinuation(ctx, run, task, session, wait)
}

func (s *Service) resumeFixedProfileContinuation(ctx context.Context, run *automation.AutomationRun, task *models.Task, session *models.TaskSession, wait map[string]interface{}) bool {
	reader, ok := s.automationService.(interface {
		GetAutomation(context.Context, string) (*automation.Automation, error)
	})
	if !ok {
		return false
	}
	a, err := reader.GetAutomation(ctx, run.AutomationID)
	if err != nil || a == nil || !a.Enabled {
		return false
	}
	if initial, _ := wait["initial_start"].(bool); initial {
		return s.resumeFixedProfileInitialStart(ctx, a, run, task, session, wait)
	}
	metadata, _ := wait["metadata"].(map[string]interface{})
	bound := s.dispatchAutomationContinuation(ctx, a, task, session, models.StringFromAny(wait["prompt"]), metadata,
		run.ID, automation.ThreadAction(models.StringFromAny(wait["action"])), models.StringFromAny(wait["reason"]))
	if bound {
		s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
	}
	return bound
}

func (s *Service) resumeFixedProfileRetryRun(ctx context.Context, run *automation.AutomationRun, session *models.TaskSession, wait interface{}) bool {
	owner, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return false
	}
	launch, err := owner.GetRetryLaunch(ctx, run.ID)
	if err != nil || launch.CancelRequested || launch.Generation != run.RetryGroupGeneration ||
		launch.TaskID != session.TaskID || launch.SessionID != session.ID {
		return false
	}
	if launch.State == automation.RetryLaunchAccepted || launch.State == automation.RetryLaunchStopped {
		s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
		return true
	}
	if launch.State != automation.RetryLaunchQueued && launch.State != automation.RetryLaunchPrepared {
		return false
	}
	s.dispatchReservedRetryLaunch(ctx, run, launch)
	if !s.retryRunHasExactBinding(ctx, run.ID) {
		return false
	}
	s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
	return true
}

func (s *Service) fixedProfileAutomationTask(ctx context.Context, session *models.TaskSession, run *automation.AutomationRun) (*models.Task, bool) {
	if (run.TaskID != "" && run.TaskID != session.TaskID) || (run.SessionID != "" && run.SessionID != session.ID) {
		return nil, false
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil || task.State == v1.TaskStateCompleted || task.State == v1.TaskStateCancelled {
		return nil, false
	}
	return task, true
}

func (s *Service) retainFixedProfileAutomationStart(
	ctx context.Context, a *automation.Automation, task *models.Task, runID string,
	action automation.ThreadAction, reason string, err error,
) bool {
	var refusal *FixedProfileSuspendedError
	if runID == "" || !errors.As(err, &refusal) || refusal.SessionID == "" {
		return false
	}
	ctx = context.WithoutCancel(ctx)
	session, readErr := s.repo.GetTaskSession(ctx, refusal.SessionID)
	if readErr != nil || session == nil || session.TaskID != task.ID {
		return false
	}
	metadata := map[string]interface{}{"automation_id": a.ID, "automation_run_id": runID}
	return s.retainFixedProfileAutomation(ctx, task, session, task.Description, metadata, runID, action, reason, true)
}

// An unattempted start has no resumable executor. Re-enter the normal task
// start and run binder; do not send its initial prompt as a continuation.
func (s *Service) resumeFixedProfileInitialStart(ctx context.Context, a *automation.Automation, run *automation.AutomationRun, task *models.Task, session *models.TaskSession, wait map[string]interface{}) bool {
	if s.validateFixedProfileWaitIdentity(ctx, session.ID) != nil {
		return false
	}
	pendingTask := *task
	pendingTask.Description = models.StringFromAny(wait["prompt"])
	bound := s.autoStartAutomationTaskForRun(ctx, a, &pendingTask, task.WorkflowStepID, run.ID,
		automation.ThreadAction(models.StringFromAny(wait["action"])), models.StringFromAny(wait["reason"]), true, nil)
	if bound {
		s.clearFixedProfileMetadataWait(ctx, session.ID, fixedProfileAutomationWaitKey, wait)
	}
	return bound
}
