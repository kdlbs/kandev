package backendapp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAppendSessionModelsMessageFallsBackToPersistedFlatModels(t *testing.T) {
	persistedModels := make([]streams.SessionModelInfo, 15)
	for i := range persistedModels {
		modelID := fmt.Sprintf("claude-model-%d", i+1)
		persistedModels[i] = streams.SessionModelInfo{ModelID: modelID, Name: modelID}
	}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID: "claude-model-1",
				Models:         persistedModels,
			},
		},
	}
	liveState := &lifecycle.CachedModelState{CurrentModelID: "stale-live-model"}

	messages := appendSessionModelsMessageFromState(session.ID, session, liveState, nil)

	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	var payload lifecycle.SessionModelsEventPayload
	if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode session models payload: %v", err)
	}
	if len(payload.Models) != len(persistedModels) {
		t.Fatalf("models = %d, want %d", len(payload.Models), len(persistedModels))
	}
	if payload.CurrentModelID != "claude-model-1" {
		t.Fatalf("current model = %q, want %q", payload.CurrentModelID, "claude-model-1")
	}
}
