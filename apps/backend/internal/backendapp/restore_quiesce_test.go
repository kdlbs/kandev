package backendapp

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
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

func TestRestoreCleanupRegistrationDrainsGatedWorker(t *testing.T) {
	var runtimeCleanups []func() error
	var restoreCleanups []func() error
	addRuntimeCleanup := func(cleanup func() error) { runtimeCleanups = append(runtimeCleanups, cleanup) }
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerStarted := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		close(workerStarted)
		<-workerCtx.Done()
		close(workerDone)
	}()
	select {
	case <-workerStarted:
	case <-time.After(time.Second):
		t.Fatal("gated worker did not start")
	}

	var stopCalls atomic.Int32
	restoreWorkerCleanupRegistrar(addRuntimeCleanup, &restoreCleanups)(func() error {
		stopCalls.Add(1)
		cancelWorker()
		<-workerDone
		return nil
	})
	if len(runtimeCleanups) != 1 || len(restoreCleanups) != 1 {
		t.Fatalf("registered cleanups = runtime %d, restore %d; want one of each", len(runtimeCleanups), len(restoreCleanups))
	}
	if err := quiesceForRestore(nil, nil, nil, nil, restoreCleanups); err != nil {
		t.Fatalf("restore quiesce: %v", err)
	}
	select {
	case <-workerDone:
	default:
		t.Fatal("restore quiesce returned before the gated worker stopped")
	}
	for _, cleanup := range runtimeCleanups {
		if err := cleanup(); err != nil {
			t.Fatalf("runtime cleanup: %v", err)
		}
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("worker stop calls = %d, want one across restore and shutdown", got)
	}
}

func TestRegisterImproveKandevRoutesUsesRestoreCleanup(t *testing.T) {
	var runtimeCleanups []func() error
	var restoreCleanups []func() error
	addRuntimeCleanup := func(cleanup func() error) { runtimeCleanups = append(runtimeCleanups, cleanup) }
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	t.Cleanup(cancelWorker)
	workerStarted := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		close(workerStarted)
		<-workerCtx.Done()
		close(workerDone)
	}()
	select {
	case <-workerStarted:
	case <-time.After(time.Second):
		t.Fatal("gated worker did not start")
	}
	p := routeParams{
		router:            gin.New(),
		services:          &Services{},
		repoCloner:        &repoclone.Cloner{},
		log:               logger.Default(),
		addCleanup:        addRuntimeCleanup,
		addRestoreCleanup: restoreWorkerCleanupRegistrar(addRuntimeCleanup, &restoreCleanups),
	}
	registerImproveKandevRoutes(p)
	p.addRestoreCleanup(func() error {
		cancelWorker()
		<-workerDone
		return nil
	})
	if len(runtimeCleanups) != 2 || len(restoreCleanups) != 2 {
		t.Fatalf("Improve Kandev cleanup registration = runtime %d, restore %d; want two of each", len(runtimeCleanups), len(restoreCleanups))
	}
	if err := quiesceForRestore(nil, nil, nil, nil, restoreCleanups); err != nil {
		t.Fatalf("restore quiesce: %v", err)
	}
	select {
	case <-workerDone:
	default:
		t.Fatal("restore quiesce returned before the gated worker stopped")
	}
	for _, cleanup := range runtimeCleanups {
		if err := cleanup(); err != nil {
			t.Fatalf("runtime cleanup: %v", err)
		}
	}
}
