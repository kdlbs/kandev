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
		Language: "kotlin", Generation: 1, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
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

func TestFailedExplicitStartSchedulesAutomaticRecovery(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startErr = errors.New("task host start failed")
	host.startErrorSnapshot = &RuntimeSnapshot{
		Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed,
	}
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	snapshot, err := controller.Start(context.Background(), "task-1", "kotlin", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Phase != PhaseError {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if timer := scheduler.next(t); timer.delay != time.Second {
		t.Fatalf("first recovery delay = %s", timer.delay)
	}
}

func TestWatchFailureStopsReconnectLoopAndSchedulesRecovery(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 2, Phase: PhaseReady}
	host.watchErr = errors.New("task host connection lost")
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	if timer := scheduler.next(t); timer.delay != time.Second {
		t.Fatalf("watch recovery delay = %s", timer.delay)
	}
	state := storedLSPState(t, store, "task-1", "go")
	if state.Phase != PhaseError || state.ErrorCode != "task_host_watch_lost" {
		t.Fatalf("watch failure state = %#v", state)
	}
}

func TestRecoveryEvictsDeadTaskHostBeforeEnsuringReplacement(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "rust", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError, Generation: 3,
		LastInitiator: InitiatorAutomatic,
	})
	dead := newFakeLSPHost()
	dead.snapshotErr = errors.New("connection refused")
	replacement := newFakeLSPHost()
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = dead
	runtimes.ensured["env-task-1"] = replacement
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "rust"}
	controller.scheduleRecovery(key)
	scheduler.next(t).Fire()

	state := storedLSPState(t, store, "task-1", "rust")
	if runtimes.recoverCalls != 1 || runtimes.ensureCalls == 0 || replacement.startCalls != 1 {
		t.Fatalf("recover=%d ensure=%d starts=%d state=%#v",
			runtimes.recoverCalls, runtimes.ensureCalls, replacement.startCalls, state)
	}
	if state.Generation != 4 || state.Phase != PhaseReady {
		t.Fatalf("replacement state = %#v", state)
	}
}

func TestStartupSnapshotFailureSchedulesDeadTaskHostRecovery(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	dead := newFakeLSPHost()
	dead.snapshotErr = errors.New("connection refused")
	replacement := newFakeLSPHost()
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = dead
	runtimes.ensured["env-task-1"] = replacement
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	if err := controller.StartReconciler(context.Background()); err == nil {
		t.Fatal("startup reconcile unexpectedly accepted a dead task host")
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	timer := scheduler.next(t)
	if timer.delay != time.Second {
		t.Fatalf("startup recovery delay = %s", timer.delay)
	}
	timer.Fire()
	state := storedLSPState(t, store, "task-1", "go")
	if runtimes.recoverCalls != 1 || replacement.startCalls != 1 ||
		state.Generation != 3 || state.Phase != PhaseReady {
		t.Fatalf("recover=%d starts=%d state=%#v", runtimes.recoverCalls, replacement.startCalls, state)
	}
}

func TestProcessExitReleasesCapacityAndPromotesQueuedGeneration(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"crashed": readyEnvironment("crashed", "local_pc"),
		"queued":  readyEnvironment("queued", "local_pc"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "crashed", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	if err := controller.observeRuntimeSnapshot(context.Background(), TaskLanguageKey{
		TaskID: "crashed", Language: "go",
	}, RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
	}); err != nil {
		t.Fatal(err)
	}

	promoted, _, err := store.GetTaskLSPLanguage(context.Background(), "queued", "kotlin")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Phase != PhaseReady || controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("promoted=%#v active=%d queued=%d", promoted, controller.capacity.Active(), controller.capacity.Queued())
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

func TestStoppedReadyTimerUsesCanceledLifecycleContext(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.scheduleReadyReset(key, 1)
	timer := scheduler.next(t)
	observed := make(chan error, 1)
	store.getContextHook = func(ctx context.Context) { observed <- ctx.Err() }

	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	timer.ForceFire()
	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ready-reset context error = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stopped ready timer callback did not complete")
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

func (t *fakeScheduledTimer) ForceFire() {
	t.mu.Lock()
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
