package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

func disabledLaunchManager(store *keyedStore) *Manager {
	m := newLaunchManager(store)
	m.sessionMCPPolicy = NewSessionMCPPolicy(false)
	return m
}

func TestSessionMCPPolicyRequiredCapabilities(t *testing.T) {
	before := slices.Clone(RequiredSurvivalCapabilities)
	off := NewSessionMCPPolicy(false).RequiredCapabilities()
	on := NewSessionMCPPolicy(true).RequiredCapabilities()
	if !slices.Equal(off, before) {
		t.Fatalf("disabled set = %v, want %v", off, before)
	}
	if !slices.Equal(on, append(slices.Clone(before), "session_mcp_token")) {
		t.Fatalf("enabled set = %v", on)
	}
	if !slices.Equal(RequiredSurvivalCapabilities, before) {
		t.Fatalf("package-level required set was modified: %v", RequiredSurvivalCapabilities)
	}
	on[0] = "mutated"
	if RequiredSurvivalCapabilities[0] != before[0] {
		t.Fatal("returned set aliases the package-level value")
	}
	var zero SessionMCPPolicy
	if zero.Enabled() {
		t.Fatal("zero policy must be disabled")
	}
}

func TestSessionMCPPolicyBrokerUpstream(t *testing.T) {
	env := map[string]string{githubauth.CredentialBrokerURLEnv: testBrokerURL}
	local := string(models.ExecutorTypeLocal)
	if got := NewSessionMCPPolicy(false).BrokerUpstream(local, env); got != "" {
		t.Fatalf("disabled upstream = %q, want empty", got)
	}
	if got := NewSessionMCPPolicy(true).BrokerUpstream(local, env); got != testBrokerURL {
		t.Fatalf("enabled upstream = %q", got)
	}
}

func TestBrokerRelayEnvFollowsPolicy(t *testing.T) {
	env := map[string]string{githubauth.CredentialBrokerURLEnv: testBrokerURL}
	local := string(models.ExecutorTypeLocal)
	off := &Manager{}
	if got := off.brokerRelayEnv(local, env, 4242); !reflect.DeepEqual(got, env) {
		t.Fatalf("disabled env = %v, want unchanged", got)
	}
	on := &Manager{sessionMCPPolicy: NewSessionMCPPolicy(true)}
	if got := on.brokerRelayEnv(local, env, 4242)[githubauth.CredentialBrokerURLEnv]; got == testBrokerURL || got == "" {
		t.Fatalf("enabled env broker URL = %q, want the instance relay", got)
	}
}

// Flag-off parity: the create request, execution metadata and runtime call are
// what a launch produced before the capability existed. The key store is never
// touched, so even a failing one cannot refuse the launch.
func TestCreateInstanceFlagOffLeavesRequestUntouched(t *testing.T) {
	b := newCreateBackend(t, nil)
	m := disabledLaunchManager(&keyedStore{err: errors.New("store must not be read")})
	req := &ExecutorCreateRequest{InstanceID: "exec-1", Metadata: map[string]interface{}{"k": "v"}}
	want := &ExecutorCreateRequest{InstanceID: "exec-1", Metadata: map[string]interface{}{"k": "v"}}

	inst, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req, want) {
		t.Fatalf("request changed: %+v", req)
	}
	if len(b.creates) != 1 || b.creates[0] != "" {
		t.Fatalf("creates = %q, want one create without a nonce", b.creates)
	}
	if _, ok := inst.Metadata[MetadataKeyMCPTokenNonce]; ok {
		t.Fatalf("instance metadata carries a nonce: %v", inst.Metadata)
	}
	if len(b.stopped) != 0 {
		t.Fatalf("flag-off launch stopped instances: %v", b.stopped)
	}
}

func TestEnsureSessionMCPKeyFlagOffReadsNoKey(t *testing.T) {
	m := disabledLaunchManager(&keyedStore{err: errors.New("store must not be read")})
	if err := m.EnsureSessionMCPKey(context.Background()); err != nil {
		t.Fatalf("EnsureSessionMCPKey with the capability disabled: %v", err)
	}
}

func TestCarrierRulesFlagOffMatchToday(t *testing.T) {
	m := disabledLaunchManager(&keyedStore{key: launchKey()})
	for _, strategy := range []mcpconfig.PassthroughMCPStrategy{
		mcpconfig.ClaudeStrategy{}, mcpconfig.OpenCodeStrategy{}, mcpconfig.CodexStrategy{},
		mcpconfig.CursorStrategy{}, mcpconfig.PiStrategy{}, unknownStrategy{},
	} {
		in := carrierServers()
		out, err := m.withSessionMCPAuth(t.Context(), carrierExecution(""), nil, strategy, in)
		if err != nil {
			t.Fatalf("%T: %v", strategy, err)
		}
		if !reflect.DeepEqual(out, carrierServers()) {
			t.Fatalf("%T: servers changed: %+v", strategy, out)
		}
		if err := m.requireSessionMCPCarrier(carrierExecution(""), nil, strategy); err != nil {
			t.Fatalf("%T: carrier refused with the capability disabled: %v", strategy, err)
		}
	}
	tui := agents.NewTUIAgent(agents.TUIAgentConfig{AgentID: "custom-tui", AgentName: "Custom", Command: "tool", MCPStrategy: mcpconfig.CursorStrategy{}})
	if err := m.requireSessionMCPCarrier(carrierExecution(""), tui, mcpconfig.CursorStrategy{}); err != nil {
		t.Fatalf("custom TUI refused with the capability disabled: %v", err)
	}
}

func TestCodexLaunchFlagOffMatchesToday(t *testing.T) {
	m := disabledLaunchManager(&keyedStore{key: launchKey()})
	request := process.InteractiveStartRequest{
		Command: []string{"codex", sessionmcp.CodexTokenArg},
		Env:     map[string]string{"A": "b"},
	}
	got, err := m.withCodexSessionTokenEnv(t.Context(), carrierExecution("n1"), request)
	if err != nil || !reflect.DeepEqual(got, request) {
		t.Fatalf("request = %+v, err = %v; want unchanged", got, err)
	}
	if err := m.checkCodexCarrier(t.Context(), carrierExecution("n1"), request.Command, process.InteractiveStartRequest{}.LaunchWrapper); err != nil {
		t.Fatalf("codex carrier check with the capability disabled: %v", err)
	}
}

// A retained instance is adopted unverified while the capability is off, even
// for the runtimes it would otherwise refuse, and nothing is contacted.
func TestVerifyRetainedSessionTokenFlagOffAdoptsWithoutContact(t *testing.T) {
	m := disabledLaunchManager(&keyedStore{key: launchKey()})
	record := &models.ExecutorRunning{Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n1"}}
	for _, rt := range []agentruntime.Runtime{agentruntime.RuntimeKubernetes, agentruntime.RuntimeSSH, agentruntime.RuntimeSprites} {
		ctl := &fakeControl{}
		ri := remoteTokenInstance(t, rt, "stale", ctl)
		if err := m.verifyRetainedSessionToken(context.Background(), record, ri); err != nil {
			t.Fatalf("%s: %v", rt, err)
		}
		if len(ctl.deleted) != 0 || ctl.revokes != 0 {
			t.Fatalf("%s: deleted=%v revokes=%d, want no contact", rt, ctl.deleted, ctl.revokes)
		}
	}
}

// With the capability on and no key store, a record that carries a nonce
// cannot be checked: the instance is revoked and never adopted. A record
// without a nonce is a mismatch whatever the store state.
func TestVerifyRetainedSessionTokenWithoutKeyStoreFailsClosed(t *testing.T) {
	m := &Manager{logger: newTestLogger(), sessionMCPPolicy: NewSessionMCPPolicy(true)}
	t.Run("nonce and no key store", func(t *testing.T) {
		ctl := &fakeControl{}
		record := &models.ExecutorRunning{Metadata: map[string]interface{}{MetadataKeyMCPTokenNonce: "n1"}}
		err := m.verifyRetainedSessionToken(context.Background(), record, remoteTokenInstance(t, agentruntime.RuntimeSprites, "anything", ctl))
		if !errors.Is(err, errRetainedTokenUnverified) {
			t.Fatalf("err = %v, want errRetainedTokenUnverified", err)
		}
		if len(ctl.deleted) != 0 || ctl.revokes != 1 {
			t.Fatalf("deleted=%v revokes=%d, want revoke only", ctl.deleted, ctl.revokes)
		}
	})
	t.Run("no nonce and no key store", func(t *testing.T) {
		ctl := &fakeControl{}
		record := &models.ExecutorRunning{Metadata: map[string]interface{}{}}
		err := m.verifyRetainedSessionToken(context.Background(), record, remoteTokenInstance(t, agentruntime.RuntimeSprites, "stale", ctl))
		if !errors.Is(err, errRetainedTokenUnverified) {
			t.Fatalf("err = %v, want errRetainedTokenUnverified", err)
		}
		if len(ctl.deleted) != 1 || ctl.deleted[0] != "stale" || ctl.revokes != 0 {
			t.Fatalf("deleted=%v revokes=%d, want delete only", ctl.deleted, ctl.revokes)
		}
	})
}

func TestRevokeMCPTokenRetriesOnATimerAndOutlivesTheCallerContext(t *testing.T) {
	ctl := &fakeControl{revokeErr: []error{errors.New("down"), errors.New("down")}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !revokeMCPToken(ctx, ctl, "exec-1", 3, time.Millisecond) {
		t.Fatal("revoke did not complete on a detached context")
	}
	if ctl.revokes != 3 {
		t.Fatalf("attempts = %d, want 3", ctl.revokes)
	}

	failing := &fakeControl{revokeErr: []error{errors.New("a"), errors.New("b"), errors.New("c")}}
	if revokeMCPToken(context.Background(), failing, "exec-1", 3, time.Millisecond) || failing.revokes != 3 {
		t.Fatalf("exhausted revoke: revokes=%d", failing.revokes)
	}
}
