package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// errPRTaskBusy fails a per-PR firing whose task became busy after admission.
var errPRTaskBusy = errors.New("PR task is busy: its session is running another turn")

// automationPRContinuationState is the slice of the automation service that
// stores PR subject to task mappings.
type automationPRContinuationState interface {
	GetPRContinuationTaskID(ctx context.Context, automationID, subject string) (string, error)
	SetPRContinuationTaskID(ctx context.Context, automationID, subject, taskID string) error
}

// automationPRTaskLifecycle is the slice of the task service the per-PR policy
// uses to move, archive, and delete PR tasks.
type automationPRTaskLifecycle interface {
	MoveTask(ctx context.Context, id, workflowID, workflowStepID string, position int) (*taskservice.MoveTaskResult, error)
	ArchiveTask(ctx context.Context, id string) error
	DeleteTaskWithLifecycleAndReason(ctx context.Context, id, reason string) error
}

// SetAutomationPRTaskLifecycle wires task moves, archive, and delete for PR tasks.
func (s *Service) SetAutomationPRTaskLifecycle(lifecycle automationPRTaskLifecycle) {
	s.prTaskLifecycle = lifecycle
}

// prSessionReadiness classifies a PR task's primary session.
type prSessionReadiness int

const (
	prSessionReady prSessionReadiness = iota
	prSessionBusy
	prSessionEnded
)

func classifyPRSession(session *models.TaskSession) prSessionReadiness {
	if session == nil {
		return prSessionEnded
	}
	switch session.State {
	case models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		return prSessionReady
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateRunning:
		return prSessionBusy
	default:
		return prSessionEnded
	}
}

func (s *Service) primaryTaskSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	lookup, ok := s.repo.(interface {
		GetTaskSessionByTaskID(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil, errors.New("task session lookup is unavailable")
	}
	session, err := lookup.GetTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		return nil, nil //nolint:nilerr // a missing session classifies as ended
	}
	return session, nil
}

// AutomationPRTaskBusy reports whether a PR task's primary session is
// starting or running a turn.
func (s *Service) AutomationPRTaskBusy(ctx context.Context, taskID string) (bool, error) {
	session, err := s.primaryTaskSession(ctx, taskID)
	if err != nil {
		return false, err
	}
	return classifyPRSession(session) == prSessionBusy, nil
}

// ArchiveAutomationPRTask archives a PR task. A missing or already archived
// task counts as done.
func (s *Service) ArchiveAutomationPRTask(ctx context.Context, taskID string) error {
	if s.prTaskLifecycle == nil {
		return errors.New("task lifecycle is unavailable")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return nil //nolint:nilerr // a missing task needs no archive
	}
	return s.prTaskLifecycle.ArchiveTask(ctx, taskID)
}

// DeleteAutomationPRTask deletes a PR task through the lifecycle-aware path.
// A missing task counts as done.
func (s *Service) DeleteAutomationPRTask(ctx context.Context, taskID, reason string) error {
	if s.prTaskLifecycle == nil {
		return errors.New("task lifecycle is unavailable")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil //nolint:nilerr // a missing task needs no delete
	}
	return s.prTaskLifecycle.DeleteTaskWithLifecycleAndReason(ctx, taskID, reason)
}

// findPRContinuation resolves the task a per-PR firing continues. It returns
// a nil task when the firing must create one; the returned action then says
// whether that task is the subject's first or a replacement.
func (s *Service) findPRContinuation(
	ctx context.Context, a *automation.Automation, evt *automation.AutomationTriggeredEvent, subject string,
) (*models.Task, *models.TaskSession, automation.ThreadAction, string, error) {
	state, ok := s.automationService.(automationPRContinuationState)
	if !ok {
		return nil, nil, automation.ThreadActionCreated, "", errors.New("PR continuation state is unavailable")
	}
	taskID, err := state.GetPRContinuationTaskID(ctx, a.ID, subject)
	if err != nil {
		return nil, nil, automation.ThreadActionCreated, "", fmt.Errorf("load PR continuation: %w", err)
	}
	if taskID == "" {
		return nil, nil, automation.ThreadActionCreated, "no task exists for this pull request; created the first task", nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, nil, automation.ThreadActionReplaced, "the pull request's task was not found; created a replacement task", nil
	}
	// A reviewed task normally moves past its start step, so per-PR
	// compatibility matches the workflow but not the start step.
	relaxed := *a
	relaxed.WorkflowStepID = ""
	if reason := s.continuationTaskIncompatibility(ctx, &relaxed, evt, task); reason != "" {
		return nil, nil, automation.ThreadActionReplaced, reason + "; created a replacement task", nil
	}
	session, err := s.primaryTaskSession(ctx, task.ID)
	if err != nil {
		return nil, nil, automation.ThreadActionReplaced, "", err
	}
	switch classifyPRSession(session) {
	case prSessionReady:
		return task, session, automation.ThreadActionResumed, "continued the pull request's task", nil
	case prSessionBusy:
		return nil, nil, automation.ThreadActionResumed, "", errPRTaskBusy
	default:
		return nil, nil, automation.ThreadActionReplaced, "the pull request's task session has ended; created a replacement task", nil
	}
}

// recordPRContinuation maps the subject to a newly created task.
func (s *Service) recordPRContinuation(ctx context.Context, automationID, subject, taskID string) error {
	state, ok := s.automationService.(automationPRContinuationState)
	if !ok {
		return errors.New("PR continuation state is unavailable")
	}
	return state.SetPRContinuationTaskID(ctx, automationID, subject, taskID)
}

// prContinuationPrompt selects the follow-up prompt for a firing that
// continues an existing PR task.
func prContinuationPrompt(
	a *automation.Automation, evt *automation.AutomationTriggeredEvent,
	snapshot *automation.RetryLaunchConfigSnapshot, useSnapshot bool, prompt string,
) string {
	if a.ContinuationPolicy != automation.ContinuationPolicyReusePerPR || a.FollowUpPrompt == "" {
		return prompt
	}
	if snapshot != nil && useSnapshot {
		if snapshot.ResolvedFollowUpPrompt != "" {
			return snapshot.ResolvedFollowUpPrompt
		}
		return prompt
	}
	if rendered := automation.InterpolateAgentPrompt(a.FollowUpPrompt, evt.TriggerType, evt.TriggerData); rendered != "" {
		return rendered
	}
	return prompt
}

// movePRTaskToContinuationStep moves a continued normal task to the
// automation's continuation step while the run owns its agent start, so the
// step's workflow auto-start does not send a second turn. The returned
// release clears that ownership after dispatch.
func (s *Service) movePRTaskToContinuationStep(
	ctx context.Context, a *automation.Automation, task *models.Task, runID string,
) (func(), error) {
	noop := func() {}
	if a.ContinuationPolicy != automation.ContinuationPolicyReusePerPR || a.PRContinuationStepID == "" ||
		task.WorkflowStepID == a.PRContinuationStepID {
		return noop, nil
	}
	if s.prTaskLifecycle == nil {
		return noop, errors.New("continuation step unavailable: task lifecycle is unavailable")
	}
	owner := runID
	if owner == "" {
		owner = "pr-continuation:" + task.ID
	}
	if err := s.repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyAutomationStartOwned, owner); err != nil {
		return noop, fmt.Errorf("continuation step unavailable: %w", err)
	}
	release := func() { s.releaseAutomationStartOwnership(ctx, task.ID, owner) }
	if _, err := s.prTaskLifecycle.MoveTask(ctx, task.ID, task.WorkflowID, a.PRContinuationStepID, 0); err != nil {
		release()
		return noop, fmt.Errorf("continuation step unavailable: %w", err)
	}
	s.logger.Info("moved PR task to its continuation step",
		zap.String("automation_id", a.ID), zap.String("task_id", task.ID),
		zap.String("step_id", a.PRContinuationStepID))
	task.WorkflowStepID = a.PRContinuationStepID
	return release, nil
}
