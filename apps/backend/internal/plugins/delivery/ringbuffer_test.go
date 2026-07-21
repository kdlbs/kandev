package delivery

import (
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

func delivery(id string) Delivery {
	return Delivery{Event: &pluginsdk.Event{EventID: id}}
}

func TestRingBuffer_DrainReturnsInOrder(t *testing.T) {
	rb := newRingBuffer(100, 5*time.Minute, nil)

	rb.Add(delivery("1"))
	rb.Add(delivery("2"))
	rb.Add(delivery("3"))

	got := rb.Drain()
	if len(got) != 3 {
		t.Fatalf("Drain() returned %d items, want 3", len(got))
	}
	for i, id := range []string{"1", "2", "3"} {
		if got[i].Event.EventID != id {
			t.Errorf("Drain()[%d].Event.EventID = %q, want %q", i, got[i].Event.EventID, id)
		}
	}
}

func TestRingBuffer_DrainEmptiesBuffer(t *testing.T) {
	rb := newRingBuffer(100, 5*time.Minute, nil)
	rb.Add(delivery("1"))

	_ = rb.Drain()

	if got := rb.Drain(); len(got) != 0 {
		t.Errorf("second Drain() returned %d items, want 0", len(got))
	}
}

func TestRingBuffer_OverflowDropsOldest(t *testing.T) {
	rb := newRingBuffer(2, 5*time.Minute, nil)

	rb.Add(delivery("1"))
	rb.Add(delivery("2"))
	dropped := rb.Add(delivery("3"))

	if dropped != "1" {
		t.Errorf("Add() dropped = %q, want 1", dropped)
	}
	got := rb.Drain()
	if len(got) != 2 {
		t.Fatalf("Drain() returned %d items, want 2", len(got))
	}
	if got[0].Event.EventID != "2" || got[1].Event.EventID != "3" {
		t.Errorf("Drain() = %v, want [2 3]", got)
	}
}

func TestRingBuffer_TTLExpiresOldEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }
	rb := newRingBuffer(100, 5*time.Minute, nowFn)

	rb.Add(delivery("stale"))
	now = now.Add(6 * time.Minute)
	rb.Add(delivery("fresh"))

	got := rb.Drain()
	if len(got) != 1 {
		t.Fatalf("Drain() returned %d items, want 1 (stale entry should have expired)", len(got))
	}
	if got[0].Event.EventID != "fresh" {
		t.Errorf("Drain()[0].Event.EventID = %q, want fresh", got[0].Event.EventID)
	}
}

func TestRingBuffer_LenReflectsNonExpiredCount(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }
	rb := newRingBuffer(100, 5*time.Minute, nowFn)

	rb.Add(delivery("1"))
	rb.Add(delivery("2"))
	if got := rb.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	now = now.Add(6 * time.Minute)
	if got := rb.Len(); got != 0 {
		t.Errorf("Len() after TTL expiry = %d, want 0", got)
	}
}
