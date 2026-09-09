package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	workflowSessionRoutePrepared  = "prepared"
	workflowSessionRouteCommitted = "committed"
)

type workflowSessionBindingStore interface {
	GetWorkflowSessionBinding(context.Context, string, string) (*models.WorkflowSessionBinding, error)
	UpsertWorkflowSessionBinding(context.Context, *models.WorkflowSessionBinding) (bool, error)
}

type workflowSessionTargetResolution struct {
	session   *models.TaskSession
	profileID string
}

func workflowSessionBindingTargetKey(stepID string) string {
	return "step:" + stepID
}

// resolveInitialWorkflowSession returns the exact immutable initial session
// and its original logical profile. A missing snapshot is resolved through
// originalTaskSession, which conservatively backfills older tasks only when
// provenance is unambiguous.
func (s *Service) resolveInitialWorkflowSession(ctx context.Context, taskID string) (*models.TaskSession, string, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", fmt.Errorf("load task for initial session target: %w", err)
	}
	if task == nil {
		return nil, "", fmt.Errorf("task %s not found for initial session target", taskID)
	}
	initial, err := s.originalTaskSession(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	profileID := ""
	if snapshot, ok := models.LoadWorkflowInitialSessionSnapshot(task.Metadata); ok {
		profileID = snapshot.AgentProfileID
	}
	if profileID == "" && initial != nil {
		profileID = initial.AgentProfileID
	}
	if profileID == "" {
		profileID, _ = task.Metadata[models.MetaKeyAgentProfileID].(string)
	}
	return initial, profileID, nil
}

// resolveWorkflowSessionTarget resolves the exact recipient owned by a step.
// Profile-only lookup is intentionally not used here: explicit targets are
// either the immutable initial conversation or a source-step binding.
func (s *Service) resolveWorkflowSessionTarget(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
) (workflowSessionTargetResolution, error) {
	if step == nil || step.SessionTarget == nil {
		return workflowSessionTargetResolution{}, fmt.Errorf("workflow session target is required")
	}
	if err := wfmodels.ValidateWorkflowSessionTarget(step.SessionTarget); err != nil {
		return workflowSessionTargetResolution{}, err
	}
	if step.SessionTarget.Kind == wfmodels.WorkflowSessionTargetInitial {
		session, profileID, err := s.resolveInitialWorkflowSession(ctx, taskID)
		if err != nil {
			return workflowSessionTargetResolution{}, err
		}
		return workflowSessionTargetResolution{session: session, profileID: profileID}, nil
	}

	return s.resolveSourceWorkflowSessionTarget(ctx, taskID, step)
}

func (s *Service) resolveSourceWorkflowSessionTarget(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
) (workflowSessionTargetResolution, error) {
	sourceStep, err := s.loadWorkflowStepForLifecycle(ctx, step.SessionTarget.StepID, "session target source")
	if err != nil {
		return workflowSessionTargetResolution{}, err
	}
	if err := validateWorkflowSessionTargetSource(step, sourceStep); err != nil {
		return workflowSessionTargetResolution{}, err
	}
	session, err := s.resolveBoundSourceWorkflowSession(ctx, taskID, step.WorkflowID, sourceStep)
	if err != nil {
		return workflowSessionTargetResolution{}, err
	}
	return workflowSessionTargetResolution{session: session, profileID: sourceStep.AgentProfileID}, nil
}

func validateWorkflowSessionTargetSource(destination, source *wfmodels.WorkflowStep) error {
	if source.WorkflowID != destination.WorkflowID {
		return fmt.Errorf("session target source step %q belongs to another workflow", source.ID)
	}
	if source.Position >= destination.Position {
		return fmt.Errorf("session target source step %q is not earlier than destination step %q", source.ID, destination.ID)
	}
	if source.AgentProfileID == "" || source.SessionTarget != nil {
		return fmt.Errorf("session target source step %q must use a direct agent profile", source.ID)
	}
	return nil
}

func (s *Service) resolveBoundSourceWorkflowSession(
	ctx context.Context,
	taskID string,
	workflowID string,
	sourceStep *wfmodels.WorkflowStep,
) (*models.TaskSession, error) {
	store, ok := s.repo.(workflowSessionBindingStore)
	if !ok {
		return nil, nil
	}
	binding, err := store.GetWorkflowSessionBinding(ctx, taskID, workflowSessionBindingTargetKey(sourceStep.ID))
	if err != nil {
		return nil, err
	}
	if binding == nil || binding.TaskID != taskID || binding.WorkflowID != workflowID ||
		binding.AgentProfileID != sourceStep.AgentProfileID || binding.SessionID == "" {
		return nil, nil
	}
	session, err := s.repo.GetTaskSession(ctx, binding.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load session target binding %q: %w", sourceStep.ID, err)
	}
	if session == nil || session.TaskID != taskID || session.AgentProfileID != sourceStep.AgentProfileID {
		return nil, nil
	}
	return session, nil
}

func (s *Service) persistWorkflowSessionRoute(ctx context.Context, taskID string, route models.WorkflowSessionRoute) {
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		return
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyWorkflowSessionRoute, route); err != nil {
		s.logger.Warn("failed to persist workflow session route",
			zap.String("task_id", taskID),
			zap.String("operation_id", route.OperationID),
			zap.String("phase", route.Phase),
			zap.Error(err),
		)
	}
}

func workflowSessionRouteID(taskID, stepID, currentSessionID string, target *wfmodels.WorkflowSessionTarget, startPolicy models.WorkflowProfileSessionStartPolicy) string {
	targetID := ""
	targetKind := ""
	if target != nil {
		targetKind = string(target.Kind)
		targetID = target.StepID
	}
	return fmt.Sprintf("workflow-session:%s:%s:%s:%s:%s:%s", taskID, stepID, currentSessionID, targetKind, targetID, startPolicy)
}

func (s *Service) prepareExplicitWorkflowSession(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	step *wfmodels.WorkflowStep,
	sourceStep *wfmodels.WorkflowStep,
) (*models.TaskSession, bool, error) {
	resolution, err := s.resolveWorkflowSessionTarget(ctx, taskID, step)
	if err != nil {
		return nil, false, err
	}
	targetSession, targetProfile := resolution.session, resolution.profileID
	if targetProfile == "" {
		return nil, false, fmt.Errorf("workflow session target has no logical agent profile")
	}
	startPolicy := s.resolveStepProfileSessionStartPolicy(step)
	endPolicy := s.resolveStepProfileSessionEndPolicy(sourceStep)
	operationID := workflowSessionRouteID(taskID, step.ID, currentSession.ID, step.SessionTarget, startPolicy)
	baseRoute := models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: step.ID,
		TargetKind:        string(step.SessionTarget.Kind),
		AgentProfileID:    targetProfile,
		SourceSessionID:   currentSession.ID,
	}

	if startPolicy == models.WorkflowProfileSessionStartPolicyReuse && targetSession != nil && !isTerminalSessionState(targetSession.State) {
		if targetSession.ID == currentSession.ID {
			return currentSession, false, nil
		}
		baseRoute.DestinationID = targetSession.ID
		baseRoute.Phase = workflowSessionRoutePrepared
		s.persistWorkflowSessionRoute(ctx, taskID, baseRoute)
		if err := s.preflightWorkflowSessionTarget(ctx, taskID, targetSession); err != nil {
			return nil, false, err
		}
		reused, reuseErr := s.reuseSessionForStepWithEndPolicy(ctx, taskID, currentSession, targetSession, endPolicy)
		if reuseErr == nil {
			baseRoute.Phase = workflowSessionRouteCommitted
			s.persistWorkflowSessionRoute(ctx, taskID, baseRoute)
			return reused, true, nil
		}
		if !errors.Is(reuseErr, errReusableSessionNoLongerActive) {
			return nil, false, reuseErr
		}
	}

	baseRoute.Phase = workflowSessionRoutePrepared
	s.persistWorkflowSessionRoute(ctx, taskID, baseRoute)
	newSession, err := s.createNewSessionForStepWithEndPolicy(ctx, taskID, currentSession, targetProfile, endPolicy)
	if err != nil {
		return nil, false, err
	}
	baseRoute.DestinationID = newSession.ID
	baseRoute.Phase = workflowSessionRouteCommitted
	s.persistWorkflowSessionRoute(ctx, taskID, baseRoute)
	return newSession, true, nil
}

func (s *Service) recordWorkflowSourceBinding(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
	session *models.TaskSession,
) error {
	if step == nil || session == nil || step.AgentProfileID == "" || step.SessionTarget != nil {
		return nil
	}
	store, ok := s.repo.(workflowSessionBindingStore)
	if !ok {
		return nil
	}
	operationID := fmt.Sprintf("workflow-step-entry:%s:%s:%s:%d", taskID, step.ID, session.ID, time.Now().UTC().UnixNano())
	accepted, err := store.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      workflowSessionBindingTargetKey(step.ID),
		WorkflowID:     step.WorkflowID,
		AgentProfileID: step.AgentProfileID,
		SessionID:      session.ID,
		OperationID:    operationID,
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		s.logger.Error("failed to persist workflow source session binding",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.String("session_id", session.ID),
			zap.Error(err),
		)
		return fmt.Errorf("persist workflow source session binding: %w", err)
	}
	if !accepted {
		return fmt.Errorf("workflow source session binding was superseded for step %q", step.ID)
	}
	return nil
}

func (s *Service) preflightWorkflowSessionTarget(ctx context.Context, taskID string, target *models.TaskSession) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task for explicit session target: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found for explicit session target", taskID)
	}
	return s.executor.PreflightManagedGitCredentials(ctx, task.WorkspaceID, taskID, target.ExecutorID, target.ExecutorProfileID)
}

// selectExplicitWorkflowStartSession resolves a target for a launch that has
// no current primary session. It returns a reusable exact session when the
// destination asks for reuse; otherwise the caller creates a fresh session
// from profileID.
func (s *Service) selectExplicitWorkflowStartSession(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
) (*models.TaskSession, string, error) {
	resolution, err := s.resolveWorkflowSessionTarget(ctx, taskID, step)
	if err != nil {
		return nil, "", err
	}
	if resolution.profileID == "" {
		return nil, "", fmt.Errorf("workflow session target has no logical agent profile")
	}
	if s.resolveStepProfileSessionStartPolicy(step) != models.WorkflowProfileSessionStartPolicyReuse || resolution.session == nil {
		return nil, resolution.profileID, nil
	}
	if isTerminalSessionState(resolution.session.State) {
		return nil, resolution.profileID, nil
	}
	return resolution.session, resolution.profileID, nil
}
