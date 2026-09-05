package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// TestProbeBackgroundWorkloads_AuthorizedCallReachesExecutor pins the happy
// path the session-scope matrix doesn't cover: with the guard satisfied,
// the call actually reaches the executor with the caller's session ID, and
// the executor's result round-trips back unchanged.
func TestProbeBackgroundWorkloads_AuthorizedCallReachesExecutor(t *testing.T) {
	var gotSessionID string
	mgr := &mockAgentManager{
		probeBackgroundWorkloadsFunc: func(_ context.Context, sessionID string) (client.ProbeResult, error) {
			gotSessionID = sessionID
			return client.ProbeResultLive, nil
		},
	}

	s := &Service{
		logger:                testLogger(),
		sessionAccessCheck:    func(context.Context, string) error { return nil },
		executor:              executor.NewExecutor(mgr, nil, testLogger(), executor.ExecutorConfig{}),
		backgroundProbeConfig: BackgroundProbeConfig{Budget: defaultParkedProbeBudget, Interval: defaultParkedProbeInterval},
	}

	result, err := s.ProbeBackgroundWorkloads(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != client.ProbeResultLive {
		t.Errorf("result = %q, want %q", result, client.ProbeResultLive)
	}
	if gotSessionID != "sess-1" {
		t.Errorf("executor received session_id %q, want %q", gotSessionID, "sess-1")
	}
}

// A zero-value BackgroundProbeConfig (Service constructed without going
// through NewService, e.g. in older/other tests) must not disable the
// timeout entirely — it falls back to the compiled-in default budget.
func TestProbeBackgroundWorkloads_ZeroConfigFallsBackToDefaultBudget(t *testing.T) {
	mgr := &mockAgentManager{
		probeBackgroundWorkloadsFunc: func(ctx context.Context, _ string) (client.ProbeResult, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("expected the probe call to carry a deadline")
			}
			return client.ProbeResultSettled, nil
		},
	}

	s := &Service{
		logger:             testLogger(),
		sessionAccessCheck: func(context.Context, string) error { return nil },
		executor:           executor.NewExecutor(mgr, nil, testLogger(), executor.ExecutorConfig{}),
	}

	if _, err := s.ProbeBackgroundWorkloads(context.Background(), "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
