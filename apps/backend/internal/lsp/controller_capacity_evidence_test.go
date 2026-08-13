package lsp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRestartRetainsCapacityWhenTaskHostUnavailable(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorUser,
	})
	runtimes := &ensureFailureRuntimes{
		fakeLSPRuntimes: &fakeLSPRuntimes{},
		err:             errors.New("task host unavailable"),
	}
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{}, Store: store, Settings: &fakeLSPSettings{},
		Runtimes: runtimes, Capacity: NewCapacity(1),
		Clock: func() time.Time { return time.Unix(500, 0).UTC() },
	})
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.capacity.Adopt(key, 1)

	restarted, err := controller.Restart(context.Background(), key.TaskID, key.Language, Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Phase != PhaseError || restarted.Generation != 2 {
		t.Fatalf("restart failure = %#v", restarted)
	}
	if active := controller.capacity.Active(); active != 1 {
		t.Fatalf("capacity after pre-RPC restart failure = %d, want old server reservation", active)
	}
}

func TestProvenAbsenceSurvivesDisplayErrorAndBackendRestart(t *testing.T) {
	tasks := &fakeControllerTasks{}
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startErr = errors.New("process did not start")
	host.startErrorSnapshot = &RuntimeSnapshot{
		Phase: PhaseError, ErrorCode: errorCodeProcessStartFailed,
	}
	first := newTestController(tasks, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})
	first.capacity = NewCapacity(1)
	if _, err := first.Start(context.Background(), "failed", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	}); err != nil {
		t.Fatal(err)
	}

	restartedRuntimes := newReconcileRuntimes()
	restartedRuntimes.existingErrors["env-failed"] = errors.New("task host inspection unavailable")
	second := NewController(ControllerConfig{
		Tasks: tasks, Store: store, Settings: &fakeLSPSettings{},
		Runtimes: restartedRuntimes, Capacity: NewCapacity(1),
		Clock: func() time.Time { return time.Unix(501, 0).UTC() },
	})
	if _, ready, err := second.reconcileAllWithInventory(context.Background()); err == nil || !ready {
		t.Fatalf("restart reconciliation ready=%t err=%v, want inspect error with usable inventory", ready, err)
	}
	next, err := second.Start(context.Background(), "next", "kotlin", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Phase != PhaseReady {
		t.Fatalf("unrelated start after proven absence = %q, want ready", next.Phase)
	}
}

func TestStopWithoutTaskHostPersistsProcessAbsence(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 3,
		LastInitiator: InitiatorUser,
	})
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{},
	)
	controller.capacity.Adopt(TaskLanguageKey{TaskID: "task-1", Language: "go"}, 3)

	stopped, err := controller.Stop(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Phase != PhaseOff || stopped.ProcessAbsentGeneration != stopped.Generation {
		t.Fatalf("stopped state lacks durable process-absence evidence: %#v", stopped)
	}
}

type ensureFailureRuntimes struct {
	*fakeLSPRuntimes
	err error
}

func (r *ensureFailureRuntimes) EnsureTaskHost(context.Context, string, string) (TaskHost, error) {
	return nil, r.err
}
