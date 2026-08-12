package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCapacityAndUnsupportedExecutorDoNotEnsureTaskResources(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"unsupported": readyEnvironment("unsupported", "ssh"),
		"first":       readyEnvironment("first", "local_docker"),
		"queued":      readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{host: newFakeLSPHost()}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, runtimes)
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	unsupported, err := controller.Start(context.Background(), "unsupported", "go", origin)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Phase != PhaseUnsupported || runtimes.ensureCalls != 0 || controller.capacity.Active() != 0 {
		t.Fatalf("unsupported=%#v ensure=%d active=%d", unsupported, runtimes.ensureCalls, controller.capacity.Active())
	}
	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Phase != PhaseQueued || runtimes.ensureCalls != 1 || controller.capacity.Queued() != 1 {
		t.Fatalf("queued=%#v ensure=%d queued-count=%d", queued, runtimes.ensureCalls, controller.capacity.Queued())
	}
}

func TestCapacityReleaseStartsQueuedAcceptedGeneration(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued || queued.Generation != 1 {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}

	promoted := waitForStoredPhase(t, store, "queued", "kotlin", PhaseReady)
	if promoted.Phase != PhaseReady || promoted.Generation != 1 ||
		controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("promoted=%#v active=%d queued=%d", promoted, controller.capacity.Active(), controller.capacity.Queued())
	}
	if host.startCalls != 2 || store.allocations != 2 {
		t.Fatalf("start calls=%d allocations=%d, want accepted generation reused", host.startCalls, store.allocations)
	}
}

func TestStopDoesNotWaitForAnotherTasksQueuedPromotion(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	var releaseOnce sync.Once
	releasePromotion := func() { releaseOnce.Do(func() { close(host.startRelease) }) }
	defer releasePromotion()

	stopDone := make(chan error, 1)
	go func() {
		_, stopErr := controller.Stop(context.Background(), "first", "go", origin)
		stopDone <- stopErr
	}()
	select {
	case <-host.startEntered:
	case <-time.After(time.Second):
		t.Fatal("queued promotion did not begin")
	}
	select {
	case stopErr := <-stopDone:
		if stopErr != nil {
			t.Fatal(stopErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop waited for another task's queued server promotion")
	}

	releasePromotion()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		promoted, _, getErr := store.GetTaskLSPLanguage(context.Background(), "queued", "kotlin")
		if getErr == nil && promoted.Phase == PhaseReady {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queued promotion did not finish after its start barrier released")
}

func TestCanceledQueuedPromotionReleasesReservationAndPromotesNextOnce(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"second": readyEnvironment("second", "local_docker"),
		"third":  readyEnvironment("third", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	if queued, err := controller.Start(context.Background(), "second", "kotlin", origin); err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("second queued=%#v error=%v", queued, err)
	}
	if queued, err := controller.Start(context.Background(), "third", "typescript", origin); err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("third queued=%#v error=%v", queued, err)
	}

	secondKey := TaskLanguageKey{TaskID: "second", Language: "kotlin"}
	blockerEntered := make(chan struct{})
	blockerDone := make(chan error, 1)
	go func() {
		_, err := controller.commands.submitExclusive(
			context.Background(), secondKey, ActionReconcile,
			func(ctx context.Context) (*LanguageSnapshot, error) {
				close(blockerEntered)
				<-ctx.Done()
				return nil, context.Cause(ctx)
			},
		)
		blockerDone <- err
	}()
	<-blockerEntered

	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	if !commandQueuedWithin(controller, secondKey, time.Second) {
		t.Fatal("second task promotion did not reserve its lane")
	}
	if _, err := controller.Stop(context.Background(), "second", "kotlin", origin); err != nil {
		t.Fatal(err)
	}
	if err := <-blockerDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted blocker error = %v", err)
	}

	waitForStoredPhase(t, store, "third", "typescript", PhaseReady)
	second := waitForStoredPhase(t, store, "second", "kotlin", PhaseOff)
	if second.Policy != PolicyDisabled {
		t.Fatalf("second policy = %q, want disabled", second.Policy)
	}
	if host.startCalls != 2 || controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("starts=%d active=%d queued=%d, want initial plus one next promotion",
			host.startCalls, controller.capacity.Active(), controller.capacity.Queued())
	}
}

func TestCloseCancelsAndJoinsQueuedCapacityPromotion(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	if queued, err := controller.Start(context.Background(), "queued", "kotlin", origin); err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	select {
	case <-host.startEntered:
	case <-time.After(time.Second):
		t.Fatal("queued promotion did not begin")
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	defer closeCancel()
	if err := controller.Close(closeCtx); err != nil {
		t.Fatalf("Close did not cancel and join queued promotion: %v", err)
	}
}

func TestCapacityPromotionRechecksTaskAdmissionBeforeLaunch(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	tasks.admissionErr = errors.New("task environment teardown in progress")

	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}

	notStarted := waitForStoredPhase(t, store, "queued", "kotlin", PhaseWaitingForTask)
	if notStarted.Phase != PhaseWaitingForTask {
		t.Fatalf("blocked promotion phase = %q, want %q", notStarted.Phase, PhaseWaitingForTask)
	}
	if host.startCalls != 1 {
		t.Fatalf("task-host start calls = %d, want only the initial server", host.startCalls)
	}
	if controller.capacity.Active() != 0 {
		t.Fatalf("active capacity after blocked promotion = %d, want 0", controller.capacity.Active())
	}
}

func waitForStoredPhase(
	t *testing.T,
	store Store,
	taskID, language string,
	phase Phase,
) *TaskLanguageState {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, _, err := store.GetTaskLSPLanguage(context.Background(), taskID, language)
		if err == nil && state.Phase == phase {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	state, _, err := store.GetTaskLSPLanguage(context.Background(), taskID, language)
	t.Fatalf("stored phase = %q, error = %v; want %q", state.Phase, err, phase)
	return nil
}

func TestCapacityPromotionSchedulesRecoveryWhenWaitingStatePersistenceFails(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	scheduler := newFakeScheduler()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	controller.scheduler = scheduler
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	if timer := scheduler.next(t); timer.delay != readyRecoveryReset {
		t.Fatalf("initial ready-reset delay = %s, want %s", timer.delay, readyRecoveryReset)
	}
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	store.mu.Lock()
	store.compareErrPhase = PhaseWaitingForTask
	store.compareErr = errors.New("persistence unavailable")
	store.mu.Unlock()
	tasks.admissionErr = errors.New("task environment teardown in progress")

	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}

	if timer := scheduler.next(t); timer.delay != time.Second {
		t.Fatalf("promotion recovery delay = %s, want 1s", timer.delay)
	}
	if host.startCalls != 1 {
		t.Fatalf("task-host start calls = %d, want only the initial server", host.startCalls)
	}
	if controller.capacity.Active() != 0 {
		t.Fatalf("active capacity after blocked promotion = %d, want 0", controller.capacity.Active())
	}
}

func TestExplicitStartResetsExhaustedRecoveryBeforeFailedPromotion(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"first":  readyEnvironment("first", "local_docker"),
		"queued": readyEnvironment("queued", "local_docker"),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	scheduler := newFakeScheduler()
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	controller.scheduler = scheduler
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	initialReady := scheduler.next(t)
	if initialReady.delay != readyRecoveryReset {
		t.Fatalf("initial ready-reset delay = %s, want %s", initialReady.delay, readyRecoveryReset)
	}
	queuedKey := TaskLanguageKey{TaskID: "queued", Language: "kotlin"}
	controller.lifecycleMu.Lock()
	controller.recoveries[queuedKey] = &recoveryState{attempts: len(recoveryBackoffs)}
	controller.lifecycleMu.Unlock()
	queued, err := controller.Start(context.Background(), "queued", "kotlin", origin)
	if err != nil || queued.Phase != PhaseQueued {
		t.Fatalf("queued=%#v error=%v", queued, err)
	}
	store.mu.Lock()
	store.compareErrPhase = PhaseWaitingForTask
	store.compareErr = errors.New("persistence unavailable")
	store.mu.Unlock()
	tasks.admissionErr = errors.New("task environment teardown in progress")

	if _, err := controller.Stop(context.Background(), "first", "go", origin); err != nil {
		t.Fatal(err)
	}
	if timer := scheduler.next(t); timer.delay != time.Second {
		t.Fatalf("recovery delay after new explicit start = %s, want 1s", timer.delay)
	}
}

func TestStopIgnoresSnapshotAlreadyReadByCancelledWatch(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.watchSnapshotEntered = make(chan struct{})
	host.watchSnapshotRelease = make(chan struct{})
	host.watchSnapshotDone = make(chan struct{})
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	controller.capacity = NewCapacity(1)
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	controller.lifecycleCtx = lifecycleCtx
	controller.lifecycleCancel = cancel
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "task-1", "go", origin); err != nil {
		t.Fatal(err)
	}
	<-host.watchSnapshotEntered
	if _, err := controller.Stop(context.Background(), "task-1", "go", origin); err != nil {
		t.Fatal(err)
	}
	close(host.watchSnapshotRelease)
	<-host.watchSnapshotDone

	state, _, err := store.GetTaskLSPLanguage(context.Background(), "task-1", "go")
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != PhaseOff {
		t.Fatalf("phase after cancelled watch delivered stale snapshot = %q, want %q", state.Phase, PhaseOff)
	}
	if active := controller.capacity.Active(); active != 0 {
		t.Fatalf("capacity after cancelled watch delivered stale snapshot = %d, want 0", active)
	}
}

func TestProvenStartFailureReleasesCapacity(t *testing.T) {
	tasks := &fakeControllerTasks{environments: map[string]*models.TaskEnvironment{
		"failed": readyEnvironment("failed", executorTypeLocalPC),
		"next":   readyEnvironment("next", executorTypeLocalPC),
	}}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startErr = errors.New("process did not start")
	host.startErrorSnapshot = &RuntimeSnapshot{Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed}
	controller := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	if _, err := controller.Start(context.Background(), "failed", "go", origin); err != nil {
		t.Fatalf("failed start: %v", err)
	}
	if active := controller.capacity.Active(); active != 0 {
		t.Fatalf("capacity active after proven process-start failure = %d, want 0", active)
	}
	host.mu.Lock()
	host.startErr = nil
	host.startErrorSnapshot = nil
	host.mu.Unlock()
	next, err := controller.Start(context.Background(), "next", "kotlin", origin)
	if err != nil {
		t.Fatalf("next start: %v", err)
	}
	if next.Phase != PhaseReady {
		t.Fatalf("next phase = %q, want ready", next.Phase)
	}
}

func TestSuccessfulStopReleasesCapacityWhenPersistenceFails(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	controller.capacity = NewCapacity(1)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	if _, err := controller.Start(context.Background(), "task-1", "go", origin); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	store.compareErrAt = store.compareCalls + 2 // mark stopping succeeds; runtime persistence fails.
	store.compareErr = errors.New("persistence unavailable")
	store.mu.Unlock()
	if _, err := controller.Stop(context.Background(), "task-1", "go", origin); err == nil {
		t.Fatal("stop unexpectedly succeeded")
	}
	if active := controller.capacity.Active(); active != 0 {
		t.Fatalf("capacity active after successful task-host stop = %d, want 0", active)
	}
}

func TestConcurrentDuplicateStartCoalescesOneGeneration(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	controller := newTestController(&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}

	const callers = 20
	results := make(chan *LanguageSnapshot, callers)
	errorsSeen := make(chan error, callers)
	go func() {
		result, err := controller.Start(context.Background(), "task-1", "go", origin)
		results <- result
		errorsSeen <- err
	}()
	<-host.startEntered
	for range callers - 1 {
		go func() {
			result, err := controller.Start(context.Background(), "task-1", "go", origin)
			results <- result
			errorsSeen <- err
		}()
	}
	close(host.startRelease)
	for range callers {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
		if result := <-results; result == nil || result.Generation != 1 {
			t.Fatalf("coalesced result = %#v", result)
		}
	}
	if host.startCalls != 1 || store.allocations != 1 {
		t.Fatalf("start calls=%d allocations=%d", host.startCalls, store.allocations)
	}
}

func TestTaskLifecycleBarrierRejectsStartBeforeCapacityOrRuntimeAcquisition(t *testing.T) {
	store := newMemoryLSPStore()
	runtimes := &fakeLSPRuntimes{}
	controller := newTestController(
		&fakeControllerTasks{admissionErr: errors.New("environment reset active")},
		store,
		&fakeLSPSettings{},
		runtimes,
	)

	_, err := controller.Start(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("Start error = %v, want task-not-ready", err)
	}
	if store.allocations != 0 || runtimes.ensureCalls != 0 || controller.capacity.Active() != 0 {
		t.Fatalf("allocations=%d ensure=%d active=%d",
			store.allocations, runtimes.ensureCalls, controller.capacity.Active())
	}
}
