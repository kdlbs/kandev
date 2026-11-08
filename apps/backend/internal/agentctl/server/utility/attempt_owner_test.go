package utility

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestAttemptCorruptOwnerMetadataIsPreservedAndCannotStart(t *testing.T) {
	retained, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := retained.Close(); err != nil {
			t.Error(err)
		}
	})
	original := []byte(`{"version":7,"attempts":{"old":{"status":{"attempt_id":"old"}}}}`)
	if err := retained.WriteInferenceOwner(context.Background(), original, false); err != nil {
		t.Fatal(err)
	}
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance"}, retained)
	snapshot := owner.snapshot()
	if snapshot.Known {
		t.Fatalf("corrupt metadata reported known: %+v", snapshot)
	}
	if _, err := owner.prepare(uuid.NewString(), ""); err == nil {
		t.Fatal("corrupt owner admitted a preparation")
	}
	actual, found, err := retained.ReadInferenceOwner()
	if err != nil || !found || string(actual) != string(original) {
		t.Fatalf("old evidence overwritten: %s, %t, %v", actual, found, err)
	}
}

func TestAttemptForeignOwnerAndConsumedCancellationCannotSettle(t *testing.T) {
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance", InferenceOwnerFresh: true}, nil)
	status, err := owner.prepare(uuid.NewString(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.consume(status.AttemptID); err != nil {
		t.Fatal(err)
	}
	owner.change(status.AttemptID, func(a *ownedAttempt) { a.PID = 123; a.Status.State = AttemptRunning })
	cancelled, err := owner.cancelPrepared(status.AttemptID, "")
	if err == nil || cancelled.Settled || cancelled.State != AttemptRunning {
		t.Fatalf("parent cancellation settled started child: %+v %v", cancelled, err)
	}
	foreign := status
	foreign.AttemptID = uuid.NewString()
	foreign.OwnerID = "previous-owner"
	owner.mu.Lock()
	owner.state.Attempts[foreign.AttemptID] = &ownedAttempt{Status: foreign}
	owner.mu.Unlock()
	if _, err := owner.consume(foreign.AttemptID); err == nil {
		t.Fatal("foreign owner token was consumed")
	}
	got, ok := owner.get(foreign.AttemptID)
	if !ok || got.Settled || got.OwnerID != "previous-owner" {
		t.Fatalf("foreign token was erased or settled: %+v", got)
	}
}

func TestAttemptFinalSnapshotSettlesOnlyUnconsumedPreparation(t *testing.T) {
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance", InferenceOwnerFresh: true}, nil)
	pending, err := owner.prepare(uuid.NewString(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	started, err := owner.prepare(uuid.NewString(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.consume(started.AttemptID); err != nil {
		t.Fatal(err)
	}
	owner.change(started.AttemptID, func(a *ownedAttempt) { a.PID = 123; a.Status.State = AttemptRunning })
	final := owner.finalSnapshot()
	if len(final.Attempts) != 2 {
		t.Fatalf("attempts lost: %+v", final)
	}
	got, _ := owner.get(pending.AttemptID)
	if !got.Known || !got.Settled || got.State != AttemptNotStarted {
		t.Fatalf("unsent preparation not compensated: %+v", got)
	}
	got, _ = owner.get(started.AttemptID)
	if got.Settled || got.State != AttemptRunning {
		t.Fatalf("parent teardown invented started-child termination: %+v", got)
	}
}

func TestAttemptConsumeCancelRaceHasOneWinner(t *testing.T) {
	owner := newAttemptOwner(&config.InstanceConfig{InstanceID: "instance", InferenceOwnerFresh: true}, nil)
	for range 64 {
		prepared, err := owner.prepare(uuid.NewString(), "", true)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		result := make(chan error, 2)
		go func() { <-start; _, err := owner.consume(prepared.AttemptID); result <- err }()
		go func() { <-start; _, err := owner.cancelPrepared(prepared.AttemptID, ""); result <- err }()
		close(start)
		first, second := <-result, <-result
		if (first == nil) == (second == nil) {
			t.Fatalf("consume/cancel winners = %v, %v", first, second)
		}
		got, ok := owner.get(prepared.AttemptID)
		if !ok {
			t.Fatal("race erased owner")
		}
		if got.State == AttemptNotStarted {
			if !got.Settled || !got.Known {
				t.Fatalf("cancelled token lacks proof: %+v", got)
			}
		} else if got.State != AttemptStarting || got.Settled {
			t.Fatalf("consume winner released: %+v", got)
		}
	}
}
