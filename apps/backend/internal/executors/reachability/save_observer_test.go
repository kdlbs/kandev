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

// newTestSaveObserver builds a SaveObserver over a started Poller wired to
// repo, with probe replaced by a deterministic success so ProbeNow's
// dispatched probe is observable through repo's upsert count instead of real
// network I/O. Callers must defer poller.Stop().
func newTestSaveObserver(t *testing.T, repo *fakeRepository) (*SaveObserver, *Poller) {
	t.Helper()
	poller := New(repo, 0, logger.Default())
	poller.probe = func(_ context.Context, e *models.Executor) lifecycle.SSHProbeOutcome {
		return lifecycle.SSHProbeOutcome{Success: true, Host: e.Config["ssh_host"]}
	}
	poller.Start(context.Background())
	return NewSaveObserver(poller), poller
}

func sshExecutorWithConfig(id string, status models.ExecutorStatus, config map[string]string) *models.Executor {
	return &models.Executor{ID: id, Name: id, Type: models.ExecutorTypeSSH, Status: status, Config: config}
}

// waitForUpsertCount blocks until repo has observed at least want upserts, or
// fails the test after a short deadline — a happens-before proof that the
// observer's fire-and-forget ProbeNow dispatch actually ran, without a fixed
// sleep.
func waitForUpsertCount(t *testing.T, repo *fakeRepository, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if repo.upsertCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("upsertCount = %d, want >= %d", repo.upsertCount(), want)
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.2
//
// A host change on an active SSH executor must reset the stored record to
// the new host and dispatch an off-cycle probe — the reset's whole purpose is
// that a stale unreachable/reachable record about the OLD host must never be
// shown against the new one, even for the single tick before the next probe
// lands.
func TestSaveObserverResetsAndProbesOnHostChange(t *testing.T) {
	repo := newFakeRepository()
	observer, poller := newTestSaveObserver(t, repo)
	defer poller.Stop()

	before := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.1"})
	after := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.2"})

	observer.OnExecutorSaved(context.Background(), before, after)

	rec, err := repo.GetExecutorReachability(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("GetExecutorReachability: %v", err)
	}
	if rec.Host != "10.0.0.2" {
		t.Fatalf("record.Host = %q, want the new host 10.0.0.2 (reset must run before the probe)", rec.Host)
	}

	waitForUpsertCount(t, repo, 1)
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// Creating an executor has no prior state to compare against, so it must
// always be treated as a change — before=nil is not "nothing changed".
func TestSaveObserverTreatsNilBeforeAsAlwaysChanged(t *testing.T) {
	repo := newFakeRepository()
	observer, poller := newTestSaveObserver(t, repo)
	defer poller.Stop()

	after := sshExecutorWithConfig("exec-created", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.5"})
	observer.OnExecutorSaved(context.Background(), nil, after)

	if _, err := repo.GetExecutorReachability(context.Background(), "exec-created"); err != nil {
		t.Fatalf("GetExecutorReachability: %v, want a reset record from the create", err)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// A non-SSH executor save must never reset or probe — there is no
// reachability record for it to begin with.
func TestSaveObserverSkipsNonSSHExecutor(t *testing.T) {
	repo := newFakeRepository()
	observer, poller := newTestSaveObserver(t, repo)
	defer poller.Stop()

	after := &models.Executor{ID: "exec-local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	observer.OnExecutorSaved(context.Background(), nil, after)

	if _, err := repo.GetExecutorReachability(context.Background(), "exec-local"); err == nil {
		t.Fatalf("expected no record for a non-SSH executor")
	}
	if repo.upsertCount() != 0 {
		t.Fatalf("upsertCount = %d, want 0 (no probe should have been dispatched)", repo.upsertCount())
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// A disabled SSH executor must not be reset or probed — probing an executor
// no launch path can use wastes a dial and would misleadingly imply it is
// monitored.
func TestSaveObserverSkipsInactiveSSHExecutor(t *testing.T) {
	repo := newFakeRepository()
	observer, poller := newTestSaveObserver(t, repo)
	defer poller.Stop()

	after := sshExecutorWithConfig("exec-disabled", models.ExecutorStatusDisabled, map[string]string{"ssh_host": "10.0.0.5"})
	observer.OnExecutorSaved(context.Background(), nil, after)

	if _, err := repo.GetExecutorReachability(context.Background(), "exec-disabled"); err == nil {
		t.Fatalf("expected no record for a disabled executor")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.1
//
// Only the named connection-configuration keys matter. A save that changes
// something else entirely (here: nothing in Config at all differs) must not
// reset an existing record — a reset would wipe legitimate history (state,
// consecutive_failures, last_success_at) for no reason tied to the host's
// actual reachability.
func TestSaveObserverSkipsWhenConnectionConfigUnchanged(t *testing.T) {
	repo := newFakeRepository()
	observer, poller := newTestSaveObserver(t, repo)
	defer poller.Stop()

	config := map[string]string{"ssh_host": "10.0.0.1", "ssh_user": "deploy"}
	before := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, config)
	after := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, config)

	observer.OnExecutorSaved(context.Background(), before, after)

	if _, err := repo.GetExecutorReachability(context.Background(), "exec-1"); err == nil {
		t.Fatalf("expected no record written when connection config is unchanged")
	}
	if repo.upsertCount() != 0 {
		t.Fatalf("upsertCount = %d, want 0", repo.upsertCount())
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-003.2, AC-EXECUTORS-SSH-REACHABILITY-002.3
//
// A reset that actually changes what a client would see (the previously
// stored record was not already the unknown/empty-reason placeholder) must
// publish a change event, so a UI watching this executor doesn't have to poll
// to notice the reset.
func TestSaveObserverPublishesChangeEventWhenPriorRecordWasObserved(t *testing.T) {
	repo := newFakeRepository()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	published := make(chan RecordDTO, 4)
	if _, err := eventBus.Subscribe(events.ExecutorReachabilityChanged, func(_ context.Context, ev *bus.Event) error {
		published <- ev.Data.(RecordDTO)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	poller := New(repo, 0, logger.Default())
	poller.SetPublisher(NewPublisher(eventBus))
	poller.probe = func(_ context.Context, e *models.Executor) lifecycle.SSHProbeOutcome {
		return lifecycle.SSHProbeOutcome{Success: true, Host: e.Config["ssh_host"]}
	}
	poller.Start(context.Background())
	defer poller.Stop()
	observer := NewSaveObserver(poller)

	checkedAt := time.Now().UTC()
	repo.records["exec-1"] = &models.ExecutorReachability{
		ExecutorID: "exec-1", State: models.ExecutorReachabilityStateReachable,
		Host: "10.0.0.1", CheckedAt: &checkedAt, UpdatedAt: checkedAt,
	}

	before := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.1"})
	after := sshExecutorWithConfig("exec-1", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.2"})
	observer.OnExecutorSaved(context.Background(), before, after)

	select {
	case dto := <-published:
		if dto.ExecutorID != "exec-1" || dto.State != string(models.ExecutorReachabilityStateUnknown) || dto.Host != "10.0.0.2" {
			t.Fatalf("dto = %+v, want the reset unknown/10.0.0.2 shape", dto)
		}
	case <-time.After(time.Second):
		t.Fatal("no change event published for a reset over a previously observed record")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.3
//
// Resetting an executor that was never probed before (no stored record at
// all) is not itself a client-visible change — the client already sees the
// synthesized unknown placeholder for "no record yet" — so the reset step
// must not publish. The dispatched off-cycle probe that follows is a
// separate, legitimate publish opportunity (a real unknown -> reachable
// transition) and is deliberately excluded here via a blocking probe gate:
// OnExecutorSaved's synchronous portion (the reset and its publish decision)
// completes fully before the probe goroutine can even call probe, so
// asserting silence right after OnExecutorSaved returns isolates the reset
// step's own behavior without racing the probe.
func TestSaveObserverDoesNotPublishFromResetWhenNoPriorRecord(t *testing.T) {
	repo := newFakeRepository()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	published := make(chan RecordDTO, 4)
	if _, err := eventBus.Subscribe(events.ExecutorReachabilityChanged, func(_ context.Context, ev *bus.Event) error {
		published <- ev.Data.(RecordDTO)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	probeGate := make(chan struct{})
	poller := New(repo, 0, logger.Default())
	poller.SetPublisher(NewPublisher(eventBus))
	poller.probe = func(_ context.Context, e *models.Executor) lifecycle.SSHProbeOutcome {
		<-probeGate
		return lifecycle.SSHProbeOutcome{Success: true, Host: e.Config["ssh_host"]}
	}
	poller.Start(context.Background())
	defer poller.Stop()
	observer := NewSaveObserver(poller)

	after := sshExecutorWithConfig("exec-created", models.ExecutorStatusActive, map[string]string{"ssh_host": "10.0.0.5"})
	observer.OnExecutorSaved(context.Background(), nil, after)

	select {
	case dto := <-published:
		t.Fatalf("reset step itself must not publish when there was no prior record: %+v", dto)
	default:
	}

	close(probeGate)
	waitForUpsertCount(t, repo, 1)
}
