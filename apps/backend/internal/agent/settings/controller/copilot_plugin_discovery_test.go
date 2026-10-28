package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

type recordingCopilotRunner struct {
	stdout []byte
	calls  [][]string
	envs   []map[string]string
}

func (r *recordingCopilotRunner) Run(_ context.Context, argv []string, env map[string]string, _ string) (copilotplugins.CommandResult, error) {
	r.calls = append(r.calls, append([]string{}, argv...))
	r.envs = append(r.envs, env)
	return copilotplugins.CommandResult{Stdout: r.stdout}, nil
}

func newCopilotDiscoveryController(t *testing.T) (*Controller, *fakeStore, string) {
	t.Helper()
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-row-1", Name: agents.CopilotACPAgentID, SupportsMCP: true}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	profile := &models.AgentProfile{
		ID: "profile-1", AgentID: agent.ID, Name: "Work", AgentDisplayName: "Copilot",
		EnvVars: []models.ProfileEnvVar{{Key: "COPILOT_HOME", Value: "/tmp/copilot-home"}},
	}
	st.profiles[agent.ID] = []*models.AgentProfile{profile}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewCopilotACP()); err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(st, nil, reg, nil, log)
	return ctrl, st, profile.ID
}

func TestCopilotPluginDiscoveryReturnsSafeRows(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	runner := &recordingCopilotRunner{stdout: []byte(`[
		{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","enabled":true,"source":"installed"},
		{"name":"browser-use","version":"0.0.4","enabled":false,"source":"builtin"},
		{"name":"manual","version":"1.0.0","enabled":true,"source":"external"}
	]`)}
	ctrl.copilotPluginRunnerImpl = runner
	req := dto.CopilotPluginDiscoveryRequest{
		ProfileID:          profileID,
		AuthorizationScope: "user-1",
	}
	got, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("DiscoverCopilotPlugins: %v", err)
	}
	if got.Status != "ready" || got.ProviderID != agents.CopilotACPAgentID || got.ProfileID != profileID {
		t.Fatalf("response = %#v", got)
	}
	if len(got.Plugins) != 3 {
		t.Fatalf("plugins = %#v", got.Plugins)
	}
	if got.Plugins[1].Availability != "unavailable" || got.Plugins[1].ReasonCode != "builtin_not_mountable" {
		t.Fatalf("builtin row = %#v", got.Plugins[1])
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/tmp/copilot-home", "cache_path", "installed-from"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("discovery leaked %q: %s", forbidden, encoded)
		}
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d", len(runner.calls))
	}
	if !strings.Contains(strings.Join(runner.calls[0], " "), "--config-dir=/tmp/copilot-home") {
		t.Fatalf("inventory argv = %#v", runner.calls[0])
	}
}

func TestCopilotPluginDiscoveryRejectsForeignAgent(t *testing.T) {
	ctrl, st, _ := newCopilotDiscoveryController(t)
	other := &models.Agent{ID: "agent-row-2", Name: "claude-acp"}
	st.agents[other.ID] = other
	got, err := ctrl.DiscoverCopilotPlugins(context.Background(), other.ID, dto.CopilotPluginDiscoveryRequest{AuthorizationScope: "user-1"})
	if err != nil {
		t.Fatalf("DiscoverCopilotPlugins: %v", err)
	}
	if got.Status != "unsupported" || got.Reason != "unsupported_agent" {
		t.Fatalf("response = %#v", got)
	}
}

func TestCopilotPluginDiscoveryRequiresDraftLaunchSettings(t *testing.T) {
	ctrl, _, _ := newCopilotDiscoveryController(t)
	_, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", dto.CopilotPluginDiscoveryRequest{AuthorizationScope: "user-1"})
	if err != ErrInvalidProfileDiscovery {
		t.Fatalf("error = %v, want invalid profile discovery", err)
	}
}

func TestCopilotPluginDiscoveryContext(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	runner := &recordingCopilotRunner{stdout: []byte(`[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","enabled":true,"source":"installed"}]`)}
	ctrl.copilotPluginRunnerImpl = runner
	req := dto.CopilotPluginDiscoveryRequest{ProfileID: profileID, AuthorizationScope: "user-1"}
	first, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	second, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("second discovery: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("successful snapshots were not cached: %d calls", len(runner.calls))
	}
	if first.ContextRevision != second.ContextRevision {
		t.Fatalf("context revisions differ: %q / %q", first.ContextRevision, second.ContextRevision)
	}

	refresh := req
	refresh.Refresh = true
	if _, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", refresh); err != nil {
		t.Fatalf("refresh discovery: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("refresh did not bypass cache: %d calls", len(runner.calls))
	}

	envVars := []dto.ProfileEnvVarDTO{{Key: "COPILOT_HOME", Value: "/different/home"}}
	flags := []dto.CLIFlagDTO{}
	prefix := ""
	draft := req
	draft.ProfileID = ""
	draft.LaunchSettings = &dto.ProfileLaunchSettingsRequest{EnvVars: &envVars, CLIFlags: &flags, CommandPrefix: &prefix}
	if _, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", draft); err != nil {
		t.Fatalf("draft discovery: %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("different launch context reused a cached snapshot: %d calls", len(runner.calls))
	}
}

func TestCopilotPluginDiscoveryRejectsWrapper(t *testing.T) {
	ctrl, st, profileID := newCopilotDiscoveryController(t)
	profile, err := st.GetAgentProfile(context.Background(), profileID)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	profile.CommandPrefix = "sandbox --"
	if err := st.UpdateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	runner := &recordingCopilotRunner{stdout: []byte(`[]`)}
	ctrl.copilotPluginRunnerImpl = runner
	got, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", dto.CopilotPluginDiscoveryRequest{
		ProfileID: profileID, AuthorizationScope: "user-1",
	})
	if err != nil {
		t.Fatalf("DiscoverCopilotPlugins: %v", err)
	}
	if got.Status != "unsupported" || got.Reason != "unsupported_launch_context" {
		t.Fatalf("wrapped discovery = %#v", got)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("wrapped discovery ran %d native commands", len(runner.calls))
	}
}

func TestCopilotPluginDiscoveryUsesSavedProfileDraftLaunchSettings(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	runner := &recordingCopilotRunner{stdout: []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"builtin"}]`)}
	ctrl.copilotPluginRunnerImpl = runner
	envVars := []dto.ProfileEnvVarDTO{{Key: "COPILOT_HOME", Value: "/draft/copilot-home"}}
	flags := []dto.CLIFlagDTO{}
	prefix := ""
	got, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", dto.CopilotPluginDiscoveryRequest{
		ProfileID: profileID, AuthorizationScope: "user-1",
		LaunchSettings: &dto.ProfileLaunchSettingsRequest{EnvVars: &envVars, CLIFlags: &flags, CommandPrefix: &prefix},
	})
	if err != nil {
		t.Fatalf("DiscoverCopilotPlugins: %v", err)
	}
	if got.Status != "ready" || len(runner.calls) != 1 {
		t.Fatalf("response = %#v, calls = %d", got, len(runner.calls))
	}
	if !strings.Contains(strings.Join(runner.calls[0], " "), "--config-dir=/draft/copilot-home") {
		t.Fatalf("inventory argv = %#v, want draft config dir", runner.calls[0])
	}
	if strings.Contains(strings.Join(runner.calls[0], " "), "/tmp/copilot-home") {
		t.Fatalf("inventory argv used the saved config dir: %#v", runner.calls[0])
	}
}
