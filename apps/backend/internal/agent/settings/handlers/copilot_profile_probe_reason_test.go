package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestCopilotPluginProfileProbeReasonContract(t *testing.T) {
	tests := []struct {
		name     string
		response any
	}{
		{
			name: "capability response",
			response: dto.DynamicModelsResponse{
				AgentName:  "copilot-acp",
				Status:     "unsupported",
				ReasonCode: "policy_unverified",
				Models:     []dto.ModelEntryDTO{},
				Error:      stringPointer("profile launch settings are unsupported by this agent"),
			},
		},
		{
			name: "model option response",
			response: dto.AgentModelConfigResponse{
				AgentName:     "copilot-acp",
				Model:         "model-a",
				Status:        "unsupported",
				ReasonCode:    "policy_unverified",
				ConfigOptions: []dto.ConfigOptionDTO{},
				Error:         stringPointer("model option resolution failed"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			var reasonCode, status string
			if err := json.Unmarshal(fields["reason_code"], &reasonCode); err != nil {
				t.Fatalf("decode reason_code: %v", err)
			}
			if err := json.Unmarshal(fields["status"], &status); err != nil {
				t.Fatalf("decode status: %v", err)
			}
			if reasonCode != "policy_unverified" || status != "unsupported" {
				t.Fatalf("reason/status = %q/%q, want policy_unverified/unsupported", reasonCode, status)
			}
			if !strings.Contains(string(payload), `"error"`) {
				t.Fatalf("response omitted its existing error envelope: %s", payload)
			}
		})
	}

	router, _, _ := newSettingsHarnessAs(t, newFakeSettingsRepo(), nil,
		authn.Identity{UserID: "member-1", Role: authn.RoleMember})
	for _, request := range []struct {
		name string
		path string
		body string
	}{
		{
			name: "profile capability probe",
			path: "/api/v1/agent-models/copilot-acp/probe",
			body: `{"profile_id":"profile-1"}`,
		},
		{
			name: "profile model options",
			path: "/api/v1/agent-models/copilot-acp/resolve",
			body: `{"model":"model-a","profile_id":"profile-1"}`,
		},
	} {
		t.Run(request.name+" still requires config scope", func(t *testing.T) {
			response := doSettingsRequest(router, http.MethodPost, request.path, request.body)
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "org.config.manage") {
				t.Fatalf("status = %d, body = %s, want org.config.manage refusal", response.Code, response.Body.String())
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
