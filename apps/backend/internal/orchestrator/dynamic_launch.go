package orchestrator

import (
	"context"
	"errors"
	"fmt"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const dynamicRouteStatusWaiting = "waiting"

// dynamicTaskDownstream adapts the task executor to the provider-neutral
// conductor. The callback updates the task-session attribution before every
// concrete launch, including a cross-profile fallback.
type dynamicTaskDownstream struct {
	service   *Service
	task      *v1.Task
	sessionID string
	options   executor.LaunchOptions
	execution *executor.TaskExecution
}

func (d *dynamicTaskDownstream) Launch(
	ctx context.Context,
	launch dynamicruntime.DownstreamLaunch,
) (dynamicruntime.DownstreamExecution, error) {
	if err := d.service.persistDynamicLaunchDecision(ctx, d.sessionID, launch.Decision); err != nil {
		return dynamicruntime.DownstreamExecution{}, err
	}
	options := d.options
	options.AgentProfileID = launch.ExecutionProfileID
	options.Prompt = launch.Prompt
	execution, err := d.service.executor.LaunchPreparedSession(ctx, d.task, d.sessionID, options)
	if err != nil {
		var classified *routingerr.Error
		if errors.As(err, &classified) {
			return dynamicruntime.DownstreamExecution{}, err
		}
		classified = routingerr.Classify(routingerr.Input{
			Phase:      routingerr.PhaseProcessStart,
			ProviderID: launch.ExecutionProfileID,
			Stderr:     err.Error(),
		})
		// Unknown low-confidence launch failures are workspace/runtime errors,
		// not provider failures. Let the ordinary launch recovery own them.
		if classified.Confidence == routingerr.ConfLow {
			return dynamicruntime.DownstreamExecution{}, err
		}
		return dynamicruntime.DownstreamExecution{}, fmt.Errorf("%w: %v", classified, err)
	}
	d.execution = execution
	return dynamicruntime.DownstreamExecution{
		ID:                 execution.AgentExecutionID,
		ExecutionProfileID: launch.ExecutionProfileID,
		ACPSessionID:       "",
	}, nil
}

func (d *dynamicTaskDownstream) Resume(context.Context, string, string) error {
	return errors.New("dynamic task downstream resume is owned by the orchestrator")
}

func (d *dynamicTaskDownstream) Stop(context.Context, string, string) error {
	return nil
}

func (s *Service) persistDynamicLaunchDecision(
	ctx context.Context,
	sessionID string,
	decision dynamicruntime.RouteDecision,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.ExecutionProfileID == decision.ExecutionProfileID &&
		session.RouteGeneration == decision.Generation {
		return nil
	}
	previousExecutionProfileID := session.ExecutionProfileID
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = "starting"
	session.RouteReason = decision.Reason
	// The executor marks a failed provider launch terminal before the
	// conductor can claim the next candidate. Re-open the logical session for
	// that immediate retry so the second launch can persist STARTING state.
	if session.State == models.TaskSessionStateFailed {
		session.State = models.TaskSessionStateCreated
		session.ErrorMessage = ""
	}
	if previousExecutionProfileID != decision.ExecutionProfileID {
		session.DownstreamACPSessionID = ""
	}
	return s.repo.UpdateTaskSession(ctx, session)
}

// launchPreparedSessionWithDynamicFallback keeps the logical session stable
// while delegating classified pre-result provider fallback to dynamic.Conductor.
// Concrete profiles continue through the ordinary executor path.
func (s *Service) launchPreparedSessionWithDynamicFallback(
	ctx context.Context,
	task *v1.Task,
	sessionID string,
	options executor.LaunchOptions,
) (*executor.TaskExecution, error) {
	if s.profileExecutionResolver == nil {
		return s.executor.LaunchPreparedSession(ctx, task, sessionID, options)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	decision, dynamic, err := s.dynamicLaunchDecision(ctx, session)
	if err != nil {
		return nil, err
	}
	if !dynamic {
		return s.executor.LaunchPreparedSession(ctx, task, sessionID, options)
	}
	downstream := &dynamicTaskDownstream{
		service: s, task: task, sessionID: sessionID, options: options,
	}
	conductor := s.profileExecutionResolver.NewConductor(downstream)
	result, err := conductor.LaunchSelected(ctx, dynamicruntime.ConductorSelectedLaunch{
		SessionID:        session.ID,
		LogicalProfileID: session.AgentProfileID,
		Decision:         decision,
		Prompt:           options.Prompt,
	})
	if err != nil {
		return nil, err
	}
	if downstream.execution == nil {
		return nil, errors.New("dynamic conductor returned no task execution")
	}
	if result.Execution.ExecutionProfileID == "" {
		result.Execution.ExecutionProfileID = session.ExecutionProfileID
	}
	return downstream.execution, nil
}

func (s *Service) dynamicLaunchDecision(
	ctx context.Context,
	session *models.TaskSession,
) (dynamicruntime.RouteDecision, bool, error) {
	decision := dynamicruntime.RouteDecision{
		SessionID:          session.ID,
		LogicalProfileID:   session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		Generation:         session.RouteGeneration,
		Reason:             session.RouteReason,
	}
	dynamic := decision.Generation > 0 && decision.ExecutionProfileID != ""
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return decision, dynamic, nil
	}
	state, err := loader.LoadRouteState(ctx, session.ID)
	if err != nil {
		return dynamicruntime.RouteDecision{}, false, err
	}
	if state == nil || state.LogicalProfileID != session.AgentProfileID || state.Generation <= 0 {
		return decision, dynamic, nil
	}
	if state.ExecutionProfileID == "" {
		if state.Status != dynamicRouteStatusWaiting {
			return dynamicruntime.RouteDecision{}, false, &dynamicruntime.NoEligibleCandidateError{
				SessionID: session.ID, LogicalProfile: session.AgentProfileID,
				Generation: state.Generation,
			}
		}
		resolved, err := s.profileExecutionResolver.Resolve(
			ctx, session.ID, session.AgentProfileID, state.Generation, "",
		)
		if err != nil {
			return dynamicruntime.RouteDecision{}, false, err
		}
		decision = resolved.Decision
		if err := s.persistDynamicLaunchDecision(ctx, session.ID, decision); err != nil {
			return dynamicruntime.RouteDecision{}, false, fmt.Errorf("persist dynamic route attribution: %w", err)
		}
		return decision, true, nil
	}
	resolved, err := s.profileExecutionResolver.ResolveExisting(
		ctx, session.ID, session.AgentProfileID, state.ExecutionProfileID,
		state.Generation, state.ProfileVersion, "durable_route_state",
	)
	if err != nil {
		return dynamicruntime.RouteDecision{}, false, err
	}
	if session.ExecutionProfileID == resolved.ExecutionProfileID &&
		session.RouteGeneration == resolved.Generation {
		return resolved.Decision, true, nil
	}
	applyResolvedExecution(session, resolved)
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		return dynamicruntime.RouteDecision{}, false, fmt.Errorf("persist dynamic route attribution: %w", err)
	}
	return resolved.Decision, true, nil
}

// routeDynamicAgentFailure applies the configured action for a classified
// task failure. It is intentionally limited to dynamic sessions and provider
// errors that explicitly allow fallback; user/runtime errors keep the normal
// recovery surface.
func (s *Service) routeDynamicAgentFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
) bool {
	if classified == nil || !classified.FallbackAllowed {
		return false
	}
	session, ok := s.dynamicFailureSession(ctx, data)
	if !ok {
		return false
	}
	conductor := s.profileExecutionResolver.NewConductor(nil)
	decision, err := conductor.RouteAfterFailure(
		ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
		session.RouteGeneration, classified,
	)
	if err != nil {
		return false
	}
	next, err := s.profileExecutionResolver.ResolveExisting(
		ctx, session.ID, session.AgentProfileID, decision.ExecutionProfileID,
		decision.Generation, decision.ProfileVersion, decision.Reason,
	)
	if err != nil || next.ExecutionProfileID == "" {
		return false
	}
	if err := s.persistDynamicLaunchDecision(ctx, session.ID, next.Decision); err != nil {
		s.logger.Warn("failed to persist dynamic fallback attribution",
			zap.String("session_id", session.ID), zap.Error(err))
		return false
	}
	return s.relaunchDynamicTaskAfterFailure(ctx, data, next.ExecutionProfileID)
}

func (s *Service) dynamicFailureSession(
	ctx context.Context,
	data watcher.AgentEventData,
) (*models.TaskSession, bool) {
	if s.profileExecutionResolver == nil || data.SessionID == "" {
		return nil, false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return nil, false
	}
	if session.AgentExecutionID != "" && data.AgentExecutionID != "" &&
		session.AgentExecutionID != data.AgentExecutionID {
		return nil, false
	}
	return session, true
}

func (s *Service) relaunchDynamicTaskAfterFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
) bool {
	v, ok := s.lastTurnPrompt.Load(data.SessionID)
	if !ok {
		return false
	}
	prompt, ok := v.(capturedPrompt)
	if !ok {
		return false
	}
	task, err := s.scheduler.GetTask(ctx, data.TaskID)
	if err != nil {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		return false
	}
	if err := s.executor.StopExecution(ctx, data.AgentExecutionID, "dynamic route fallback", true); err != nil {
		s.logger.Debug("failed to stop dynamic fallback predecessor", zap.Error(err))
	}
	if err := s.repo.UpdateTaskSessionState(ctx, data.SessionID, models.TaskSessionStateCreated, ""); err != nil {
		return false
	}
	s.completeTurnForSession(ctx, data.SessionID)
	s.retireExecutionActivityAndPublish(ctx, data.TaskID, data.SessionID, data.AgentExecutionID)
	isOfficeTask, officeErr := s.lookupOfficeTask(ctx, data.TaskID)
	if officeErr == nil && !isOfficeTask {
		_, err := s.StartCreatedSession(
			ctx, data.TaskID, data.SessionID, session.AgentProfileID,
			prompt.text, true, prompt.planMode, true, prompt.attachments, nil,
		)
		return err == nil
	}
	_, err = s.launchPreparedSessionWithDynamicFallback(ctx, task, data.SessionID, executor.LaunchOptions{
		AgentProfileID:       executionProfileID,
		OfficeAgentProfileID: session.AgentProfileID,
		ExecutorID:           "",
		Prompt:               prompt.text,
		StartAgent:           true,
		McpMode:              executor.McpModeOffice,
	})
	return err == nil
}
