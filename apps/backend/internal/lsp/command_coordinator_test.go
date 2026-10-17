package lsp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInterruptingCommandCancelsEarlierLaneWork(t *testing.T) {
	coordinator := &commandCoordinator{}
	key := TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}
	startEntered := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		_, err := coordinator.submit(
			context.Background(), key, ActionStart, "",
			func(ctx context.Context) (*LanguageSnapshot, error) {
				close(startEntered)
				<-ctx.Done()
				return nil, context.Cause(ctx)
			},
		)
		startDone <- err
	}()
	<-startEntered

	stopRan := make(chan struct{})
	stopDone := make(chan error, 1)
	go func() {
		_, err := coordinator.submitInterrupting(
			context.Background(), key, ActionStop, "",
			func(context.Context) (*LanguageSnapshot, error) {
				close(stopRan)
				return nil, nil
			},
		)
		stopDone <- err
	}()
	select {
	case <-stopRan:
	case <-time.After(time.Second):
		t.Fatal("interrupting Stop remained queued behind canceled Start")
	}
	if !errors.Is(<-startDone, context.Canceled) {
		t.Fatal("running Start did not receive cancellation")
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveCommandCannotBecomeCoalescingTarget(t *testing.T) {
	coordinator := &commandCoordinator{}
	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	exclusiveEntered := make(chan struct{})
	exclusiveRelease := make(chan struct{})
	exclusiveDone := make(chan error, 1)
	go func() {
		_, err := coordinator.submitExclusive(
			context.Background(), key, ActionReconcile,
			func(context.Context) (*LanguageSnapshot, error) {
				close(exclusiveEntered)
				<-exclusiveRelease
				return nil, nil
			},
		)
		exclusiveDone <- err
	}()
	<-exclusiveEntered

	regularRan := make(chan struct{})
	regularDone := make(chan *LanguageSnapshot, 1)
	go func() {
		snapshot, _ := coordinator.submit(
			context.Background(), key, ActionReconcile, "",
			func(context.Context) (*LanguageSnapshot, error) {
				close(regularRan)
				return &LanguageSnapshot{TaskLanguageState: TaskLanguageState{
					TaskID: "task-1", Language: "go",
				}}, nil
			},
		)
		regularDone <- snapshot
	}()

	close(exclusiveRelease)
	if err := <-exclusiveDone; err != nil {
		t.Fatal(err)
	}
	snapshot := <-regularDone
	select {
	case <-regularRan:
	default:
		t.Fatal("regular command coalesced into the exclusive batch")
	}
	if snapshot == nil || snapshot.TaskID != "task-1" {
		t.Fatalf("regular command snapshot = %#v", snapshot)
	}
}
