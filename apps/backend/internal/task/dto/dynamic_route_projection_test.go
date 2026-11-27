package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskSessionDTOsPreserveDynamicRouteAttribution(t *testing.T) {
	session := &models.TaskSession{
		ID:                 "session-1",
		TaskID:             "task-1",
		AgentProfileID:     "dynamic-profile",
		ExecutionProfileID: "candidate-profile",
		RouteGeneration:    4,
		RouteState:         "action_required",
		RouteReason:        "unclassified_failure",
	}

	for name, value := range map[string]any{
		"detail":  FromTaskSession(session),
		"summary": FromTaskSessionSummary(session),
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("marshal session DTO: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("unmarshal session DTO: %v", err)
			}
			for key, want := range map[string]any{
				"execution_profile_id": "candidate-profile",
				"route_generation":     float64(4),
				"route_state":          "action_required",
				"route_reason":         "unclassified_failure",
			} {
				if got[key] != want {
					t.Errorf("%s = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}
