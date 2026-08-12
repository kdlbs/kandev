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

func TestStartupReconciliationBlocksControlsUntilCapacityIsRebuilt(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 4,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{Language: "kotlin", Generation: 4, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	runtimes.existingEntered = make(chan struct{})
	runtimes.existingRelease = make(chan struct{})
	runtimes.blockEnvironment = "env-task-1"
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"task-1": readyEnvironment("task-1", executorTypeLocalPC),
		"other":  readyEnvironment("other", executorTypeLocalPC),
	}}
	controller := NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(1), Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})
	reconcileDone := make(chan error, 1)
	go func() { reconcileDone <- controller.StartReconciler(context.Background()) }()
	<-runtimes.existingEntered

	type startResult struct {
		snapshot *LanguageSnapshot
		err      error
	}
	startDone := make(chan startResult, 1)
	go func() {
		snapshot, err := controller.Start(context.Background(), "other", "go", Origin{
			Initiator: InitiatorUser, Reason: "user_control",
		})
		startDone <- startResult{snapshot: snapshot, err: err}
	}()
	select {
	case result := <-startDone:
		t.Fatalf("control escaped startup barrier: %#v, %v", result.snapshot, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	close(runtimes.existingRelease)
	if err := <-reconcileDone; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	result := <-startDone
	if result.err != nil || result.snapshot == nil || result.snapshot.Phase != PhaseQueued {
		t.Fatalf("post-reconcile start = %#v, %v; want queued behind survivor", result.snapshot, result.err)
	}
	if controller.capacity.Active() != 1 || controller.capacity.Queued() != 1 {
		t.Fatalf("capacity active=%d queued=%d", controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestFailedDiscoveryRetriesWithoutAnotherSnapshot(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{discoveryErr: errors.New("task host still materializing")}
	scheduler := newFakeScheduler()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"task-1": readyEnvironment("task-1", executorTypeLocalPC),
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(8), Scheduler: scheduler,
		Clock: func() time.Time { return time.Unix(300, 0).UTC() },
	})
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	if _, err := controller.Snapshot(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	timer := scheduler.next(t)
	if timer.delay != time.Second {
		t.Fatalf("discovery retry delay = %s, want 1s", timer.delay)
	}
	runtimes.mu.Lock()
	runtimes.discoveryErr = nil
	runtimes.discovery = &DiscoveryResult{
		Languages: []string{"kotlin"}, State: DetectionComplete, ScannedAt: time.Unix(301, 0).UTC(),
	}
	runtimes.mu.Unlock()
	timer.Fire()

	state := storedLSPState(t, store, "task-1", "kotlin")
	if !state.Detected || state.DetectionState != DetectionComplete {
		t.Fatalf("automatic discovery retry state = %#v", state)
	}
	runtimes.mu.Lock()
	discoveryCalls := runtimes.discoveryCalls
	runtimes.mu.Unlock()
	if discoveryCalls != 2 {
		t.Fatalf("discovery calls = %d, want initial plus retry", discoveryCalls)
	}
	scheduler.assertNoActiveTimers(t)
}

func TestCloseCancelsPendingDiscoveryRetry(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{discoveryErr: errors.New("scanner unavailable")}
	scheduler := newFakeScheduler()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"task-1": readyEnvironment("task-1", executorTypeLocalPC),
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(8), Scheduler: scheduler,
	})
	if err := controller.StartReconciler(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Snapshot(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	timer := scheduler.next(t)
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !timer.Stopped() {
		t.Fatal("pending discovery retry remained live after controller close")
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

func TestRecoveryTreatsLiveRuntimeAdoptionAsConverged(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "rust", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError, Generation: 3,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["rust"] = RuntimeSnapshot{
		Language: "rust", Generation: 3, Phase: PhaseReady,
	}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	controller := newReconcileControllerWithScheduler(store, runtimes, newFakeScheduler())

	state := storedLSPState(t, store, "task-1", "rust")
	if converged := controller.attemptRecovery(
		context.Background(),
		TaskLanguageKey{TaskID: "task-1", Language: "rust"},
		state,
	); !converged {
		t.Fatal("live runtime adoption consumed another recovery attempt")
	}
	if stored := storedLSPState(t, store, "task-1", "rust"); stored.Phase != PhaseReady {
		t.Fatalf("adopted state = %#v, want ready", stored)
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
		"crashed": readyEnvironment("crashed", executorTypeLocalPC),
		"queued":  readyEnvironment("queued", executorTypeLocalPC),
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

	promoted := waitForStoredPhase(t, store, "queued", "kotlin", PhaseReady)
	if promoted.Phase != PhaseReady || controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("promoted=%#v active=%d queued=%d", promoted, controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestStaleRuntimeRevisionCannotRegressPhaseOrReacquireCapacity(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"crashed": readyEnvironment("crashed", executorTypeLocalPC),
		"queued":  readyEnvironment("queued", executorTypeLocalPC),
	}}
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "crashed", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "queued", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseQueued, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: newFakeLSPHost()})
	controller.capacity = NewCapacity(1)
	crashedKey := TaskLanguageKey{TaskID: "crashed", Language: "go"}
	queuedKey := TaskLanguageKey{TaskID: "queued", Language: "kotlin"}
	controller.capacity.Adopt(crashedKey, 1)
	if controller.capacity.Admit(queuedKey, 1, time.Unix(1, 0)) {
		t.Fatal("queued generation unexpectedly admitted")
	}
	runtimeStartedAt := time.Unix(100, 0).UTC()

	if err := controller.observeRuntimeSnapshot(context.Background(), crashedKey, RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
		Revision: 3, Incarnation: "task-host-a", RuntimeStartedAt: runtimeStartedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := controller.observeRuntimeSnapshot(context.Background(), crashedKey, RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseProcessStarted,
		Revision: 2, Incarnation: "task-host-a", RuntimeStartedAt: runtimeStartedAt,
	}); err != nil {
		t.Fatal(err)
	}

	crashed := storedLSPState(t, store, "crashed", "go")
	queued := waitForStoredPhase(t, store, "queued", "kotlin", PhaseReady)
	if crashed.Phase != PhaseError || crashed.RuntimeRevision != 3 {
		t.Fatalf("crashed state regressed to %#v", crashed)
	}
	if queued.Phase != PhaseReady || controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("queued=%#v active=%d queued-count=%d", queued, controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestNewTaskHostIncarnationReplacesOldHighWaterAndRejectsLateWatch(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseStarting, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{})
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	oldStartedAt := time.Unix(100, 0).UTC()
	newStartedAt := time.Unix(200, 0).UTC()

	for _, runtime := range []RuntimeSnapshot{
		{Language: "go", Generation: 1, Phase: PhaseInitializing, Revision: 8, Incarnation: "old", RuntimeStartedAt: oldStartedAt},
		{Language: "go", Generation: 1, Phase: PhaseReady, Revision: 1, Incarnation: "new", RuntimeStartedAt: newStartedAt},
		{Language: "go", Generation: 1, Phase: PhaseError, Revision: 9, Incarnation: "old", RuntimeStartedAt: oldStartedAt, ErrorCode: errorCodeProcessExited},
	} {
		if err := controller.observeRuntimeSnapshot(context.Background(), key, runtime); err != nil {
			t.Fatal(err)
		}
	}

	state := storedLSPState(t, store, key.TaskID, key.Language)
	if state.Phase != PhaseReady || state.RuntimeIncarnation != "new" || state.RuntimeRevision != 1 {
		t.Fatalf("late retired-host watch replaced current state: %#v", state)
	}
}

func TestProcessCleanupFailureRetainsCapacityWithoutRecovery(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "crashed", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "queued", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseQueued, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	controller.capacity = NewCapacity(1)
	crashedKey := TaskLanguageKey{TaskID: "crashed", Language: "go"}
	queuedKey := TaskLanguageKey{TaskID: "queued", Language: "kotlin"}
	controller.capacity.Adopt(crashedKey, 1)
	if controller.capacity.Admit(queuedKey, 1, time.Unix(1, 0)) {
		t.Fatal("queued generation unexpectedly admitted")
	}
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	if err := controller.observeRuntimeSnapshot(context.Background(), crashedKey, RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseError, ErrorCode: "process_cleanup_failed",
	}); err != nil {
		t.Fatal(err)
	}

	queued := storedLSPState(t, store, "queued", "kotlin")
	if queued.Phase != PhaseQueued || controller.capacity.Active() != 1 || controller.capacity.Queued() != 1 {
		t.Fatalf("queued=%#v active=%d queued-count=%d",
			queued, controller.capacity.Active(), controller.capacity.Queued())
	}
	scheduler.assertNoActiveTimers(t)
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
	host.snapshots["go"] = RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
	}
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
		Language: "rust", Generation: 2, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
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
		Language: "rust", Generation: 2, Phase: PhaseError, ErrorCode: errorCodeProcessExited,
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

func TestCanceledWatchCannotDeleteReplacementRegistration(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	host := newControlledWatchHost()
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	controller := newReconcileControllerWithScheduler(store, runtimes, newFakeScheduler())
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.ensureWatch(key)
	<-host.firstStarted
	controller.cancelWatch(key)
	<-host.firstCanceled
	controller.ensureWatch(key)
	<-host.secondStarted
	close(host.firstRelease)
	<-host.firstReturned

	controller.lifecycleMu.Lock()
	replacementRegistered := controller.watches[key] != nil
	controller.lifecycleMu.Unlock()
	if !replacementRegistered {
		t.Fatal("canceled watch deleted its replacement registration")
	}
}

type controlledWatchHost struct {
	*fakeLSPHost
	mu            sync.Mutex
	calls         int
	firstStarted  chan struct{}
	firstCanceled chan struct{}
	firstRelease  chan struct{}
	firstReturned chan struct{}
	secondStarted chan struct{}
}

func newControlledWatchHost() *controlledWatchHost {
	return &controlledWatchHost{
		fakeLSPHost:  newFakeLSPHost(),
		firstStarted: make(chan struct{}), firstCanceled: make(chan struct{}),
		firstRelease: make(chan struct{}), firstReturned: make(chan struct{}),
		secondStarted: make(chan struct{}),
	}
}

func (h *controlledWatchHost) WatchTaskLSP(
	ctx context.Context,
	_ string,
	_ func(RuntimeSnapshot) error,
) error {
	h.mu.Lock()
	h.calls++
	call := h.calls
	h.mu.Unlock()
	if call == 1 {
		close(h.firstStarted)
		<-ctx.Done()
		close(h.firstCanceled)
		<-h.firstRelease
		close(h.firstReturned)
		return context.Cause(ctx)
	}
	close(h.secondStarted)
	<-ctx.Done()
	return context.Cause(ctx)
}

func newReconcileControllerWithScheduler(
	store *memoryLSPStore,
	runtimes *reconcileRuntimes,
	scheduler *fakeScheduler,
) *Controller {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"task-1": readyEnvironment("task-1", executorTypeLocalPC),
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
