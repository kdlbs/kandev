package reachability

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.3
//
// A scheduled-pass probe (ProbeNow's own code path, probeAndPersist) must
// publish exactly like the immediate-probe route on a real change, and stay
// silent on a repeat that changes nothing — the "one propagation path" rule
// from the system design applies regardless of which primitive ran the
// probe.
func TestPollerProbeNow_PublishesOnlyOnChange(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(logger.Default())
	published := make(chan RecordDTO, 4)
	_, err := eventBus.Subscribe(events.ExecutorReachabilityChanged, func(_ context.Context, ev *bus.Event) error {
		published <- ev.Data.(RecordDTO)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	repo := newFakeRepository()
	p := New(repo, 60, logger.Default())
	p.SetPublisher(NewPublisher(eventBus))
	p.Start(context.Background())
	defer p.Stop()

	executor := sshExecutor("exec-publish")

	// First success: unknown -> reachable is a change.
	p.probe = func(_ context.Context, e *models.Executor) lifecycle.SSHProbeOutcome {
		return lifecycle.SSHProbeOutcome{Success: true, Host: e.Config["ssh_host"]}
	}
	if !p.ProbeNow(executor) {
		t.Fatal("ProbeNow refused")
	}
	select {
	case dto := <-published:
		if dto.ExecutorID != executor.ID || dto.State != "reachable" {
			t.Fatalf("dto = %+v, want reachable exec-publish", dto)
		}
	case <-time.After(time.Second):
		t.Fatal("no event published for the first, change-carrying probe")
	}

	// Second success, same outcome: reachable -> reachable is not a change.
	if !p.ProbeNow(executor) {
		t.Fatal("ProbeNow refused")
	}
	// Stop drains the second ProbeNow's probeAndPersist goroutine (it is
	// registered on the same WaitGroup) before we check for the absence of a
	// publish — a happens-before barrier instead of a raw sleep.
	p.Stop()
	select {
	case dto := <-published:
		t.Fatalf("unexpected event published for a repeat outcome: %+v", dto)
	default:
	}
}
