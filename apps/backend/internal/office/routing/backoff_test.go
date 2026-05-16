package routing

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/models"
)

func TestSchedule_ResetHintShortCircuits(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	hint := now.Add(7 * time.Minute)
	e := &routingerr.Error{
		Code:          routingerr.CodeRateLimited,
		AutoRetryable: true,
		ResetHint:     &hint,
	}
	current := models.ProviderHealth{BackoffStep: 3}
	retryAt, step := Schedule(current, e, now)
	if !retryAt.Equal(hint) {
		t.Fatalf("retryAt = %v, want %v", retryAt, hint)
	}
	if step != 3 {
		t.Fatalf("step = %d, want 3 (no escalation on reset hint)", step)
	}
}

func TestSchedule_ExponentialProgression(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	e := &routingerr.Error{
		Code:          routingerr.CodeQuotaLimited,
		AutoRetryable: true,
	}
	wantBase := []time.Duration{
		2 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
		60 * time.Minute,
		60 * time.Minute, // cap at the last entry
	}
	for i, want := range wantBase {
		current := models.ProviderHealth{BackoffStep: i}
		retryAt, step := Schedule(current, e, now)
		delta := retryAt.Sub(now)
		quarter := want / 4
		if delta < want-quarter-time.Second || delta > want+quarter+time.Second {
			t.Errorf("step=%d delta=%v want %v ±25%%", i, delta, want)
		}
		if step != i+1 {
			t.Errorf("step=%d next=%d want %d", i, step, i+1)
		}
	}
}

func TestSchedule_UserActionReturnsZero(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	e := &routingerr.Error{
		Code:       routingerr.CodeAuthRequired,
		UserAction: true,
	}
	current := models.ProviderHealth{BackoffStep: 2}
	retryAt, step := Schedule(current, e, now)
	if !retryAt.IsZero() {
		t.Fatalf("retryAt = %v, want zero", retryAt)
	}
	if step != 2 {
		t.Fatalf("step = %d, want 2 (no escalation on user action)", step)
	}
}

func TestSchedule_JitterBounded(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	e := &routingerr.Error{Code: routingerr.CodeRateLimited, AutoRetryable: true}
	current := models.ProviderHealth{BackoffStep: 0}
	for i := 0; i < 100; i++ {
		retryAt, _ := Schedule(current, e, now)
		delta := retryAt.Sub(now)
		lo := (2 * time.Minute) - (30 * time.Second) - time.Second
		hi := (2 * time.Minute) + (30 * time.Second) + time.Second
		if delta < lo || delta > hi {
			t.Fatalf("delta %v out of bounds [%v, %v]", delta, lo, hi)
		}
	}
}

func TestSchedule_NilError(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	current := models.ProviderHealth{BackoffStep: 1}
	retryAt, step := Schedule(current, nil, now)
	if !retryAt.IsZero() {
		t.Fatalf("retryAt = %v, want zero", retryAt)
	}
	if step != 1 {
		t.Fatalf("step = %d, want 1", step)
	}
}
