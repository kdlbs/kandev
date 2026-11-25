package reachability

import (
	"context"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"testing"
)

func TestManagedProbeUsesSavedExecutorAndFailsClosed(t *testing.T) {
	executor := &models.Executor{ID: "saved", Config: map[string]string{"ssh_connection_id": "connection", "ssh_host": "ignored"}}
	poller := New(nil, 0, logger.Default())
	called := false
	poller.SetManagedProbe(func(_ context.Context, id string) agentruntime.SSHProbeOutcome {
		called = true
		if id != "saved" {
			t.Fatal("wrong executor")
		}
		return agentruntime.SSHProbeOutcome{Success: true}
	})
	if got := poller.probe(context.Background(), executor); !got.Success || !called {
		t.Fatal("managed probe not routed")
	}
	poller.SetManagedProbe(nil)
	if got := poller.probe(context.Background(), executor); got.Success || got.Reason != agentruntime.SSHReachabilityReasonConfig {
		t.Fatalf("disabled managed selector: %+v", got)
	}
}
