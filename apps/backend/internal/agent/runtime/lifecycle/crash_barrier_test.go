package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func newTestBarriers(enabled bool) (*crashBarrierSet, *atomic.Int32) {
	exits := &atomic.Int32{}
	set := newCrashBarrierSet(func() bool { return enabled }, func(int) { exits.Add(1) })
	return set, exits
}

func TestBarrierArmingIsRefusedOutsideE2EMockMode(t *testing.T) {
	set, _ := newTestBarriers(false)
	if err := set.arm(BarrierAfterSideEffect, CrashBarrierModeBlock); !errors.Is(err, ErrCrashBarrierDisabled) {
		t.Fatalf("arm() error = %v, want ErrCrashBarrierDisabled", err)
	}
	set.reach(context.Background(), BarrierAfterSideEffect, nil) // must return at once
}

func TestBarrierDefaultSetFollowsTheE2EMockEnvironment(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	if err := ArmCrashBarrier(BarrierAfterDrain, CrashBarrierModeBlock); !errors.Is(err, ErrCrashBarrierDisabled) {
		t.Fatalf("ArmCrashBarrier() error = %v, want ErrCrashBarrierDisabled", err)
	}
	t.Setenv("KANDEV_E2E_MOCK", "true")
	if err := ArmCrashBarrier(BarrierAfterDrain, CrashBarrierModeBlock); err != nil {
		t.Fatalf("ArmCrashBarrier() error = %v", err)
	}
	ReleaseCrashBarrier(BarrierAfterDrain)
}

func TestBarrierNamesAreAClosedProviderNeutralSet(t *testing.T) {
	set, _ := newTestBarriers(true)
	want := []string{
		"after_side_effect", "after_drain", "after_handshake", "before_publish",
		"after_destroy_confirm", "after_placement_persist", "before_placement_commit", "after_cancel_mark",
	}
	got := CrashBarrierNames()
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("names = %v, want %v", got, want)
		}
		if err := set.arm(name, CrashBarrierModeBlock); err != nil {
			t.Fatalf("arm(%q) = %v", name, err)
		}
	}
	if err := set.arm("vendor_specific", CrashBarrierModeBlock); !errors.Is(err, ErrCrashBarrierUnknown) {
		t.Fatalf("arm(unknown) error = %v, want ErrCrashBarrierUnknown", err)
	}
	if err := set.arm(BarrierAfterDrain+"x", "pause"); err == nil {
		t.Fatal("arm() accepted an unknown mode")
	}
}

func TestBarrierArmingAnArmedNameIsAConflict(t *testing.T) {
	set, _ := newTestBarriers(true)
	if err := set.arm(BarrierBeforePublish, CrashBarrierModeBlock); err != nil {
		t.Fatal(err)
	}
	if err := set.arm(BarrierBeforePublish, CrashBarrierModeTerminate); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("second arm() error = %v, want conflict", err)
	}
}

func TestBarrierBlockModeFiresOnceForTheFirstCallerUntilReleased(t *testing.T) {
	set, exits := newTestBarriers(true)
	if err := set.arm(BarrierAfterHandshake, CrashBarrierModeBlock); err != nil {
		t.Fatal(err)
	}
	var fired atomic.Int32
	onFire := func(string) { fired.Add(1) }
	var passed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			set.reach(context.Background(), BarrierAfterHandshake, onFire)
			passed.Add(1)
		}()
	}
	waitForBarrierCondition(t, func() bool { return passed.Load() == 3 && fired.Load() == 1 })
	set.release(BarrierAfterHandshake)
	wg.Wait()
	if passed.Load() != 4 || fired.Load() != 1 || exits.Load() != 0 {
		t.Fatalf("passed = %d fired = %d exits = %d, want 4, 1, 0", passed.Load(), fired.Load(), exits.Load())
	}
	set.reach(context.Background(), BarrierAfterHandshake, onFire) // released: inert
	if fired.Load() != 1 {
		t.Fatal("a released barrier fired again")
	}
}

func TestBarrierBlockedCallerReturnsWhenItsContextEnds(t *testing.T) {
	set, _ := newTestBarriers(true)
	if err := set.arm(BarrierAfterSideEffect, CrashBarrierModeBlock); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { set.reach(ctx, BarrierAfterSideEffect, nil); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled caller stayed blocked")
	}
}

func TestBarrierTerminateModeExitsTheProcessOnce(t *testing.T) {
	set, exits := newTestBarriers(true)
	if err := set.arm(BarrierAfterDestroyConfirm, CrashBarrierModeTerminate); err != nil {
		t.Fatal(err)
	}
	set.reach(context.Background(), BarrierAfterDestroyConfirm, nil)
	set.reach(context.Background(), BarrierAfterDestroyConfirm, nil)
	if exits.Load() != 1 {
		t.Fatalf("exits = %d, want exactly one", exits.Load())
	}
}

func TestBarrierReleasingAnUnarmedNameDoesNothingAndDisarmsAnUnfiredOne(t *testing.T) {
	set, exits := newTestBarriers(true)
	set.release(BarrierBeforePublish) // unarmed: no effect
	if err := set.arm(BarrierBeforePublish, CrashBarrierModeTerminate); err != nil {
		t.Fatal(err)
	}
	set.release(BarrierBeforePublish)
	set.reach(context.Background(), BarrierBeforePublish, nil)
	if exits.Load() != 0 {
		t.Fatal("a disarmed barrier fired")
	}
	if err := set.arm(BarrierBeforePublish, CrashBarrierModeBlock); err != nil {
		t.Fatalf("re-arm after release: %v", err)
	}
}

func TestBarrierUnarmedLookupAllocatesNothing(t *testing.T) {
	set, _ := newTestBarriers(true)
	allocations := testing.AllocsPerRun(100, func() { set.reach(context.Background(), BarrierAfterSideEffect, nil) })
	if allocations != 0 {
		t.Fatalf("allocations = %v, want 0", allocations)
	}
}

func waitForBarrierCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}
