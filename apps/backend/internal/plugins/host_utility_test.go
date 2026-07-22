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
	d.host.configs = &fakeConfigReader{configs: map[string]any{utilityAgentConfigKey: "utility-agent-42"}}
	d.utilAgents.agent = &UtilityAgent{Name: "summarizer", AgentID: "claude-acp", Model: "claude-opus-4-8", Enabled: true}
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
	if d.utilRun.gotAgentType != "claude-acp" || d.utilRun.gotModel != "claude-opus-4-8" {
		t.Fatalf("runner got (%q, %q)", d.utilRun.gotAgentType, d.utilRun.gotModel)
	}
	if d.utilAgents.gotSelector != "utility-agent-42" {
		t.Fatalf("looked up utility agent %q", d.utilAgents.gotSelector)
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
