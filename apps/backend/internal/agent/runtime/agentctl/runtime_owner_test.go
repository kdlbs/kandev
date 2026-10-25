package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestRuntimeBindingRejectsRetiredResults(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	first := configuredCandidate(t, owner, "secret-one", nil)
	if err := first.Commit(); err != nil {
		t.Fatalf("commit first binding: %v", err)
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire first binding: %v", err)
	}
	defer lease.Close()

	accepted := make(chan struct{})
	releaseResult := make(chan struct{})
	result := make(chan bool, 1)
	var attempted atomic.Int32
	var applied atomic.Int32
	go func() {
		attempted.Add(1)
		close(accepted)
		<-releaseResult // The remote mutation was accepted, but its result was delayed.
		if lease.CheckCurrent() == nil {
			applied.Add(1)
			result <- true
			return
		}
		result <- false
	}()
	<-accepted

	if !owner.MarkUnavailableEpoch(first.Epoch(), AvailabilityReasonAgentctlExited) {
		t.Fatal("current runtime epoch was not retired")
	}
	second := configuredCandidate(t, owner, "secret-two", nil)
	if err := second.Commit(); err != nil {
		t.Fatalf("commit successor binding: %v", err)
	}
	close(releaseResult)
	if appliedResult := <-result; appliedResult {
		t.Fatal("delayed result from a retired runtime was applied")
	}
	if attempted.Load() != 1 || applied.Load() != 0 {
		t.Fatalf("accepted operation attempts=%d applied=%d, want one attempt and no stale result", attempted.Load(), applied.Load())
	}
	if lease.Context().Err() == nil {
		t.Fatal("retiring the binding did not cancel its operation context")
	}
	if first.MarkUnexpectedExit() {
		t.Fatal("late exit callback for a retired epoch changed owner state")
	}
	current, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("successor binding was not available: %v", err)
	}
	defer current.Close()
	if current.Epoch() != second.Epoch() || !current.IsCurrent() {
		t.Fatalf("current lease = epoch %d, want active successor %d", current.Epoch(), second.Epoch())
	}
}

func TestRuntimeExitEpochFence(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-epoch-fence")
	defer owner.Stop()
	first := configuredCandidate(t, owner, "first-secret", nil)
	if err := first.Commit(); err != nil {
		t.Fatalf("Commit(first): %v", err)
	}
	if !first.MarkUnexpectedExit() {
		t.Fatal("first runtime exit was not accepted")
	}
	second := configuredCandidate(t, owner, "second-secret", nil)
	if err := second.Commit(); err != nil {
		t.Fatalf("Commit(second): %v", err)
	}
	if first.MarkUnexpectedExitWithReason(AvailabilityReasonOwnershipUnverified) {
		t.Fatal("stale exit from the first epoch was accepted")
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(successor): %v", err)
	}
	defer lease.Close()
	if lease.Epoch() != second.Epoch() {
		t.Fatalf("active epoch = %d, want successor %d", lease.Epoch(), second.Epoch())
	}
}

func TestRetiringRuntimeClosesOwnedBindingResources(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-retire-cleanup")
	defer owner.Stop()
	cleaned := make(chan struct{})
	candidate := configuredCandidate(t, owner, "retired-secret", func() error {
		close(cleaned)
		return nil
	})
	if err := candidate.Commit(); err != nil {
		t.Fatalf("commit binding: %v", err)
	}
	if !candidate.MarkUnexpectedExit() {
		t.Fatal("retiring active binding failed")
	}
	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Fatal("retired binding resources were not cleaned")
	}
}

func TestRuntimeBindingPublishAtomic(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(eventBus.Close)
	firstPublishStarted := make(chan struct{})
	releaseFirstPublish := make(chan struct{})
	var publishCount atomic.Int32
	published := make(chan AvailabilitySnapshot, 2)
	_, err := eventBus.Subscribe(events.AgentRuntimeAvailabilityChanged, func(_ context.Context, event *bus.Event) error {
		snapshot, ok := event.Data.(AvailabilitySnapshot)
		if !ok {
			t.Fatalf("runtime event data = %T, want AvailabilitySnapshot", event.Data)
		}
		if publishCount.Add(1) == 1 {
			close(firstPublishStarted)
			<-releaseFirstPublish
		}
		published <- snapshot
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe runtime events: %v", err)
	}

	owner := NewRuntimeOwner(eventBus, newTestLogger(), "boot-one")
	t.Cleanup(owner.Stop)
	first := configuredCandidate(t, owner, "secret-one", nil)
	firstPublished := make(chan error, 1)
	go func() { firstPublished <- first.Commit() }()
	<-firstPublishStarted

	exitDone := make(chan bool, 1)
	go func() {
		exitDone <- first.MarkUnexpectedExit()
	}()
	if changed := <-exitDone; !changed {
		t.Fatal("current process exit was not published")
	}
	close(releaseFirstPublish)
	if err := <-firstPublished; err != nil {
		t.Fatalf("publish available snapshot: %v", err)
	}
	firstSnapshot := <-published
	secondSnapshot := <-published
	if firstSnapshot.Status != AvailabilityStatusAvailable || secondSnapshot.Status != AvailabilityStatusUnavailable ||
		firstSnapshot.Revision >= secondSnapshot.Revision {
		t.Fatalf("published runtime snapshots = %+v then %+v, want increasing available then unavailable", firstSnapshot, secondSnapshot)
	}
}

func TestRuntimeSnapshotRevisionOrdering(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	first := configuredCandidate(t, owner, "private-token", nil)
	if err := first.Commit(); err != nil {
		t.Fatalf("commit first binding: %v", err)
	}
	available, ok := owner.Snapshot()
	if !ok || available.Status != AvailabilityStatusAvailable || available.BootID != "boot-one" ||
		available.RuntimeEpoch != first.Epoch() || available.Revision == 0 {
		t.Fatalf("initial snapshot = %+v, published=%v", available, ok)
	}
	lease, err := owner.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire initial binding: %v", err)
	}
	if lease.BootID() != "boot-one" {
		t.Fatalf("lease boot id = %q, want boot-one", lease.BootID())
	}
	lease.Close()

	if !owner.MarkUnavailableEpoch(first.Epoch(), AvailabilityReasonAgentctlExited) {
		t.Fatal("failed to publish runtime outage")
	}
	unavailable, _ := owner.Snapshot()
	second := configuredCandidate(t, owner, "private-token-two", nil)
	if err := second.Commit(); err != nil {
		t.Fatalf("commit successor binding: %v", err)
	}
	current, _ := owner.Snapshot()
	if available.Revision >= unavailable.Revision || unavailable.Revision >= current.Revision {
		t.Fatalf("snapshot revisions = available %d, unavailable %d, current %d", available.Revision, unavailable.Revision, current.Revision)
	}
	if current.RuntimeEpoch != second.Epoch() || current.BootID != "boot-one" {
		t.Fatalf("successor snapshot = %+v, want epoch %d in boot-one", current, second.Epoch())
	}
}

func TestRuntimeBindingAbortAndDuplicatePublication(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	var cleanupCount atomic.Int32
	aborted := configuredCandidate(t, owner, "unused-secret", func() error {
		cleanupCount.Add(1)
		return nil
	})
	if err := aborted.Abort(); err != nil {
		t.Fatalf("abort candidate: %v", err)
	}
	if err := aborted.Abort(); err != nil {
		t.Fatalf("repeat abort: %v", err)
	}
	if cleanupCount.Load() != 1 {
		t.Fatalf("aborted candidate cleanup count = %d, want 1", cleanupCount.Load())
	}
	if err := aborted.Commit(); !errors.Is(err, ErrRuntimeCandidateState) {
		t.Fatalf("commit aborted candidate error = %v, want ErrRuntimeCandidateState", err)
	}

	committed := configuredCandidate(t, owner, "active-secret", nil)
	if err := committed.Commit(); err != nil {
		t.Fatalf("commit candidate: %v", err)
	}
	before, _ := owner.Snapshot()
	if err := committed.Commit(); !errors.Is(err, ErrRuntimeCandidateState) {
		t.Fatalf("duplicate commit error = %v, want ErrRuntimeCandidateState", err)
	}
	after, _ := owner.Snapshot()
	if after.Revision != before.Revision || after.RuntimeEpoch != before.RuntimeEpoch {
		t.Fatalf("duplicate commit changed snapshot from %+v to %+v", before, after)
	}

	var rejectedCleanup atomic.Int32
	rejected := configuredCandidate(t, owner, "successor-secret", func() error {
		rejectedCleanup.Add(1)
		return nil
	})
	if err := rejected.Commit(); !errors.Is(err, ErrRuntimeBindingActive) {
		t.Fatalf("publish over a live binding error = %v, want ErrRuntimeBindingActive", err)
	}
	if rejectedCleanup.Load() != 1 {
		t.Fatalf("rejected candidate cleanup count = %d, want 1", rejectedCleanup.Load())
	}
	final, _ := owner.Snapshot()
	if final.RuntimeEpoch != committed.Epoch() || final.Status != AvailabilityStatusAvailable {
		t.Fatalf("rejected candidate displaced live binding: %+v", final)
	}
}

func TestRuntimeOwnerStopPreventsCandidatePublication(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	var cleanupCount atomic.Int32
	candidate := configuredCandidate(t, owner, "secret", func() error {
		cleanupCount.Add(1)
		return nil
	})
	owner.Stop()
	if err := candidate.Commit(); !errors.Is(err, ErrRuntimeOwnerStopped) {
		t.Fatalf("commit after stop error = %v, want ErrRuntimeOwnerStopped", err)
	}
	if _, ok := owner.Snapshot(); ok {
		t.Fatal("stopped unpublished owner exposed a runtime snapshot")
	}
	if cleanupCount.Load() != 1 {
		t.Fatalf("candidate cleanup count = %d, want 1", cleanupCount.Load())
	}
	if candidate.MarkUnexpectedExit() {
		t.Fatal("exit callback after shutdown was accepted")
	}
}

func TestRuntimeCandidateExitBeforePublicationBlocksCommit(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatalf("prepare runtime binding: %v", err)
	}
	if !candidate.MarkUnexpectedExit() {
		t.Fatal("prepared candidate exit was not recorded")
	}
	var cleanupCount atomic.Int32
	err = candidate.Configure("127.0.0.1", 39429, "secret", 1234, func() error {
		cleanupCount.Add(1)
		return nil
	})
	if !errors.Is(err, ErrRuntimeCandidateState) {
		t.Fatalf("configure exited candidate error = %v, want ErrRuntimeCandidateState", err)
	}
	if err := candidate.Commit(); !errors.Is(err, ErrRuntimeCandidateState) {
		t.Fatalf("commit exited candidate error = %v, want ErrRuntimeCandidateState", err)
	}
	if _, ok := owner.Snapshot(); ok {
		t.Fatal("exited candidate became publicly available")
	}
	if cleanupCount.Load() != 1 {
		t.Fatalf("exited candidate cleanup count = %d, want 1", cleanupCount.Load())
	}
}

func TestRuntimeBindingSnapshotOmitsCredentials(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	candidate := configuredCandidate(t, owner, "do-not-publish-this-token", nil)
	if err := candidate.Commit(); err != nil {
		t.Fatalf("commit binding: %v", err)
	}
	snapshot, _ := owner.Snapshot()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal availability snapshot: %v", err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), "do-not-publish-this-token") || strings.Contains(string(encoded), "127.0.0.1") {
		t.Fatalf("snapshot exposed private runtime binding data: %s", encoded)
	}
}

func TestRuntimeOwnerStopRacingPublicationClosesAdmission(t *testing.T) {
	owner := NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	candidate := configuredCandidate(t, owner, "secret", nil)
	start := make(chan struct{})
	commitDone := make(chan error, 1)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		commitDone <- candidate.Commit()
	}()
	go func() {
		defer wait.Done()
		<-start
		owner.Stop()
	}()
	close(start)
	wait.Wait()
	commitErr := <-commitDone
	if commitErr != nil && !errors.Is(commitErr, ErrRuntimeOwnerStopped) {
		t.Fatalf("concurrent commit error = %v", commitErr)
	}
	if _, err := owner.Acquire(context.Background()); !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("acquire after stop = %v, want ErrRuntimeUnavailable", err)
	}
	if candidate.MarkUnexpectedExit() {
		t.Fatal("shutdown allowed an exit callback to publish an outage")
	}
}

func configuredCandidate(t *testing.T, owner *RuntimeOwner, credential string, cleanup func() error) *RuntimeBindingCandidate {
	t.Helper()
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatalf("prepare runtime binding: %v", err)
	}
	if err := candidate.Configure("127.0.0.1", 39429, credential, 1234, cleanup); err != nil {
		t.Fatalf("configure runtime binding: %v", err)
	}
	return candidate
}
