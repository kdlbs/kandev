package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStartupInventoryFailureFailsControlsClosed(t *testing.T) {
	baseStore := newMemoryLSPStore()
	seedLSPState(t, baseStore, TaskLanguageState{
		TaskID: "survivor", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 4,
		LastInitiator: InitiatorAutomatic,
	})
	inventoryErr := errors.New("durable LSP inventory unavailable")
	store := &failingStartupInventoryStore{memoryLSPStore: baseStore, listErr: inventoryErr}
	host := newFakeLSPHost()
	host.snapshots["kotlin"] = RuntimeSnapshot{
		Language: "kotlin", Generation: 4, Phase: PhaseReady,
	}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-survivor"] = host
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"survivor": readyEnvironment("survivor", executorTypeLocalPC),
			"other":    readyEnvironment("other", executorTypeLocalPC),
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: runtimes,
		Capacity: NewCapacity(1),
		Clock:    func() time.Time { return time.Unix(400, 0).UTC() },
	})
	if err := controller.StartReconciler(context.Background()); !errors.Is(err, inventoryErr) {
		t.Fatalf("startup error = %v, want inventory failure", err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	snapshot, err := controller.Start(context.Background(), "other", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if snapshot != nil || !errors.Is(err, inventoryErr) {
		t.Fatalf("control result = %#v, %v; want sticky inventory failure", snapshot, err)
	}
	if controller.capacity.Active() != 0 || controller.capacity.Queued() != 0 {
		t.Fatalf("capacity active=%d queued=%d after failed inventory", controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestReconcileInventoryReturnsPostReconcileStates(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 4,
		LastInitiator: InitiatorAutomatic,
	})
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"task-1": {
				ID: "env-task-1", TaskID: "task-1", ExecutorType: executorTypeLocalPC,
				Status: models.TaskEnvironmentStatusStopped,
			},
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: newReconcileRuntimes(),
		Capacity: NewCapacity(1), Clock: func() time.Time { return time.Unix(400, 0).UTC() },
	})

	states, inventoryReady, err := controller.reconcileAllWithInventory(context.Background())
	if err != nil || !inventoryReady {
		t.Fatalf("reconcile inventory = %#v, ready=%t, err=%v", states, inventoryReady, err)
	}
	if len(states) != 1 || states[0].Phase != PhaseWaitingForTask {
		t.Fatalf("returned states = %#v, want post-reconcile waiting state", states)
	}
}

func TestWatchLossPreservesNonServerState(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseWaitingForTask, Generation: 2,
		LastInitiator: InitiatorAutomatic,
	})
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"task-1": {
				ID: "env-task-1", TaskID: "task-1", ExecutorType: executorTypeLocalPC,
				Status: models.TaskEnvironmentStatusStopped,
			},
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: newReconcileRuntimes(),
		Capacity: NewCapacity(1), Clock: func() time.Time { return time.Unix(400, 0).UTC() },
	})
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.ensureWatch(key)
	waitForWatchRemoval(t, controller, key)
	state := storedLSPState(t, store, key.TaskID, key.Language)
	if state.Phase != PhaseWaitingForTask || state.ErrorCode != "" {
		t.Fatalf("non-server state after watch loss = %#v", state)
	}
}

func TestCloseJoinsFiredRecoveryCommand(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError,
		LastInitiator: InitiatorAutomatic,
	})
	host := newBlockingRecoveryHost()
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	installTestLifecycle(controller)

	key := TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}
	controller.scheduleRecovery(key)
	timer := scheduler.next(t)
	fireDone := make(chan struct{})
	go func() {
		timer.Fire()
		close(fireDone)
	}()
	<-host.entered

	closeDone := make(chan error, 1)
	go func() { closeDone <- controller.Close(context.Background()) }()
	assertStillRunning(t, closeDone, "Close returned while a fired recovery command was running")
	close(host.release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	<-fireDone
	<-host.returned
}

func TestCloseJoinsFiredRecoveryRead(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	installTestLifecycle(controller)
	controller.scheduleRecovery(TaskLanguageKey{TaskID: "task-1", Language: "go"})
	timer := scheduler.next(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once
	store.getContextHook = func(context.Context) {
		blockOnce.Do(func() {
			close(entered)
			<-release
		})
	}
	fireDone := make(chan struct{})
	go func() {
		timer.Fire()
		close(fireDone)
	}()
	<-entered

	closeDone := make(chan error, 1)
	go func() { closeDone <- controller.Close(context.Background()) }()
	assertStillRunning(t, closeDone, "Close returned while a fired recovery read was running")
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	<-fireDone
}

func TestCloseJoinsFiredReadyReset(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	installTestLifecycle(controller)
	controller.scheduleReadyReset(TaskLanguageKey{TaskID: "task-1", Language: "go"}, 1)
	timer := scheduler.next(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once
	store.getContextHook = func(context.Context) {
		blockOnce.Do(func() {
			close(entered)
			<-release
		})
	}
	fireDone := make(chan struct{})
	go func() {
		timer.Fire()
		close(fireDone)
	}()
	<-entered

	closeDone := make(chan error, 1)
	go func() { closeDone <- controller.Close(context.Background()) }()
	assertStillRunning(t, closeDone, "Close returned while a fired ready-reset callback was running")
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	<-fireDone
}

func TestCloseRetryWaitsForOriginalLifecycleJoin(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	installTestLifecycle(controller)
	controller.scheduleReadyReset(TaskLanguageKey{TaskID: "task-1", Language: "go"}, 1)
	timer := scheduler.next(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	store.getContextHook = func(context.Context) {
		select {
		case <-entered:
		default:
			close(entered)
			<-release
		}
	}
	fireDone := make(chan struct{})
	go func() {
		timer.Fire()
		close(fireDone)
	}()
	<-entered

	firstCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	firstErr := controller.Close(firstCtx)
	cancel()
	if !errors.Is(firstErr, context.DeadlineExceeded) {
		close(release)
		<-fireDone
		t.Fatalf("first Close error = %v, want deadline exceeded", firstErr)
	}
	retryDone := make(chan error, 1)
	go func() { retryDone <- controller.Close(context.Background()) }()
	retryReturnedEarly := false
	select {
	case <-retryDone:
		retryReturnedEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-fireDone
	if retryReturnedEarly {
		t.Fatal("retried Close returned before the original lifecycle joined")
	}
	if err := <-retryDone; err != nil {
		t.Fatal(err)
	}
}

func TestCanceledRecoveryTimerCannotConsumeReplacementEpoch(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "kotlin", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseError, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	host := newFakeLSPHost()
	runtimes := newReconcileRuntimes()
	runtimes.ensured["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}
	controller.scheduleRecovery(key)
	stale := scheduler.next(t)
	controller.cancelRecovery(key)
	controller.scheduleRecovery(key)
	replacement := scheduler.next(t)
	stale.ForceFire()

	controller.lifecycleMu.Lock()
	current := controller.recoveries[key]
	controller.lifecycleMu.Unlock()
	if current == nil || current.timer != replacement {
		t.Fatal("stale recovery callback consumed the replacement timer")
	}
	if host.startCalls != 0 {
		t.Fatalf("stale recovery callback launched %d server(s)", host.startCalls)
	}
}

func TestCanceledReadyResetCannotConsumeReplacementEpoch(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, newReconcileRuntimes(), scheduler)
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.scheduleReadyReset(key, 1)
	stale := scheduler.next(t)
	controller.cancelRecovery(key)
	controller.scheduleReadyReset(key, 1)
	replacement := scheduler.next(t)
	controller.lifecycleMu.Lock()
	controller.recoveries[key].attempts = 2
	controller.lifecycleMu.Unlock()
	stale.ForceFire()

	controller.lifecycleMu.Lock()
	current := controller.recoveries[key]
	controller.lifecycleMu.Unlock()
	if current == nil || current.readyTimer != replacement || current.attempts != 2 {
		t.Fatalf("stale ready-reset callback mutated replacement state: %#v", current)
	}
}

func TestCanceledDiscoveryTimerCannotConsumeReplacementEpoch(t *testing.T) {
	store := newMemoryLSPStore()
	scheduler := newFakeScheduler()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
			"task-1": readyEnvironment("task-1", executorTypeLocalPC),
		}},
		Store: store, Settings: &fakeLSPSettings{}, Runtimes: &fakeLSPRuntimes{},
		Capacity: NewCapacity(1), Scheduler: scheduler,
		Clock: func() time.Time { return time.Unix(400, 0).UTC() },
	})
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	controller.scheduleDiscoveryRetry("task-1")
	stale := scheduler.next(t)
	controller.cancelDiscoveryRetry("task-1")
	controller.scheduleDiscoveryRetry("task-1")
	replacement := scheduler.next(t)
	stale.ForceFire()

	controller.lifecycleMu.Lock()
	current := controller.discoveryRetries["task-1"]
	controller.lifecycleMu.Unlock()
	if current == nil || current.timer != replacement {
		t.Fatal("stale discovery callback consumed the replacement timer")
	}
}

type failingStartupInventoryStore struct {
	*memoryLSPStore
	listErr error
}

func (s *failingStartupInventoryStore) ListAllTaskLSPLanguages(
	ctx context.Context,
) ([]TaskLanguageState, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.memoryLSPStore.ListAllTaskLSPLanguages(ctx)
}

type blockingRecoveryHost struct {
	*fakeLSPHost
	entered  chan struct{}
	release  chan struct{}
	returned chan struct{}
}

func newBlockingRecoveryHost() *blockingRecoveryHost {
	return &blockingRecoveryHost{
		fakeLSPHost: newFakeLSPHost(),
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
		returned:    make(chan struct{}),
	}
}

func (h *blockingRecoveryHost) StartTaskLSP(
	_ context.Context,
	request TaskHostStartRequest,
) (*RuntimeSnapshot, error) {
	close(h.entered)
	<-h.release
	snapshot := h.setReady(request.Language, request.Generation)
	close(h.returned)
	return snapshot, nil
}

func installTestLifecycle(controller *Controller) {
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
}

func assertStillRunning(t *testing.T, done <-chan error, message string) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s: %v", message, err)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForWatchRemoval(t *testing.T, controller *Controller, key TaskLanguageKey) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		controller.lifecycleMu.Lock()
		watch := controller.watches[key]
		controller.lifecycleMu.Unlock()
		if watch == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("watch did not exit")
}
