package startup

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.2
func TestHousekeepingActivationWaitsForReadiness(t *testing.T) {
	reporter := New(nil)
	ctx := WithReporter(t.Context(), reporter)
	var started []string
	AfterReady(ctx, func() { started = append(started, "poller") })
	AfterReady(ctx, func() { started = append(started, "plugin") })
	if err := Activate(ctx); err == nil || len(started) != 0 {
		t.Fatalf("activation before readiness: %v, %v", err, started)
	}
	reporter.Set(Ready)
	if len(started) != 0 {
		t.Fatal("phase transition activated workers before router publication")
	}
	if err := Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Activate(ctx); err != nil {
		t.Fatal(err)
	}
	AfterReady(ctx, func() { started = append(started, "late") })
	if !reflect.DeepEqual(started, []string{"poller", "plugin", "late"}) {
		t.Fatalf("activation order or duplication: %v", started)
	}
}

func TestHousekeepingActivationFailureStopsLaterStarts(t *testing.T) {
	r := New(nil)
	ctx := WithReporter(t.Context(), r)
	want := errors.New("worker start failed")
	if err := AfterReadyError(ctx, func() error { return want }); err != nil {
		t.Fatal(err)
	}
	started := false
	AfterReady(ctx, func() { started = true })
	r.Set(Ready)
	if err := Activate(ctx); !errors.Is(err, want) || started {
		t.Fatalf("activation swallowed failure or admitted later work: %v %t", err, started)
	}
	if err := AfterReadyError(ctx, func() error { started = true; return nil }); !errors.Is(err, want) || started {
		t.Fatalf("failed activation admitted a late worker: %v %t", err, started)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3
func TestHousekeepingCancellationPreventsActivation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reporter := New(nil)
	ctx = WithReporter(ctx, reporter)
	started := false
	AfterReady(ctx, func() { started = true })
	cancel()
	reporter.Set(Ready)
	if err := Activate(ctx); err == nil || started {
		t.Fatalf("cancelled preparation activated a worker: %v, %t", err, started)
	}
}

func TestHousekeepingActivationWithoutBootstrapRunsImmediately(t *testing.T) {
	started := false
	AfterReady(t.Context(), func() { started = true })
	if !started {
		t.Fatal("non-bootstrap composition never started its worker")
	}
}

func TestHousekeepingActivationCanBeJoinedBeforeRestoreCleanup(t *testing.T) {
	r := New(nil)
	ctx := WithReporter(t.Context(), r)
	entered, release := make(chan struct{}), make(chan struct{})
	AfterReady(ctx, func() { close(entered); <-release })
	r.Set(Ready)
	activated := make(chan error, 1)
	go func() { activated <- Activate(ctx) }()
	<-entered
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := WaitForActivation(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("activation join ignored cancellation: %v", err)
	}
	close(release)
	if err := <-activated; err != nil {
		t.Fatal(err)
	}
	if err := WaitForActivation(ctx); err != nil {
		t.Fatal(err)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3
func TestHousekeepingLateRegistrationWaitsForActivationOutcome(t *testing.T) {
	r := New(nil)
	ctx := WithReporter(t.Context(), r)
	entered, release := make(chan struct{}), make(chan struct{})
	want := errors.New("earlier start failed")
	if err := AfterReadyError(ctx, func() error { close(entered); <-release; return want }); err != nil {
		t.Fatal(err)
	}
	r.Set(Ready)
	done := make(chan error, 1)
	go func() { done <- Activate(ctx) }()
	<-entered
	late := false
	err := AfterReadyError(ctx, func() error { late = true; return nil })
	close(release)
	activationErr := <-done
	if err != nil || !errors.Is(activationErr, want) || late {
		t.Fatalf("late registration bypassed earlier failure: registration=%v activation=%v started=%t", err, activationErr, late)
	}
}

func TestHousekeepingActivationDrainsNestedRegistration(t *testing.T) {
	r := New(nil)
	ctx := WithReporter(t.Context(), r)
	var order []string
	AfterReady(ctx, func() {
		AfterReady(ctx, func() { order = append(order, "nested") })
		order = append(order, "first")
	})
	r.Set(Ready)
	if err := Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "nested"}) {
		t.Fatalf("nested registration bypassed ordered activation: %v", order)
	}
}
