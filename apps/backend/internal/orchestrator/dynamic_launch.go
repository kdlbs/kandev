package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/planinjection"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const dynamicRouteStatusWaiting = "waiting"
const dynamicRouteStatusActive = "active"
const dynamicRouteStatusActionRequired = "action_required"

type dynamicStartupAttemptContextKey struct{}

type dynamicStartupAttempt struct {
	ID                 string
	SessionID          string
	LogicalProfileID   string
	ExecutionProfileID string
	Generation         int64
}

type unclassifiedFallbackLaunchClaimContextKey struct{}

func withUnclassifiedFallbackLaunchClaim(
	ctx context.Context,
	claim *unclassifiedFallbackLaunchClaim,
) context.Context {
	if claim == nil {
		return ctx
	}
	return context.WithValue(ctx, unclassifiedFallbackLaunchClaimContextKey{}, claim)
}

func unclassifiedFallbackLaunchClaimFromContext(ctx context.Context) *unclassifiedFallbackLaunchClaim {
	claim, _ := ctx.Value(unclassifiedFallbackLaunchClaimContextKey{}).(*unclassifiedFallbackLaunchClaim)
	return claim
}

func withDynamicStartupAttempt(ctx context.Context, launch dynamicruntime.DownstreamLaunch) context.Context {
	if launch.AttemptID == "" {
		return ctx
	}
	return context.WithValue(ctx, dynamicStartupAttemptContextKey{}, dynamicStartupAttempt{
		ID: launch.AttemptID, SessionID: launch.Decision.SessionID,
		LogicalProfileID:   launch.Decision.LogicalProfileID,
		ExecutionProfileID: launch.ExecutionProfileID,
		Generation:         launch.Decision.Generation,
	})
}

func dynamicStartupAttemptFromContext(ctx context.Context) (dynamicStartupAttempt, bool) {
	attempt, ok := ctx.Value(dynamicStartupAttemptContextKey{}).(dynamicStartupAttempt)
	return attempt, ok && attempt.ID != ""
}

// dynamicSuccessorDetachedTimeout bounds one detached fallback launch: the
// predecessor stop plus the successor launch. Without it the repository calls
// inside the launch could hold the session's cancel guard indefinitely and
// block every later stop or message for that session.
const dynamicSuccessorDetachedTimeout = 2 * time.Minute

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
	ctx = withDynamicStartupAttempt(ctx, launch)
	if claim := unclassifiedFallbackLaunchClaimFromContext(ctx); claim != nil {
		if d.service.profileExecutionResolver == nil {
			claim.launchError = dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable
			return dynamicruntime.DownstreamExecution{}, claim.launchError
		}
		if err := d.service.profileExecutionResolver.ClaimUnclassifiedFallbackLaunch(
			ctx, claim.decision, claim.evidence,
		); err != nil {
			claim.launchError = err
			return dynamicruntime.DownstreamExecution{}, err
		}
	}
	if err := d.service.persistDynamicLaunchDecision(ctx, d.sessionID, launch.Decision); err != nil {
		return dynamicruntime.DownstreamExecution{}, err
	}
	options := d.options
	options.AgentProfileID = launch.ExecutionProfileID
	options.Prompt = launch.Prompt
	options.PriorACPSession = launch.PriorACPSession
	if d.task != nil {
		if err := d.service.admitCeilingDispatch(ctx, d.task.ID); err != nil {
			return dynamicruntime.DownstreamExecution{}, err
		}
	}
	d.service.beginDynamicAttempt(d.sessionID)
	taskID := ""
	if d.task != nil {
		taskID = d.task.ID
	}
	dispatchCtx, releaseDispatchCommit, err := d.service.commitCeilingEntryDispatch(
		ctx, taskID, ceilingEntryBindingFromContext(ctx),
	)
	if err != nil {
		return dynamicruntime.DownstreamExecution{}, err
	}
	defer releaseDispatchCommit()
	execution, err := d.service.executor.LaunchPreparedSession(dispatchCtx, d.task, d.sessionID, options)
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
	d.service.bindPromptAttemptToExecution(dispatchCtx, d.sessionID, execution.AgentExecutionID)
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

// markDynamicRouteActive completes the durable "starting" -> "active"
// transition after a concrete launch has actually succeeded, and mirrors the
// status onto the task-session projection so both durable stores agree.
// Best-effort: a launch that already succeeded is not undone by a failure to
// record this side transition.
func (s *Service) markDynamicRouteActive(ctx context.Context, sessionID string, generation int64) {
	if s.profileExecutionResolver == nil || sessionID == "" || generation <= 0 {
		return
	}
	if err := s.profileExecutionResolver.MarkRouteActive(ctx, sessionID, generation); err != nil {
		if !errors.Is(err, dynamicruntime.ErrStaleGeneration) && !errors.Is(err, dynamicruntime.ErrRouteStateNotFound) {
			s.logger.Warn("failed to mark dynamic route active",
				zap.String("session_id", sessionID), zap.Error(err))
		}
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration != generation {
		return
	}
	if loader, ok := s.repo.(dynamicRouteStateLoader); ok {
		state, loadErr := loader.LoadRouteState(ctx, sessionID)
		if loadErr != nil || state == nil || state.Generation != generation || state.Status != dynamicRouteStatusActive {
			return
		}
	}
	s.mirrorDynamicRouteProjection(ctx, session, generation, dynamicRouteStatusActive, session.RouteReason)
}

// markDynamicRouteActionRequired transitions a claimed dynamic route to
// durable action_required and mirrors the status onto the task-session
// projection. It is the catch-all recovery marker every declined or failed
// dynamic launch path falls back to, so a claimed route is never left
// silently stuck at "starting" or "retrying" with no recovery affordance.
// The underlying engine call only transitions a route that is still
// "starting" or "retrying" (see Engine.MarkActionRequired), so calling this
// on a route a launch already carried to "active" is a safe no-op: neither
// store is touched, and an unrelated later failure cannot overwrite a
// healthy route's reason.
func (s *Service) markDynamicRouteActionRequired(ctx context.Context, sessionID string, generation int64, reason string) {
	if s.profileExecutionResolver == nil || sessionID == "" || generation <= 0 {
		return
	}
	decision, err := s.profileExecutionResolver.MarkRouteActionRequired(ctx, sessionID, generation, reason)
	if err != nil {
		if !errors.Is(err, dynamicruntime.ErrStaleGeneration) && !errors.Is(err, dynamicruntime.ErrRouteStateNotFound) {
			s.logger.Warn("failed to mark dynamic route action_required",
				zap.String("session_id", sessionID), zap.Error(err))
		}
		return
	}
	if decision.Status != dynamicRouteStatusActionRequired {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration != generation {
		return
	}
	s.mirrorDynamicRouteProjection(ctx, session, generation, decision.Status, reason)
}

type dynamicRouteSessionProjector interface {
	UpdateTaskSessionDynamicRouteIfCurrent(
		context.Context,
		string,
		int64,
		string,
		string,
		string,
	) (bool, time.Time, error)
}

func (s *Service) mirrorDynamicRouteProjection(
	ctx context.Context,
	session *models.TaskSession,
	generation int64,
	status, reason string,
) {
	if session == nil || session.RouteGeneration != generation || status == "" {
		return
	}
	if session.RouteState == status && session.RouteReason == reason {
		return
	}
	oldState := session.State
	if projector, ok := s.repo.(dynamicRouteSessionProjector); ok {
		changed, updatedAt, err := projector.UpdateTaskSessionDynamicRouteIfCurrent(
			ctx, session.ID, generation, session.RouteState, status, reason,
		)
		if err != nil {
			s.logger.Warn("failed to mirror dynamic route state to task session",
				zap.String("session_id", session.ID), zap.Error(err))
			return
		}
		if !changed {
			return
		}
		session.RouteState = status
		session.RouteReason = reason
		session.UpdatedAt = updatedAt
		s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, oldState, session.State, session.ErrorMessage, &updatedAt, session)
		return
	}
	// Legacy repositories do not expose the narrow route projection update. Keep
	// the fallback for tests and older adapters, while production SQLite uses the
	// generation/status-guarded method above.
	session.RouteState = status
	session.RouteReason = reason
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to mirror dynamic route state to task session",
			zap.String("session_id", session.ID), zap.Error(err))
		return
	}
	updatedAt := session.UpdatedAt
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, oldState, session.State, session.ErrorMessage, &updatedAt, session)
}

// handleAgentProcessStarted settles a dynamic route only after the asynchronous
// process start succeeds. LaunchPreparedSession returns before this point.
func (s *Service) handleAgentProcessStarted(
	ctx context.Context,
	taskID, sessionID, agentExecutionID string,
) {
	if !s.ceilingCallbackOwnsSession(ctx, sessionID, agentExecutionID) {
		return
	}
	s.retireWorkflowStartPromptAttempt(ctx, taskID, sessionID, agentExecutionID)
	// AC-52's acceptance edge, composed first and unconditionally on
	// sessionID alone (AC-56a): profileExecutionResolver is a dynamic-routing
	// precondition, not a launch one, so an instance without it configured
	// must still confirm the reservation on every ordinary launch.
	s.confirmCeilingReservation(sessionID)
	if s.profileExecutionResolver == nil || sessionID == "" {
		return
	}
	// The executor callback preserves the launch context's attempt value. An
	// ordinary launch has no recovery identity and must be rejected while a
	// newer recovery attempt is active; borrowing that newer identity here
	// would let a delayed finalizeLaunch callback mutate the replacement route.
	if !s.resumeAttemptAllowsExecution(sessionID, agentExecutionID, executor.ResumeAttemptIDFromContext(ctx)) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return
	}
	if session.AgentExecutionID != "" && agentExecutionID != "" && session.AgentExecutionID != agentExecutionID {
		return
	}
	if session.State != models.TaskSessionStateStarting && session.State != models.TaskSessionStateRunning {
		return
	}
	s.markDynamicRouteActive(ctx, sessionID, session.RouteGeneration)
}

// handleAgentProcessStartFailed settles a claimed dynamic route when process
// startup fails after LaunchPreparedSession already returned successfully.
func (s *Service) handleAgentProcessStartFailed(
	ctx context.Context,
	_, sessionID, agentExecutionID string,
	_ error,
) {
	if !s.ceilingCallbackOwnsSession(ctx, sessionID, agentExecutionID) {
		return
	}
	// AC-52's failure edge, composed first and unconditionally on sessionID
	// alone (AC-56a) — see handleAgentProcessStarted's acceptance edge.
	s.releaseCeilingReservation(sessionID)
	if s.profileExecutionResolver == nil || sessionID == "" {
		return
	}
	// See handleAgentProcessStarted: missing origin is intentionally fail-closed
	// during an active recovery attempt rather than being relabelled as the
	// current attempt.
	if !s.resumeAttemptAllowsExecution(sessionID, agentExecutionID, executor.ResumeAttemptIDFromContext(ctx)) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return
	}
	if session.AgentExecutionID != "" && agentExecutionID != "" && session.AgentExecutionID != agentExecutionID {
		return
	}
	if session.State == models.TaskSessionStateCancelled || session.State == models.TaskSessionStateCompleted {
		return
	}
	s.markDynamicRouteActionRequired(ctx, sessionID, session.RouteGeneration, "agent_process_start_failed")
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
	if task != nil {
		if err := s.validateContextCeilingEntry(ctx, task.ID); err != nil {
			return nil, err
		}
	}
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
	if task != nil {
		if err := s.validateContextCeilingEntry(ctx, task.ID); err != nil {
			return nil, err
		}
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
	if task != nil {
		if err := s.admitCeilingDispatch(ctx, task.ID); err != nil {
			return nil, err
		}
	}
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	dispatchCtx, releaseDispatchCommit, err := s.commitCeilingEntryDispatch(
		ctx, taskID, ceilingEntryBindingFromContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	defer releaseDispatchCommit()
	if options.StartAgent && (options.Prompt != "" || len(options.Attachments) > 0) {
		s.beginInitialPromptAttempt(sessionID, false)
	}
	execution, err := s.executor.LaunchPreparedSession(dispatchCtx, task, sessionID, options)
	if execution != nil {
		s.bindPromptAttemptToExecution(dispatchCtx, sessionID, execution.AgentExecutionID)
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

func limitDynamicRecoveryContext(input *dynamicruntime.ContinuationInput) {
	if input == nil {
		return
	}
	// A recovery handoff can cross providers. Keep user requests and durable
	// task artifacts, but omit agent text and tool output from the failed
	// attempt because they can contain untrusted provider diagnostics.
	input.Conversation = ""
	input.ToolSummary = ""
	input.FailureReason = "The previous agent attempt failed."
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
	result := s.routeDynamicAgentFailureWithEvidence(ctx, data, classified, nil, false)
	return result.handled && !result.manualRecovery
}

type dynamicFailureRouteResult struct {
	handled        bool
	manualRecovery bool
}

func dynamicFailureRecoveryResult(handled bool, decision dynamicruntime.RouteDecision) dynamicFailureRouteResult {
	return dynamicFailureRouteResult{
		handled: handled, manualRecovery: handled && decision.Status == dynamicRouteStatusActionRequired,
	}
}

type unclassifiedFallbackLaunchClaim struct {
	decision    dynamicruntime.RouteDecision
	evidence    dynamicruntime.UnclassifiedFailureEvidence
	launchError error
}

func (s *Service) routeDynamicAgentFailureWithEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	classified *routingerr.Error,
	startupEvidence *dynamicruntime.UnclassifiedFailureEvidence,
	guardHeld bool,
) dynamicFailureRouteResult {
	unclassified := classified != nil && routingerr.ClassForCode(classified.Code) == routingerr.ClassUnclassified
	if unclassified && !guardHeld && data.SessionID != "" {
		lock, release := s.acquireCancelInFlightGuard(data.SessionID)
		lock.Lock()
		defer func() {
			lock.Unlock()
			release()
		}()
		if s.isCancelInFlight(data.SessionID) {
			return dynamicFailureRouteResult{}
		}
	}
	data = s.withDynamicAttemptEvidence(data)
	session, ok := s.dynamicFailureSession(ctx, data)
	if !ok {
		return dynamicFailureRouteResult{}
	}
	reason := "dynamic_recovery_declined"
	if classified != nil {
		reason = string(classified.Code)
	}
	// The route already holds a claimed generation the moment
	// dynamicFailureSession succeeds. Every return below is a decline, so the
	// deferred guard covers every early return uniformly (including any added
	// later) instead of relying on each branch to remember its own
	// transition. `generation` is updated in place once RouteAfterFailure
	// claims a new one so the guard marks the generation actually holding the
	// failed attempt, not the one this call started from. The guard is safe
	// to arm this early even for a failure the classifier forbids fallback
	// for: markDynamicRouteActionRequired only transitions a route that is
	// still "starting" or "retrying", so it is a no-op once this generation's
	// launch has already reached "active" (an ordinary runtime failure on a
	// healthy session), and only fires for a route genuinely stranded
	// mid-launch.
	generation := session.RouteGeneration
	handled := false
	defer func() {
		if !handled {
			s.markDynamicRouteActionRequired(ctx, session.ID, generation, reason)
		}
	}()
	if classified == nil {
		return dynamicFailureRouteResult{}
	}
	evidence, eligible := s.prepareDynamicFailureEvidence(ctx, data, session, classified, startupEvidence, unclassified)
	if !eligible {
		return dynamicFailureRouteResult{}
	}
	conductor := s.profileExecutionResolver.NewConductor(nil)
	task, err := s.scheduler.GetTask(ctx, data.TaskID)
	if err != nil {
		return dynamicFailureRouteResult{}
	}
	decision, continuationInput, err := s.routeDynamicFailureDecision(
		ctx, session, task, conductor, classified, evidence, unclassified,
	)
	if decision.Generation > 0 {
		generation = decision.Generation
	}
	if err != nil {
		handled = s.persistPendingDynamicRecovery(ctx, session, decision, err)
		return dynamicFailureRecoveryResult(handled, decision)
	}
	var launchClaim *unclassifiedFallbackLaunchClaim
	if unclassified {
		launchClaim = &unclassifiedFallbackLaunchClaim{decision: decision, evidence: evidence}
	}
	handled = s.launchDynamicSuccessorAfterFailure(ctx, data, session, conductor, decision, continuationInput, launchClaim)
	return dynamicFailureRouteResult{handled: handled}
}

func (s *Service) routeDynamicFailureDecision(
	ctx context.Context,
	session *models.TaskSession,
	task *v1.Task,
	conductor *dynamicruntime.Conductor,
	classified *routingerr.Error,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
	unclassified bool,
) (dynamicruntime.RouteDecision, dynamicruntime.ContinuationInput, error) {
	var decision dynamicruntime.RouteDecision
	var continuationInput dynamicruntime.ContinuationInput
	var err error
	if unclassified {
		decision, err = conductor.RouteAfterUnclassifiedFailure(
			ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
			session.RouteGeneration, classified, evidence,
		)
	} else {
		continuationInput, err = s.buildDynamicContinuation(
			ctx, task, session.ID, "", "The previous agent attempt failed.",
		)
		limitDynamicRecoveryContext(&continuationInput)
		if err == nil {
			decision, err = conductor.RouteAfterFailure(
				ctx, session.ID, session.AgentProfileID, session.ExecutionProfileID,
				session.RouteGeneration, classified,
			)
		}
	}
	if err != nil || !unclassified {
		return decision, continuationInput, err
	}
	continuationInput, err = s.buildDynamicContinuation(
		ctx, task, session.ID, "", "The previous agent attempt failed.",
	)
	limitDynamicRecoveryContext(&continuationInput)
	return decision, continuationInput, err
}

func (s *Service) prepareDynamicFailureEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	classified *routingerr.Error,
	startupEvidence *dynamicruntime.UnclassifiedFailureEvidence,
	unclassified bool,
) (dynamicruntime.UnclassifiedFailureEvidence, bool) {
	if unclassified {
		return s.prepareUnclassifiedFailureEvidence(ctx, data, session, classified, startupEvidence)
	}
	if !s.clearStreakAfterCurrentClassifiedFailure(ctx, data, session) {
		return dynamicruntime.UnclassifiedFailureEvidence{}, false
	}
	return dynamicruntime.UnclassifiedFailureEvidence{}, classified.FallbackAllowed && dynamicPreResultSafe(data)
}

func (s *Service) prepareUnclassifiedFailureEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	classified *routingerr.Error,
	startupEvidence *dynamicruntime.UnclassifiedFailureEvidence,
) (dynamicruntime.UnclassifiedFailureEvidence, bool) {
	evidence := s.unclassifiedFailureRouteEvidence(ctx, data, session, startupEvidence)
	if !evidence.TaskScope || !evidence.CurrentAttempt {
		return dynamicruntime.UnclassifiedFailureEvidence{}, false
	}
	if classified.Code == routingerr.CodeUnknownProvider || classified.Code == routingerr.CodeAgentRuntime {
		return evidence, true
	}
	if err := s.profileExecutionResolver.ClearUnclassifiedStreak(
		ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID,
	); err != nil {
		s.logger.Warn("failed to reset unclassified route streak after an ineligible unknown failure",
			zap.String("session_id", session.ID), zap.Error(err))
	}
	return dynamicruntime.UnclassifiedFailureEvidence{}, false
}

func (s *Service) unclassifiedFailureRouteEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	startupEvidence *dynamicruntime.UnclassifiedFailureEvidence,
) dynamicruntime.UnclassifiedFailureEvidence {
	if startupEvidence != nil {
		return *startupEvidence
	}
	return s.unclassifiedPromptEvidence(ctx, data, session)
}

func (s *Service) clearStreakAfterCurrentClassifiedFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) bool {
	current := data.DynamicRouteAttempt && data.EvidenceKnown && data.AgentExecutionID != "" &&
		data.PromptGeneration != 0 && s.currentDynamicPromptAttempt(
		data.SessionID, data.AgentExecutionID, data.PromptGeneration,
	)
	if !current {
		return true
	}
	if err := s.profileExecutionResolver.ClearUnclassifiedStreak(
		ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID,
	); err != nil {
		s.logger.Warn("failed to reset unclassified route streak after classified failure",
			zap.String("session_id", session.ID), zap.Error(err))
		return false
	}
	return true
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
	oldState := session.State
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
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, oldState, session.State, session.ErrorMessage, &session.UpdatedAt, session)
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
	launchClaim *unclassifiedFallbackLaunchClaim,
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
	return s.launchDynamicSuccessorDetachedWithClaim(ctx, data, next.ExecutionProfileID, launchClaim)
}

// launchDynamicSuccessorDetached runs the predecessor stop and successor
// launch outside the agent.failed dispatch that decided the route.
//
// The failure event is published synchronously by the lifecycle completion
// handler while it still holds the execution's prompt lifecycle lock, and the
// in-memory event bus delivers it on the publisher's goroutine. Stopping the
// predecessor from that goroutine publishes agent.stopped for the same
// execution, which needs the same lock to snapshot the prompt turn: the
// dispatch deadlocks, the successor never starts, and the session stays
// RUNNING without a process. Leaving the dispatch first lets the completion
// handler release the lock before the stop runs.
// The worker runs under the service-owned dynamicSuccessorCtx rather than
// context.WithoutCancel(ctx): the launch must survive the dispatch that
// scheduled it, but it must not outlive Stop and keep mutating session state
// after shutdown has begun. Returning false means no worker was scheduled, so
// the caller falls through to the ordinary terminal-failure path instead of
// reporting a route that will never be taken.
func (s *Service) launchDynamicSuccessorDetached(
	_ context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
) bool {
	return s.launchDynamicSuccessorDetachedWithClaim(context.Background(), data, executionProfileID, nil)
}

func (s *Service) launchDynamicSuccessorDetachedWithClaim(
	_ context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
	claim *unclassifiedFallbackLaunchClaim,
) bool {
	s.dynamicSuccessorMu.Lock()
	if s.dynamicSuccessorStopped {
		s.dynamicSuccessorMu.Unlock()
		s.logger.Warn("dynamic successor launch skipped; service is stopping",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("execution_profile_id", executionProfileID))
		return false
	}
	if s.dynamicSuccessorCtx == nil {
		s.dynamicSuccessorCtx, s.dynamicSuccessorCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.dynamicSuccessorCtx
	s.dynamicSuccessorWorkers.Add(1)
	s.dynamicSuccessorMu.Unlock()
	go func() {
		defer s.dynamicSuccessorWorkers.Done()
		launchCtx, cancel := context.WithTimeout(workerCtx, dynamicSuccessorDetachedTimeout)
		defer cancel()
		s.runDetachedDynamicSuccessorLaunchWithClaim(launchCtx, data, executionProfileID, claim)
	}()
	return true
}

// runDetachedDynamicSuccessorLaunch serializes with the session's cancel guard
// like every other session-backed failure decision, then launches the
// successor. A launch that cannot complete falls back to the ordinary
// recoverable-failure surface instead of leaving the session RUNNING. A
// ceiling deferral is different: the durable replay record owns the retry, so
// the automation run must remain open until that replay either succeeds or
// fails for a non-ceiling reason.
func (s *Service) runDetachedDynamicSuccessorLaunch(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
) {
	s.runDetachedDynamicSuccessorLaunchWithClaim(ctx, data, executionProfileID, nil)
}

func (s *Service) runDetachedDynamicSuccessorLaunchWithClaim(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
	claim *unclassifiedFallbackLaunchClaim,
) {
	lock, release := s.acquireCancelInFlightGuard(data.SessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if ctx.Err() != nil {
		s.logger.Warn("dynamic successor launch abandoned before validation",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("execution_profile_id", executionProfileID),
			zap.Error(ctx.Err()))
		return
	}
	// A coordinator stop can win the guard between the route decision and this
	// goroutine: it persists the session as cancelled and releases the guard
	// before the launch acquires it. Relaunching from the stale event would
	// reset that session back to CREATED and resurrect work after an
	// acknowledged stop, so re-read the current state under the guard and
	// abort unless the session is still nonterminal and still owns this
	// execution.
	if drop, _ := s.shouldDropSessionFailure(ctx, data, "dynamic.successor.detached", true); drop {
		s.logger.Debug("dropping detached dynamic successor launch for stale session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID),
			zap.String("execution_profile_id", executionProfileID))
		return
	}
	if ctx.Err() != nil {
		s.logger.Warn("dynamic successor launch abandoned before start",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("execution_profile_id", executionProfileID),
			zap.Error(ctx.Err()))
		return
	}
	ctx = withUnclassifiedFallbackLaunchClaim(ctx, claim)
	ctx = withCancelInFlightGuardHeld(ctx)
	switch s.relaunchDynamicTaskAfterFailureOutcome(ctx, data, executionProfileID, launchOriginAutomatic) {
	case dynamicRelaunchSucceeded:
		return
	case dynamicRelaunchDeferred:
		s.logger.Debug("dynamic successor launch deferred by session ceiling",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("execution_profile_id", executionProfileID))
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		s.logger.Warn("dynamic successor launch abandoned after cancellation",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("execution_profile_id", executionProfileID),
			zap.Error(ctx.Err()))
		return
	}
	if claim != nil && unclassifiedWorkflowContextRejection(claim.launchError) {
		s.settleDetachedUnclassifiedWorkflowRejection(context.WithoutCancel(ctx), data, claim)
		return
	}
	s.logger.Warn("dynamic successor launch failed; surfacing recoverable failure",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("execution_profile_id", executionProfileID))
	s.settleDetachedDynamicSuccessorFailure(context.WithoutCancel(ctx), data, executionProfileID)
}

func unclassifiedWorkflowContextRejection(err error) bool {
	return errors.Is(err, dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable) ||
		errors.Is(err, dynamicruntime.ErrUnclassifiedWorkflowContextChanged)
}

func (s *Service) settleDetachedUnclassifiedWorkflowRejection(
	failureCtx context.Context,
	data watcher.AgentEventData,
	claim *unclassifiedFallbackLaunchClaim,
) {
	if claim == nil {
		return
	}
	state, err := s.repo.GetTaskSession(failureCtx, data.SessionID)
	if err != nil || state == nil || state.RouteGeneration != claim.decision.Generation ||
		state.RouteState != dynamicRouteStatusActionRequired {
		return
	}
	if err := s.profileExecutionResolver.ClearUnclassifiedStreak(
		failureCtx, data.SessionID, claim.decision.Generation, claim.decision.ExecutionProfileID,
	); err != nil && !errors.Is(err, dynamicruntime.ErrStaleGeneration) {
		s.logger.Warn("failed to clear unclassified streak after workflow-context rejection",
			zap.String("session_id", data.SessionID), zap.Error(err))
	}
	s.retireExecutionActivityAndPublish(failureCtx, data.TaskID, data.SessionID, data.AgentExecutionID)
	errMsg := data.ErrorMessage
	if errMsg == "" {
		errMsg = "workflow context was rejected before dynamic successor launch"
	}
	s.finalizeAutomationRun(failureCtx, data.TaskID, false, errMsg)
	data.ErrorMessage = errMsg
	// A workflow-context rejection is a user-owned recovery boundary, so do
	// not run the returned workflow trigger after settling the failed turn.
	_ = s.handleRecoverableFailureLockedState(failureCtx, data, agentruntime.StopReasonRecoverableAgentFailure)
	state, err = s.repo.GetTaskSession(failureCtx, data.SessionID)
	if err != nil || state == nil || state.RouteGeneration != claim.decision.Generation {
		return
	}
	oldState := state.State
	state.State = models.TaskSessionStateWaitingForInput
	state.DownstreamACPSessionID = ""
	state.RouteState = dynamicRouteStatusActionRequired
	state.RouteReason = "unclassified_workflow_context_changed"
	if errors.Is(claim.launchError, dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable) {
		state.RouteReason = "unclassified_workflow_context_unavailable"
	}
	if err := s.repo.UpdateTaskSession(failureCtx, state); err != nil {
		s.logger.Warn("failed to preserve manual recovery after workflow-context rejection",
			zap.String("session_id", data.SessionID), zap.Error(err))
		return
	}
	s.publishTaskSessionStateChanged(
		failureCtx, state.TaskID, state.ID, oldState, state.State, state.ErrorMessage, &state.UpdatedAt, state,
	)
}

func (s *Service) settleDetachedDynamicSuccessorFailure(
	failureCtx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
) {
	// The synchronous failure handler returned when the automatic route was
	// accepted. This worker therefore owns terminal cleanup whenever the
	// successor cannot start, including a workflow-context veto.
	s.retireExecutionActivityAndPublish(failureCtx, data.TaskID, data.SessionID, data.AgentExecutionID)
	errMsg := data.ErrorMessage
	if errMsg == "" {
		errMsg = "dynamic successor launch failed for " + executionProfileID
	}
	s.finalizeAutomationRun(failureCtx, data.TaskID, false, errMsg)
	if dispatch := s.handleRecoverableFailureLockedState(failureCtx, data, agentruntime.StopReasonRecoverableAgentFailure); dispatch != nil {
		s.startAgentFailureRecovery(dispatch)
	}
}

func (s *Service) resetDynamicSuccessorWorkers() {
	s.dynamicSuccessorMu.Lock()
	defer s.dynamicSuccessorMu.Unlock()
	s.dynamicSuccessorStopped = false
	s.dynamicSuccessorCtx, s.dynamicSuccessorCancel = context.WithCancel(context.Background())
}

func (s *Service) stopDynamicSuccessorWorkers() {
	s.dynamicSuccessorMu.Lock()
	s.dynamicSuccessorStopped = true
	cancel := s.dynamicSuccessorCancel
	s.dynamicSuccessorMu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.dynamicSuccessorWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(sendNowClaimRecoveryTimeout):
		s.logger.Warn("timed out waiting for dynamic successor workers during shutdown")
	}
}

// LaunchDynamicRouteAction completes a manual retry/try-next operation after
// the backend route-action handler has claimed the successor generation. It
// owns the predecessor shutdown, continuation persistence, successor launch,
// and final launch error surface so the UI never has to issue a second launch
// request.
func (s *Service) LaunchDynamicRouteAction(ctx context.Context, sessionID string) error {
	return s.launchDynamicRouteAction(ctx, sessionID, launchOriginManual)
}

// LaunchDynamicRouteActionForRecovery is the unattended counterpart used by
// the policy recovery timer sweep: same successor-launch transaction, but the
// session was never routed by a human action in this call.
func (s *Service) LaunchDynamicRouteActionForRecovery(ctx context.Context, sessionID string) error {
	return s.launchDynamicRouteAction(ctx, sessionID, launchOriginAutomatic)
}

func (s *Service) launchDynamicRouteAction(ctx context.Context, sessionID string, origin launchOrigin) error {
	if s.profileExecutionResolver == nil || sessionID == "" {
		return errors.New("dynamic route action launch is not configured")
	}
	if !s.profileExecutionResolver.Enabled() {
		return agentruntime.ErrDynamicRoutingDisabled
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		if err != nil {
			return err
		}
		return errors.New("dynamic route action session not found")
	}
	// The backend route-action handler has already claimed session.RouteGeneration
	// as "starting" before calling this. Every error return below leaves that
	// claim without an owner unless the deferred guard marks it action_required.
	succeeded := false
	defer func() {
		if !succeeded {
			s.markDynamicRouteActionRequired(ctx, sessionID, session.RouteGeneration, string(origin)+" dynamic route action failed")
		}
	}()
	task, err := s.scheduler.GetTask(ctx, session.TaskID)
	if err != nil {
		return err
	}
	reason := string(origin) + " dynamic route action"
	input, err := s.buildDynamicContinuation(ctx, task, sessionID, "", reason)
	if err != nil {
		return err
	}
	limitDynamicRecoveryContext(&input)
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
		ErrorMessage:       reason,
	}
	if !s.relaunchDynamicTaskAfterFailure(ctx, data, session.ExecutionProfileID, origin) {
		return errors.New("dynamic route action successor launch failed")
	}
	succeeded = true
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

func (s *Service) unclassifiedPromptEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) dynamicruntime.UnclassifiedFailureEvidence {
	currentAttempt := data.EvidenceKnown && data.AgentExecutionID != "" &&
		data.PromptGeneration != 0 && s.currentDynamicPromptAttempt(
		data.SessionID, data.AgentExecutionID, data.PromptGeneration,
	)
	providerID, diagnostic, complete := "", "", false
	if providerError := data.ProviderError; providerError != nil {
		providerID = providerError.ProviderID
		diagnostic = providerError.Message
		complete = providerError.Valid() && providerError.DiagnosticIdentityComplete &&
			completeProviderDiagnosticSource(providerError.Source)
	}
	return s.unclassifiedFailureEvidence(
		ctx, data, session, currentAttempt, dynamicruntime.UnclassifiedOriginTerminalProvider,
		routingerr.PhasePromptSend, promptUnclassifiedAttemptID(data), providerID, diagnostic, complete,
		data.EvidenceKnown, data.OutputObserved, data.EffectObserved,
	)
}

func (s *Service) unclassifiedStartupEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	attempt dynamicStartupAttempt,
	startup *routingerr.AgentStartupFailure,
) dynamicruntime.UnclassifiedFailureEvidence {
	phase, providerID, diagnostic := routingerr.Phase(""), "", ""
	diagnosticComplete := false
	if startup != nil {
		phase, providerID, diagnostic = startup.Phase, startup.ProviderID, startup.Diagnostic
		diagnosticComplete = startup.DiagnosticIdentityComplete &&
			completeProviderDiagnosticSource(startup.DiagnosticSource) &&
			streams.IsCompleteProviderDiagnostic(startup.Diagnostic)
	}
	return s.unclassifiedFailureEvidence(
		ctx, data, session, startup != nil && startup.Cause != nil && session != nil && attempt.ID != "" &&
			attempt.SessionID == data.SessionID && attempt.Generation == session.RouteGeneration &&
			attempt.LogicalProfileID == session.AgentProfileID &&
			attempt.ExecutionProfileID == session.ExecutionProfileID,
		dynamicruntime.UnclassifiedOriginAgentStartup,
		phase, attempt.ID, providerID, diagnostic, diagnosticComplete, true, false, false,
	)
}

func (s *Service) unclassifiedFailureEvidence(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	currentAttempt bool,
	origin dynamicruntime.UnclassifiedFailureOrigin,
	phase routingerr.Phase,
	attemptID, providerID, diagnostic string,
	diagnosticComplete, evidenceKnown, outputObserved, effectObserved bool,
) dynamicruntime.UnclassifiedFailureEvidence {
	evidence := dynamicruntime.UnclassifiedFailureEvidence{
		SessionID: data.SessionID, AttemptID: attemptID, Origin: origin,
		Phase: phase, ProviderID: providerID, DiagnosticText: diagnostic,
		DiagnosticComplete: diagnosticComplete, CurrentAttempt: currentAttempt,
		EvidenceKnown: evidenceKnown, OutputObserved: outputObserved, EffectObserved: effectObserved,
	}
	task, ok := s.unclassifiedFailureTask(ctx, data, session)
	if !ok {
		return evidence
	}
	evidence.TaskScope = true
	evidence.TaskID = task.ID
	evidence.WorkflowID = task.WorkflowID
	evidence.SessionID = session.ID
	evidence.LogicalProfileID = session.AgentProfileID
	evidence.ExecutionProfileID = session.ExecutionProfileID
	evidence.RouteGeneration = session.RouteGeneration
	evidence.StepID = task.WorkflowStepID
	evidence.StepKnown, evidence.StepVeto, evidence.StepUpdatedAt = s.unclassifiedStepEvidence(ctx, task)
	return evidence
}

func (s *Service) unclassifiedFailureTask(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) (*models.Task, bool) {
	if session == nil || s.repo == nil || data.TaskID == "" || data.OwnerKind != queueStatusScopeTask {
		return nil, false
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || task.ID == "" || task.ID != session.TaskID || task.ID != data.TaskID || task.IsFromOffice {
		return nil, false
	}
	return task, true
}

func (s *Service) unclassifiedStepEvidence(
	ctx context.Context,
	task *models.Task,
) (known, veto bool, updatedAt time.Time) {
	if task.WorkflowStepID == "" {
		return true, false, time.Time{}
	}
	if s.workflowStepGetter == nil {
		return false, false, time.Time{}
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil || step.ID != task.WorkflowStepID ||
		(task.WorkflowID != "" && step.WorkflowID != task.WorkflowID) {
		return false, false, time.Time{}
	}
	return true, step.DisableUnclassifiedFallback, step.UpdatedAt
}

func (s *Service) currentDynamicPromptAttempt(sessionID, executionID string, promptGeneration uint64) bool {
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.dynamic && evidence.evidenceKnown &&
		evidence.executionID == executionID && evidence.promptGeneration == promptGeneration
}

func completeProviderDiagnosticSource(source string) bool {
	switch source {
	case streams.ProviderErrorSourceOpenCodeStderr, streams.ProviderErrorSourceOpenCodeACP,
		streams.ProviderErrorSourceACPPrompt:
		return true
	default:
		return false
	}
}

func promptUnclassifiedAttemptID(data watcher.AgentEventData) string {
	if data.AgentExecutionID == "" || data.PromptGeneration == 0 {
		return ""
	}
	return fmt.Sprintf("prompt:%s:%d", data.AgentExecutionID, data.PromptGeneration)
}

type dynamicRelaunchOutcome uint8

type dynamicRelaunchLaunchMode uint8

const (
	dynamicRelaunchLaunchModePrepared dynamicRelaunchLaunchMode = iota
	dynamicRelaunchLaunchModeCreated
	dynamicRelaunchLaunchModePrompt
)

const (
	dynamicRelaunchFailed dynamicRelaunchOutcome = iota
	dynamicRelaunchDeferred
	dynamicRelaunchSucceeded
	dynamicRelaunchSuperseded
)

// relaunchDynamicTaskAfterFailure preserves the historical boolean API for
// synchronous route actions. Callers that own terminalization use the richer
// outcome so a durable ceiling deferral is not mistaken for a failed launch.
func (s *Service) relaunchDynamicTaskAfterFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
	origin launchOrigin,
) bool {
	return s.relaunchDynamicTaskAfterFailureOutcome(ctx, data, executionProfileID, origin) == dynamicRelaunchSucceeded
}

func (s *Service) relaunchDynamicTaskAfterFailureOutcome(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
	origin launchOrigin,
) (outcome dynamicRelaunchOutcome) {
	return s.relaunchDynamicTaskAfterFailureOutcomeWithBinding(ctx, data, executionProfileID, origin, ceilingEntryBindingFromContext(ctx))
}

//nolint:cyclop // Dynamic relaunch keeps admission, failure recovery, and generation fencing in one boundary.
func (s *Service) relaunchDynamicTaskAfterFailureOutcomeWithBinding(
	ctx context.Context,
	data watcher.AgentEventData,
	executionProfileID string,
	origin launchOrigin,
	binding *models.CeilingWorkflowEntryBinding,
) (outcome dynamicRelaunchOutcome) {
	if binding != nil {
		if err := s.validateClaimedCeilingBinding(ctx, data.TaskID, binding); err != nil {
			if errors.Is(err, ErrCeilingLaunchSuperseded) {
				return dynamicRelaunchSuperseded
			}
			return dynamicRelaunchFailed
		}
		ctx = withCeilingEntryBinding(ctx, binding)
	}
	seam5Res, deferredLaunch, err := s.admitOrDeferSeam5WithBinding(ctx, data.TaskID, origin, seam5DynamicRelaunchPayloadWithBinding(data, executionProfileID, binding), binding)
	if err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the dynamic relaunch could not be admitted or recorded",
			zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
		return dynamicRelaunchFailed
	}
	if deferredLaunch {
		return dynamicRelaunchDeferred
	}
	defer seam5Res.releaseIfNotConsumed()
	s.recordManualOverrideIfAdmitted(ctx, data.TaskID, data.SessionID, seam5Res.manualOverride, seam5Res.population, seam5Res.populationKnown, seam5Res.ceiling)

	defer func() {
		if outcome == dynamicRelaunchSucceeded || s.profileExecutionResolver == nil || data.SessionID == "" {
			return
		}
		// This helper is used by both the synchronous route-failure path and
		// the detached successor worker. The caller's deferred guard covers
		// the former, but the detached path reports handled=true before the
		// worker completes, so this helper must settle the claimed generation
		// when the relaunch fails.
		markCtx := context.WithoutCancel(ctx)
		session, err := s.repo.GetTaskSession(markCtx, data.SessionID)
		if err != nil || session == nil || session.RouteGeneration <= 0 {
			return
		}
		s.markDynamicRouteActionRequired(
			markCtx,
			data.SessionID,
			session.RouteGeneration,
			"dynamic_successor_launch_failed",
		)
	}()

	task, session, prompt, launchMode, ok := s.prepareDynamicRelaunchAfterFailure(ctx, data, origin)
	if !ok {
		return dynamicRelaunchFailed
	}
	if binding != nil {
		if err := s.validateClaimedCeilingBinding(ctx, data.TaskID, binding); err != nil {
			if errors.Is(err, ErrCeilingLaunchSuperseded) {
				return dynamicRelaunchSuperseded
			}
			return dynamicRelaunchFailed
		}
	}
	return s.launchPreparedDynamicRelaunch(
		ctx, data, task, session, prompt, executionProfileID, launchMode, origin, seam5Res,
	)
}

func (s *Service) prepareDynamicRelaunchAfterFailure(
	ctx context.Context,
	data watcher.AgentEventData,
	origin launchOrigin,
) (*v1.Task, *models.TaskSession, capturedPrompt, dynamicRelaunchLaunchMode, bool) {
	prompt, ok := s.dynamicRelaunchPrompt(ctx, data.SessionID)
	if !ok {
		return nil, nil, capturedPrompt{}, dynamicRelaunchLaunchModePrepared, false
	}
	task, err := s.scheduler.GetTask(ctx, data.TaskID)
	if err != nil {
		return nil, nil, capturedPrompt{}, dynamicRelaunchLaunchModePrepared, false
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		return nil, nil, capturedPrompt{}, dynamicRelaunchLaunchModePrepared, false
	}
	isOfficeTask, officeErr := s.lookupOfficeTask(ctx, data.TaskID)
	launchMode := dynamicRelaunchLaunchModeCreated
	if officeErr != nil || isOfficeTask {
		launchMode = dynamicRelaunchLaunchModePrepared
	} else if origin == launchOriginManual {
		launchMode = dynamicRelaunchLaunchModePrompt
	}
	if !s.stopDynamicRelaunchPredecessor(ctx, data.AgentExecutionID) {
		return nil, nil, capturedPrompt{}, launchMode, false
	}
	targetState := models.TaskSessionStateWaitingForInput
	if launchMode != dynamicRelaunchLaunchModePrompt {
		targetState = models.TaskSessionStateCreated
	}
	if !s.resetDynamicRelaunchSession(ctx, data.SessionID, targetState) {
		return nil, nil, capturedPrompt{}, launchMode, false
	}
	s.reconcileCIAutoFixTurnBeforeCompletion(ctx, data.TaskID, data.SessionID, "")
	s.completeTurnForSession(ctx, data.SessionID)
	s.retireExecutionActivityAndPublish(ctx, data.TaskID, data.SessionID, data.AgentExecutionID)
	return task, session, prompt, launchMode, true
}

func (s *Service) stopDynamicRelaunchPredecessor(ctx context.Context, agentExecutionID string) bool {
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForExecution(agentExecutionID)
	}
	err := s.executor.StopExecution(ctx, agentExecutionID, "dynamic route fallback", true)
	if err == nil {
		return true
	}
	if errors.Is(err, agentruntime.ErrNotFound) {
		s.logger.Debug("dynamic fallback predecessor is already absent", zap.Error(err))
		return true
	}
	s.logger.Debug("failed to stop dynamic fallback predecessor", zap.Error(err))
	return false
}

func (s *Service) resetDynamicRelaunchSession(
	ctx context.Context,
	sessionID string,
	targetState models.TaskSessionState,
) bool {
	// Reload immediately before the reset: StopExecution is I/O and a
	// coordinator stop can commit a terminal state while it runs. A stale
	// pre-stop snapshot would let this write resurrect a session the user
	// already cancelled, so refuse on a reloaded terminal state and CAS the
	// write on that same reload to catch a cancellation landing after it.
	preResetState, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || preResetState == nil {
		return false
	}
	if isTerminalSessionState(preResetState.State) {
		return false
	}
	changed, _, err := s.repo.UpdateTaskSessionStateIfCurrent(
		ctx, sessionID, preResetState.State, targetState, "",
	)
	return err == nil && changed
}

func (s *Service) launchPreparedDynamicRelaunch(
	ctx context.Context,
	data watcher.AgentEventData,
	task *v1.Task,
	session *models.TaskSession,
	prompt capturedPrompt,
	executionProfileID string,
	launchMode dynamicRelaunchLaunchMode,
	origin launchOrigin,
	seam5Res *sessionKeyedCeilingReservation,
) dynamicRelaunchOutcome {
	switch launchMode {
	case dynamicRelaunchLaunchModePrompt:
		_, err := s.promptTask(
			ctx,
			data.TaskID,
			data.SessionID,
			prompt.text,
			"",
			prompt.planMode,
			prompt.attachments,
			true,
			origin,
			promptTaskOptions{allowRouteActionPrompt: true},
		)
		if err != nil {
			return dynamicRelaunchFailed
		}
		if session != nil {
			s.markDynamicRouteActive(context.WithoutCancel(ctx), data.SessionID, session.RouteGeneration)
		}
		seam5Res.consume()
		return dynamicRelaunchSucceeded
	case dynamicRelaunchLaunchModeCreated:
		_, err := s.startDynamicRelaunchCreatedSession(
			ctx, data.TaskID, session, prompt,
		)
		if err != nil {
			return dynamicRelaunchFailed
		}
		seam5Res.consume()
		return dynamicRelaunchSucceeded
	case dynamicRelaunchLaunchModePrepared:
		officeAgentProfileID := data.AgentProfileID
		if officeAgentProfileID == "" {
			officeAgentProfileID = session.AgentProfileID
		}
		_, err := s.launchPreparedSessionWithDynamicFallback(ctx, task, data.SessionID, executor.LaunchOptions{
			AgentProfileID:       executionProfileID,
			OfficeAgentProfileID: officeAgentProfileID,
			ExecutorID:           "",
			Prompt:               prompt.text,
			StartAgent:           true,
			McpMode:              executor.McpModeOffice,
		})
		if err != nil {
			return dynamicRelaunchFailed
		}
		seam5Res.consume()
		return dynamicRelaunchSucceeded
	default:
		return dynamicRelaunchFailed
	}
}

func (s *Service) startDynamicRelaunchCreatedSession(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	prompt capturedPrompt,
) (*executor.TaskExecution, error) {
	attempt, owner, err := s.beginResumeAttempt(ctx, taskID, session.ID)
	if err != nil {
		return nil, err
	}
	if !owner {
		return nil, fmt.Errorf("%w: dynamic successor startup is already owned", ErrResumeAttemptCancelled)
	}
	defer attempt.finish(s.resumeAttemptStore())

	registry := s.resumeAttemptStore()
	launchCtx := cancellableResumeContext(attempt)
	execution, err := s.startCreatedSession(
		launchCtx, taskID, session.ID, session.AgentProfileID,
		prompt.text, true, prompt.planMode, true, prompt.attachments, nil, "", startCreatedSessionOptions{
			beforeInitialPromptDispatch: func() error {
				if !registry.holdForInitialPrompt(attempt) {
					return ErrResumeAttemptCancelled
				}
				return nil
			},
			onExecutionAdmitted: func(executionID string) {
				attempt.setExecutionID(executionID)
				if s.validateResumeAttempt(attempt) != nil {
					go s.cleanupCancelledResumeAttempt(attempt)
				}
			},
			onInitialPromptAccepted: func(executionID string) {
				if s.acceptResumeAttemptAtPromptAcceptance(executionID, attempt) {
					registry.finishAfterInitialPromptAcceptance(attempt)
				}
			},
			onInitialPromptFailed: func() {
				registry.abortInitialPromptHold(attempt)
			},
		},
	)
	if execution != nil {
		attempt.setExecutionID(execution.AgentExecutionID)
	} else {
		registry.releaseInitialPromptHold(attempt)
	}
	if err != nil {
		registry.abortInitialPromptHold(attempt)
	}
	if attemptErr := s.validateResumeAttempt(attempt); attemptErr != nil {
		s.cleanupCancelledResumeAttempt(attempt)
		return nil, attemptErr
	}
	return execution, err
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
