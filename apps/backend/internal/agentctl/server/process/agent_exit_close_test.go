package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

// TestAgentExitClosesStreamAsAgentExited pins the "Capability and close
// reason" bullet of system design part 2: when the agent process exits on
// its own (crash, self-exit -- anything that isn't an intentional Stop),
// agentctl closes the current backend stream with code 1000 and reason
// agent_exited, so the backend learns the stream ended for that specific
// reason instead of seeing a raw disconnect.
func TestAgentExitClosesStreamAsAgentExited(t *testing.T) {
	mgr := newManagerForCloseReasonTest(t)

	type closeCall struct {
		code   int
		reason string
	}
	calls := make(chan closeCall, 1)
	installCurrentStream(t, mgr, func(code int, reason string) { calls <- closeCall{code, reason} })

	killAgentProcess(t, mgr)

	select {
	case got := <-calls:
		if got.code != CloseCodeNormal || got.reason != CloseReasonAgentExited {
			t.Fatalf("close call = %+v, want {%d %q}", got, CloseCodeNormal, CloseReasonAgentExited)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent exit never closed the current stream with reason agent_exited")
	}

	// waitForExit only reaps the agent process; the adapter/updates/
	// workspace-tracker goroutines it left running are torn down by Stop,
	// same as every other test that kills the process out from under the
	// manager (see TestAgentPgidRecord).
	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after kill error = %v", err)
	}
}

// TestIntentionalStopDoesNotCloseStreamAsAgentExited pins the boundary of the
// same behavior: an agentctl-initiated Stop is not an unexpected agent exit,
// so it must not close the current stream with reason agent_exited. (The
// stream's own StreamEnd/teardown path, driven by handleAgentStreamWS itself
// ending, is unaffected -- this only guards the extra explicit close call.)
func TestIntentionalStopDoesNotCloseStreamAsAgentExited(t *testing.T) {
	mgr := newManagerForCloseReasonTest(t)

	calls := make(chan struct{}, 1)
	installCurrentStream(t, mgr, func(int, string) { calls <- struct{}{} })

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	select {
	case <-calls:
		t.Fatal("intentional Stop must not close the current stream with reason agent_exited")
	case <-time.After(200 * time.Millisecond):
	}
}

func newManagerForCloseReasonTest(t *testing.T) *Manager {
	t.Helper()
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   t.TempDir(),
		SessionID: "session-1",
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })
	return mgr
}

// installCurrentStream registers closeFn as the current stream's close
// function via the same StreamStart/FinalizeStreamStart path
// handleAgentStreamWS uses, without needing a real WebSocket connection.
func installCurrentStream(t *testing.T, mgr *Manager, closeFn func(code int, reason string)) {
	t.Helper()
	if _, err := mgr.StreamStart(context.Background(), "s1"); err != nil {
		t.Fatalf("StreamStart() error = %v", err)
	}
	if _, _, stillCurrent := mgr.FinalizeStreamStart("s1", "", closeFn, doneCh()); !stillCurrent {
		t.Fatal("FinalizeStreamStart() stillCurrent = false, want true")
	}
}
