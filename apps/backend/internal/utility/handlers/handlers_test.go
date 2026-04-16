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

// wantModel is the expected shape of a single model in the response for a
// given agent. Used by TestHttpListInferenceAgentsNeverReturnsNullModels to
// assert both IDs and is_default propagation.
type wantModel struct {
	id        string
	isDefault bool
}

// TestHttpListInferenceAgentsNeverReturnsNullModels guards the
// /api/v1/utility/inference-agents response contract: each agent's `models`
// field must always be a JSON array, never null, and `is_default` must be
// set on the model matching the agent's CurrentModelID. The frontend
// iterates `ia.models` unconditionally during render (utility-agents-section)
// and the create-agent dialog auto-selects the default model via
// `find((m) => m.is_default)` (utility-agent-dialog). A null slice crashes
// the settings page; a missing is_default silently breaks default selection.
func TestHttpListInferenceAgentsNeverReturnsNullModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		agents      []lifecycle.InferenceAgentInfo
		caps        map[string]hostutility.AgentCapabilities
		nilHost     bool
		wantByAgent map[string][]wantModel
	}{
		// Fixtures deliberately use the real built-in ACP agent shape
		// where ID ("claude-acp") and Name ("Claude ACP Agent") differ,
		// so a Name/ID mixup in the cache lookup will always fail this
		// test. The cache is keyed by ag.ID() in production.
		{
			name: "agent with no cached capabilities yields empty models array",
			agents: []lifecycle.InferenceAgentInfo{
				{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"},
			},
			caps:        nil,
			wantByAgent: map[string][]wantModel{"Claude ACP Agent": {}},
		},
		{
			name: "agent with cached models yields populated array with is_default set",
			agents: []lifecycle.InferenceAgentInfo{
				{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"},
			},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp": { // keyed by ID, matches bootstrapAgent
					AgentType:      "claude-acp",
					CurrentModelID: "sonnet",
					Models: []hostutility.Model{
						{ID: "sonnet", Name: "Sonnet"},
						{ID: "opus", Name: "Opus"},
					},
				},
			},
			wantByAgent: map[string][]wantModel{
				"Claude ACP Agent": {
					{id: "sonnet", isDefault: true},
					{id: "opus", isDefault: false},
				},
			},
		},
		{
			name:        "no inference agents returns empty agents array",
			agents:      nil,
			caps:        nil,
			wantByAgent: map[string][]wantModel{},
		},
		{
			// Guards against a panic when hostExecutor isn't wired up (the
			// dep is treated as optional throughout the package — see the
			// nil-guard in executeSessionless).
			name: "nil hostExecutor does not panic and yields empty models",
			agents: []lifecycle.InferenceAgentInfo{
				{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"},
			},
			nilHost:     true,
			wantByAgent: map[string][]wantModel{"Claude ACP Agent": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			h := &Handlers{
				executor: &stubInferenceExecutor{agents: tt.agents},
				logger:   log,
			}
			if !tt.nilHost {
				h.hostExecutor = &stubHostUtility{caps: tt.caps}
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
						ID        string `json:"id"`
						IsDefault bool   `json:"is_default"`
					} `json:"models"`
				} `json:"agents"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if len(resp.Agents) != len(tt.wantByAgent) {
				t.Fatalf("got %d agents, want %d", len(resp.Agents), len(tt.wantByAgent))
			}

			for _, a := range resp.Agents {
				if a.Models == nil {
					t.Errorf("agent %q: models slice decoded as nil (should be empty slice, never nil)", a.Name)
				}
				want, ok := tt.wantByAgent[a.Name]
				if !ok {
					t.Errorf("unexpected agent %q in response", a.Name)
					continue
				}
				if len(a.Models) != len(want) {
					t.Errorf("agent %q: got %d models, want %d", a.Name, len(a.Models), len(want))
					continue
				}
				for i, m := range a.Models {
					if m.ID != want[i].id {
						t.Errorf("agent %q model[%d]: got id %q, want %q", a.Name, i, m.ID, want[i].id)
					}
					if m.IsDefault != want[i].isDefault {
						t.Errorf("agent %q model %q: got is_default=%v, want %v", a.Name, m.ID, m.IsDefault, want[i].isDefault)
					}
				}
			}
		})
	}
}
