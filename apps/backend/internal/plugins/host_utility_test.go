package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeUtilityAgentSource struct {
	agent       *UtilityAgent
	err         error
	calls       int
	gotSelector string
}

func (f *fakeUtilityAgentSource) GetAgentByID(_ context.Context, selector string) (*UtilityAgent, error) {
	f.calls++
	f.gotSelector = selector
	return f.agent, f.err
}

type fakeConfigReader struct {
	configs map[string]any
	err     error
}

func (f *fakeConfigReader) GetConfig(string) (map[string]any, error) { return f.configs, f.err }

type fakeUtilityRunner struct {
	calls        int
	gotProfileID string
	gotPrompt    string
	text         string
	err          error
}

func (f *fakeUtilityRunner) ExecuteProfilePrompt(_ context.Context, profileID, prompt string) (string, error) {
	f.calls++
	f.gotProfileID, f.gotPrompt = profileID, prompt
	return f.text, f.err
}

func configuredUtilityHost(t *testing.T) *testDataHost {
	t.Helper()
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.host.configs = &fakeConfigReader{configs: map[string]any{utilityAgentConfigKey: "utility-agent-42"}}
	d.utilAgents.agent = &UtilityAgent{Name: "summarizer", AgentID: "claude-acp", Model: "claude-opus-4-8", AgentProfileID: "profile-42", ProfileBindingState: "explicit", Enabled: true}
	d.utilRun.text = "the summary"
	return d
}

func TestPluginHost_InvokeUtilityAgent_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	assertPermissionDenied(t, err, "agent_invoke")
	if d.utilAgents.calls != 0 || d.utilRun.calls != 0 {
		t.Fatalf("unauthorized call touched agent lookup %d times and runner %d times", d.utilAgents.calls, d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_UsesPluginConfiguredUtilityAgent(t *testing.T) {
	d := configuredUtilityHost(t)
	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.utilRun.gotProfileID != "profile-42" {
		t.Fatalf("runner got profile %q", d.utilRun.gotProfileID)
	}
	if d.utilAgents.gotSelector != "utility-agent-42" {
		t.Fatalf("looked up utility agent %q", d.utilAgents.gotSelector)
	}
}

func TestPluginHost_InvokeUtilityAgent_UsesConfiguredAgentProfileBeforeUtilityAgent(t *testing.T) {
	d := configuredUtilityHost(t)
	d.host.configSchema = map[string]any{
		"properties": map[string]any{
			agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
		},
	}
	d.host.configs = &fakeConfigReader{configs: map[string]any{
		agentProfileConfigKey: "profile-99",
		utilityAgentConfigKey: "utility-agent-42",
	}}
	d.profiles.profilesByID = map[string]*AgentProfile{
		"profile-99": {Enabled: true},
	}

	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.utilRun.gotProfileID != "profile-99" {
		t.Fatalf("runner got profile %q", d.utilRun.gotProfileID)
	}
	if d.utilAgents.calls != 0 {
		t.Fatalf("direct profile invocation looked up utility agents %d times", d.utilAgents.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_FallsBackToLegacyUtilityAgentWhenDirectProfileIsUnset(t *testing.T) {
	d := configuredUtilityHost(t)
	d.host.configSchema = map[string]any{
		"properties": map[string]any{
			agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
		},
	}
	d.host.configs = &fakeConfigReader{configs: map[string]any{utilityAgentConfigKey: "utility-agent-42"}}

	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.utilAgents.gotSelector != "utility-agent-42" {
		t.Fatalf("looked up utility agent %q", d.utilAgents.gotSelector)
	}
	if d.utilRun.gotProfileID != "profile-42" {
		t.Fatalf("runner got profile %q", d.utilRun.gotProfileID)
	}
}

func TestPluginHost_InvokeUtilityAgent_RejectsMissingOrIneligibleConfiguredAgentProfile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   map[string]any
		profiles map[string]*AgentProfile
		want     string
	}{
		{
			name:   "missing selection",
			config: map[string]any{},
			want:   "no agent profile configured for this plugin",
		},
		{
			name:     "missing profile",
			config:   map[string]any{agentProfileConfigKey: "missing-profile"},
			profiles: map[string]*AgentProfile{},
			want:     `configured agent profile "missing-profile" not found`,
		},
		{
			name:     "disabled profile",
			config:   map[string]any{agentProfileConfigKey: "disabled-profile"},
			profiles: map[string]*AgentProfile{"disabled-profile": {Enabled: false}},
			want:     `configured agent profile "disabled-profile" is not eligible for utility execution`,
		},
		{
			name:     "CLI profile",
			config:   map[string]any{agentProfileConfigKey: "cli-profile"},
			profiles: map[string]*AgentProfile{"cli-profile": {Enabled: true, CLIPassthrough: true}},
			want:     `configured agent profile "cli-profile" is not eligible for utility execution`,
		},
		{
			name:     "workspace profile",
			config:   map[string]any{agentProfileConfigKey: "workspace-profile"},
			profiles: map[string]*AgentProfile{"workspace-profile": {Enabled: true, WorkspaceID: "workspace-1"}},
			want:     `configured agent profile "workspace-profile" is not eligible for utility execution`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := configuredUtilityHost(t)
			d.host.configSchema = map[string]any{
				"properties": map[string]any{
					agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
				},
			}
			d.host.configs = &fakeConfigReader{configs: tc.config}
			if tc.profiles != nil {
				d.profiles.profilesByID = tc.profiles
			}

			_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("InvokeUtilityAgent() status = %s, want %s; err = %v", status.Code(err), codes.FailedPrecondition, err)
			}
			if status.Convert(err).Message() != tc.want {
				t.Fatalf("InvokeUtilityAgent() message = %q, want %q", status.Convert(err).Message(), tc.want)
			}
			if d.utilAgents.calls != 0 || d.utilRun.calls != 0 {
				t.Fatalf("invalid direct profile touched utility lookup %d times and runner %d times", d.utilAgents.calls, d.utilRun.calls)
			}
		})
	}
}

func TestPluginHost_InvokeUtilityAgent_PreservesDirectProfileRunnerFailure(t *testing.T) {
	runnerErr := status.Error(codes.Unavailable, "agentctl unavailable")
	d := configuredUtilityHost(t)
	d.host.configSchema = map[string]any{
		"properties": map[string]any{
			agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
		},
	}
	d.host.configs = &fakeConfigReader{configs: map[string]any{agentProfileConfigKey: "profile-99"}}
	d.profiles.profilesByID = map[string]*AgentProfile{"profile-99": {Enabled: true}}
	d.utilRun.err = runnerErr

	_, err := d.host.InvokeUtilityAgent(context.Background(), "summarize")
	if !errors.Is(err, runnerErr) || status.Code(err) != codes.Unavailable {
		t.Fatalf("InvokeUtilityAgent() error = %v, want runner error %v", err, runnerErr)
	}
}

func TestPluginHost_InvokeUtilityAgent_DirectProfile_PreservesStoreFailure(t *testing.T) {
	storeErr := status.Error(codes.Unavailable, "agent profile store unavailable")
	d := configuredUtilityHost(t)
	d.host.configSchema = map[string]any{
		"properties": map[string]any{
			agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
		},
	}
	d.host.configs = &fakeConfigReader{configs: map[string]any{agentProfileConfigKey: "profile-99"}}
	d.profiles.profileErr = storeErr

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if !errors.Is(err, storeErr) {
		t.Fatalf("InvokeUtilityAgent() error = %v, want wrapped store error", err)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("InvokeUtilityAgent() status = %s, want %s", status.Code(err), codes.Unavailable)
	}
	if d.utilRun.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_DirectProfile_PreservesLookupCancellation(t *testing.T) {
	d := configuredUtilityHost(t)
	d.host.configSchema = map[string]any{
		"properties": map[string]any{
			agentProfileConfigKey: map[string]any{"type": "string", "format": "agent-profile"},
		},
	}
	d.host.configs = &fakeConfigReader{configs: map[string]any{agentProfileConfigKey: "profile-99"}}
	d.profiles.profileErr = context.Canceled

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if status.Code(err) != codes.Canceled {
		t.Fatalf("InvokeUtilityAgent() status = %s, want %s", status.Code(err), codes.Canceled)
	}
}

func TestPluginHost_InvokeUtilityAgent_NotConfigured(t *testing.T) {
	d := configuredUtilityHost(t)
	d.host.configs = &fakeConfigReader{configs: map[string]any{}}
	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if status.Code(err) != codes.FailedPrecondition || d.utilRun.calls != 0 {
		t.Fatalf("err = %v, calls = %d", err, d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_MissingOrDisabled(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent *UtilityAgent
		err   error
	}{
		{"missing", nil, ErrUtilityAgentNotFound},
		{"disabled", &UtilityAgent{Name: "summarizer"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := configuredUtilityHost(t)
			d.utilAgents.agent, d.utilAgents.err = tc.agent, tc.err
			_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
			if status.Code(err) != codes.FailedPrecondition || d.utilRun.calls != 0 {
				t.Fatalf("err = %v, calls = %d", err, d.utilRun.calls)
			}
		})
	}
}

func TestPluginHost_InvokeUtilityAgent_PreservesLookupFailure(t *testing.T) {
	storeErr := status.Error(codes.Unavailable, "utility agent store unavailable")
	d := configuredUtilityHost(t)
	d.utilAgents.err = storeErr

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if !errors.Is(err, storeErr) {
		t.Fatalf("InvokeUtilityAgent() error = %v, want wrapped store error", err)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("InvokeUtilityAgent() status = %s, want %s", status.Code(err), codes.Unavailable)
	}
	if d.utilRun.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_PreservesLookupCancellation(t *testing.T) {
	d := configuredUtilityHost(t)
	d.utilAgents.err = context.Canceled

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if status.Code(err) != codes.Canceled {
		t.Fatalf("InvokeUtilityAgent() status = %s, want %s", status.Code(err), codes.Canceled)
	}
}
