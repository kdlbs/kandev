package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type alreadyProjectedProjectionRepository struct {
	recordingAgentDeliveryRepository
}

func (r *alreadyProjectedProjectionRepository) ProjectCanonicalAgentDeliveryEvent(
	context.Context, *models.AgentDeliveryEvent, *models.AgentDeliveryEffect,
) (models.CanonicalAgentDeliveryProjectionResult, error) {
	return models.CanonicalAgentDeliveryProjectionResult{
		CanonicalMessageID: "projected-message",
		AlreadyProjected:   true,
	}, nil
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
func TestAlreadyProjectedSingleEventStillNotifies(t *testing.T) {
	repository := &alreadyProjectedProjectionRepository{}
	execution := &AgentExecution{SessionID: "session-1"}
	event := agentctl.AgentEvent{
		Type:             streams.EventTypeReasoning,
		ReasoningText:    "already projected",
		DeliveryStreamID: "stream-1",
		DeliverySequence: 1,
	}
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	projected, canonicalProjected, notify, err := sm.projectCanonicalAgentEvent(
		context.Background(), execution, event, nil, repository, nil, false,
	)
	if err != nil {
		t.Fatalf("project already-projected event: %v", err)
	}
	if !canonicalProjected || !notify {
		t.Fatalf("projection=%t notify=%t, want the unchanged single-event behavior", canonicalProjected, notify)
	}
	if projected.CanonicalMessageID != "projected-message" {
		t.Fatalf("canonical message ID = %q", projected.CanonicalMessageID)
	}
}
