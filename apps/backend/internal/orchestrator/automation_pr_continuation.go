package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
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
	MoveTaskWithOptions(
		ctx context.Context, id, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions,
	) (*taskservice.MoveTaskResult, error)
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

// primaryTaskSession returns the task's primary session, or nil when it has
// none. Other read failures are returned so callers never mistake them for a
// missing session.
func (s *Service) primaryTaskSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	reader, ok := s.repo.(interface {
		GetPrimarySessionByTaskID(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil, errors.New("primary session lookup is unavailable")
	}
	session, err := reader.GetPrimarySessionByTaskID(ctx, taskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) || errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, nil
	}
	return session, err
}

// loadPRTask returns the task, or nil when it no longer exists.
func (s *Service) loadPRTask(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return nil, nil
	}
	return task, err
}

// AutomationPRTaskBusy reports whether any session of a PR task is starting
// or running a turn.
func (s *Service) AutomationPRTaskBusy(ctx context.Context, taskID string) (bool, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, session := range sessions {
		if classifyPRSession(session) == prSessionBusy {
			return true, nil
		}
	}
	return false, nil
}

// ArchiveAutomationPRTask archives a PR task. A missing or already archived
// task counts as done.
func (s *Service) ArchiveAutomationPRTask(ctx context.Context, taskID string) error {
	if s.prTaskLifecycle == nil {
		return errors.New("task lifecycle is unavailable")
	}
	task, err := s.loadPRTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return err
	}
	return s.prTaskLifecycle.ArchiveTask(ctx, taskID)
}

// DeleteAutomationPRTask deletes a PR task through the lifecycle-aware path.
// A missing or already archived task counts as done, so a task a person
// archived is never deleted.
func (s *Service) DeleteAutomationPRTask(ctx context.Context, taskID, reason string) error {
	if s.prTaskLifecycle == nil {
		return errors.New("task lifecycle is unavailable")
	}
	task, err := s.loadPRTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return err
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
	task, err := s.loadPRTask(ctx, taskID)
	if err != nil {
		return nil, nil, automation.ThreadActionResumed, "", fmt.Errorf("load PR task: %w", err)
	}
	if task == nil {
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
		return nil, nil, automation.ThreadActionResumed, "", fmt.Errorf("load PR task session: %w", err)
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
// automation's continuation step. The move skips the step's own prompt, so the
// automation's follow-up is the only turn the task receives.
func (s *Service) movePRTaskToContinuationStep(ctx context.Context, a *automation.Automation, task *models.Task) error {
	if a.ContinuationPolicy != automation.ContinuationPolicyReusePerPR || a.PRContinuationStepID == "" ||
		task.WorkflowStepID == a.PRContinuationStepID {
		return nil
	}
	if s.prTaskLifecycle == nil {
		return errors.New("continuation step unavailable: task lifecycle is unavailable")
	}
	res, err := s.prTaskLifecycle.MoveTaskWithOptions(ctx, task.ID, task.WorkflowID, a.PRContinuationStepID, 0,
		taskservice.MoveTaskOptions{EntryOptions: &workflowmove.EntryOptions{SkipStepPrompt: true}})
	if err != nil {
		return fmt.Errorf("continuation step unavailable: %w", err)
	}
	if res != nil && res.Task != nil && (!res.Task.WIPAdmitted || res.Task.QueuedForStepID != "") {
		return fmt.Errorf("continuation step unavailable: target step %q is full (WIP limit reached)", a.PRContinuationStepID)
	}
	s.logger.Info("moved PR task to its continuation step",
		zap.String("automation_id", a.ID), zap.String("task_id", task.ID),
		zap.String("step_id", a.PRContinuationStepID))
	task.WorkflowStepID = a.PRContinuationStepID

	// Synchronize with the move lifecycle so any on_enter actions (such as
	// context reset or profile switch) finish before the continuation prompt dispatches.
	if s.repo != nil {
		latest, err := s.repo.GetTask(ctx, task.ID)
		if err == nil && latest != nil && manualMoveLifecyclePending(latest) {
			s.recoverManualMoveLifecycle(ctx, latest)
		}
	}
	return nil
}
