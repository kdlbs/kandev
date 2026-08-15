package lsp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRestartCleanupFailureFromPreviousGenerationRemainsActionable(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorUser,
	})
	host := newFakeLSPHost()
	host.restartResponse = &RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseError,
		ErrorCode: "replacement_cleanup_failed", ErrorMessage: "old process tree still alive",
	}
	host.restartErr = errors.New("old process tree still alive")
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)

	restarted, err := controller.Restart(context.Background(), "task-1", "go", Origin{
		Initiator: InitiatorUser, Reason: "user_control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Phase != PhaseError || restarted.Generation != 2 ||
		restarted.ErrorCode != "task_host_control_failed" ||
		!strings.Contains(restarted.ErrorMessage, "old process tree still alive") {
		t.Fatalf("restart cleanup failure was suppressed: %#v", restarted)
	}
	if host.restartCalls != 1 || controller.capacity.Active() != 1 {
		t.Fatalf("restart calls=%d active=%d, want one unresolved runtime slot", host.restartCalls, controller.capacity.Active())
	}
}
