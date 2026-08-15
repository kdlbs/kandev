package backendapp

import (
	"errors"
	"reflect"
	"testing"
)

func TestQuiesceForRestoreStopsAllWritersAfterFailures(t *testing.T) {
	firstErr := errors.New("scheduler failed")
	secondErr := errors.New("worker failed")
	var calls []string
	mark := func(name string) func() error {
		return func() error {
			calls = append(calls, name)
			if name == "scheduling" {
				return firstErr
			}
			if name == "worker" {
				return secondErr
			}
			return nil
		}
	}

	err := quiesceForRestore(
		func() { calls = append(calls, "cancel") },
		mark("scheduling"),
		mark("orchestrator"),
		mark("agents"),
		[]func() error{mark("worker")},
	)

	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("error = %v, want both stop errors", err)
	}
	wantCalls := []string{"cancel", "scheduling", "orchestrator", "agents", "worker"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}
