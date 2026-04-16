package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
)

type stubInferenceExecutor struct {
	agents []lifecycle.InferenceAgentInfo
}

func (s *stubInferenceExecutor) ExecuteInferencePrompt(_ context.Context, _, _, _, _ string) (*agentctlutil.PromptResponse, error) {
	return nil, nil
}

func (s *stubInferenceExecutor) ListInferenceAgentsWithContext(_ context.Context) []lifecycle.InferenceAgentInfo {
	return s.agents
}

type stubHostUtility struct {
	caps map[string]hostutility.AgentCapabilities
}

func (s *stubHostUtility) ExecutePrompt(_ context.Context, _, _, _, _ string) (*hostutility.PromptResult, error) {
	return nil, nil
}

func (s *stubHostUtility) Get(agentType string) (hostutility.AgentCapabilities, bool) {
	c, ok := s.caps[agentType]
	return c, ok
}

// TestHttpListInferenceAgentsNeverReturnsNullModels guards the
// /api/v1/utility/inference-agents response contract: each agent's `models`
// field must always be a JSON array, never null. The frontend iterates
// `ia.models` unconditionally during render (see the utility-agents-section
// component), so a null here crashes the entire settings page.
func TestHttpListInferenceAgentsNeverReturnsNullModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		agents       []lifecycle.InferenceAgentInfo
		caps         map[string]hostutility.AgentCapabilities
		wantModelIDs map[string][]string
	}{
		{
			name: "agent with no cached capabilities yields empty models array",
			agents: []lifecycle.InferenceAgentInfo{
				{ID: "claude", Name: "claude-code", DisplayName: "Claude Code"},
			},
			caps:         nil,
			wantModelIDs: map[string][]string{"claude-code": {}},
		},
		{
			name: "agent with cached models yields populated models array",
			agents: []lifecycle.InferenceAgentInfo{
				{ID: "claude", Name: "claude-code", DisplayName: "Claude Code"},
			},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-code": {
					AgentType:      "claude-code",
					CurrentModelID: "sonnet",
					Models: []hostutility.Model{
						{ID: "sonnet", Name: "Sonnet"},
						{ID: "opus", Name: "Opus"},
					},
				},
			},
			wantModelIDs: map[string][]string{"claude-code": {"sonnet", "opus"}},
		},
		{
			name:         "no inference agents returns empty agents array",
			agents:       nil,
			caps:         nil,
			wantModelIDs: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			h := &Handlers{
				executor:     &stubInferenceExecutor{agents: tt.agents},
				hostExecutor: &stubHostUtility{caps: tt.caps},
				logger:       log,
			}

			router := gin.New()
			router.GET("/api/v1/utility/inference-agents", h.httpListInferenceAgents)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/utility/inference-agents", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}

			// The raw body must never contain "models":null — that's the
			// exact shape that crashed the frontend before this fix.
			body := rec.Body.String()
			if strings.Contains(body, `"models":null`) {
				t.Fatalf("response must never contain \"models\":null, got body: %s", body)
			}

			var resp struct {
				Agents []struct {
					Name   string `json:"name"`
					Models []struct {
						ID string `json:"id"`
					} `json:"models"`
				} `json:"agents"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if len(resp.Agents) != len(tt.wantModelIDs) {
				t.Fatalf("got %d agents, want %d", len(resp.Agents), len(tt.wantModelIDs))
			}

			for _, a := range resp.Agents {
				if a.Models == nil {
					t.Errorf("agent %q: models slice decoded as nil (should be empty slice, never nil)", a.Name)
				}
				wantIDs, ok := tt.wantModelIDs[a.Name]
				if !ok {
					t.Errorf("unexpected agent %q in response", a.Name)
					continue
				}
				if len(a.Models) != len(wantIDs) {
					t.Errorf("agent %q: got %d models, want %d", a.Name, len(a.Models), len(wantIDs))
					continue
				}
				for i, m := range a.Models {
					if m.ID != wantIDs[i] {
						t.Errorf("agent %q model[%d]: got %q, want %q", a.Name, i, m.ID, wantIDs[i])
					}
				}
			}
		})
	}
}
