package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/planinjection"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
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
	options.PriorACPSession = launch.PriorACPSession
	d.service.beginDynamicAttempt(d.sessionID)
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
	d.service.bindDynamicAttemptExecution(d.sessionID, execution.AgentExecutionID)
	d.service.bindPromptAttemptToExecution(ctx, d.sessionID, execution.AgentExecutionID)
	d.execution = execution
	acpSessionID := ""
	if session, sessionErr := d.service.repo.GetTaskSession(ctx, d.sessionID); sessionErr == nil && session != nil {
		acpSessionID = session.DownstreamACPSessionID
	}
	if acpSessionID != "" {
		if err := d.service.persistDynamicACPSession(ctx, d.sessionID, launch.Decision, acpSessionID); err != nil {
			return dynamicruntime.DownstreamExecution{}, err
		}
	}
	return dynamicruntime.DownstreamExecution{
		ID:                 execution.AgentExecutionID,
		ExecutionProfileID: launch.ExecutionProfileID,
		ACPSessionID:       acpSessionID,
	}, nil
}

func (d *dynamicTaskDownstream) LoadExecution(
	ctx context.Context,
	sessionID string,
) (dynamicruntime.DownstreamExecution, bool, error) {
	if sessionID == "" {
		return dynamicruntime.DownstreamExecution{}, false, nil
	}
	session, err := d.service.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return dynamicruntime.DownstreamExecution{}, false, err
	}
	if session == nil || session.AgentExecutionID == "" {
		return dynamicruntime.DownstreamExecution{}, false, nil
	}
	return dynamicruntime.DownstreamExecution{
		ID:                 session.AgentExecutionID,
		ExecutionProfileID: session.ExecutionProfileID,
		ACPSessionID:       session.DownstreamACPSessionID,
	}, true, nil
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
		session.RouteGeneration == decision.Generation &&
		decision.Status == "" {
		return nil
	}
	previousExecutionProfileID := session.ExecutionProfileID
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = decision.Status
	if session.RouteState == "" {
		session.RouteState = "starting"
	}
	session.RouteReason = decision.Reason
	applyDynamicRouteDecisionProjection(session, decision)
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

func (s *Service) persistDynamicACPSession(
	ctx context.Context,
	sessionID string,
	decision dynamicruntime.RouteDecision,
	acpSessionID string,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.RouteGeneration != decision.Generation ||
		session.ExecutionProfileID != decision.ExecutionProfileID {
		return dynamicruntime.ErrStaleGeneration
	}
	if session.DownstreamACPSessionID == acpSessionID {
		return nil
	}
	session.DownstreamACPSessionID = acpSessionID
	return s.repo.UpdateTaskSession(ctx, session)
}

func applyDynamicRouteDecisionProjection(
	session *models.TaskSession,
	decision dynamicruntime.RouteDecision,
) {
	if session == nil {
		return
	}
	session.RouteErrorCode = string(decision.ErrorCode)
	session.RouteErrorClass = string(decision.ErrorClass)
	session.RouteCatalogueVersion = decision.CatalogueVersion
	session.RouteRetryOrdinal = decision.RetryOrdinal
	session.RoutePendingOutcome = string(decision.PendingOutcome)
	session.RouteDeadline = nil
	if decision.Deadline != nil {
		deadline := decision.Deadline.UTC()
		session.RouteDeadline = &deadline
	}
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
	return s.launchPreparedSessionWithDynamicFallbackWithContinuation(ctx, task, sessionID, options, nil)
}

func (s *Service) launchPreparedSessionWithDynamicFallbackWithContinuation(
	ctx context.Context,
	task *v1.Task,
	sessionID string,
	options executor.LaunchOptions,
	continuationInput *dynamicruntime.ContinuationInput,
) (*executor.TaskExecution, error) {
	if s.profileExecutionResolver == nil {
		return s.launchConcretePreparedSession(ctx, task, sessionID, options)
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
		return s.launchConcretePreparedSession(ctx, task, sessionID, options)
	}
	downstream := &dynamicTaskDownstream{
		service: s, task: task, sessionID: sessionID, options: options,
	}
	conductor := s.profileExecutionResolver.NewConductor(downstream)
	prebuiltContinuation, continuationInput, err := s.dynamicContinuationForLaunch(
		ctx, task, sessionID, options.Prompt, continuationInput,
	)
	if err != nil {
		return nil, err
	}
	selected := dynamicruntime.ConductorSelectedLaunch{
		SessionID: session.ID, LogicalProfileID: session.AgentProfileID,
		Decision: decision, Prompt: options.Prompt,
		PriorACPSession: session.DownstreamACPSessionID,
	}
	if prebuiltContinuation != nil {
		selected.PrebuiltContinuation = prebuiltContinuation
	} else {
		selected.Continuation = *continuationInput
	}
	result, err := conductor.LaunchSelected(ctx, selected)
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

func (s *Service) launchConcretePreparedSession(
	ctx context.Context,
	task *v1.Task,
	sessionID string,
	options executor.LaunchOptions,
) (*executor.TaskExecution, error) {
	if options.StartAgent && (options.Prompt != "" || len(options.Attachments) > 0) {
		s.beginInitialPromptAttempt(sessionID, false)
	}
	execution, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, options)
	if execution != nil {
		s.bindPromptAttemptToExecution(ctx, sessionID, execution.AgentExecutionID)
	}
	return execution, err
}

func (s *Service) dynamicContinuationForLaunch(
	ctx context.Context,
	task *v1.Task,
	sessionID, prompt string,
	continuationInput *dynamicruntime.ContinuationInput,
) (*dynamicruntime.Continuation, *dynamicruntime.ContinuationInput, error) {
	if loader, ok := s.repo.(dynamicRouteStateLoader); ok {
		state, err := loader.LoadRouteState(ctx, sessionID)
		if err != nil {
			return nil, nil, err
		}
		if state != nil && state.ContinuationJSON != "" {
			var continuation dynamicruntime.Continuation
			if err := json.Unmarshal([]byte(state.ContinuationJSON), &continuation); err != nil {
				return nil, nil, fmt.Errorf("decode dynamic continuation: %w", err)
			}
			return &continuation, nil, nil
		}
	}
	if continuationInput != nil {
		return nil, continuationInput, nil
	}
	input, err := s.buildDynamicContinuation(ctx, task, sessionID, prompt, "")
	if err != nil {
		return nil, nil, err
	}
	return nil, &input, nil
}

func (s *Service) buildDynamicContinuation(
	ctx context.Context,
	task *v1.Task,
	sessionID, prompt, failureReason string,
) (dynamicruntime.ContinuationInput, error) {
	if task == nil || sessionID == "" {
		return dynamicruntime.ContinuationInput{}, errors.New("dynamic continuation requires task and session")
	}
	input := dynamicruntime.ContinuationInput{
		TaskDescription: task.Description,
		FailureReason:   failureReason,
	}
	if strings.TrimSpace(prompt) != "" {
		input.UserMessages = append(input.UserMessages, prompt)
	}
	if err := s.addDynamicTaskMetadata(ctx, task, &input); err != nil {
		return dynamicruntime.ContinuationInput{}, err
	}
	if err := s.addDynamicConversation(ctx, sessionID, &input); err != nil {
		return dynamicruntime.ContinuationInput{}, err
	}
	if err := s.addDynamicPlan(ctx, task.ID, &input); err != nil {
		return dynamicruntime.ContinuationInput{}, err
	}
	input.RepositorySummary = dynamicRepositorySummary(task)
	return input, nil
}

func (s *Service) addDynamicTaskMetadata(ctx context.Context, task *v1.Task, input *dynamicruntime.ContinuationInput) error {
	dbTask, err := s.repo.GetTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("load task for dynamic continuation: %w", err)
	}
	if dbTask != nil {
		input.WorkflowStep = dbTask.WorkflowStepID
	}
	return nil
}

func (s *Service) addDynamicConversation(ctx context.Context, sessionID string, input *dynamicruntime.ContinuationInput) error {
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load conversation for dynamic continuation: %w", err)
	}
	conversation := make([]string, 0, len(messages))
	toolSummary := make([]string, 0)
	for _, message := range messages {
		if message == nil || strings.TrimSpace(message.Content) == "" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if message.AuthorType == models.MessageAuthorUser {
			input.UserMessages = append(input.UserMessages, content)
			continue
		}
		switch message.Type {
		case models.MessageTypeToolCall, models.MessageTypeToolEdit,
			models.MessageTypeToolRead, models.MessageTypeToolExecute:
			toolSummary = append(toolSummary, string(message.Type)+": "+content)
		default:
			conversation = append(conversation, string(message.AuthorType)+": "+content)
		}
	}
	input.Conversation = strings.Join(conversation, "\n")
	input.ToolSummary = strings.Join(toolSummary, "\n")
	return nil
}

func (s *Service) addDynamicPlan(ctx context.Context, taskID string, input *dynamicruntime.ContinuationInput) error {
	plan, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load plan for dynamic continuation: %w", err)
	}
	if plan == nil {
		return nil
	}

	composed := strings.TrimSpace(plan.Title + "\n" + plan.Content)
	// ContainTags is not called here: PlanSummary lands in the plain-text
	// continuation package (ContinuationPrompt), not in a <kandev-system>
	// block. If that ever changes, add ContainTags(composed) before Reduce.
	reducedPlan, reduced, omitted := planinjection.Reduce(composed, planinjection.DynamicBudget)
	input.PlanSummary = reducedPlan
	if reduced {
		s.logger.Info("reducing dynamic continuation plan",
			zap.String("site", "dynamic_continuation"),
			zap.String("task_id", taskID),
			zap.Int("plan_input_bytes", len(composed)),
			zap.Int("plan_output_bytes", len(reducedPlan)),
			zap.Int("plan_sections_omitted", omitted),
		)
	}
	return nil
}

func dynamicRepositorySummary(task *v1.Task) string {
	repositories := make([]string, 0, len(task.Repositories)+len(task.WorkspaceFolders))
	for _, repository := range task.Repositories {
		repositories = append(repositories, repository.RepositoryID+" @ "+repository.BaseBranch)
	}
	for _, folder := range task.WorkspaceFolders {
		repositories = append(repositories, "folder: "+folder.DisplayName+" @ "+folder.LocalPath)
	}
	return strings.Join(repositories, "\n")
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
	data = s.withDynamicAttemptEvidence(data)
	session, ok := s.dynamicFailureSession(ctx, data)
	if !ok {
		return false
	}
	if !dynamicPreResultSafe(data) {
		// A dynamic route is never allowed to guess that a failed turn was
		// pre-result. Missing evidence is as unsafe as observed output or a
		// tool effect because the replacement could repeat a side effect.
		return false
	}
	conductor := s.profileExecutionResolver.NewConductor(nil)
	task, err := s.scheduler.GetTask(ctx, data.TaskID)
	if err != nil {
		return false
	}
	continuationInput, err := s.buildDynamicContinuation(
		ctx, task, session.ID, "", classified.Error(),
	)
	if err != nil {
		return false
	}
	decision, err := conductor.RouteAfterFailure(
		ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
		session.RouteGeneration, classified,
	)
	if err != nil {
		return s.persistPendingDynamicRecovery(ctx, session, decision, err)
	}
	return s.launchDynamicSuccessorAfterFailure(ctx, data, session, conductor, decision, continuationInput)
}

func (s *Service) persistPendingDynamicRecovery(
	ctx context.Context,
	session *models.TaskSession,
	decision dynamicruntime.RouteDecision,
	err error,
) bool {
	if !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		return false
	}
	// The evaluator durably owns the retry/reset deadline. Keep the logical
	// session available for the authoritative recovery surface; no successor
	// launch is allowed until a due/manual action claims the same generation.
	session.RouteState = decision.Status
	session.RouteReason = decision.Reason
	applyDynamicRouteDecisionProjection(session, decision)
	session.State = models.TaskSessionStateWaitingForInput
	session.DownstreamACPSessionID = ""
	if updateErr := s.repo.UpdateTaskSession(ctx, session); updateErr != nil {
		s.logger.Warn("failed to persist pending dynamic recovery",
			zap.String("session_id", session.ID), zap.Error(updateErr))
		return false
	}
	s.schedulePersistedDynamicRecovery(ctx, session.ID, decision.Generation)
	return true
}

func (s *Service) schedulePersistedDynamicRecovery(ctx context.Context, sessionID string, generation int64) {
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return
	}
	state, err := loader.LoadRouteState(ctx, sessionID)
	if err != nil || state == nil {
		return
	}
	s.scheduleDynamicPolicyRecovery(sessionID, generation, state.PolicyStateJSON)
}

func (s *Service) launchDynamicSuccessorAfterFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	conductor *dynamicruntime.Conductor,
	decision dynamicruntime.RouteDecision,
	continuationInput dynamicruntime.ContinuationInput,
) bool {
	continuation, err := conductor.BuildContinuation(ctx, continuationInput)
	if err != nil {
		return false
	}
	if err := conductor.PersistContinuation(ctx, decision, continuation); err != nil {
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

// LaunchDynamicRouteAction completes a manual retry/try-next operation after
// the backend route-action handler has claimed the successor generation. It
// owns the predecessor shutdown, continuation persistence, successor launch,
// and final launch error surface so the UI never has to issue a second launch
// request.
func (s *Service) LaunchDynamicRouteAction(ctx context.Context, sessionID string) error {
	if s.profileExecutionResolver == nil || sessionID == "" {
		return errors.New("dynamic route action launch is not configured")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		if err != nil {
			return err
		}
		return errors.New("dynamic route action session not found")
	}
	task, err := s.scheduler.GetTask(ctx, session.TaskID)
	if err != nil {
		return err
	}
	input, err := s.buildDynamicContinuation(ctx, task, sessionID, "", "manual dynamic route action")
	if err != nil {
		return err
	}
	conductor := s.profileExecutionResolver.NewConductor(nil)
	continuation, err := conductor.BuildContinuation(ctx, input)
	if err != nil {
		return err
	}
	decision := dynamicruntime.RouteDecision{
		SessionID:          session.ID,
		LogicalProfileID:   session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		Generation:         session.RouteGeneration,
		Reason:             session.RouteReason,
	}
	if err := conductor.PersistContinuation(ctx, decision, continuation); err != nil {
		return err
	}
	data := watcher.AgentEventData{
		TaskID:             task.ID,
		SessionID:          session.ID,
		AgentExecutionID:   session.AgentExecutionID,
		AgentProfileID:     session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		ErrorMessage:       "manual dynamic route action",
	}
	if !s.relaunchDynamicTaskAfterFailure(ctx, data, session.ExecutionProfileID) {
		return errors.New("dynamic route action successor launch failed")
	}
	return nil
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
	prompt, ok := s.dynamicRelaunchPrompt(ctx, data.SessionID)
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
		if errors.Is(err, agentruntime.ErrNotFound) {
			s.logger.Debug("dynamic fallback predecessor is already absent", zap.Error(err))
		} else {
			s.logger.Debug("failed to stop dynamic fallback predecessor", zap.Error(err))
			return false
		}
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

// dynamicRelaunchPrompt prefers the in-memory prompt cache for an automatic
// fallback, but reconstructs the latest durable user prompt for a manual route
// action or a backend restart. A route action must not strand a claimed route
// merely because the process-local retry cache was lost.
func (s *Service) dynamicRelaunchPrompt(ctx context.Context, sessionID string) (capturedPrompt, bool) {
	if value, ok := s.lastTurnPrompt.Load(sessionID); ok {
		if prompt, ok := value.(capturedPrompt); ok && strings.TrimSpace(prompt.text) != "" {
			return prompt, true
		}
	}
	messages, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return capturedPrompt{}, false
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.AuthorType != models.MessageAuthorUser ||
			strings.TrimSpace(message.Content) == "" {
			continue
		}
		return capturedPrompt{text: message.Content}, true
	}
	return capturedPrompt{}, false
}
