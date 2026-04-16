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
// given agent.
type wantModel struct {
	id        string
	isDefault bool
	meta      map[string]any
}

// TestHttpListInferenceAgents covers the full /api/v1/utility/inference-agents
// response contract:
//
//   - Only agents whose host utility probe reached StatusOK are included —
//     an agent that needs auth, isn't installed, or is still probing can't
//     actually run a utility prompt, so it must be filtered out of the
//     picker rather than leading the user into a dead end.
//   - `models` is always a JSON array, never null — a null slice would
//     crash the frontend's flatMap over `ia.models`.
//   - `is_default` is set on the model matching CurrentModelID.
//   - `meta` (e.g. Copilot's `copilotUsage` cost multiplier) propagates
//     through to the DTO so the model combobox can render cost badges.
func TestHttpListInferenceAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Fixtures use the real built-in ACP agent shape (distinct ID/Name) so
	// a future ID/Name mixup in the cache lookup fails the test.
	claude := lifecycle.InferenceAgentInfo{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}
	codex := lifecycle.InferenceAgentInfo{ID: "codex-acp", Name: "Codex ACP Agent", DisplayName: "Codex"}
	copilot := lifecycle.InferenceAgentInfo{ID: "copilot-acp", Name: "Copilot ACP Agent", DisplayName: "Copilot"}

	tests := []struct {
		name        string
		agents      []lifecycle.InferenceAgentInfo
		caps        map[string]hostutility.AgentCapabilities
		nilHost     bool
		wantByAgent map[string][]wantModel
	}{
		{
			name:   "healthy agent with cached models is included with is_default",
			agents: []lifecycle.InferenceAgentInfo{claude},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp": {
					AgentType:      "claude-acp",
					Status:         hostutility.StatusOK,
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
			// Primary UX bug this filter guards against: Codex/Auggie with
			// lock icons (auth_required) appearing in the utility picker.
			name:   "auth_required agent is filtered out",
			agents: []lifecycle.InferenceAgentInfo{claude, codex},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp": {
					AgentType: "claude-acp",
					Status:    hostutility.StatusOK,
					Models:    []hostutility.Model{{ID: "sonnet", Name: "Sonnet"}},
				},
				"codex-acp": {
					AgentType: "codex-acp",
					Status:    hostutility.StatusAuthRequired,
				},
			},
			wantByAgent: map[string][]wantModel{
				"Claude ACP Agent": {{id: "sonnet", isDefault: false}},
			},
		},
		{
			name:   "failed and probing agents are filtered out",
			agents: []lifecycle.InferenceAgentInfo{claude, codex, copilot},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp":  {Status: hostutility.StatusFailed},
				"codex-acp":   {Status: hostutility.StatusProbing},
				"copilot-acp": {Status: hostutility.StatusOK, Models: []hostutility.Model{{ID: "gpt-5", Name: "GPT-5"}}},
			},
			wantByAgent: map[string][]wantModel{
				"Copilot ACP Agent": {{id: "gpt-5", isDefault: false}},
			},
		},
		{
			name:        "agent with no cache entry is filtered out",
			agents:      []lifecycle.InferenceAgentInfo{claude},
			caps:        nil,
			wantByAgent: map[string][]wantModel{},
		},
		{
			name:   "model meta (copilot cost) propagates to DTO",
			agents: []lifecycle.InferenceAgentInfo{copilot},
			caps: map[string]hostutility.AgentCapabilities{
				"copilot-acp": {
					AgentType:      "copilot-acp",
					Status:         hostutility.StatusOK,
					CurrentModelID: "gpt-5",
					Models: []hostutility.Model{
						{ID: "gpt-5", Name: "GPT-5", Meta: map[string]any{"copilotUsage": "1x"}},
						{ID: "gpt-5-mini", Name: "GPT-5 Mini", Meta: map[string]any{"copilotUsage": "0.33x"}},
					},
				},
			},
			wantByAgent: map[string][]wantModel{
				"Copilot ACP Agent": {
					{id: "gpt-5", isDefault: true, meta: map[string]any{"copilotUsage": "1x"}},
					{id: "gpt-5-mini", isDefault: false, meta: map[string]any{"copilotUsage": "0.33x"}},
				},
			},
		},
		{
			name:        "no inference agents returns empty list",
			agents:      nil,
			caps:        nil,
			wantByAgent: map[string][]wantModel{},
		},
		{
			// hostExecutor is optional throughout the package (see the nil
			// guard in executeSessionless). Without it we can't check
			// health, so the list is empty — never a panic.
			name:        "nil hostExecutor yields empty list, no panic",
			agents:      []lifecycle.InferenceAgentInfo{claude},
			nilHost:     true,
			wantByAgent: map[string][]wantModel{},
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

			// The raw body must never contain "models":null — that shape
			// crashed the frontend before the original fix.
			body := rec.Body.String()
			if strings.Contains(body, `"models":null`) {
				t.Fatalf("response must never contain \"models\":null, got body: %s", body)
			}

			var resp struct {
				Agents []struct {
					Name   string `json:"name"`
					Models []struct {
						ID        string         `json:"id"`
						IsDefault bool           `json:"is_default"`
						Meta      map[string]any `json:"meta,omitempty"`
					} `json:"models"`
				} `json:"agents"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if len(resp.Agents) != len(tt.wantByAgent) {
				t.Fatalf("got %d agents (%v), want %d", len(resp.Agents), agentNames(resp.Agents), len(tt.wantByAgent))
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
					if !equalMeta(m.Meta, want[i].meta) {
						t.Errorf("agent %q model %q: got meta=%v, want %v", a.Name, m.ID, m.Meta, want[i].meta)
					}
				}
			}
		})
	}
}

func agentNames(agents []struct {
	Name   string `json:"name"`
	Models []struct {
		ID        string         `json:"id"`
		IsDefault bool           `json:"is_default"`
		Meta      map[string]any `json:"meta,omitempty"`
	} `json:"models"`
}) []string {
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name)
	}
	return names
}

// equalMeta does a shallow equality check on the meta maps. A nil want map
// matches a nil or empty got map (JSON omits empty maps via omitempty).
func equalMeta(got, want map[string]any) bool {
	if len(want) == 0 {
		return len(got) == 0
	}
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
