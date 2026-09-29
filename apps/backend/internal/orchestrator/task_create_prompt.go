package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const metaKeyInitialCreatePromptPassthrough = models.SessionMetaKeyInitialCreatePromptPassthrough

type initialCreatePromptPassthroughQueueContextKey struct{}

func withInitialCreatePromptPassthroughQueue(ctx context.Context) context.Context {
	return context.WithValue(ctx, initialCreatePromptPassthroughQueueContextKey{}, true)
}

func initialCreatePromptPassthroughQueueFromContext(ctx context.Context) bool {
	marked, _ := ctx.Value(initialCreatePromptPassthroughQueueContextKey{}).(bool)
	return marked
}

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
	QueueIncarnationID string `json:"queue_incarnation_id,omitempty"`
	ExecutionID        string `json:"execution_id,omitempty"`
	ExecutionBound     bool   `json:"execution_bound"`
	TurnID             string `json:"turn_id,omitempty"`
	PromptGeneration   uint64 `json:"prompt_generation,omitempty"`
	Consumed           bool   `json:"consumed,omitempty"`
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
	evidence := initialCreatePromptPassthroughEvidence{
		QueueIncarnationID: session.QueueIncarnationID,
		ExecutionID:        executionID,
		ExecutionBound:     bindExecution && executionID != "",
		TurnID:             turnID,
		PromptGeneration:   s.promptGenerationForSession(ctx, session.ID),
	}
	s.initialCreatePromptPassthrough[session.ID] = evidence
	s.persistInitialCreatePromptPassthroughLocked(ctx, session.ID, &evidence)
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
	if s.initialCreatePromptPassthrough == nil {
		s.initialCreatePromptPassthrough = make(map[string]initialCreatePromptPassthroughEvidence)
	}
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
	s.persistInitialCreatePromptPassthroughLocked(ctx, sessionID, &evidence)
}

func (s *Service) persistInitialCreatePromptPassthroughLocked(
	ctx context.Context,
	sessionID string,
	evidence *initialCreatePromptPassthroughEvidence,
) {
	if s.repo == nil || sessionID == "" {
		return
	}
	if err := s.repo.SetSessionMetadataKey(
		context.WithoutCancel(ctx),
		sessionID,
		models.SessionMetaKeyInitialCreatePromptPassthrough,
		evidence,
	); err != nil && s.logger != nil {
		s.logger.Warn("failed to persist initial creation prompt passthrough evidence",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

func (s *Service) clearInitialCreatePromptPassthroughLocked(ctx context.Context, sessionID string) {
	if s.repo == nil || sessionID == "" {
		return
	}
	if err := s.repo.SetSessionMetadataKey(
		context.WithoutCancel(ctx),
		sessionID,
		models.SessionMetaKeyInitialCreatePromptPassthrough,
		nil,
	); err != nil && s.logger != nil {
		s.logger.Warn("failed to clear initial creation prompt passthrough evidence",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

func initialCreatePromptPassthroughEvidenceFromMetadata(
	metadata map[string]interface{},
) (initialCreatePromptPassthroughEvidence, bool) {
	if metadata == nil {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	raw, ok := metadata[models.SessionMetaKeyInitialCreatePromptPassthrough]
	if !ok || raw == nil {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	var evidence initialCreatePromptPassthroughEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	return evidence, true
}

func (s *Service) hydrateInitialCreatePromptPassthrough(
	session *models.TaskSession,
) (initialCreatePromptPassthroughEvidence, bool) {
	if session == nil || session.ID == "" {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	if evidence, ok := s.initialCreatePromptPassthrough[session.ID]; ok {
		return evidence, true
	}
	evidence, ok := initialCreatePromptPassthroughEvidenceFromMetadata(session.Metadata)
	if !ok {
		return initialCreatePromptPassthroughEvidence{}, false
	}
	if s.initialCreatePromptPassthrough == nil {
		s.initialCreatePromptPassthrough = make(map[string]initialCreatePromptPassthroughEvidence)
	}
	s.initialCreatePromptPassthrough[session.ID] = evidence
	return evidence, true
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

	evidence, ok := s.hydrateInitialCreatePromptPassthrough(session)
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
		s.initialCreatePromptMu.Lock()
		delete(s.initialCreatePromptPassthrough, session.ID)
		s.clearInitialCreatePromptPassthroughLocked(ctx, session.ID)
		s.initialCreatePromptMu.Unlock()
		return disposition
	}
	if disposition != initialCreatePromptPassthroughAbsent {
		return disposition
	}
	bindInitialCreatePromptEventIdentity(&evidence, data, liveExecutionID, currentTurnID, currentGeneration)
	s.initialCreatePromptMu.Lock()
	if evidence.Consumed {
		s.initialCreatePromptPassthrough[session.ID] = evidence
		s.persistInitialCreatePromptPassthroughLocked(ctx, session.ID, &evidence)
		s.initialCreatePromptMu.Unlock()
		return initialCreatePromptPassthroughSuppressed
	}
	evidence.Consumed = true
	s.initialCreatePromptPassthrough[session.ID] = evidence
	s.persistInitialCreatePromptPassthroughLocked(ctx, session.ID, &evidence)
	s.initialCreatePromptMu.Unlock()
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
	if evidence.ExecutionBound {
		if !initialCreatePromptExecutionMatches(
			evidence.ExecutionID, data.AgentExecutionID, liveExecutionID,
		) {
			return initialCreatePromptPassthroughStale
		}
	} else if data.AgentExecutionID == "" || liveExecutionID == "" || data.AgentExecutionID != liveExecutionID {
		// Queue admission can arm the marker before the passthrough runtime has
		// claimed an execution. The first running event may race the callback
		// that normally binds it, so accept only the event whose identity also
		// matches the live execution lookup.
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
	if evidence.ExecutionID != "" {
		evidence.ExecutionBound = true
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

func initialCreatePromptTerminalEventMatches(
	evidence initialCreatePromptPassthroughEvidence,
	data watcher.AgentEventData,
	liveExecutionID, currentTurnID string,
) bool {
	if !initialCreatePromptExecutionMatches(evidence.ExecutionID, data.AgentExecutionID, liveExecutionID) {
		return false
	}
	// Terminal lifecycle events are published while the lifecycle manager may
	// still hold its prompt-generation lock. The event's generation is the
	// authoritative identity when present; a zero-generation event still has
	// the exact execution and current turn checks above, so do not synchronously
	// query the manager and risk deadlocking the publisher.
	if evidence.PromptGeneration != 0 && data.PromptGeneration != 0 &&
		evidence.PromptGeneration != data.PromptGeneration {
		return false
	}
	return evidence.TurnID == "" || currentTurnID == "" || evidence.TurnID == currentTurnID
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
	s.hydrateInitialCreatePromptPassthrough(session)
	liveExecutionID := ""
	if s.agentManager != nil {
		liveExecutionID, _ = s.agentManager.GetExecutionIDForSession(ctx, data.SessionID)
	}
	currentTurnID := s.initialCreatePromptCurrentTurnID(ctx, data.SessionID)

	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[data.SessionID]
	if !ok || (evidence.QueueIncarnationID != "" && evidence.QueueIncarnationID != session.QueueIncarnationID) {
		return
	}
	if !evidence.ExecutionBound {
		return
	}
	if !initialCreatePromptTerminalEventMatches(evidence, data, liveExecutionID, currentTurnID) {
		return
	}
	delete(s.initialCreatePromptPassthrough, data.SessionID)
	s.clearInitialCreatePromptPassthroughLocked(ctx, data.SessionID)
}

func (s *Service) retireInitialCreatePromptPassthroughForQueue(
	sessionID, queueIncarnationID, turnID string,
	promptGeneration uint64,
) {
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
	if evidence.ExecutionBound || (evidence.TurnID != "" &&
		(turnID == "" || evidence.TurnID != turnID)) {
		return
	}
	if evidence.PromptGeneration != 0 && promptGeneration != 0 &&
		evidence.PromptGeneration != promptGeneration {
		return
	}
	delete(s.initialCreatePromptPassthrough, sessionID)
	s.clearInitialCreatePromptPassthroughLocked(context.Background(), sessionID)
}

func (s *Service) retireInitialCreatePromptPassthroughForQueueEvent(
	ctx context.Context,
	sessionID, queueIncarnationID, executionID, turnID string,
	promptGeneration uint64,
) {
	if sessionID == "" || executionID == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	if queueIncarnationID == "" {
		queueIncarnationID = session.QueueIncarnationID
	}
	s.hydrateInitialCreatePromptPassthrough(session)
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[sessionID]
	if !ok || !initialCreatePromptQueueEventMatchesEvidence(
		evidence, queueIncarnationID, executionID, turnID, promptGeneration,
	) {
		return
	}
	delete(s.initialCreatePromptPassthrough, sessionID)
	s.clearInitialCreatePromptPassthroughLocked(ctx, sessionID)
}

func initialCreatePromptQueueEventMatchesEvidence(
	evidence initialCreatePromptPassthroughEvidence,
	queueIncarnationID, executionID, turnID string,
	promptGeneration uint64,
) bool {
	if evidence.QueueIncarnationID != "" &&
		(evidence.QueueIncarnationID != queueIncarnationID || queueIncarnationID == "") {
		return false
	}
	if !evidence.ExecutionBound || evidence.ExecutionID != executionID {
		return false
	}
	if evidence.TurnID != "" && (turnID == "" || evidence.TurnID != turnID) {
		return false
	}
	return evidence.PromptGeneration == 0 || promptGeneration == 0 ||
		evidence.PromptGeneration == promptGeneration
}

func (s *Service) clearInitialCreatePromptPassthroughForNewTurn(sessionID, turnID string) {
	if sessionID == "" || turnID == "" {
		return
	}
	// A service restart can leave only the durable marker in the session row.
	// Hydrate it before comparing turns so the first accepted later turn also
	// clears evidence that was not present in the new process's memory.
	s.initialCreatePromptMu.Lock()
	_, inMemory := s.initialCreatePromptPassthrough[sessionID]
	s.initialCreatePromptMu.Unlock()
	if !inMemory && s.repo != nil {
		if session, err := s.repo.GetTaskSession(context.Background(), sessionID); err == nil {
			s.hydrateInitialCreatePromptPassthrough(session)
		}
	}
	s.clearInitialCreatePromptPassthroughForNewTurnInMemory(sessionID, turnID)
}

func (s *Service) clearInitialCreatePromptPassthroughForNewTurnInMemory(sessionID, turnID string) {
	if sessionID == "" || turnID == "" {
		return
	}
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	evidence, ok := s.initialCreatePromptPassthrough[sessionID]
	if ok && evidence.TurnID != "" && evidence.TurnID != turnID {
		delete(s.initialCreatePromptPassthrough, sessionID)
		s.clearInitialCreatePromptPassthroughLocked(context.Background(), sessionID)
	}
}

func (s *Service) clearInitialCreatePromptPassthroughForAcceptedUserTurn(
	ctx context.Context,
	session *models.TaskSession,
) {
	if session == nil || session.ID == "" {
		return
	}
	s.hydrateInitialCreatePromptPassthrough(session)
	s.initialCreatePromptMu.Lock()
	defer s.initialCreatePromptMu.Unlock()
	if _, ok := s.initialCreatePromptPassthrough[session.ID]; !ok {
		return
	}
	delete(s.initialCreatePromptPassthrough, session.ID)
	s.clearInitialCreatePromptPassthroughLocked(ctx, session.ID)
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

// initialCreatePromptPassthroughQueuePending reports whether a destination
// on_enter must wait for the admitted creation prompt. Queue promotion can run
// on the passthrough path before the ordinary queue worker claims its row; an
// automatic step prompt in that gap would otherwise become a second user turn.
func (s *Service) initialCreatePromptPassthroughQueuePending(
	ctx context.Context,
	sessionID string,
) bool {
	if sessionID == "" || s.messageQueue == nil {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return false
	}
	_, ok := s.hydrateInitialCreatePromptPassthrough(session)
	if !ok {
		return false
	}
	status := s.messageQueue.GetStatus(ctx, sessionID)
	for _, entry := range status.Entries {
		if initialCreatePromptPassthroughQueued(entry.Metadata) {
			return true
		}
	}
	return s.isQueuedDispatchInFlight(sessionID)
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
	failure := func(err error) (initialCreatePromptAdmission, error) {
		return initialCreatePromptAdmission{
			SessionID: s.resolveInitialCreatePromptFailureSession(
				ctx, taskID, sessionID, initialWorkflowStepID,
			),
		}, err
	}
	result, err := s.processOnTurnStartAdmission(ctx, taskID, sessionID, true)
	if err != nil {
		return failure(fmt.Errorf("process initial creation prompt turn start: %w", err))
	}

	activeSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return failure(fmt.Errorf("reload initial creation prompt session: %w", err))
	}
	if activeSession == nil {
		return failure(fmt.Errorf("initial creation prompt session %q was not found", sessionID))
	}
	// Profile routing can park the source instead of completing it. Resolve the
	// task's newest active session whenever the prepared session is no longer the
	// primary owner, so the prompt cannot be delivered to a parked predecessor.
	if activeSession.State == models.TaskSessionStateCompleted || !activeSession.IsPrimary {
		activeSession, err = s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
		if err != nil {
			return failure(fmt.Errorf("resolve initial creation prompt replacement session: %w", err))
		}
		if activeSession == nil {
			return failure(fmt.Errorf("initial creation prompt session %q was replaced without an active session", sessionID))
		}
	}
	if activeSession.TaskID != taskID {
		return failure(fmt.Errorf("initial creation prompt session does not belong to task"))
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
		return s.queueInitialCreatePrompt(ctx, req, admission.SessionID)
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
		if errors.Is(err, executor.ErrExecutionAlreadyRunning) {
			return nil, err
		}
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

func (s *Service) queueInitialCreatePrompt(
	ctx context.Context,
	req *LaunchSessionRequest,
	sessionID string,
) (*LaunchSessionResponse, error) {
	if err := s.QueueUserPrompt(
		ctx, req.TaskID, sessionID, req.Prompt, "", req.PlanMode, req.Attachments,
		map[string]interface{}{
			MetaKeyTurnStartAlreadyProcessed:      true,
			metaKeyInitialCreatePromptPassthrough: true,
		}, false,
	); err != nil {
		return nil, s.handleSessionLaunchFailure(
			ctx, req.TaskID, sessionID, fmt.Errorf("queue initial creation prompt: %w", err),
		)
	}
	if session, sessionErr := s.repo.GetTaskSession(ctx, sessionID); sessionErr == nil && session != nil {
		s.armInitialCreatePromptPassthroughForLaunch(
			ctx, session, s.initialCreatePromptCurrentTurnID(ctx, sessionID),
		)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, s.handleSessionLaunchFailure(
			ctx, req.TaskID, sessionID,
			fmt.Errorf("reload queued initial creation prompt session: %w", err),
		)
	}
	return &LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: sessionID,
		State:     string(session.State),
	}, nil
}
