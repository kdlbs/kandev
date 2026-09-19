package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

const metaKeyInitialCreatePromptPassthrough = "initial_create_prompt_passthrough"

func initialCreatePromptPassthroughQueued(metadata map[string]interface{}) bool {
	marked, _ := metadata[metaKeyInitialCreatePromptPassthrough].(bool)
	return marked
}

type initialCreatePromptAdmission struct {
	SessionID string
	Queued    bool
}

// initialCreatePromptPassthroughEvidence binds creation-admission evidence to
// the execution and turn that admitted the prompt. Queue incarnation alone is
// insufficient because an execution can be replaced without replacing the
// session.
type initialCreatePromptPassthroughEvidence struct {
	QueueIncarnationID string
	ExecutionID        string
	ExecutionBound     bool
	TurnID             string
	PromptGeneration   uint64
	Consumed           bool
}

type initialCreatePromptPassthroughDisposition uint8

const (
	initialCreatePromptPassthroughAbsent initialCreatePromptPassthroughDisposition = iota
	initialCreatePromptPassthroughSuppressed
	initialCreatePromptPassthroughStale
	initialCreatePromptPassthroughNewTurn
)

type workflowTransitionErrorContextKey struct{}

type workflowTransitionErrorCapture struct {
	err error
}

func withWorkflowTransitionErrorCapture(ctx context.Context, capture *workflowTransitionErrorCapture) context.Context {
	return context.WithValue(ctx, workflowTransitionErrorContextKey{}, capture)
}

func recordWorkflowTransitionError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	capture, _ := ctx.Value(workflowTransitionErrorContextKey{}).(*workflowTransitionErrorCapture)
	if capture != nil && capture.err == nil {
		capture.err = err
	}
}

func (s *Service) armInitialCreatePromptPassthrough(
	ctx context.Context,
	session *models.TaskSession,
	turnIDs ...string,
) {
	s.armInitialCreatePromptPassthroughWithExecution(ctx, session, true, turnIDs...)
}

// armInitialCreatePromptPassthroughForLaunch leaves execution identity
// unbound until the executor admits the execution that will receive the
// prompt. This prevents a delayed predecessor event from consuming or
// retiring the marker while a prepared workspace is being replaced.
func (s *Service) armInitialCreatePromptPassthroughForLaunch(
	ctx context.Context,
	session *models.TaskSession,
	turnIDs ...string,
) {
	s.armInitialCreatePromptPassthroughWithExecution(ctx, session, false, turnIDs...)
}

func (s *Service) armInitialCreatePromptPassthroughWithExecution(
	ctx context.Context,
	session *models.TaskSession,
	bindExecution bool,
	turnIDs ...string,
) {
	if session == nil || session.ID == "" {
		return
	}
	turnID := ""
	if len(turnIDs) > 0 {
		turnID = turnIDs[0]
	}
	if turnID == "" {
		turnID = s.initialCreatePromptCurrentTurnID(ctx, session.ID)
	}
	executionID := ""
	if bindExecution && s.agentManager != nil {
		executionID, _ = s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	if s.initialCreatePromptPassthrough == nil {
		s.initialCreatePromptPassthrough = make(map[string]initialCreatePromptPassthroughEvidence)
	}
	s.initialCreatePromptPassthrough[session.ID] = initialCreatePromptPassthroughEvidence{
		QueueIncarnationID: session.QueueIncarnationID,
		ExecutionID:        executionID,
		ExecutionBound:     bindExecution && executionID != "",
		TurnID:             turnID,
		PromptGeneration:   s.promptGenerationForSession(ctx, session.ID),
	}
}

func (s *Service) bindInitialCreatePromptPassthroughExecution(
	ctx context.Context,
	sessionID, turnID, executionID string,
) {
	if sessionID == "" {
		return
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[sessionID]
	if !ok {
		return
	}
	if evidence.TurnID != "" && turnID != "" && evidence.TurnID != turnID {
		return
	}
	if evidence.TurnID == "" {
		evidence.TurnID = turnID
	}
	if executionID != "" && evidence.ExecutionID != executionID {
		// The executor calls this at the admission boundary. A prepared
		// workspace can be replaced while retaining the session and queue
		// incarnation, so the admitted execution supersedes any predecessor
		// identity captured while the marker was armed.
		evidence.ExecutionID = executionID
		evidence.Consumed = false
	}
	if executionID != "" {
		evidence.ExecutionBound = true
	}
	if evidence.PromptGeneration == 0 {
		evidence.PromptGeneration = s.promptGenerationForSession(ctx, sessionID)
	}
	s.initialCreatePromptPassthrough[sessionID] = evidence
}

// initialCreatePromptCurrentTurnID reads the current turn without creating a
// turn. Creating one while validating a lifecycle event would make a delayed
// event look like a new user turn.
func (s *Service) initialCreatePromptCurrentTurnID(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	if value, ok := s.activeTurns.Load(sessionID); ok {
		if turnID, ok := value.(string); ok && turnID != "" {
			return turnID
		}
	}
	turnID, _ := s.peekActiveTurnID(ctx, sessionID)
	return turnID
}

// consumeInitialCreatePromptPassthrough classifies one agent.running event.
// A stale event is handled, but deliberately does not mutate the evidence.
// Once the matching event is seen, the evidence remains consumed until the
// matching execution/turn ends, so duplicate running notifications cannot be
// mistaken for another user turn.
func (s *Service) consumeInitialCreatePromptPassthrough(
	ctx context.Context,
	session *models.TaskSession,
	data watcher.AgentEventData,
) initialCreatePromptPassthroughDisposition {
	if session == nil || session.ID == "" {
		return initialCreatePromptPassthroughAbsent
	}
	liveExecutionID := ""
	if s.agentManager != nil {
		liveExecutionID, _ = s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	}
	currentTurnID := s.initialCreatePromptCurrentTurnID(ctx, session.ID)
	currentGeneration := s.promptGenerationForSession(ctx, session.ID)

	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[session.ID]
	if !ok {
		return initialCreatePromptPassthroughAbsent
	}
	disposition := initialCreatePromptEvidenceDisposition(
		evidence,
		session.QueueIncarnationID,
		data,
		liveExecutionID,
		currentTurnID,
		currentGeneration,
	)
	if disposition == initialCreatePromptPassthroughNewTurn {
		delete(s.initialCreatePromptPassthrough, session.ID)
		return disposition
	}
	if disposition != initialCreatePromptPassthroughAbsent {
		return disposition
	}
	bindInitialCreatePromptEventIdentity(&evidence, data, liveExecutionID, currentTurnID, currentGeneration)
	if evidence.Consumed {
		s.initialCreatePromptPassthrough[session.ID] = evidence
		return initialCreatePromptPassthroughSuppressed
	}
	evidence.Consumed = true
	s.initialCreatePromptPassthrough[session.ID] = evidence
	return initialCreatePromptPassthroughSuppressed
}

func initialCreatePromptEvidenceDisposition(
	evidence initialCreatePromptPassthroughEvidence,
	queueIncarnationID string,
	data watcher.AgentEventData,
	liveExecutionID, currentTurnID string,
	currentGeneration uint64,
) initialCreatePromptPassthroughDisposition {
	if evidence.QueueIncarnationID != "" && evidence.QueueIncarnationID != queueIncarnationID {
		return initialCreatePromptPassthroughStale
	}
	if !evidence.ExecutionBound || !initialCreatePromptExecutionMatches(
		evidence.ExecutionID, data.AgentExecutionID, liveExecutionID,
	) {
		return initialCreatePromptPassthroughStale
	}
	if initialCreatePromptGenerationMatches(evidence.PromptGeneration, data.PromptGeneration, currentGeneration) {
		if evidence.TurnID != "" && currentTurnID != "" && evidence.TurnID != currentTurnID {
			// Running events carry no turn ID. A turn change is actionable only
			// when its prompt generation identifies the new turn. Otherwise this
			// can be a duplicate or delayed event for the old turn.
			return initialCreatePromptPassthroughStale
		}
		return initialCreatePromptPassthroughAbsent
	}
	if evidence.PromptGeneration != 0 && currentGeneration == data.PromptGeneration && data.PromptGeneration != 0 {
		return initialCreatePromptPassthroughNewTurn
	}
	return initialCreatePromptPassthroughStale
}

func bindInitialCreatePromptEventIdentity(
	evidence *initialCreatePromptPassthroughEvidence,
	data watcher.AgentEventData,
	liveExecutionID, currentTurnID string,
	currentGeneration uint64,
) {
	if evidence.TurnID == "" {
		evidence.TurnID = currentTurnID
	}
	if evidence.ExecutionID == "" {
		evidence.ExecutionID = data.AgentExecutionID
		if evidence.ExecutionID == "" {
			evidence.ExecutionID = liveExecutionID
		}
	}
	if evidence.PromptGeneration == 0 {
		evidence.PromptGeneration = data.PromptGeneration
		if evidence.PromptGeneration == 0 {
			evidence.PromptGeneration = currentGeneration
		}
	}
}

func initialCreatePromptExecutionMatches(expected, event, live string) bool {
	// A bound marker is consumed only by an event that carries the exact
	// admitted execution. The live lookup is a second consistency check; it
	// cannot substitute for an event identity because a delayed predecessor
	// notification may arrive while the successor is live.
	if expected == "" || event == "" || event != expected {
		return false
	}
	return live == "" || live == event
}

func initialCreatePromptGenerationMatches(expected, event, current uint64) bool {
	if expected == 0 {
		return event == 0 || current == 0 || event == current
	}
	if event != 0 && event != expected {
		return false
	}
	return event != 0 || current == 0 || current == expected
}

func (s *Service) retireInitialCreatePromptPassthroughForEvent(
	ctx context.Context,
	data watcher.AgentEventData,
) {
	if data.SessionID == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		return
	}
	liveExecutionID := ""
	if s.agentManager != nil {
		liveExecutionID, _ = s.agentManager.GetExecutionIDForSession(ctx, data.SessionID)
	}
	currentTurnID := s.initialCreatePromptCurrentTurnID(ctx, data.SessionID)
	currentGeneration := s.promptGenerationForSession(ctx, data.SessionID)

	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[data.SessionID]
	if !ok || (evidence.QueueIncarnationID != "" && evidence.QueueIncarnationID != session.QueueIncarnationID) {
		return
	}
	if !evidence.ExecutionBound {
		return
	}
	if !initialCreatePromptExecutionMatches(evidence.ExecutionID, data.AgentExecutionID, liveExecutionID) ||
		!initialCreatePromptGenerationMatches(evidence.PromptGeneration, data.PromptGeneration, currentGeneration) {
		return
	}
	if evidence.TurnID != "" && currentTurnID != "" && evidence.TurnID != currentTurnID {
		return
	}
	delete(s.initialCreatePromptPassthrough, data.SessionID)
}

func (s *Service) retireInitialCreatePromptPassthroughForQueue(sessionID, queueIncarnationID string) {
	if sessionID == "" {
		return
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[sessionID]
	if !ok || (evidence.QueueIncarnationID != "" &&
		(queueIncarnationID == "" || evidence.QueueIncarnationID != queueIncarnationID)) {
		return
	}
	delete(s.initialCreatePromptPassthrough, sessionID)
}

func (s *Service) retireInitialCreatePromptPassthroughForQueueEvent(
	ctx context.Context,
	sessionID, queueIncarnationID string,
) {
	if sessionID == "" {
		return
	}
	if queueIncarnationID == "" {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			return
		}
		queueIncarnationID = session.QueueIncarnationID
	}
	s.retireInitialCreatePromptPassthroughForQueue(sessionID, queueIncarnationID)
}

func (s *Service) clearInitialCreatePromptPassthroughForNewTurn(sessionID, turnID string) {
	if sessionID == "" || turnID == "" {
		return
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[sessionID]
	if ok && evidence.TurnID != "" && evidence.TurnID != turnID {
		delete(s.initialCreatePromptPassthrough, sessionID)
	}
}

func (s *Service) armQueuedInitialCreatePromptPassthrough(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
	identity messagequeue.QueueSessionIdentity,
) {
	if queuedMsg == nil || !initialCreatePromptPassthroughQueued(queuedMsg.Metadata) || s.agentManager == nil {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, queuedMsg.SessionID)
	if err != nil || !s.queuedSessionMatchesIdentity(session, identity) ||
		!s.agentManager.IsPassthroughSession(ctx, session.ID) {
		return
	}
	s.armInitialCreatePromptPassthrough(ctx, session, s.initialCreatePromptCurrentTurnID(ctx, session.ID))
}

func (s *Service) armQueuedInitialCreatePromptPassthroughForLaunch(
	ctx context.Context,
	queuedMsg *messagequeue.QueuedMessage,
	identity messagequeue.QueueSessionIdentity,
) {
	if queuedMsg == nil || !initialCreatePromptPassthroughQueued(queuedMsg.Metadata) || s.agentManager == nil {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, queuedMsg.SessionID)
	if err != nil || !s.queuedSessionMatchesIdentity(session, identity) ||
		!s.agentManager.IsPassthroughSession(ctx, session.ID) {
		return
	}
	// Queue admission has accepted the message, but the passthrough runtime
	// that will publish agent.running has not been claimed yet. Keep the marker
	// inert until deliverQueuedPassthroughPrompt binds that exact execution.
	s.armInitialCreatePromptPassthroughForLaunch(ctx, session, s.initialCreatePromptCurrentTurnID(ctx, session.ID))
}

// admitInitialCreatePrompt runs the explicit creation prompt through the same
// turn-start boundary as an ordinary user message and resolves the session that
// owns the resulting workflow step. The caller must pass the original prepared
// session; a profile switch may retire that session during admission.
func (s *Service) admitInitialCreatePrompt(
	ctx context.Context,
	taskID, sessionID string,
) (initialCreatePromptAdmission, error) {
	initialWorkflowStepID := ""
	if task, taskErr := s.repo.GetTask(ctx, taskID); taskErr == nil && task != nil {
		initialWorkflowStepID = task.WorkflowStepID
	}
	result, err := s.processOnTurnStartAdmission(ctx, taskID, sessionID, true)
	if err != nil {
		return initialCreatePromptAdmission{
			SessionID: s.resolveInitialCreatePromptFailureSession(
				ctx, taskID, sessionID, initialWorkflowStepID,
			),
		}, fmt.Errorf("process initial creation prompt turn start: %w", err)
	}

	activeSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return initialCreatePromptAdmission{}, fmt.Errorf("reload initial creation prompt session: %w", err)
	}
	if activeSession == nil {
		return initialCreatePromptAdmission{}, fmt.Errorf("initial creation prompt session %q was not found", sessionID)
	}
	// Profile routing can park the source instead of completing it. Resolve the
	// task's newest active session whenever the prepared session is no longer the
	// primary owner, so the prompt cannot be delivered to a parked predecessor.
	if activeSession.State == models.TaskSessionStateCompleted || !activeSession.IsPrimary {
		activeSession, err = s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if err != nil {
			return initialCreatePromptAdmission{}, fmt.Errorf("resolve initial creation prompt replacement session: %w", err)
		}
		if activeSession == nil {
			return initialCreatePromptAdmission{}, fmt.Errorf("initial creation prompt session %q was replaced without an active session", sessionID)
		}
	}
	if activeSession.TaskID != taskID {
		return initialCreatePromptAdmission{}, fmt.Errorf("initial creation prompt session does not belong to task")
	}
	return initialCreatePromptAdmission{
		SessionID: activeSession.ID,
		Queued:    result.Queued,
	}, nil
}

// resolveInitialCreatePromptFailureSession returns the session that still owns
// a failed strict admission. A source that was parked or completed during the
// transition no longer owns the prompt; only a nonterminal primary successor
// whose workflow step changed during this admission can receive the error. If
// the source was independently cancelled or superseded, return no owner so a
// stale creation error cannot terminalize an unrelated successor.
func (s *Service) resolveInitialCreatePromptFailureSession(
	ctx context.Context,
	taskID, sourceSessionID, initialWorkflowStepID string,
) string {
	source, err := s.repo.GetTaskSession(ctx, sourceSessionID)
	if err != nil || source == nil {
		return ""
	}
	if source.TaskID != taskID {
		return ""
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ""
	}
	if initialCreatePromptSourceOwnsFailure(source, task.WorkflowStepID, initialWorkflowStepID) {
		return source.ID
	}
	if task.WorkflowStepID == initialWorkflowStepID {
		return ""
	}
	active, err := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || !initialCreatePromptSuccessorOwnsFailure(active, source.ID, taskID) {
		return ""
	}
	return active.ID
}

func initialCreatePromptSourceOwnsFailure(
	source *models.TaskSession,
	currentWorkflowStepID, initialWorkflowStepID string,
) bool {
	return models.IsTaskLookupActiveSessionState(source.State) &&
		(currentWorkflowStepID == initialWorkflowStepID || source.IsPrimary)
}

func initialCreatePromptSuccessorOwnsFailure(
	active *models.TaskSession, sourceSessionID, taskID string,
) bool {
	return active != nil && active.ID != sourceSessionID && active.TaskID == taskID &&
		active.IsPrimary && models.IsTaskLookupActiveSessionState(active.State)
}

func (s *Service) launchInitialCreatePrompt(
	ctx context.Context,
	req *LaunchSessionRequest,
) (*LaunchSessionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("initial creation prompt requires non-empty prompt")
	}
	parkingStamp := s.captureWorkflowParkingStamp(ctx, req.SessionID)
	admission, err := s.admitInitialCreatePrompt(ctx, req.TaskID, req.SessionID)
	if err != nil {
		return nil, s.handleSessionLaunchFailure(ctx, req.TaskID, admission.SessionID, err)
	}

	if admission.Queued {
		if err := s.QueueUserPrompt(
			ctx,
			req.TaskID,
			admission.SessionID,
			req.Prompt,
			"",
			req.PlanMode,
			req.Attachments,
			map[string]interface{}{
				MetaKeyTurnStartAlreadyProcessed:      true,
				metaKeyInitialCreatePromptPassthrough: true,
			},
			false,
		); err != nil {
			return nil, s.handleSessionLaunchFailure(
				ctx, req.TaskID, admission.SessionID,
				fmt.Errorf("queue initial creation prompt: %w", err),
			)
		}
		session, sessionErr := s.repo.GetTaskSession(ctx, admission.SessionID)
		if sessionErr != nil {
			return nil, s.handleSessionLaunchFailure(
				ctx, req.TaskID, admission.SessionID,
				fmt.Errorf("reload queued initial creation prompt session: %w", sessionErr),
			)
		}
		return &LaunchSessionResponse{
			Success:   true,
			TaskID:    req.TaskID,
			SessionID: admission.SessionID,
			State:     string(session.State),
		}, nil
	}

	initialReq := *req
	initialReq.SessionID = admission.SessionID
	initialReq.InitialCreatePrompt = false
	autoStart := initialReq.AutoStart || initialReq.ActivationSource == LaunchActivationSourceSessionOpen
	execution, err := s.startCreatedSession(
		ctx,
		initialReq.TaskID,
		initialReq.SessionID,
		initialReq.AgentProfileID,
		initialReq.Prompt,
		initialReq.SkipMessageRecord,
		initialReq.PlanMode,
		autoStart,
		initialReq.Attachments,
		nil,
		"",
		startCreatedSessionOptions{initialCreatePrompt: true},
	)
	if err != nil {
		return nil, s.handleSessionLaunchFailure(ctx, req.TaskID, admission.SessionID, err)
	}
	if execution != nil {
		s.bindInitialCreatePromptPassthroughExecution(
			ctx,
			admission.SessionID,
			s.initialCreatePromptCurrentTurnID(ctx, admission.SessionID),
			execution.AgentExecutionID,
		)
		s.clearWorkflowParkingForSession(ctx, admission.SessionID, parkingStamp)
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}
