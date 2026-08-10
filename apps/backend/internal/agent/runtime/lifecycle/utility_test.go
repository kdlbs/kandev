package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// inferenceTestAgent is an agent whose inference support is controlled by the
// test, so both the "supported" and "declared but unsupported" branches are
// reachable without depending on a real CLI.
type inferenceTestAgent struct {
	*agents.MockAgent
	id        string
	installed bool
	config    *agents.InferenceConfig
	stripEnv  []string
}

func (a *inferenceTestAgent) ID() string          { return a.id }
func (a *inferenceTestAgent) Name() string        { return a.id + "-name" }
func (a *inferenceTestAgent) DisplayName() string { return a.id + "-display" }

func (a *inferenceTestAgent) InferenceConfig() *agents.InferenceConfig { return a.config }

func (a *inferenceTestAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{StripEnv: a.stripEnv}
}

func (a *inferenceTestAgent) IsInstalled(context.Context) (*agents.DiscoveryResult, error) {
	if !a.installed {
		return &agents.DiscoveryResult{Available: false}, nil
	}
	return &agents.DiscoveryResult{Available: true}, nil
}

var (
	_ agents.Agent          = (*inferenceTestAgent)(nil)
	_ agents.InferenceAgent = (*inferenceTestAgent)(nil)
)

func newInferenceAgent(id string, supported, installed bool) *inferenceTestAgent {
	agent := &inferenceTestAgent{MockAgent: agents.NewMockAgent(), id: id, installed: installed}
	agent.SetEnabled(true)
	if supported {
		agent.config = &agents.InferenceConfig{
			Supported: true,
			Command:   agents.NewCommand("npx", "-y", "@acp/"+id),
			ModelFlag: agents.NewParam("--model"),
		}
	} else {
		agent.config = &agents.InferenceConfig{Supported: false}
	}
	return agent
}

// stubProfileResolver returns a fixed profile (or error) for every lookup.
type stubProfileResolver struct {
	profile *AgentProfileInfo
	err     error
}

func (r *stubProfileResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	return r.profile, r.err
}

// inferenceHarness wires a Manager whose execution for "session-1" points at an
// in-process agentctl serving /api/v1/inference/prompt.
type inferenceHarness struct {
	manager *Manager

	mu       sync.Mutex
	requests []utility.PromptRequest
}

// recorded returns a copy of the prompt requests the fake agentctl saw. The
// handler appends from a server goroutine, so every read goes through the lock.
func (h *inferenceHarness) recorded() []utility.PromptRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]utility.PromptRequest(nil), h.requests...)
}

func newInferenceHarness(t *testing.T, resolver ProfileResolver, registered ...agents.Agent) *inferenceHarness {
	t.Helper()
	h := &inferenceHarness{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/inference/prompt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req utility.PromptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		h.requests = append(h.requests, req)
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(utility.PromptResponse{
			Success:  true,
			Response: "answer for " + req.Prompt,
			Model:    req.Model,
		})
	}))
	t.Cleanup(server.Close)

	log := newTestLogger()
	reg := newTestRegistry()
	for _, agent := range registered {
		if err := reg.Register(agent); err != nil {
			t.Fatalf("register agent %s: %v", agent.ID(), err)
		}
	}
	if resolver == nil {
		resolver = &MockProfileResolver{}
	}
	manager := NewManager(reg, &MockEventBus{}, nil, &MockCredentialsManager{}, resolver,
		nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, manager)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	if err := manager.executionStore.Add(&AgentExecution{
		ID:            "execution-1",
		SessionID:     "session-1",
		WorkspacePath: "/workspace/task-1",
		agentctl:      agentctlclient.NewClient(parsed.Hostname(), port, log),
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}
	// An execution with no agentctl client, to cover the "client unavailable" path.
	if err := manager.executionStore.Add(&AgentExecution{
		ID:        "execution-2",
		SessionID: "session-no-client",
	}); err != nil {
		t.Fatalf("seed clientless execution: %v", err)
	}
	h.manager = manager
	return h
}

func TestManagerExecuteInferencePromptValidation(t *testing.T) {
	harness := newInferenceHarness(t, nil,
		newInferenceAgent("inference-ok", true, true),
		newInferenceAgent("inference-off", false, true),
	)

	t.Run("session id is required", func(t *testing.T) {
		_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "", "inference-ok", "", "hi")
		if err == nil || !strings.Contains(err.Error(), "session_id is required") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("agent id is required and is a client error sentinel", func(t *testing.T) {
		_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", "", "", "hi")
		if !errors.Is(err, ErrInferenceAgentIDRequired) {
			t.Fatalf("error = %v, want ErrInferenceAgentIDRequired", err)
		}
	})

	t.Run("unknown agent", func(t *testing.T) {
		_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", "nope", "", "hi")
		if err == nil || !strings.Contains(err.Error(), `agent "nope" does not support inference`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("agent declaring unsupported inference", func(t *testing.T) {
		// The registry itself filters unsupported agents, so this surfaces as
		// the same "does not support inference" rejection.
		_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", "inference-off", "", "hi")
		if err == nil || !strings.Contains(err.Error(), "does not support inference") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("no agentctl client for the session", func(t *testing.T) {
		_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-no-client", "inference-ok", "", "hi")
		if err == nil || !strings.Contains(err.Error(), "agentctl client not available") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestManagerExecuteInferencePromptSendsTheAgentInferenceConfig(t *testing.T) {
	agent := newInferenceAgent("inference-ok", true, true)
	agent.stripEnv = []string{"NODE_OPTIONS"}
	harness := newInferenceHarness(t, nil, agent)

	resp, err := harness.manager.ExecuteInferencePrompt(t.Context(),
		"session-1", "inference-ok", "claude-haiku-4-5", "summarise this")
	if err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	if !resp.Success || resp.Response != "answer for summarise this" {
		t.Fatalf("response = %+v", resp)
	}
	recorded := harness.recorded()
	if len(recorded) != 1 {
		t.Fatalf("agentctl calls = %d, want 1", len(recorded))
	}
	got := recorded[0]
	if got.Prompt != "summarise this" || got.AgentID != "inference-ok" || got.Model != "claude-haiku-4-5" {
		t.Fatalf("request = %+v", got)
	}
	if got.InferenceConfig == nil {
		t.Fatal("request must carry the agent's inference config")
	}
	wantCommand := []string{"npx", "-y", "@acp/inference-ok"}
	if !equalStrings(got.InferenceConfig.Command, wantCommand) {
		t.Fatalf("Command = %v, want %v", got.InferenceConfig.Command, wantCommand)
	}
	if !equalStrings(got.InferenceConfig.ModelFlag, []string{"--model"}) {
		t.Fatalf("ModelFlag = %v", got.InferenceConfig.ModelFlag)
	}
	if got.InferenceConfig.WorkDir != "/workspace/task-1" {
		t.Fatalf("WorkDir = %q, want the execution's workspace", got.InferenceConfig.WorkDir)
	}
	if !equalStrings(got.InferenceConfig.StripEnv, []string{"NODE_OPTIONS"}) {
		t.Fatalf("StripEnv = %v, want the agent runtime's list", got.InferenceConfig.StripEnv)
	}
}

func TestManagerExecuteInferenceProfilePrompt(t *testing.T) {
	t.Run("no resolver configured", func(t *testing.T) {
		manager := NewManager(newTestRegistry(), &MockEventBus{}, nil, &MockCredentialsManager{},
			nil, nil, ExecutorFallbackWarn, "", newTestLogger())
		cleanupManagerStopCh(t, manager)
		_, err := manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if err == nil || !strings.Contains(err.Error(), "agent profile resolver is not configured") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("resolver failure bubbles up", func(t *testing.T) {
		wantErr := errors.New("profile lookup failed")
		harness := newInferenceHarness(t, &stubProfileResolver{err: wantErr})
		_, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})

	t.Run("profile without an agent is not executable", func(t *testing.T) {
		harness := newInferenceHarness(t, &stubProfileResolver{profile: &AgentProfileInfo{ProfileID: "p"}})
		_, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if err == nil || !strings.Contains(err.Error(), "is not executable") {
			t.Fatalf("error = %v", err)
		}
		harness = newInferenceHarness(t, &stubProfileResolver{profile: nil})
		_, err = harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if err == nil || !strings.Contains(err.Error(), "is not executable") {
			t.Fatalf("nil profile error = %v", err)
		}
	})

	t.Run("session id is required", func(t *testing.T) {
		harness := newInferenceHarness(t,
			&stubProfileResolver{profile: &AgentProfileInfo{AgentID: "inference-ok"}},
			newInferenceAgent("inference-ok", true, true))
		_, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "", "profile-1", "hi")
		if err == nil || !strings.Contains(err.Error(), "session_id is required") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("profile agent must support inference", func(t *testing.T) {
		harness := newInferenceHarness(t,
			&stubProfileResolver{profile: &AgentProfileInfo{AgentID: "not-registered"}})
		_, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if err == nil || !strings.Contains(err.Error(), "does not support inference") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid command prefix is rejected", func(t *testing.T) {
		harness := newInferenceHarness(t, &stubProfileResolver{profile: &AgentProfileInfo{
			AgentID:       "inference-ok",
			CommandPrefix: "unterminated 'quote",
		}}, newInferenceAgent("inference-ok", true, true))
		_, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi")
		if err == nil {
			t.Fatal("expected the malformed command prefix to be rejected")
		}
	})

	t.Run("applies model, mode, flags, prefix, env and permission policy", func(t *testing.T) {
		harness := newInferenceHarness(t, &stubProfileResolver{profile: &AgentProfileInfo{
			ProfileID:     "profile-1",
			AgentID:       "inference-ok",
			Model:         "claude-haiku-4-5",
			Mode:          "plan",
			AutoApprove:   true,
			CommandPrefix: "greywall --",
			CLIFlags: []settingsmodels.CLIFlag{
				{Flag: "--verbose", Enabled: true},
			},
			EnvVars: []settingsmodels.ProfileEnvVar{
				{Key: "PROFILE_VAR", Value: "profile-value"},
				{Key: "SECRET_VAR", Value: "ignored", SecretID: "secret-1"},
				{Key: "", Value: "dropped"},
			},
		}}, newInferenceAgent("inference-ok", true, true))

		resp, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hello")
		if err != nil {
			t.Fatalf("ExecuteInferenceProfilePrompt: %v", err)
		}
		if resp.Response != "answer for hello" {
			t.Fatalf("response = %+v", resp)
		}
		recorded := harness.recorded()
		if len(recorded) != 1 {
			t.Fatalf("agentctl calls = %d", len(recorded))
		}
		got := recorded[0]
		if got.AgentID != "inference-ok" || got.Model != "claude-haiku-4-5" || got.Mode != "plan" {
			t.Fatalf("request = %+v", got)
		}
		if got.AutoApprovePermissions == nil || !*got.AutoApprovePermissions {
			t.Fatalf("AutoApprovePermissions = %v, want the profile value", got.AutoApprovePermissions)
		}
		if got.InferenceConfig == nil {
			t.Fatal("missing inference config")
		}
		if !equalStrings(got.InferenceConfig.CommandPrefix, []string{"greywall", "--"}) {
			t.Fatalf("CommandPrefix = %v", got.InferenceConfig.CommandPrefix)
		}
		if !equalStrings(got.InferenceConfig.CLIFlags, []string{"--verbose"}) {
			t.Fatalf("CLIFlags = %v", got.InferenceConfig.CLIFlags)
		}
		if got.InferenceConfig.Env["PROFILE_VAR"] != "profile-value" {
			t.Fatalf("Env = %+v, want the plain profile var", got.InferenceConfig.Env)
		}
		if _, leaked := got.InferenceConfig.Env["SECRET_VAR"]; leaked {
			t.Fatalf("secret-backed env vars must not be inlined: %+v", got.InferenceConfig.Env)
		}
		if _, present := got.InferenceConfig.Env[""]; present {
			t.Fatalf("blank env keys must be dropped: %+v", got.InferenceConfig.Env)
		}
		if got.InferenceConfig.WorkDir != "/workspace/task-1" {
			t.Fatalf("WorkDir = %q", got.InferenceConfig.WorkDir)
		}
	})
}

func TestManagerListInferenceAgentsOnlyReturnsInstalledOnes(t *testing.T) {
	installed := newInferenceAgent("inference-installed", true, true)
	missing := newInferenceAgent("inference-missing", true, false)
	harness := newInferenceHarness(t, nil, installed, missing)

	got := harness.manager.ListInferenceAgents()

	var ids []string
	for _, info := range got {
		ids = append(ids, info.ID)
	}
	if !sshContainsStep(ids, "inference-installed") {
		t.Fatalf("agents = %v, want the installed inference agent", ids)
	}
	if sshContainsStep(ids, "inference-missing") {
		t.Fatalf("agents = %v, must exclude uninstalled agents", ids)
	}
	for _, info := range got {
		if info.ID == "inference-installed" {
			if info.Name != "inference-installed-name" || info.DisplayName != "inference-installed-display" {
				t.Fatalf("agent info = %+v, want the agent's own identity fields", info)
			}
		}
	}

	// The context-taking variant is what the HTTP layer uses; it must agree.
	withCtx := harness.manager.ListInferenceAgentsWithContext(t.Context())
	if len(withCtx) != len(got) {
		t.Fatalf("ListInferenceAgentsWithContext returned %d agents, want %d", len(withCtx), len(got))
	}
}
