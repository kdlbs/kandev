package stepevents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestUnclassifiedStepVetoPayload(t *testing.T) {
	var step models.WorkflowStep
	if err := json.Unmarshal([]byte(`{"id":"step-1","workflow_id":"workflow-1","disable_unclassified_fallback":true}`), &step); err != nil {
		t.Fatalf("unmarshal workflow step: %v", err)
	}
	eventBus := &recordingBus{}
	NewPublisher(eventBus, "test", testLogger(t)).Publish(context.Background(), events.WorkflowStepUpdated, &step)
	if len(eventBus.published) != 1 {
		t.Fatalf("published %d events, want 1", len(eventBus.published))
	}
	data, ok := eventBus.published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event payload type = %T, want map", eventBus.published[0].Data)
	}
	payload, ok := data["step"].(map[string]interface{})
	if !ok {
		t.Fatalf("step payload type = %T, want map", data["step"])
	}
	if got, ok := payload["disable_unclassified_fallback"].(bool); !ok || !got {
		t.Fatalf("disable_unclassified_fallback = %v, want true", payload["disable_unclassified_fallback"])
	}
}
