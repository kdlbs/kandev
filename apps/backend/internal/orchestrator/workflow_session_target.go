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

type workflowSessionRouteCreator interface {
	CreateTaskSessionWithWorkflowSessionRoute(context.Context, *models.TaskSession, *models.WorkflowSessionRoute) error
}

type workflowSessionRoutePromoter interface {
	SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(context.Context, string, models.WorkflowSessionRoute) (bool, error)
}

type workflowStepTransitionReader interface {
	GetLatestTaskStepTransitionID(context.Context, string) (int64, error)
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
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, "", fmt.Errorf("list sessions for initial session target: %w", err)
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
	if profileID == "" && len(sessions) == 0 {
		profileID, _ = task.Metadata[models.MetaKeyAgentProfileID].(string)
	}
	if profileID == "" {
		return nil, "", fmt.Errorf("initial session provenance is unavailable for task %s", taskID)
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

func (s *Service) persistWorkflowSessionRoute(ctx context.Context, taskID string, route models.WorkflowSessionRoute) error {
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		return nil
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyWorkflowSessionRoute, route); err != nil {
		s.logger.Warn("failed to persist workflow session route",
			zap.String("task_id", taskID),
			zap.String("operation_id", route.OperationID),
			zap.String("phase", route.Phase),
			zap.Error(err),
		)
		return fmt.Errorf("persist workflow session route: %w", err)
	}
	return nil
}

func (s *Service) loadRecordedWorkflowSessionRoute(
	ctx context.Context,
	taskID string,
	operationID string,
	step *wfmodels.WorkflowStep,
	profileID string,
) (*models.WorkflowSessionRoute, *models.TaskSession, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("load task for workflow session route: %w", err)
	}
	if task == nil {
		return nil, nil, fmt.Errorf("task %s not found for workflow session route", taskID)
	}
	route, ok := models.LoadWorkflowSessionRoute(task.Metadata)
	if !ok || route.OperationID != operationID || !workflowSessionRouteMatchesStep(&route, step, profileID) {
		return nil, nil, nil
	}
	if route.DestinationID == "" ||
		(route.Phase != workflowSessionRoutePrepared && route.Phase != workflowSessionRouteCommitted) {
		return &route, nil, nil
	}
	session, err := s.repo.GetTaskSession(ctx, route.DestinationID)
	if err != nil {
		return nil, nil, fmt.Errorf("load recorded workflow session route destination: %w", err)
	}
	if session == nil || session.TaskID != taskID || session.AgentProfileID != profileID {
		return nil, nil, fmt.Errorf("recorded workflow session route destination %q is unavailable", route.DestinationID)
	}
	if isTerminalSessionState(session.State) {
		// A terminal conversation is a valid historical destination, but it
		// cannot satisfy reuse. Let the caller allocate the target profile's
		// fresh fallback and replace this bounded route record.
		return &route, nil, nil
	}
	return &route, session, nil
}

func (s *Service) supportsAtomicWorkflowSessionRouteCreation() bool {
	_, ok := s.repo.(workflowSessionRouteCreator)
	return ok
}

// promoteWorkflowSessionRoute combines destination promotion and route commit
// when the repository supports it. The compatibility path preserves the
// prepared route if a legacy adapter cannot provide the combined transaction.
func (s *Service) promoteWorkflowSessionRoute(
	ctx context.Context,
	taskID string,
	destination *models.TaskSession,
	route *models.WorkflowSessionRoute,
) (bool, error) {
	if destination == nil {
		return false, nil
	}
	if promoter, ok := s.repo.(workflowSessionRoutePromoter); ok && route != nil {
		committed := *route
		committed.DestinationID = destination.ID
		committed.Phase = workflowSessionRouteCommitted
		promoted, err := promoter.SetSessionPrimaryWithWorkflowSessionRouteIfNonterminal(ctx, destination.ID, committed)
		if err != nil || !promoted {
			return promoted, err
		}
		s.publishPrimarySessionUpdate(ctx, taskID, destination.ID)
		return true, nil
	}
	promoted, err := s.setNonterminalSessionPrimary(ctx, destination.ID)
	if err != nil || !promoted {
		return promoted, err
	}
	if route != nil {
		committed := *route
		committed.DestinationID = destination.ID
		committed.Phase = workflowSessionRouteCommitted
		if err := s.persistWorkflowSessionRoute(ctx, taskID, committed); err != nil {
			return false, err
		}
	}
	s.publishPrimarySessionUpdate(ctx, taskID, destination.ID)
	return true, nil
}

func workflowSessionRouteMatchesStep(route *models.WorkflowSessionRoute, step *wfmodels.WorkflowStep, profileID string) bool {
	if route == nil || step == nil || step.SessionTarget == nil {
		return false
	}
	return route.DestinationStepID == step.ID &&
		route.TargetKind == string(step.SessionTarget.Kind) &&
		route.TargetStepID == step.SessionTarget.StepID &&
		route.AgentProfileID == profileID
}

func (s *Service) reuseRecordedWorkflowSession(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	recordedRoute *models.WorkflowSessionRoute,
	recordedSession *models.TaskSession,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (*models.TaskSession, bool, error) {
	if recordedRoute.Phase == workflowSessionRouteCommitted {
		return recordedSession, recordedSession.ID != currentSession.ID, nil
	}
	if recordedSession.ID == currentSession.ID {
		preparedRoute := *recordedRoute
		preparedRoute.Phase = workflowSessionRoutePrepared
		promoted, err := s.promoteWorkflowSessionRoute(ctx, taskID, currentSession, &preparedRoute)
		if err != nil {
			return nil, false, err
		}
		if !promoted {
			return nil, false, errReusableSessionNoLongerActive
		}
		return currentSession, false, nil
	}

	preparedRoute := *recordedRoute
	preparedRoute.Phase = workflowSessionRoutePrepared
	if err := s.persistWorkflowSessionRoute(ctx, taskID, preparedRoute); err != nil {
		return nil, false, err
	}
	if err := s.preflightWorkflowSessionTarget(ctx, taskID, recordedSession); err != nil {
		return nil, false, err
	}
	reused, err := s.reuseSessionForStepWithEndPolicy(ctx, taskID, currentSession, recordedSession, endPolicy, &preparedRoute)
	if err == nil {
		return reused, true, nil
	}
	if !errors.Is(err, errReusableSessionNoLongerActive) {
		return nil, false, err
	}
	return nil, false, nil
}

func workflowSessionRouteID(taskID, stepID, entryIdentity string, target *wfmodels.WorkflowSessionTarget, startPolicy models.WorkflowProfileSessionStartPolicy) string {
	targetID := ""
	targetKind := ""
	if target != nil {
		targetKind = string(target.Kind)
		targetID = target.StepID
	}
	return fmt.Sprintf("workflow-session:%s:%s:%s:%s:%s:%s", taskID, stepID, entryIdentity, targetKind, targetID, startPolicy)
}

// workflowEntryIdentity is the durable identity of one workflow-step entry.
// Transition deliveries carry the immutable ledger id directly. Direct/manual
// starts use the task's latest transition, while creation entries fall back to
// the task creation stamp. The identity must not depend on the current primary
// session because a retry can legitimately reload a different primary.
func (s *Service) workflowEntryIdentity(ctx context.Context, taskID string, entryIDs ...int64) string {
	if len(entryIDs) > 0 && entryIDs[0] > 0 {
		return fmt.Sprintf("entry:%020d", entryIDs[0])
	}
	if reader, ok := s.repo.(workflowStepTransitionReader); ok {
		if transitionID, err := reader.GetLatestTaskStepTransitionID(ctx, taskID); err == nil && transitionID > 0 {
			return fmt.Sprintf("entry:%020d", transitionID)
		}
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err == nil && task != nil {
		if task.WorkflowStepTransitionID > 0 {
			return fmt.Sprintf("entry:%020d", task.WorkflowStepTransitionID)
		}
		if !task.CreatedAt.IsZero() {
			return "created:" + task.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if !task.UpdatedAt.IsZero() {
			return "legacy:" + task.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return "legacy:unknown"
}

func (s *Service) reuseResolvedWorkflowSession(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	targetSession *models.TaskSession,
	baseRoute *models.WorkflowSessionRoute,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (*models.TaskSession, bool, error) {
	if targetSession == nil || isTerminalSessionState(targetSession.State) {
		return nil, false, nil
	}
	if targetSession.ID == currentSession.ID {
		return currentSession, false, nil
	}

	baseRoute.DestinationID = targetSession.ID
	baseRoute.Phase = workflowSessionRoutePrepared
	if err := s.persistWorkflowSessionRoute(ctx, taskID, *baseRoute); err != nil {
		return nil, false, err
	}
	if err := s.preflightWorkflowSessionTarget(ctx, taskID, targetSession); err != nil {
		return nil, false, err
	}
	reused, err := s.reuseSessionForStepWithEndPolicy(ctx, taskID, currentSession, targetSession, endPolicy, baseRoute)
	if errors.Is(err, errReusableSessionNoLongerActive) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return reused, true, nil
}

func (s *Service) prepareExplicitWorkflowSession(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	step *wfmodels.WorkflowStep,
	sourceStep *wfmodels.WorkflowStep,
	entryIDs ...int64,
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
	entryIdentity := s.workflowEntryIdentity(ctx, taskID, entryIDs...)
	operationID := workflowSessionRouteID(taskID, step.ID, entryIdentity, step.SessionTarget, startPolicy)
	recordedRoute, recordedSession, err := s.loadRecordedWorkflowSessionRoute(ctx, taskID, operationID, step, targetProfile)
	if err != nil {
		return nil, false, err
	}
	if recordedSession != nil {
		reused, switched, err := s.reuseRecordedWorkflowSession(ctx, taskID, currentSession, recordedRoute, recordedSession, endPolicy)
		if err != nil || reused != nil {
			return reused, switched, err
		}
	}
	baseRoute := models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: step.ID,
		TargetKind:        string(step.SessionTarget.Kind),
		TargetStepID:      step.SessionTarget.StepID,
		AgentProfileID:    targetProfile,
		SourceSessionID:   currentSession.ID,
	}

	if startPolicy == models.WorkflowProfileSessionStartPolicyReuse {
		reused, switched, err := s.reuseResolvedWorkflowSession(ctx, taskID, currentSession, targetSession, &baseRoute, endPolicy)
		if err != nil || reused != nil {
			return reused, switched, err
		}
	}

	baseRoute.Phase = workflowSessionRoutePrepared
	if !s.supportsAtomicWorkflowSessionRouteCreation() {
		if err := s.persistWorkflowSessionRoute(ctx, taskID, baseRoute); err != nil {
			return nil, false, err
		}
	}
	newSession, err := s.createNewSessionForStepWithEndPolicyAndRoute(ctx, taskID, currentSession, targetProfile, endPolicy, &baseRoute)
	if err != nil {
		return nil, false, err
	}
	return newSession, true, nil
}

func (s *Service) recordWorkflowSourceBinding(
	ctx context.Context,
	taskID string,
	step *wfmodels.WorkflowStep,
	session *models.TaskSession,
	entryIDs ...int64,
) error {
	if step == nil || session == nil || step.AgentProfileID == "" || step.SessionTarget != nil {
		return nil
	}
	store, ok := s.repo.(workflowSessionBindingStore)
	if !ok {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task before workflow source binding: %w", err)
	}
	if task == nil || (task.WorkflowStepID != "" && task.WorkflowStepID != step.ID) {
		return nil
	}
	updatedAt := task.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	operationID := workflowSourceBindingOperationID(
		s.workflowEntryIdentity(ctx, taskID, entryIDs...),
	)
	accepted, err := store.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         taskID,
		TargetKey:      workflowSessionBindingTargetKey(step.ID),
		WorkflowID:     step.WorkflowID,
		AgentProfileID: step.AgentProfileID,
		SessionID:      session.ID,
		OperationID:    operationID,
		UpdatedAt:      updatedAt,
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
		s.logger.Debug("ignored superseded workflow source session binding",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.String("session_id", session.ID),
			zap.String("operation_id", operationID))
	}
	return nil
}

func workflowSourceBindingOperationID(entryIdentity string) string {
	return fmt.Sprintf("workflow-step-entry-v2:%s", entryIdentity)
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
