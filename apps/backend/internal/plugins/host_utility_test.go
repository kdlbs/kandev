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
	agent *UtilityAgent
	err   error
}

func (f *fakeUtilityAgentSource) GetAgentByName(_ context.Context, _ string) (*UtilityAgent, error) {
	return f.agent, f.err
}

type fakeConfigReader struct {
	configs map[string]any
	err     error
}

func (f *fakeConfigReader) GetConfig(string) (map[string]any, error) { return f.configs, f.err }

type fakeUtilityRunner struct {
	calls        int
	gotAgentType string
	gotModel     string
	gotMode      string
	gotPrompt    string
	text         string
	err          error
}

func (f *fakeUtilityRunner) ExecutePrompt(_ context.Context, agentType, model, mode, prompt string) (string, error) {
	f.calls++
	f.gotAgentType, f.gotModel, f.gotMode, f.gotPrompt = agentType, model, mode, prompt
	return f.text, f.err
}

func configuredUtilityHost(t *testing.T) *testDataHost {
	t.Helper()
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.utilAgents.agent = &UtilityAgent{Name: "summarizer", AgentID: "claude-acp", Model: "claude-opus-4-8", Enabled: true}
	d.utilRun.text = "the summary"
	return d
}

func TestPluginHost_InvokeUtilityAgent_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	assertPermissionDenied(t, err, "agent_invoke")
}

func TestPluginHost_InvokeUtilityAgent_UsesPluginConfiguredUtilityAgent(t *testing.T) {
	d := configuredUtilityHost(t)
	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.utilRun.gotAgentType != "claude-acp" || d.utilRun.gotModel != "claude-opus-4-8" {
		t.Fatalf("runner got (%q, %q)", d.utilRun.gotAgentType, d.utilRun.gotModel)
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
		{"missing", nil, errors.New("not found")},
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
