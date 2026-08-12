package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReconcileAdoptsLiveGenerationBeforeLaunching(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseInitializing, Generation: 4,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{Language: "kotlin", Generation: 4, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	if err := controller.ReconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := storedLSPState(t, store, "task-1", "kotlin")
	if state.Generation != 4 || state.Phase != PhaseReady || host.startCalls != 0 || store.allocations != 0 {
		t.Fatalf("adopted state=%#v starts=%d allocations=%d", state, host.startCalls, store.allocations)
	}
	if controller.capacity.Active() != 1 {
		t.Fatalf("adopted capacity = %d", controller.capacity.Active())
	}
}

func TestReconcileReconnectsTaskHostBeforeAllocatingReplacementGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseInitializing, Generation: 4,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{
		Language: "kotlin", Generation: 4, Phase: PhaseReady,
	}
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	if err := controller.ReconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := storedLSPState(t, store, "task-1", "kotlin")
	if state.Generation != 4 || state.Phase != PhaseReady || store.allocations != 0 || host.startCalls != 0 {
		t.Fatalf("state=%#v allocations=%d starts=%d", state, store.allocations, host.startCalls)
	}
	if runtimes.ensureCalls != 1 || controller.capacity.Active() != 1 {
		t.Fatalf("ensure=%d active=%d", runtimes.ensureCalls, controller.capacity.Active())
	}
}

func TestExplicitStartReconnectsAndAdoptsLiveGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyInherit,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 6,
		LastInitiator: InitiatorUser,
	})
	host := newFakeLSPHost()
	host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 6, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	snapshot, err := controller.Start(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 6 || snapshot.Phase != PhaseReady || store.allocations != 0 || host.startCalls != 0 {
		t.Fatalf("snapshot=%#v allocations=%d starts=%d", snapshot, store.allocations, host.startCalls)
	}
	if runtimes.ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want reconnect probe", runtimes.ensureCalls)
	}
	if snapshot.Policy != PolicyKeepWarm || snapshot.LastAction != ActionStart ||
		snapshot.LastInitiator != InitiatorUser {
		t.Fatalf("explicit Start evidence = %#v", snapshot)
	}
}

func TestReconcileStartsOneReplacementForMissingDesiredRuntime(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseStarting, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	if err := controller.ReconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := storedLSPState(t, store, "task-1", "go")
	if host.startCalls != 1 || runtimes.ensureCalls == 0 || store.allocations != 1 || state.Generation != 3 {
		t.Fatalf("state=%#v starts=%d ensure=%d allocations=%d", state, host.startCalls, runtimes.ensureCalls, store.allocations)
	}
	if state.Policy != PolicyKeepWarm {
		t.Fatalf("automatic replacement changed policy: %q", state.Policy)
	}
}

func TestReconcileMissingRechecksPolicyBeforeAllocatingGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseOff, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	stale := storedLSPState(t, store, "task-1", "go")
	current := stale
	current.Policy = PolicyDisabled
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), current, stale.Revision); err != nil {
		t.Fatal(err)
	}
	runtimes := newReconcileRuntimes()
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	snapshot, err := controller.reconcileMissing(context.Background(), reconcileCandidate{
		state: stale, settings: TaskSettings{},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := storedLSPState(t, store, "task-1", "go")
	if store.allocations != 0 || runtimes.ensureCalls != 0 || state.Generation != 2 {
		t.Fatalf("state=%#v ensure=%d allocations=%d", state, runtimes.ensureCalls, store.allocations)
	}
	if snapshot == nil || snapshot.Policy != PolicyDisabled || snapshot.Phase != PhaseOff {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestReconcileStopsDisabledOrphanAndRebuildsActualCapacity(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "disabled", Language: "kotlin", Policy: PolicyDisabled,
		DetectionState: DetectionComplete, Phase: PhaseOff, Generation: 2,
		LastInitiator: InitiatorUser,
	})
	for _, taskID := range []string{"live-a", "live-b"} {
		seedLSPState(t, store, TaskLanguageState{
			TaskID: taskID, Language: "go", Policy: PolicyKeepWarm,
			DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 5,
			LastInitiator: InitiatorAutomatic,
		})
	}
	runtimes := newReconcileRuntimes()
	disabledHost := newFakeLSPHost()
	disabledHost.snapshots["kotlin"] = RuntimeSnapshot{Language: "kotlin", Generation: 2, Phase: PhaseReady}
	runtimes.existing["env-disabled"] = disabledHost
	for _, taskID := range []string{"live-a", "live-b"} {
		host := newFakeLSPHost()
		host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 5, Phase: PhaseReady}
		runtimes.existing["env-"+taskID] = host
	}
	controller := newReconcileController(store, runtimes, NewCapacity(1))

	if err := controller.ReconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if disabledHost.stopCalls != 1 {
		t.Fatalf("disabled orphan stop calls = %d", disabledHost.stopCalls)
	}
	if controller.capacity.Active() != 2 || controller.capacity.Queued() != 0 {
		t.Fatalf("actual capacity active=%d queued=%d", controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestReconcileUnreachableTaskHostNeverLaunchesPossibleDuplicate(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "rust", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 7,
		LastInitiator: InitiatorAutomatic,
	})
	runtimes := newReconcileRuntimes()
	runtimes.existingErrors["env-task-1"] = errors.New("task host transport lost")
	controller := newReconcileController(store, runtimes, NewCapacity(8))

	if err := controller.ReconcileAll(context.Background()); err == nil {
		t.Fatal("unreachable task host was not reported")
	}
	state := storedLSPState(t, store, "task-1", "rust")
	if state.Phase != PhaseError || state.ErrorCode != "task_host_unreachable" || runtimes.ensureCalls != 0 {
		t.Fatalf("state=%#v ensure=%d", state, runtimes.ensureCalls)
	}
}

func TestCleanupPreservesPolicyAndStopsEveryTaskLanguage(t *testing.T) {
	store := newMemoryLSPStore()
	for _, language := range []string{"go", "kotlin"} {
		seedLSPState(t, store, TaskLanguageState{
			TaskID: "task-1", Language: language, Policy: PolicyKeepWarm,
			DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 3,
			LastInitiator: InitiatorUser,
		})
	}
	host := newFakeLSPHost()
	host.snapshots["go"] = RuntimeSnapshot{Language: "go", Generation: 3, Phase: PhaseReady}
	host.snapshots["kotlin"] = RuntimeSnapshot{Language: "kotlin", Generation: 3, Phase: PhaseReady}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	capacity := NewCapacity(8)
	capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "go"}, 3)
	capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}, 3)
	controller := newReconcileController(store, runtimes, capacity)

	if err := controller.CleanupTask(context.Background(), "task-1", "task_archived"); err != nil {
		t.Fatal(err)
	}
	if host.stopCalls != 2 || capacity.Active() != 0 {
		t.Fatalf("stop calls=%d active=%d", host.stopCalls, capacity.Active())
	}
	if runtimes.cleanupCalls != 1 || runtimes.cleanupEnvironment != "env-task-1" || runtimes.cleanupReason != "task_archived" {
		t.Fatalf("task-host cleanup calls=%d environment=%q reason=%q",
			runtimes.cleanupCalls, runtimes.cleanupEnvironment, runtimes.cleanupReason)
	}
	for _, language := range []string{"go", "kotlin"} {
		state := storedLSPState(t, store, "task-1", language)
		if state.Policy != PolicyKeepWarm || state.Phase != PhaseOff || state.LastStopReason != "task_archived" {
			t.Fatalf("cleaned %s state=%#v", language, state)
		}
	}
}

func TestCleanupReleasesGenerationStartedAfterInitialSnapshot(t *testing.T) {
	baseStore := newMemoryLSPStore()
	seedLSPState(t, baseStore, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseOff, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	store := &cleanupSnapshotStore{
		memoryLSPStore: baseStore,
		listed:         make(chan struct{}),
		release:        make(chan struct{}),
	}
	host := newFakeLSPHost()
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	runtimes := &fakeLSPRuntimes{host: host}
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{}, Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(1), Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})

	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- controller.CleanupTask(context.Background(), "task-1", "task_archived")
	}()
	<-store.listed
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	type startResult struct {
		snapshot *LanguageSnapshot
		err      error
	}
	startDone := make(chan startResult, 1)
	go func() {
		snapshot, startErr := controller.Start(context.Background(), "task-1", "go", origin)
		startDone <- startResult{snapshot: snapshot, err: startErr}
	}()
	<-host.startEntered
	close(store.release)
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	if !commandQueuedWithin(controller, key, time.Second) {
		close(host.startRelease)
		<-startDone
		<-cleanupDone
		t.Fatal("cleanup did not serialize behind the running successor generation")
	}
	close(host.startRelease)
	started := <-startDone
	if started.err != nil {
		t.Fatal(started.err)
	}
	if started.snapshot.Generation != 2 {
		t.Fatalf("started=%#v", started.snapshot)
	}
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	state := storedLSPState(t, baseStore, "task-1", "go")
	if state.Generation != 2 || state.Phase != PhaseOff || controller.capacity.Active() != 0 {
		t.Fatalf("cleaned state=%#v active=%d", state, controller.capacity.Active())
	}
	if host.lastStop.Generation != 2 {
		t.Fatalf("stopped generation = %d, want successor generation 2", host.lastStop.Generation)
	}
}

func TestCleanupRetainsCapacityWhenStopAndTaskHostCleanupFail(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 3,
		LastInitiator: InitiatorUser,
	})
	stopFailure := errors.New("language server may still be running")
	cleanupFailure := errors.New("task host cleanup failed")
	host := newFakeLSPHost()
	host.stopErr = stopFailure
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	runtimes.cleanupErr = cleanupFailure
	capacity := NewCapacity(8)
	capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}, 3)
	controller := newReconcileController(store, runtimes, capacity)

	err := controller.CleanupTask(context.Background(), "task-1", "task_archived")
	if !errors.Is(err, stopFailure) || !errors.Is(err, cleanupFailure) {
		t.Fatalf("cleanup error = %v, want stop and task-host failures", err)
	}
	if capacity.Active() != 1 {
		t.Fatalf("active capacity = %d, want retained slot", capacity.Active())
	}
	state := storedLSPState(t, store, "task-1", "kotlin")
	if state.Phase != PhaseError || state.ErrorCode != "task_host_stop_failed" {
		t.Fatalf("failed cleanup state = %#v", state)
	}
}

func TestCleanupTaskHostFallbackReleasesFailedLanguage(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 3,
		LastInitiator: InitiatorUser,
	})
	host := newFakeLSPHost()
	host.stopErr = errors.New("per-language stop failed")
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	capacity := NewCapacity(8)
	capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}, 3)
	controller := newReconcileController(store, runtimes, capacity)

	if err := controller.CleanupTask(context.Background(), "task-1", "task_archived"); err != nil {
		t.Fatalf("task-host fallback cleanup: %v", err)
	}
	if capacity.Active() != 0 {
		t.Fatalf("active capacity = %d, want released slot", capacity.Active())
	}
	state := storedLSPState(t, store, "task-1", "kotlin")
	if state.Phase != PhaseOff || state.ErrorCode != "" {
		t.Fatalf("fallback-cleaned state = %#v", state)
	}
}

func TestCleanupCancelsTaskWorkWhenEnvironmentLookupFails(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseQueued, Generation: 1,
		LastInitiator: InitiatorUser,
	})
	tasks := &fakeControllerTasks{environmentErr: errors.New("environment unavailable")}
	controller := NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: &fakeLSPSettings{}, Runtimes: newReconcileRuntimes(),
		Capacity: NewCapacity(1), Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.capacity.Admit(TaskLanguageKey{TaskID: "other", Language: "rust"}, 1, time.Unix(1, 0))
	controller.capacity.Admit(key, 1, time.Unix(2, 0))
	timer := &fakeScheduledTimer{}
	watchCanceled := false
	controller.recoveries[key] = &recoveryState{timer: timer}
	controller.watches[key] = func() { watchCanceled = true }

	if err := controller.CleanupTask(context.Background(), "task-1", "task_archived"); err == nil {
		t.Fatal("cleanup unexpectedly succeeded")
	}
	if !timer.Stopped() || !watchCanceled || controller.capacity.Queued() != 0 {
		t.Fatalf("cleanup cancellation timer=%v watch=%v queued=%d",
			timer.Stopped(), watchCanceled, controller.capacity.Queued())
	}
}

func newReconcileController(store *memoryLSPStore, runtimes *reconcileRuntimes, capacity *Capacity) *Controller {
	tasks := &fakeControllerTasks{environments: make(map[string]*models.TaskEnvironment)}
	for _, taskID := range []string{"task-1", "disabled", "live-a", "live-b"} {
		tasks.environments[taskID] = readyEnvironment(taskID, "local_pc")
	}
	return NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: capacity, Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})
}

func seedLSPState(t *testing.T, store *memoryLSPStore, state TaskLanguageState) {
	t.Helper()
	if state.LastTransitionAt.IsZero() {
		state.LastTransitionAt = time.Unix(100, 0).UTC()
	}
	if _, err := store.CompareAndUpdateTaskLSPLanguage(context.Background(), state, 0); err != nil {
		t.Fatal(err)
	}
}

func storedLSPState(t *testing.T, store *memoryLSPStore, taskID, language string) TaskLanguageState {
	t.Helper()
	state, _, err := store.GetTaskLSPLanguage(context.Background(), taskID, language)
	if err != nil {
		t.Fatal(err)
	}
	return *state
}

type reconcileRuntimes struct {
	mu                 sync.Mutex
	existing           map[string]*fakeLSPHost
	ensured            map[string]*fakeLSPHost
	existingErrors     map[string]error
	ensureCalls        int
	cleanupCalls       int
	cleanupEnvironment string
	cleanupReason      string
	cleanupErr         error
	recoverCalls       int
	recoverErr         error
}

type cleanupSnapshotStore struct {
	*memoryLSPStore
	listed  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *cleanupSnapshotStore) ListTaskLSPLanguages(
	ctx context.Context,
	taskID string,
) ([]TaskLanguageState, error) {
	states, err := s.memoryLSPStore.ListTaskLSPLanguages(ctx, taskID)
	s.once.Do(func() {
		close(s.listed)
		<-s.release
	})
	return states, err
}

func newReconcileRuntimes() *reconcileRuntimes {
	return &reconcileRuntimes{
		existing: make(map[string]*fakeLSPHost), ensured: make(map[string]*fakeLSPHost),
		existingErrors: make(map[string]error),
	}
}

func (r *reconcileRuntimes) ExistingTaskHost(_ context.Context, environmentID string) (TaskHost, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.existingErrors[environmentID]; err != nil {
		return nil, false, err
	}
	host := r.existing[environmentID]
	return host, host != nil, nil
}

func (r *reconcileRuntimes) EnsureTaskHost(_ context.Context, environmentID string) (TaskHost, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureCalls++
	host := r.ensured[environmentID]
	if host == nil {
		host = newFakeLSPHost()
		r.ensured[environmentID] = host
	}
	r.existing[environmentID] = host
	return host, nil
}

func (r *reconcileRuntimes) CleanupTaskHost(_ context.Context, environmentID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupCalls++
	r.cleanupEnvironment = environmentID
	r.cleanupReason = reason
	if r.cleanupErr != nil {
		return r.cleanupErr
	}
	delete(r.existing, environmentID)
	return nil
}

func (r *reconcileRuntimes) RecoverTaskHost(_ context.Context, environmentID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recoverCalls++
	if r.recoverErr != nil {
		return false, r.recoverErr
	}
	if r.existing[environmentID] == nil {
		return false, nil
	}
	delete(r.existing, environmentID)
	return true, nil
}

func (r *reconcileRuntimes) DiscoverTaskLanguages(context.Context, string) (*DiscoveryResult, error) {
	return &DiscoveryResult{State: DetectionComplete, ScannedAt: time.Unix(200, 0).UTC()}, nil
}
