package client

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestAvailabilitySnapshotsCarryMonotonicRevision(t *testing.T) {
	availability := NewAvailability(nil, newTestLogger())
	availability.MarkAvailable()
	available, ok := availability.Snapshot()
	if !ok {
		t.Fatal("available snapshot was not published")
	}
	availableRevision := available.Revision

	availability.MarkUnavailable()
	unavailable, ok := availability.Snapshot()
	if !ok {
		t.Fatal("unavailable snapshot was not published")
	}
	unavailableRevision := unavailable.Revision
	if unavailableRevision <= availableRevision {
		t.Fatalf("unavailable revision = %d, want greater than available revision %d", unavailableRevision, availableRevision)
	}
}

func TestAvailabilityPublishesSanitizedMonotonicSnapshots(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(eventBus.Close)
	published := make(chan AvailabilitySnapshot, 2)
	_, err := eventBus.Subscribe(events.AgentRuntimeAvailabilityChanged, func(_ context.Context, event *bus.Event) error {
		snapshot, ok := event.Data.(AvailabilitySnapshot)
		if !ok {
			t.Fatalf("availability event data = %T, want AvailabilitySnapshot", event.Data)
		}
		published <- snapshot
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe availability events: %v", err)
	}

	availability := NewAvailability(eventBus, newTestLogger())
	t.Cleanup(availability.Stop)
	if _, ok := availability.Snapshot(); ok {
		t.Fatal("startup availability must not be published before health and auth")
	}

	availability.MarkAvailable()
	snapshot, ok := availability.Snapshot()
	if !ok || snapshot.Status != AvailabilityStatusAvailable {
		t.Fatalf("available snapshot = %+v, published=%v", snapshot, ok)
	}
	if snapshot.Reason != "" || snapshot.OccurredAt != nil {
		t.Fatalf("available snapshot exposes failure details: %+v", snapshot)
	}

	availability.MarkUnavailable()
	unavailable, ok := availability.Snapshot()
	if !ok || unavailable.Status != AvailabilityStatusUnavailable {
		t.Fatalf("unavailable snapshot = %+v, published=%v", unavailable, ok)
	}
	if unavailable.Reason != AvailabilityReasonAgentctlExited || unavailable.OccurredAt == nil {
		t.Fatalf("unavailable snapshot = %+v", unavailable)
	}
	occurredAt := *unavailable.OccurredAt

	availability.MarkUnavailable()
	availability.MarkAvailable()
	final, ok := availability.Snapshot()
	if !ok || final.Status != AvailabilityStatusUnavailable || final.Reason != AvailabilityReasonAgentctlExited {
		t.Fatalf("availability was not monotonic: %+v, published=%v", final, ok)
	}
	if final.OccurredAt == nil || !final.OccurredAt.Equal(occurredAt) {
		t.Fatalf("unavailable occurrence changed: got %v, want %v", final.OccurredAt, occurredAt)
	}
	firstPublished := <-published
	secondPublished := <-published
	if firstPublished.Status != AvailabilityStatusAvailable || secondPublished.Status != AvailabilityStatusUnavailable ||
		firstPublished.Revision >= secondPublished.Revision {
		t.Fatalf("published transitions = %+v then %+v, want increasing available then unavailable", firstPublished, secondPublished)
	}
}

func TestAvailabilitySnapshotReadsAreSafeDuringTransition(t *testing.T) {
	availability := NewAvailability(nil, newTestLogger())
	availability.MarkAvailable()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for count := 0; count < 1000; count++ {
				_, _ = availability.Snapshot()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		availability.MarkUnavailable()
	}()
	close(start)
	wg.Wait()

	snapshot, ok := availability.Snapshot()
	if !ok || snapshot.Status != AvailabilityStatusUnavailable {
		t.Fatalf("final snapshot = %+v, published=%v", snapshot, ok)
	}
}

func TestAvailabilityPublishesTransitionsInCommitOrder(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(eventBus.Close)
	published := make(chan AvailabilitySnapshot, 2)
	firstPublishStarted := make(chan struct{})
	releaseFirstPublish := make(chan struct{})
	var publishCalls atomic.Int32
	_, err := eventBus.Subscribe(events.AgentRuntimeAvailabilityChanged, func(_ context.Context, event *bus.Event) error {
		if event.Data == nil {
			return nil
		}
		if publishCalls.Add(1) == 1 {
			close(firstPublishStarted)
			<-releaseFirstPublish
		}
		snapshot, ok := event.Data.(AvailabilitySnapshot)
		if !ok {
			t.Fatalf("availability event data = %T, want AvailabilitySnapshot", event.Data)
		}
		published <- snapshot
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe availability events: %v", err)
	}

	availability := NewAvailability(eventBus, newTestLogger())
	t.Cleanup(availability.Stop)
	availableDone := make(chan struct{})
	go func() {
		availability.MarkAvailable()
		close(availableDone)
	}()
	<-firstPublishStarted

	unavailableDone := make(chan struct{})
	go func() {
		availability.MarkUnavailable()
		close(unavailableDone)
	}()

	<-unavailableDone
	close(releaseFirstPublish)
	select {
	case <-availableDone:
	case <-time.After(time.Second):
		t.Fatal("available transition did not complete")
	}
	select {
	case <-unavailableDone:
	case <-time.After(time.Second):
		t.Fatal("unavailable transition did not complete")
	}

	firstSnapshot := <-published
	secondSnapshot := <-published
	if firstSnapshot.Status != AvailabilityStatusAvailable || secondSnapshot.Status != AvailabilityStatusUnavailable ||
		firstSnapshot.Revision >= secondSnapshot.Revision {
		t.Fatalf("published availability transitions = %+v, then %+v, want increasing available then unavailable", firstSnapshot, secondSnapshot)
	}
}
