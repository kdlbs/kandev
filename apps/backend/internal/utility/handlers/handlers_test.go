package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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
	caps         map[string]hostutility.AgentCapabilities
	refreshErr   map[string]error
	refreshCaps  map[string]hostutility.AgentCapabilities
	refreshCalls atomic.Int32
}

func (s *stubHostUtility) ExecutePrompt(_ context.Context, _, _, _, _ string) (*hostutility.PromptResult, error) {
	return nil, nil
}

func (s *stubHostUtility) Get(agentType string) (hostutility.AgentCapabilities, bool) {
	c, ok := s.caps[agentType]
	return c, ok
}

func (s *stubHostUtility) Refresh(_ context.Context, agentType string) (hostutility.AgentCapabilities, error) {
	s.refreshCalls.Add(1)
	if err, ok := s.refreshErr[agentType]; ok && err != nil {
		// Mirror Manager.Refresh: cache is updated with the failure state.
		if s.caps == nil {
			s.caps = make(map[string]hostutility.AgentCapabilities)
		}
		s.caps[agentType] = hostutility.AgentCapabilities{
			AgentType: agentType,
			Status:    hostutility.StatusFailed,
			Error:     err.Error(),
		}
		return hostutility.AgentCapabilities{}, err
	}
	c, ok := s.refreshCaps[agentType]
	if !ok {
		c = hostutility.AgentCapabilities{
			AgentType: agentType,
			Status:    hostutility.StatusOK,
		}
	}
	if s.caps == nil {
		s.caps = make(map[string]hostutility.AgentCapabilities)
	}
	s.caps[agentType] = c
	return c, nil
}

// wantModel is the expected shape of a single model in the response for a
// given agent.
type wantModel struct {
	id        string
	isDefault bool
	meta      map[string]any
}

// wantAgent is the expected shape of a single agent entry in the response.
type wantAgent struct {
	status        string
	hasMsg        bool
	models        []wantModel
	configOptions int
}

// TestHttpListInferenceAgents covers the full /api/v1/utility/inference-agents
// response contract:
//
//   - Every registered ACP-capable agent is included regardless of probe
//     status. The frontend uses `status` to render an inline note + Refresh
//     button when models aren't available, rather than silently dropping the
//     agent and leaving the user staring at an empty Model picker.
//   - `models` is always a JSON array, never null — a null slice would
//     crash the frontend's flatMap over `ia.models`.
//   - `is_default` is set on the model matching CurrentModelID.
//   - `meta` (e.g. Copilot's `copilotUsage` cost multiplier) propagates
//     through to the DTO so the model combobox can render cost badges.
//   - `status_message` is sanitized so a credential-looking substring in
//     the upstream probe error never reaches the wire verbatim.
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
		wantByAgent map[string]wantAgent
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
					ConfigOptions: []hostutility.ConfigOption{{
						Type:         "select",
						ID:           "reasoning_effort",
						Name:         "Reasoning effort",
						CurrentValue: "medium",
						Category:     "thought-level",
						Options: []hostutility.ConfigOptionChoice{
							{Value: "low", Name: "Low"},
							{Value: "medium", Name: "Medium"},
						},
					}},
				},
			},
			wantByAgent: map[string]wantAgent{
				"Claude ACP Agent": {
					status:        "ok",
					configOptions: 1,
					models: []wantModel{
						{id: "sonnet", isDefault: true},
						{id: "opus", isDefault: false},
					},
				},
			},
		},
		{
			// Previously: filter dropped non-OK agents from the response.
			// New contract: include them so the UI can render an inline
			// "sign in to Claude" / "Claude CLI not installed" note rather
			// than an empty Model picker with no explanation.
			name:   "auth_required, failed, and probing agents are included with status",
			agents: []lifecycle.InferenceAgentInfo{claude, codex, copilot},
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp": {
					AgentType: "claude-acp",
					Status:    hostutility.StatusAuthRequired,
					Error:     "please run `claude login`",
				},
				"codex-acp": {
					AgentType: "codex-acp",
					Status:    hostutility.StatusFailed,
					Error:     "probe crashed",
				},
				"copilot-acp": {
					AgentType: "copilot-acp",
					Status:    hostutility.StatusProbing,
				},
			},
			wantByAgent: map[string]wantAgent{
				"Claude ACP Agent":  {status: "auth_required", hasMsg: true, models: []wantModel{}},
				"Codex ACP Agent":   {status: "failed", hasMsg: true, models: []wantModel{}},
				"Copilot ACP Agent": {status: "probing", models: []wantModel{}},
			},
		},
		{
			// Cache miss (registry knows about the agent but the probe
			// hasn't landed yet) is surfaced as "probing" so the UI shows
			// the same "Setting up X…" affordance.
			name:   "agent with no cache entry is reported as probing",
			agents: []lifecycle.InferenceAgentInfo{claude},
			caps:   nil,
			wantByAgent: map[string]wantAgent{
				"Claude ACP Agent": {status: "probing", models: []wantModel{}},
			},
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
			wantByAgent: map[string]wantAgent{
				"Copilot ACP Agent": {
					status: "ok",
					models: []wantModel{
						{id: "gpt-5", isDefault: true, meta: map[string]any{"copilotUsage": "1x"}},
						{id: "gpt-5-mini", isDefault: false, meta: map[string]any{"copilotUsage": "0.33x"}},
					},
				},
			},
		},
		{
			name:        "no inference agents returns empty list",
			agents:      nil,
			caps:        nil,
			wantByAgent: map[string]wantAgent{},
		},
		{
			// hostExecutor is optional throughout the package (see the nil
			// guard in executeSessionless). Without it we can't check
			// probe state, so the list is empty — never a panic.
			name:        "nil hostExecutor yields empty list, no panic",
			agents:      []lifecycle.InferenceAgentInfo{claude},
			nilHost:     true,
			wantByAgent: map[string]wantAgent{},
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
					Name          string `json:"name"`
					Status        string `json:"status"`
					StatusMessage string `json:"status_message,omitempty"`
					Models        []struct {
						ID        string         `json:"id"`
						IsDefault bool           `json:"is_default"`
						Meta      map[string]any `json:"meta,omitempty"`
					} `json:"models"`
					ConfigOptions []struct {
						ID      string `json:"id"`
						Options []struct {
							Value string `json:"value"`
							Name  string `json:"name"`
						} `json:"options"`
					} `json:"config_options"`
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
				if a.Status != want.status {
					t.Errorf("agent %q: got status=%q, want %q", a.Name, a.Status, want.status)
				}
				if want.hasMsg && a.StatusMessage == "" {
					t.Errorf("agent %q: expected non-empty status_message", a.Name)
				}
				if !want.hasMsg && a.StatusMessage != "" {
					t.Errorf("agent %q: expected empty status_message, got %q", a.Name, a.StatusMessage)
				}
				if len(a.ConfigOptions) != want.configOptions {
					t.Errorf("agent %q: got %d config options, want %d", a.Name, len(a.ConfigOptions), want.configOptions)
				}
				if len(a.Models) != len(want.models) {
					t.Errorf("agent %q: got %d models, want %d", a.Name, len(a.Models), len(want.models))
					continue
				}
				for i, m := range a.Models {
					if m.ID != want.models[i].id {
						t.Errorf("agent %q model[%d]: got id %q, want %q", a.Name, i, m.ID, want.models[i].id)
					}
					if m.IsDefault != want.models[i].isDefault {
						t.Errorf("agent %q model %q: got is_default=%v, want %v", a.Name, m.ID, m.IsDefault, want.models[i].isDefault)
					}
					if !equalMeta(m.Meta, want.models[i].meta) {
						t.Errorf("agent %q model %q: got meta=%v, want %v", a.Name, m.ID, m.Meta, want.models[i].meta)
					}
				}
			}
		})
	}
}

func TestInferenceAgentDTOFromCapsPreservesConfigOptionDescriptions(t *testing.T) {
	dto := inferenceAgentDTOFromCaps(lifecycle.InferenceAgentInfo{
		ID: "codex-acp", Name: "Codex ACP", DisplayName: "Codex",
	}, hostutility.AgentCapabilities{
		Status: hostutility.StatusOK,
		ConfigOptions: []hostutility.ConfigOption{{
			Type:         "select",
			ID:           "reasoning_effort",
			Name:         "Reasoning effort",
			Description:  "Controls reasoning depth.",
			CurrentValue: "high",
			Options: []hostutility.ConfigOptionChoice{{
				Value:       "high",
				Name:        "High",
				Description: "More thorough reasoning.",
			}},
		}},
	}, true)

	raw, err := json.Marshal(dto.ConfigOptions)
	if err != nil {
		t.Fatalf("marshal config options: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal config options: %v", err)
	}
	if got := payload[0]["description"]; got != "Controls reasoning depth." {
		t.Errorf("option description = %#v, want provider description", got)
	}
	values := payload[0]["options"].([]any)
	if got := values[0].(map[string]any)["description"]; got != "More thorough reasoning." {
		t.Errorf("value description = %#v, want provider description", got)
	}
}

// TestSanitizeStatusMessage guards the wire-level redaction of credentials in
// upstream probe errors. An ACP agent that echoes the offending value back
// must not leak it through /api/v1/utility/inference-agents.
func TestSanitizeStatusMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "agent crashed", want: "agent crashed"},
		{in: "invalid api_key=sk-deadbeef", want: "invalid api_key=<redacted>"},
		{in: "auth failed: token = abc123", want: "auth failed: token=<redacted>"},
		{in: "bearer sk-xyz expired", want: "bearer=<redacted> expired"},
		{in: "API-KEY: SECRETVAL", want: "API-KEY=<redacted>"},
		// Prose without a real separator must not be mangled — guards against
		// the kw=val matcher eating the next word.
		{in: "access token was revoked", want: "access token was revoked"},
		{in: "the secret handshake failed", want: "the secret handshake failed"},
		// "api key" with a literal space must still be redacted (cubic review).
		{in: "invalid api key=sk-deadbeef", want: "invalid api key=<redacted>"},
		// Newline must NOT count as a separator — otherwise a multi-line
		// stderr ("invalid token\ncaused by network") would consume the next
		// word on the next line (greptile review).
		{
			in:   "invalid token\ncaused by network timeout",
			want: "invalid token\ncaused by network timeout",
		},
		// Tabs are valid horizontal whitespace separators.
		{in: "token\t=\tabc123", want: "token=<redacted>"},
	}
	for _, tc := range cases {
		got := sanitizeStatusMessage(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeStatusMessage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestHttpRefreshInferenceAgent covers the POST /inference-agents/:id/refresh
// route: success path, unknown agent id, error path (still surfaces the
// latest cached state instead of erroring out so the UI can re-render).
func TestHttpRefreshInferenceAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	claude := lifecycle.InferenceAgentInfo{ID: "claude-acp", Name: "Claude ACP Agent", DisplayName: "Claude"}

	t.Run("success returns updated capabilities", func(t *testing.T) {
		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		host := &stubHostUtility{
			caps: map[string]hostutility.AgentCapabilities{
				"claude-acp": {AgentType: "claude-acp", Status: hostutility.StatusAuthRequired, Error: "please login"},
			},
			refreshCaps: map[string]hostutility.AgentCapabilities{
				"claude-acp": {
					AgentType:      "claude-acp",
					Status:         hostutility.StatusOK,
					CurrentModelID: "sonnet",
					Models:         []hostutility.Model{{ID: "sonnet", Name: "Sonnet"}},
				},
			},
		}
		h := &Handlers{
			executor:     &stubInferenceExecutor{agents: []lifecycle.InferenceAgentInfo{claude}},
			hostExecutor: host,
			logger:       log,
		}
		router := gin.New()
		router.POST("/api/v1/utility/inference-agents/:id/refresh", h.httpRefreshInferenceAgent)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/utility/inference-agents/claude-acp/refresh", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if host.refreshCalls.Load() != 1 {
			t.Fatalf("refresh called %d times, want 1", host.refreshCalls.Load())
		}
		var got struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Models []struct {
				ID string `json:"id"`
			} `json:"models"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ID != "claude-acp" || got.Status != "ok" || len(got.Models) != 1 || got.Models[0].ID != "sonnet" {
			t.Fatalf("unexpected response: %+v", got)
		}
	})

	t.Run("unknown agent id returns 404", func(t *testing.T) {
		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		h := &Handlers{
			executor:     &stubInferenceExecutor{agents: []lifecycle.InferenceAgentInfo{claude}},
			hostExecutor: &stubHostUtility{},
			logger:       log,
		}
		router := gin.New()
		router.POST("/api/v1/utility/inference-agents/:id/refresh", h.httpRefreshInferenceAgent)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/utility/inference-agents/bogus/refresh", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("refresh error surfaces latest cached state", func(t *testing.T) {
		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		host := &stubHostUtility{
			refreshErr: map[string]error{"claude-acp": errors.New("connection refused")},
		}
		h := &Handlers{
			executor:     &stubInferenceExecutor{agents: []lifecycle.InferenceAgentInfo{claude}},
			hostExecutor: host,
			logger:       log,
		}
		router := gin.New()
		router.POST("/api/v1/utility/inference-agents/:id/refresh", h.httpRefreshInferenceAgent)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/utility/inference-agents/claude-acp/refresh", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (UI re-renders even on probe failure), body=%s", rec.Code, rec.Body.String())
		}
		var got struct {
			Status        string `json:"status"`
			StatusMessage string `json:"status_message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Status != "failed" {
			t.Fatalf("got status=%q, want failed", got.Status)
		}
		if got.StatusMessage == "" {
			t.Fatalf("expected non-empty status_message on probe failure")
		}
	})

	t.Run("nil host executor returns 503", func(t *testing.T) {
		log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		h := &Handlers{
			executor: &stubInferenceExecutor{agents: []lifecycle.InferenceAgentInfo{claude}},
			logger:   log,
		}
		router := gin.New()
		router.POST("/api/v1/utility/inference-agents/:id/refresh", h.httpRefreshInferenceAgent)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/utility/inference-agents/claude-acp/refresh", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})
}

func agentNames(agents []struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message,omitempty"`
	Models        []struct {
		ID        string         `json:"id"`
		IsDefault bool           `json:"is_default"`
		Meta      map[string]any `json:"meta,omitempty"`
	} `json:"models"`
	ConfigOptions []struct {
		ID      string `json:"id"`
		Options []struct {
			Value string `json:"value"`
			Name  string `json:"name"`
		} `json:"options"`
	} `json:"config_options"`
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
