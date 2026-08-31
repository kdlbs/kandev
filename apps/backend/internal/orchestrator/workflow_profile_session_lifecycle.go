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

	executionID, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if lookupErr != nil && !executor.IsNoExecutionForSessionError(lookupErr) {
		return false, fmt.Errorf("look up runtime for parked session %q: %w", session.ID, lookupErr)
	}

	var intent models.WorkflowProfileSwitchStopIntent
	if executionID != "" {
		if _, ok := s.repo.(workflowProfileSwitchStopIntentRemover); !ok {
			return false, fmt.Errorf("parked workflow profile switch requires stamped session metadata support")
		}
		if _, ok := s.repo.(workflowProfileSwitchStopIntentMarker); !ok {
			return false, fmt.Errorf("parked workflow profile switch requires stamped session metadata support")
		}
		intent = models.WorkflowProfileSwitchStopIntent{
			ExecutionID: executionID,
			Stamp:       uuid.NewString(),
		}
		if err := s.repo.SetSessionMetadataKey(
			ctx,
			session.ID,
			models.SessionMetaKeyWorkflowProfileSwitchStopIntent,
			intent,
		); err != nil {
			return false, fmt.Errorf("record parked workflow profile switch: %w", err)
		}
	}

	changed, finalState, err := s.transitionTaskSessionState(
		ctx,
		taskID,
		session.ID,
		models.TaskSessionStateWaitingForInput,
		"",
		nil,
	)
	if err != nil {
		s.clearParkedProfileSwitchIntent(ctx, session.ID, intent.Stamp)
		return false, fmt.Errorf("park workflow profile session %q: %w", session.ID, err)
	}
	if !changed && finalState != models.TaskSessionStateWaitingForInput {
		s.clearParkedProfileSwitchIntent(ctx, session.ID, intent.Stamp)
		return false, fmt.Errorf("park workflow profile session %q: state changed to %s", session.ID, finalState)
	}

	// There is no runtime to stop after a backend restart or a prior teardown.
	// The session is still safely answerable in WAITING_FOR_INPUT.
	if executionID == "" {
		return true, nil
	}
	if err := s.agentManager.StopAgent(ctx, executionID, false); err != nil {
		s.logger.Warn("failed to stop runtime for parked workflow profile session",
			zap.String("session_id", session.ID),
			zap.Error(err))
	}
	return true, nil
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
