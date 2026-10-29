package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// copyTerminalStorageIdentity attaches validated source evidence and trusted
// executor location context to one terminal payload.
func copyTerminalStorageIdentity(
	payload *AgentEventPayload,
	terminal *streams.AgentEvent,
	execution *AgentExecution,
) {
	if payload == nil || terminal == nil {
		return
	}
	payload.Storage = stampStorageEvidence(modelStorageDiagnostic(terminal.Storage), execution)
}

// stampStorageEvidence adds trusted executor location context to validated
// storage evidence without exposing an address or runtime secret.
func stampStorageEvidence(
	storage *models.AgentErrorStorage,
	execution *AgentExecution,
) *models.AgentErrorStorage {
	if storage == nil || execution == nil {
		return storage
	}
	storage.Location = safeStorageLocation(runtimeStorageLocation(execution))
	storage.ExecutorLabel = safeStorageExecutorLabel(runtimeStorageExecutorLabel(execution))
	return storage
}

func safeStorageLocation(location string) string {
	if location == runtimeStorageLocalLocation {
		return "local_runtime"
	}
	if location == runtimeStorageExecutorLocation {
		return "executor_runtime"
	}
	return string(RecoveryOutcomeUnknown)
}

func safeStorageExecutorLabel(label string) string {
	if len(label) > 64 {
		return ""
	}
	return label
}

// PublishRuntimeStorage publishes one accepted advisory observation through
// the session-authorized gateway route. Healthy polls are filtered by the
// caller; no transcript or metadata record is written.
func (p *EventPublisher) PublishRuntimeStorage(ctx context.Context, observation RuntimeStorageObservation) {
	if p == nil || p.eventBus == nil || observation.SessionID == "" {
		return
	}
	subject := events.BuildSessionRuntimeStorageSubject(observation.SessionID)
	event := bus.NewEvent(subject, "agent-manager", observation)
	if err := p.eventBus.Publish(ctx, subject, event); err != nil {
		p.logger.Warn("failed to publish runtime storage observation",
			zap.String("session_id", observation.SessionID),
			zap.String("state", observation.State),
			zap.Error(err))
	}
}
