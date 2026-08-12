package lsp

import (
	"context"
	"errors"
	"testing"

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

	promoted, _, err := store.GetTaskLSPLanguage(context.Background(), "queued", "kotlin")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Phase != PhaseReady || promoted.Generation != 1 ||
		controller.capacity.Active() != 1 || controller.capacity.Queued() != 0 {
		t.Fatalf("promoted=%#v active=%d queued=%d", promoted, controller.capacity.Active(), controller.capacity.Queued())
	}
	if host.startCalls != 2 || store.allocations != 2 {
		t.Fatalf("start calls=%d allocations=%d, want accepted generation reused", host.startCalls, store.allocations)
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
