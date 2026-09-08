package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

type workflowProfileSwitchStopIntentRemover interface {
	RemoveSessionMetadataKeyIfStamp(context.Context, string, string, string) (bool, error)
}

type workflowProfileSwitchStopIntentMarker interface {
	SetSessionMetadataKeyIfStamp(context.Context, string, string, string, interface{}) (bool, error)
}

type workflowProfileSwitchStopConsumed struct {
	expiresAt time.Time
}

type workflowProfileSwitchGuardContextKey struct{}

type workflowProfileSwitchGuardContext struct {
	sessionID           string
	terminalExecutionID string
}

// withWorkflowProfileSwitchGuardHeld carries the ownership already held by a
// caller into the synchronous workflow lifecycle path. Profile switching can
// be reached from handlers that already hold the session guard; reacquiring a
// non-reentrant mutex there would deadlock. Async step-entry work clears this
// marker before it starts, so it acquires the guard itself.
func withWorkflowProfileSwitchGuardHeld(ctx context.Context, sessionID, terminalExecutionID string) context.Context {
	return context.WithValue(ctx, workflowProfileSwitchGuardContextKey{}, workflowProfileSwitchGuardContext{
		sessionID:           sessionID,
		terminalExecutionID: terminalExecutionID,
	})
}

func withoutWorkflowProfileSwitchGuard(ctx context.Context) context.Context {
	return context.WithValue(ctx, workflowProfileSwitchGuardContextKey{}, workflowProfileSwitchGuardContext{})
}

func workflowProfileSwitchGuardIsHeld(ctx context.Context, sessionID string) bool {
	state, _ := ctx.Value(workflowProfileSwitchGuardContextKey{}).(workflowProfileSwitchGuardContext)
	return state.sessionID != "" && state.sessionID == sessionID
}

func workflowProfileSwitchTerminalEventMatches(ctx context.Context, executionID string) bool {
	state, _ := ctx.Value(workflowProfileSwitchGuardContextKey{}).(workflowProfileSwitchGuardContext)
	return state.terminalExecutionID != "" && state.terminalExecutionID == executionID
}

// parkSessionForProfileSwitch keeps the source answerable while ensuring the
// old runtime's terminal event cannot advance the destination workflow step.
// The bool reports whether the source was durably parked. Runtime teardown is
// best effort after the parked state and stop intent are committed.
func (s *Service) parkSessionForProfileSwitch(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (bool, error) {
	if session == nil {
		return false, errors.New("cannot park a nil workflow profile session")
	}
	if workflowProfileSwitchGuardIsHeld(ctx, session.ID) {
		parked, executionID, err := s.parkSessionForProfileSwitchClaimLocked(ctx, taskID, session)
		if err != nil || !parked || executionID == "" {
			return parked, err
		}
		// The caller owns the guard and must keep it through its surrounding
		// lifecycle decision. Schedule teardown after the durable park claim;
		// the caller will release the guard when it returns, allowing a
		// synchronous terminal callback from StopAgent to make progress.
		go s.stopParkedWorkflowProfileSession(context.WithoutCancel(ctx), session.ID, executionID)
		return true, nil
	}

	lock, release := s.acquireCancelInFlightGuard(session.ID)
	lock.Lock()
	ctx = withWorkflowProfileSwitchGuardHeld(ctx, session.ID, "")
	parked, executionID, err := s.parkSessionForProfileSwitchClaimLocked(ctx, taskID, session)
	lock.Unlock()
	release()
	if err != nil || !parked || executionID == "" {
		return parked, err
	}

	// StopAgent can synchronously deliver the terminal callback through the
	// same event bus. The durable claim has already linearized ownership, so
	// release the guard before entering runtime teardown.
	s.stopParkedWorkflowProfileSession(context.WithoutCancel(ctx), session.ID, executionID)
	return true, nil
}

// parkSessionForProfileSwitchClaimLocked performs the park claim while the
// source session's cancel-in-flight guard is held. Terminal lifecycle handlers
// use the same guard, so the exact execution either reaches a terminal marker
// before this claim or its delayed callback observes the durable stop intent
// after this method has committed the parked state.
func (s *Service) parkSessionForProfileSwitchClaimLocked(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (bool, string, error) {
	if session == nil {
		return false, "", errors.New("cannot park a nil workflow profile session")
	}

	if !workflowProfileSwitchGuardIsHeld(ctx, session.ID) {
		lock, release := s.acquireCancelInFlightGuard(session.ID)
		defer release()
		lock.Lock()
		defer lock.Unlock()
		ctx = withWorkflowProfileSwitchGuardHeld(ctx, session.ID, "")
	}
	currentSession, executionID, err := s.loadProfileSwitchParkCandidate(ctx, session.ID)
	if err != nil {
		return false, "", err
	}

	// Claim the exact runtime while the source session guard is still held.
	// This is the linearization point shared with terminal callbacks: a
	// callback that acquired the guard first marks the execution naturally
	// complete and makes the candidate invalid, while a callback that arrives
	// after this claim is an acknowledgement of the deliberate park stop.
	// Keep the claim until the runtime teardown has had its chance to report a
	// terminal event. Release it only when the durable park cannot be committed.
	teardownClaimed := executionID != ""
	if teardownClaimed && !s.claimExecutionTeardown(
		currentSession.ID,
		executionID,
		executionTeardownIntentGraceful,
	) {
		return false, "", fmt.Errorf("cannot park workflow profile session %q: execution %q already has a teardown owner", currentSession.ID, executionID)
	}
	releaseTeardownClaim := func() {
		if teardownClaimed {
			s.releaseExecutionTeardownClaim(currentSession.ID, executionID)
		}
	}

	intent, err := s.recordProfileSwitchStopIntent(ctx, currentSession.ID, executionID)
	if err != nil {
		releaseTeardownClaim()
		return false, "", err
	}

	changed, finalState, err := s.transitionTaskSessionState(
		ctx,
		taskID,
		currentSession.ID,
		models.TaskSessionStateWaitingForInput,
		"",
		nil,
	)
	if err != nil {
		s.clearParkedProfileSwitchIntent(ctx, currentSession.ID, intent.Stamp)
		releaseTeardownClaim()
		return false, "", fmt.Errorf("park workflow profile session %q: %w", currentSession.ID, err)
	}
	if !changed && finalState != models.TaskSessionStateWaitingForInput {
		s.clearParkedProfileSwitchIntent(ctx, currentSession.ID, intent.Stamp)
		releaseTeardownClaim()
		return false, "", fmt.Errorf("park workflow profile session %q: state changed to %s", currentSession.ID, finalState)
	}

	// There is no runtime to stop after a backend restart or a prior teardown.
	// The session is still safely answerable in WAITING_FOR_INPUT.
	if executionID == "" {
		return true, "", nil
	}
	return true, executionID, nil
}

// loadProfileSwitchParkCandidate reloads the source under the caller's
// per-session lifecycle guard and validates that its exact runtime execution
// has not already been claimed by a natural terminal event.
func (s *Service) loadProfileSwitchParkCandidate(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, string, error) {
	currentSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("reload workflow profile session %q before park: %w", sessionID, err)
	}
	if currentSession == nil {
		return nil, "", fmt.Errorf("reload workflow profile session %q before park: session is nil", sessionID)
	}
	if isTerminalSessionState(currentSession.State) {
		return nil, "", fmt.Errorf("cannot park workflow profile session %q after it became %s", sessionID, currentSession.State)
	}

	executionID, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if lookupErr != nil && !executor.IsNoExecutionForSessionError(lookupErr) {
		return nil, "", fmt.Errorf("look up runtime for parked session %q: %w", sessionID, lookupErr)
	}
	if executionID != "" && s.isExecutionCompleted(sessionID, executionID) &&
		!workflowProfileSwitchTerminalEventMatches(ctx, executionID) {
		return nil, "", fmt.Errorf("cannot park workflow profile session %q after execution %q reached a natural terminal event", sessionID, executionID)
	}
	return currentSession, executionID, nil
}

// recordProfileSwitchStopIntent stamps the exact execution before the source
// state changes. The marker is consumed by delayed terminal callbacks and is
// cleared only when the state transition cannot be committed.
func (s *Service) recordProfileSwitchStopIntent(
	ctx context.Context,
	sessionID string,
	executionID string,
) (models.WorkflowProfileSwitchStopIntent, error) {
	if executionID == "" {
		return models.WorkflowProfileSwitchStopIntent{}, nil
	}
	if _, ok := s.repo.(workflowProfileSwitchStopIntentRemover); !ok {
		return models.WorkflowProfileSwitchStopIntent{}, fmt.Errorf("parked workflow profile switch requires stamped session metadata support")
	}
	if _, ok := s.repo.(workflowProfileSwitchStopIntentMarker); !ok {
		return models.WorkflowProfileSwitchStopIntent{}, fmt.Errorf("parked workflow profile switch requires stamped session metadata support")
	}

	intent := models.WorkflowProfileSwitchStopIntent{
		ExecutionID: executionID,
		Stamp:       uuid.NewString(),
	}
	if err := s.repo.SetSessionMetadataKey(
		ctx,
		sessionID,
		models.SessionMetaKeyWorkflowProfileSwitchStopIntent,
		intent,
	); err != nil {
		return models.WorkflowProfileSwitchStopIntent{}, fmt.Errorf("record parked workflow profile switch: %w", err)
	}
	return intent, nil
}

func (s *Service) stopParkedWorkflowProfileSession(ctx context.Context, sessionID, executionID string) {
	if s.agentManager == nil || executionID == "" {
		return
	}
	if err := s.agentManager.StopAgent(ctx, executionID, false); err != nil {
		s.logger.Warn("failed to stop runtime for parked workflow profile session",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (s *Service) clearParkedProfileSwitchIntent(ctx context.Context, sessionID, stamp string) {
	if strings.TrimSpace(stamp) == "" {
		return
	}
	remover, ok := s.repo.(workflowProfileSwitchStopIntentRemover)
	if !ok {
		return
	}
	if _, err := remover.RemoveSessionMetadataKeyIfStamp(
		ctx,
		sessionID,
		models.SessionMetaKeyWorkflowProfileSwitchStopIntent,
		stamp,
	); err != nil {
		s.logger.Warn("failed to clear abandoned parked workflow profile switch intent",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// consumeParkedProfileSwitchStopIntent returns true only for the execution
// recorded by the parked switch. It marks the matching intent consumed with a
// stamped compare-and-set and keeps that tombstone durable. The in-memory
// marker is only an optimization for duplicate deliveries in this process.
func (s *Service) consumeParkedProfileSwitchStopIntent(
	ctx context.Context,
	data watcher.AgentEventData,
	preloadedSession *models.TaskSession,
) bool {
	if data.SessionID == "" || data.AgentExecutionID == "" {
		return false
	}
	key := terminalExecutionKey(data.SessionID, data.AgentExecutionID)
	if s.parkedProfileSwitchStopWasConsumed(key) {
		return true
	}

	session := preloadedSession
	if session == nil {
		var err error
		session, err = s.repo.GetTaskSession(ctx, data.SessionID)
		if err != nil || session == nil {
			return false
		}
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || intent.ExecutionID != data.AgentExecutionID {
		return false
	}

	// A durable consumed tombstone handles callbacks after a process restart.
	// Re-arm the short-lived in-memory marker so same-process duplicates avoid a
	// metadata round trip.
	if intent.Consumed {
		s.rememberParkedProfileSwitchStop(key)
		return true
	}

	// Claim the execution before the database compare-and-set. A duplicate event
	// racing this one must fail closed while the first callback marks the
	// durable tombstone.
	s.rememberParkedProfileSwitchStop(key)
	marker, ok := s.repo.(workflowProfileSwitchStopIntentMarker)
	if !ok {
		s.logger.Error("cannot consume parked workflow profile switch intent: repository lacks stamped metadata marking",
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID))
		return true
	}
	intent.Consumed = true
	marked, err := marker.SetSessionMetadataKeyIfStamp(
		ctx,
		data.SessionID,
		models.SessionMetaKeyWorkflowProfileSwitchStopIntent,
		intent.Stamp,
		intent,
	)
	if err != nil {
		s.logger.Error("failed to mark parked workflow profile switch intent consumed; suppressing lifecycle transition",
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID),
			zap.String("stop_intent_stamp", intent.Stamp),
			zap.Error(err))
		return true
	}
	if !marked {
		s.logger.Debug("parked workflow profile switch intent was already consumed or superseded",
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID),
			zap.String("stop_intent_stamp", intent.Stamp))
		return true
	}
	s.logger.Debug("marked parked workflow profile switch intent consumed",
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("stop_intent_stamp", intent.Stamp))
	return true
}

func workflowProfileSwitchStopIntentFromMetadata(
	metadata map[string]interface{},
) (models.WorkflowProfileSwitchStopIntent, bool) {
	if metadata == nil {
		return models.WorkflowProfileSwitchStopIntent{}, false
	}
	switch value := metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent].(type) {
	case models.WorkflowProfileSwitchStopIntent:
		return validWorkflowProfileSwitchStopIntent(value)
	case *models.WorkflowProfileSwitchStopIntent:
		if value == nil {
			return models.WorkflowProfileSwitchStopIntent{}, false
		}
		return validWorkflowProfileSwitchStopIntent(*value)
	case map[string]interface{}:
		return validWorkflowProfileSwitchStopIntent(models.WorkflowProfileSwitchStopIntent{
			ExecutionID: stringMetadataValue(value["execution_id"]),
			Stamp:       stringMetadataValue(value["stamp"]),
			Consumed:    boolMetadataValue(value["consumed"]),
		})
	case map[string]string:
		return validWorkflowProfileSwitchStopIntent(models.WorkflowProfileSwitchStopIntent{
			ExecutionID: value["execution_id"],
			Stamp:       value["stamp"],
			Consumed:    value["consumed"] == "true",
		})
	default:
		return models.WorkflowProfileSwitchStopIntent{}, false
	}
}

func validWorkflowProfileSwitchStopIntent(
	intent models.WorkflowProfileSwitchStopIntent,
) (models.WorkflowProfileSwitchStopIntent, bool) {
	if strings.TrimSpace(intent.ExecutionID) == "" || strings.TrimSpace(intent.Stamp) == "" {
		return models.WorkflowProfileSwitchStopIntent{}, false
	}
	return intent, true
}

func stringMetadataValue(value interface{}) string {
	valueString, _ := value.(string)
	return valueString
}

func boolMetadataValue(value interface{}) bool {
	valueBool, _ := value.(bool)
	return valueBool
}

func (s *Service) rememberParkedProfileSwitchStop(key string) {
	expiresAt := time.Now().Add(completedExecutionRetention)
	s.parkedProfileSwitchStops.Store(key, workflowProfileSwitchStopConsumed{expiresAt: expiresAt})
	time.AfterFunc(completedExecutionRetention, func() {
		value, ok := s.parkedProfileSwitchStops.Load(key)
		if !ok {
			return
		}
		claim, ok := value.(workflowProfileSwitchStopConsumed)
		if !ok || !claim.expiresAt.After(expiresAt) {
			s.parkedProfileSwitchStops.Delete(key)
		}
	})
}

func (s *Service) parkedProfileSwitchStopWasConsumed(key string) bool {
	value, ok := s.parkedProfileSwitchStops.Load(key)
	if !ok {
		return false
	}
	claim, ok := value.(workflowProfileSwitchStopConsumed)
	if !ok || time.Now().After(claim.expiresAt) {
		s.parkedProfileSwitchStops.Delete(key)
		return false
	}
	return true
}
