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
