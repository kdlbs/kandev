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
	liveState := &lifecycle.CachedModelState{CurrentModelID: "live-model"}

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
	if payload.CurrentModelID != "live-model" {
		t.Fatalf("current model = %q, want %q", payload.CurrentModelID, "live-model")
	}
}

func TestAppendSessionModelsMessageUsesPersistedConfigAfterCacheRestart(t *testing.T) {
	model := streams.SessionModelInfo{ModelID: "mock-fast", Name: "Mock Fast"}
	option := streams.ConfigOption{
		Type:         "select",
		ID:           "model",
		Name:         "Model",
		CurrentValue: "mock-fast",
		Category:     "model",
	}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID:       model.ModelID,
				Models:               []streams.SessionModelInfo{model},
				ConfigOptions:        []streams.ConfigOption{option},
				ConfigOptionsSettled: true,
			},
		},
	}

	messages := appendSessionModelsMessageFromState(session.ID, session, nil, nil)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	var payload lifecycle.SessionModelsEventPayload
	if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode session models payload: %v", err)
	}
	if payload.CurrentModelID != model.ModelID {
		t.Fatalf("current model = %q, want %q", payload.CurrentModelID, model.ModelID)
	}
	if len(payload.Models) != 1 || payload.Models[0].Name != model.Name {
		t.Fatalf("models = %#v, want persisted model %q", payload.Models, model.Name)
	}
	if len(payload.ConfigOptions) != 1 || payload.ConfigOptions[0].CurrentValue != option.CurrentValue {
		t.Fatalf("config options = %#v, want persisted config option", payload.ConfigOptions)
	}
	if !payload.ConfigOptionsSettled {
		t.Fatal("config options settled = false, want true")
	}
}

func TestAppendSessionModelsMessageKeepsPersistedSelectionWithoutStaleConfig(t *testing.T) {
	model := streams.SessionModelInfo{ModelID: "persisted-model", Name: "Persisted model"}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID: model.ModelID,
				Models:         []streams.SessionModelInfo{model},
				ConfigOptions: []streams.ConfigOption{{
					Type:         "select",
					ID:           "model",
					CurrentValue: "stale-persisted-model",
				}},
				ConfigOptionsSettled: true,
			},
		},
	}

	messages := appendSessionModelsMessageFromState(
		session.ID,
		session,
		&lifecycle.CachedModelState{},
		nil,
	)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	var payload lifecycle.SessionModelsEventPayload
	if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode session models payload: %v", err)
	}
	if payload.CurrentModelID != model.ModelID {
		t.Fatalf("current model = %q, want %q", payload.CurrentModelID, model.ModelID)
	}
	if len(payload.Models) != 1 || payload.Models[0].ModelID != model.ModelID {
		t.Fatalf("models = %#v, want persisted model %q", payload.Models, model.ModelID)
	}
	if len(payload.ConfigOptions) != 0 {
		t.Fatalf("config options = %#v, want live empty catalog", payload.ConfigOptions)
	}
	if !payload.ConfigOptionsSettled {
		t.Fatal("config options settled = false, want persisted settlement marker")
	}
}

func TestAppendSessionModelsMessagePreservesSettledEmptySnapshot(t *testing.T) {
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				ConfigOptionsSettled: true,
			},
		},
	}

	messages := appendSessionModelsMessageFromState(session.ID, session, nil, nil)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1 settled-empty notification", len(messages))
	}
	var payload lifecycle.SessionModelsEventPayload
	if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode session models payload: %v", err)
	}
	if !payload.ConfigOptionsSettled {
		t.Fatal("config options settled = false, want true")
	}
	if len(payload.ConfigOptions) != 0 {
		t.Fatalf("config options = %#v, want empty", payload.ConfigOptions)
	}
}

func TestAppendSessionModelsMessageKeepsLiveStateAuthoritative(t *testing.T) {
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID: "persisted-flat-model",
				Models: []streams.SessionModelInfo{{
					ModelID: "persisted-flat-model",
					Name:    "Persisted flat model",
				}},
				ConfigOptions: []streams.ConfigOption{{
					Type:         "select",
					ID:           "model",
					CurrentValue: "stale-persisted-model",
				}},
			},
		},
	}
	liveOptions := []streams.ConfigOption{{
		Type:         "select",
		ID:           "model",
		CurrentValue: "live-config-model",
	}}
	tests := []struct {
		name            string
		liveState       *lifecycle.CachedModelState
		wantConfigCount int
		wantConfigValue string
	}{
		{
			name: "live config options",
			liveState: &lifecycle.CachedModelState{
				CurrentModelID: "live-config-model",
				ConfigOptions:  liveOptions,
			},
			wantConfigCount: len(liveOptions),
			wantConfigValue: liveOptions[0].CurrentValue,
		},
		{
			name: "settled empty catalog",
			liveState: &lifecycle.CachedModelState{
				CurrentModelID:       "live-config-model",
				ConfigOptionsSettled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := appendSessionModelsMessageFromState(session.ID, session, tt.liveState, nil)

			if len(messages) != 1 {
				t.Fatalf("messages = %d, want 1", len(messages))
			}
			var payload lifecycle.SessionModelsEventPayload
			if err := json.Unmarshal(messages[0].Payload, &payload); err != nil {
				t.Fatalf("decode session models payload: %v", err)
			}
			if len(payload.Models) != 0 {
				t.Fatalf("models = %d, want 0", len(payload.Models))
			}
			if payload.CurrentModelID != tt.liveState.CurrentModelID {
				t.Fatalf("current model = %q, want %q", payload.CurrentModelID, tt.liveState.CurrentModelID)
			}
			if len(payload.ConfigOptions) != tt.wantConfigCount {
				t.Fatalf("config options = %d, want %d", len(payload.ConfigOptions), tt.wantConfigCount)
			}
			if tt.wantConfigValue != "" && payload.ConfigOptions[0].CurrentValue != tt.wantConfigValue {
				t.Fatalf("config option value = %q, want %q", payload.ConfigOptions[0].CurrentValue, tt.wantConfigValue)
			}
		})
	}
}
