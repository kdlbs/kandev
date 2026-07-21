package plugins

import (
	"context"
	"errors"
	"testing"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeUtilitySettingsSource struct {
	profileID string
	err       error
}

func (f *fakeUtilitySettingsSource) UtilityAgentProfileID(context.Context) (string, error) {
	return f.profileID, f.err
}

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

// profileFixture wires the agent-profiles data source to hold one profile with
// the given id, agent type, and model — the shape resolveUtilityAgentProfile
// scans.
func (d *testDataHost) profileFixture(profileID, agentType, model string) {
	d.profiles.resp = &agentsettingsdto.ListAgentsResponse{
		Agents: []agentsettingsdto.AgentDTO{{
			ID: "agent-1",
			Profiles: []agentsettingsdto.AgentProfileDTO{{
				ID: profileID, AgentID: agentType, Model: model,
			}},
		}},
	}
}

func TestPluginHost_InvokeUtilityAgent_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	assertPermissionDenied(t, err, "agent_invoke")
	if d.utilRun.calls != 0 {
		t.Fatalf("runner called %d times, want 0 when capability denied", d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_UsesConfiguredProfile(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.utilCfg.profileID = "profile-42"
	d.profileFixture("profile-42", "claude-acp", "claude-opus-4-8")
	d.utilRun.text = "the summary"

	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
	if err != nil {
		t.Fatalf("InvokeUtilityAgent() unexpected error: %v", err)
	}
	if got != "the summary" {
		t.Fatalf("text = %q, want %q", got, "the summary")
	}
	// The configured profile's agent type + model must reach the runner.
	if d.utilRun.gotAgentType != "claude-acp" || d.utilRun.gotModel != "claude-opus-4-8" {
		t.Fatalf("runner got (%q, %q), want (claude-acp, claude-opus-4-8)", d.utilRun.gotAgentType, d.utilRun.gotModel)
	}
	if d.utilRun.gotPrompt != "summarize yesterday" {
		t.Fatalf("runner prompt = %q", d.utilRun.gotPrompt)
	}
}

func TestPluginHost_InvokeUtilityAgent_NotConfigured(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.utilCfg.profileID = "" // no utility agent selected

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.FailedPrecondition {
		t.Fatalf("err = %v, want FailedPrecondition", err)
	}
	if d.utilRun.calls != 0 {
		t.Fatalf("runner called %d times, want 0 when unconfigured", d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_ProfileMissing(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.utilCfg.profileID = "profile-gone"
	d.profileFixture("profile-still-here", "claude-acp", "m") // different id

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.FailedPrecondition {
		t.Fatalf("err = %v, want FailedPrecondition for a stale profile id", err)
	}
	if d.utilRun.calls != 0 {
		t.Fatalf("runner called %d times, want 0 when profile missing", d.utilRun.calls)
	}
}

// TestPluginHost_InvokeUtilityAgent_LateWiringReachesSpawnedHost proves the
// boot-ordering fix: a host built before SetUtilityAgent runs (as happens for
// boot-active plugins, spawned before hostUtilityMgr exists) still resolves the
// utility deps once they are wired, because utilityDeps reads them live from the
// Service rather than snapshotting at spawn time.
func TestPluginHost_InvokeUtilityAgent_LateWiringReachesSpawnedHost(t *testing.T) {
	svc, _, _ := newTestService(t)
	profiles := &fakeAgentProfileDataSource{resp: &agentsettingsdto.ListAgentsResponse{
		Agents: []agentsettingsdto.AgentDTO{{
			ID:       "agent-1",
			Profiles: []agentsettingsdto.AgentProfileDTO{{ID: "p1", AgentID: "claude-acp", Model: "m"}},
		}},
	}}
	// A host as hostForPlugin would build it: utilityDeps bound to the service,
	// which has NOT been wired with a utility agent yet.
	host := &pluginHost{
		capabilities:  manifest.Capabilities{AgentInvoke: true},
		agentProfiles: profiles,
		utilityDeps:   svc.utilityAgentDeps,
	}

	// Before wiring: Unimplemented (deps still nil on the service).
	if _, err := host.InvokeUtilityAgent(context.Background(), "hi"); status.Code(err) != codes.Unimplemented {
		t.Fatalf("pre-wiring code = %v, want Unimplemented", status.Code(err))
	}

	// Wire late, exactly as backendapp does after plugins have already spawned.
	runner := &fakeUtilityRunner{text: "done"}
	svc.SetUtilityAgent(&fakeUtilitySettingsSource{profileID: "p1"}, runner)

	// The already-built host now resolves the freshly-wired deps.
	got, err := host.InvokeUtilityAgent(context.Background(), "hi")
	if err != nil {
		t.Fatalf("post-wiring error: %v", err)
	}
	if got != "done" || runner.gotAgentType != "claude-acp" {
		t.Fatalf("got %q via %q, want done via claude-acp", got, runner.gotAgentType)
	}
}

func TestPluginHost_InvokeUtilityAgent_SettingError(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.utilCfg.err = errors.New("db down")

	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected error when settings read fails")
	}
	if d.utilRun.calls != 0 {
		t.Fatalf("runner called %d times, want 0 when settings read fails", d.utilRun.calls)
	}
}
