package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRecoveryUsesOneFiveThirtySecondBackoffAndThenStops(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.startErr = errors.New("still crashed")
	host.snapshots["kotlin"] = RuntimeSnapshot{
		Language: "kotlin", Generation: 1, Phase: PhaseError, ErrorCode: "process_exited",
	}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	runtimes.ensured["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatalf("startup reconcile: %v", err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	for _, want := range []time.Duration{time.Second, 5 * time.Second, 30 * time.Second} {
		timer := scheduler.next(t)
		if timer.delay != want {
			t.Fatalf("recovery delay = %s, want %s", timer.delay, want)
		}
		timer.Fire()
	}
	scheduler.assertNoActiveTimers(t)
	if host.startCalls != 4 {
		t.Fatalf("start calls = %d, want startup attempt plus three recoveries", host.startCalls)
	}
	state := storedLSPState(t, store, "task-1", "kotlin")
	if state.Phase != PhaseError {
		t.Fatalf("exhausted recovery state = %#v", state)
	}
}

func TestRecoveryIsCanceledByExplicitStop(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.startErr = errors.New("crashed")
	host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 1, Phase: PhaseError}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	runtimes.ensured["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	timer := scheduler.next(t)

	if _, err := controller.Stop(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_stop",
	}); err != nil {
		t.Fatal(err)
	}
	before := host.startCalls
	timer.Fire()
	if host.startCalls != before || !timer.Stopped() {
		t.Fatalf("canceled timer restarted server: before=%d after=%d stopped=%v", before, host.startCalls, timer.Stopped())
	}
	if state := storedLSPState(t, store, "task-1", "go"); state.Policy != PolicyDisabled {
		t.Fatalf("stop policy = %q", state.Policy)
	}
}

func TestReadyFiveMinutesResetsRecoveryBudgetAndInitializingNeverRetries(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "rust", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["rust"] = RuntimeSnapshot{Language: "rust", Generation: 2, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	initialReady := scheduler.next(t)
	if initialReady.delay != readyRecoveryReset {
		t.Fatalf("ready reset delay = %s", initialReady.delay)
	}

	key := TaskLanguageKey{TaskID: "task-1", Language: "rust"}
	if err := controller.observeRuntimeSnapshot(context.Background(), key, RuntimeSnapshot{
		Language: "rust", Generation: 2, Phase: PhaseError,
	}); err != nil {
		t.Fatal(err)
	}
	firstRecovery := scheduler.next(t)
	if firstRecovery.delay != time.Second || !initialReady.Stopped() {
		t.Fatalf("error recovery=%s ready-stopped=%v", firstRecovery.delay, initialReady.Stopped())
	}
	if err := controller.observeRuntimeSnapshot(context.Background(), key, RuntimeSnapshot{
		Language: "rust", Generation: 2, Phase: PhaseReady,
	}); err != nil {
		t.Fatal(err)
	}
	reset := scheduler.next(t)
	if reset.delay != readyRecoveryReset || !firstRecovery.Stopped() {
		t.Fatalf("reset=%s recovery-stopped=%v", reset.delay, firstRecovery.Stopped())
	}
	reset.Fire()
	if err := controller.observeRuntimeSnapshot(context.Background(), key, RuntimeSnapshot{
		Language: "rust", Generation: 2, Phase: PhaseError,
	}); err != nil {
		t.Fatal(err)
	}
	afterReset := scheduler.next(t)
	if afterReset.delay != time.Second {
		t.Fatalf("post-reset recovery = %s", afterReset.delay)
	}

	afterReset.Stop()
	if err := controller.observeRuntimeSnapshot(context.Background(), key, RuntimeSnapshot{
		Language: "rust", Generation: 2, Phase: PhaseInitializing,
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.assertNoActiveTimers(t)
}

func TestReconcilerCloseJoinsWatchersAndStopsTimers(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 1, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	timer := scheduler.next(t)
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !timer.Stopped() {
		t.Fatal("close left ready timer active")
	}
}

func newReconcileControllerWithScheduler(
	store *memoryLSPStore,
	runtimes *reconcileRuntimes,
	scheduler *fakeScheduler,
) *Controller {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"task-1": readyEnvironment("task-1", "local_pc"),
	}}
	return NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(8), Scheduler: scheduler,
		Clock: func() time.Time { return time.Unix(300, 0).UTC() },
	})
}

type fakeScheduler struct {
	mu     sync.Mutex
	timers []*fakeScheduledTimer
	added  chan *fakeScheduledTimer
}

func newFakeScheduler() *fakeScheduler {
	return &fakeScheduler{added: make(chan *fakeScheduledTimer, 32)}
}

func (s *fakeScheduler) AfterFunc(delay time.Duration, callback func()) ScheduledTimer {
	timer := &fakeScheduledTimer{delay: delay, callback: callback}
	s.mu.Lock()
	s.timers = append(s.timers, timer)
	s.mu.Unlock()
	s.added <- timer
	return timer
}

func (s *fakeScheduler) next(t *testing.T) *fakeScheduledTimer {
	t.Helper()
	select {
	case timer := <-s.added:
		return timer
	case <-time.After(time.Second):
		t.Fatal("scheduled timer did not arrive")
		return nil
	}
}

func (s *fakeScheduler) assertNoActiveTimers(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, timer := range s.timers {
		if !timer.Stopped() && !timer.Fired() {
			t.Fatalf("active timer remains: %s", timer.delay)
		}
	}
}

type fakeScheduledTimer struct {
	mu       sync.Mutex
	delay    time.Duration
	callback func()
	stopped  bool
	fired    bool
}

func (t *fakeScheduledTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

func (t *fakeScheduledTimer) Fire() {
	t.mu.Lock()
	if t.stopped || t.fired {
		t.mu.Unlock()
		return
	}
	t.fired = true
	callback := t.callback
	t.mu.Unlock()
	callback()
}

func (t *fakeScheduledTimer) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

func (t *fakeScheduledTimer) Fired() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fired
}
