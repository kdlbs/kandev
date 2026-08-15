package lsp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExplicitStopInterruptsBlockedTaskHostStart(t *testing.T) {
	store := newMemoryLSPStore()
	host := newFakeLSPHost()
	host.startEntered = make(chan struct{})
	host.startRelease = make(chan struct{})
	controller := newTestController(
		&fakeControllerTasks{}, store, &fakeLSPSettings{}, &fakeLSPRuntimes{host: host},
	)
	origin := Origin{Initiator: InitiatorUser, Reason: "user_control"}
	startDone := make(chan error, 1)
	go func() {
		_, err := controller.Start(context.Background(), "task-1", "kotlin", origin)
		startDone <- err
	}()
	<-host.startEntered

	type stopResult struct {
		snapshot *LanguageSnapshot
		err      error
	}
	stopDone := make(chan stopResult, 1)
	go func() {
		snapshot, err := controller.Stop(context.Background(), "task-1", "kotlin", origin)
		stopDone <- stopResult{snapshot: snapshot, err: err}
	}()
	select {
	case result := <-stopDone:
		if result.err != nil || result.snapshot == nil || result.snapshot.Phase != PhaseOff ||
			result.snapshot.Policy != PolicyDisabled {
			t.Fatalf("interrupted Stop = %#v, %v", result.snapshot, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("explicit Stop remained blocked behind task-host Start")
	}
	if err := <-startDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted Start error = %v", err)
	}
	if host.stopCalls != 1 || controller.capacity.Active() != 0 {
		t.Fatalf("stop calls=%d active=%d", host.stopCalls, controller.capacity.Active())
	}
}

func TestDisabledPolicyInterruptsBlockedKeepWarmPolicy(t *testing.T) {
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
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("disabled policy: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disabled policy remained blocked behind keep-warm start")
	}
	if err := <-firstDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted keep-warm policy: %v", err)
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
