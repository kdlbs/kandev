package lsp

import (
	"context"
	"testing"
)

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
