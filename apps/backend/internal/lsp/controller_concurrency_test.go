package lsp

import (
	"context"
	"testing"
	"time"
)

func TestConcurrentDistinctPolicyWritesExecuteInOrder(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	firstDone := make(chan error, 1)
	go func() {
		_, err := controller.SetPolicy(context.Background(), "task-1", "go", PolicyKeepWarm, origin)
		firstDone <- err
	}()
	<-host.startEntered

	secondDone := make(chan error, 1)
	go func() {
		_, err := controller.SetPolicy(context.Background(), "task-1", "go", PolicyDisabled, origin)
		secondDone <- err
	}()
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	if !commandQueuedWithin(controller, key, time.Second) {
		close(host.startRelease)
		<-firstDone
		<-secondDone
		t.Fatal("distinct disabled policy was coalesced with running keep-warm policy")
	}
	close(host.startRelease)
	if err := <-firstDone; err != nil {
		t.Fatalf("first policy: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second policy: %v", err)
	}
	state := storedLSPState(t, store, "task-1", "go")
	if state.Policy != PolicyDisabled || state.Phase != PhaseOff {
		t.Fatalf("final policy state = %#v", state)
	}
}

func commandQueuedWithin(controller *Controller, key TaskLanguageKey, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		controller.commands.mu.Lock()
		lane := controller.commands.lanes[key]
		queued := lane != nil && len(lane.queued) == 1
		controller.commands.mu.Unlock()
		if queued {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
