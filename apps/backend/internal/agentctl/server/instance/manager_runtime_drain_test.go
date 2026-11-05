package instance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestRuntimeDrainAdmissionBoundsWaitAndRetryDrainsRetainedCreation(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	gate := holdStartup(t, mgr)
	createDone := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(context.Background(), &CreateRequest{ID: "drain-race", WorkspacePath: t.TempDir()})
		createDone <- err
	}()
	if !gate.waitReached(1) {
		t.Fatal("creation did not reach startup gate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := mgr.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline exceeded while creation remains admitted", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Shutdown took %v after a 25ms deadline", elapsed)
	}
	if _, ok := mgr.GetInstance("drain-race"); ok {
		t.Fatal("instance published before admitted creation was released")
	}
	mgr.mu.RLock()
	bundle := mgr.provisional["drain-race"]
	var procMgr *process.Manager
	if bundle != nil {
		bundle.cleanupMu.Lock()
		procMgr, _ = bundle.procMgr.(*process.Manager)
		bundle.cleanupMu.Unlock()
	}
	mgr.mu.RUnlock()
	if procMgr == nil {
		t.Fatal("provisional process manager was unavailable to verify the seal")
	}
	if _, err := procMgr.StartProcess(context.Background(), process.StartProcessRequest{
		SessionID: "drain-race", Command: "/path/that/must/not/start",
	}); !errors.Is(err, process.ErrManagerStopping) {
		t.Fatalf("provisional process start after timeout = %v, want ErrManagerStopping", err)
	}

	gate.open()
	if err, ok := receiveWithin(createDone); !ok {
		t.Fatal("admitted create did not finish after release")
	} else if err != nil && !errors.Is(err, ErrManagerShuttingDown) {
		t.Fatalf("admitted create: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry Shutdown after retained creation completed: %v", err)
	}
	if _, ok := mgr.GetInstance("drain-race"); ok {
		t.Fatal("retry returned before the instance created during the failed drain was stopped")
	}
}

func TestRuntimeDrainTimeoutWhileStoppingReaperStillSealsCreation(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	created, err := mgr.CreateInstance(context.Background(), &CreateRequest{ID: "existing", WorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateInstance before shutdown: %v", err)
	}
	inst, ok := mgr.GetInstance(created.ID)
	if !ok {
		t.Fatal("created instance missing")
	}
	procMgr, ok := inst.manager.(*process.Manager)
	if !ok {
		t.Fatalf("instance manager type = %T, want *process.Manager", inst.manager)
	}
	reaperRelease := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(reaperRelease) }) }
	defer release()
	mgr.reaperWG.Add(1)
	go func() {
		defer mgr.reaperWG.Done()
		<-reaperRelease
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := mgr.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline while waiting for reaper", err)
	}
	if _, err := mgr.CreateInstance(context.Background(), &CreateRequest{WorkspacePath: t.TempDir()}); !errors.Is(err, ErrManagerShuttingDown) {
		t.Fatalf("CreateInstance after timed-out drain = %v, want ErrManagerShuttingDown", err)
	}
	if _, err := procMgr.StartProcess(context.Background(), process.StartProcessRequest{
		SessionID: "existing", Command: "/path/that/must/not/start",
	}); !errors.Is(err, process.ErrManagerStopping) {
		t.Fatalf("existing process start after timeout = %v, want ErrManagerStopping", err)
	}
	release()
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry Shutdown after reaper stopped: %v", err)
	}
}

func TestWaitGroupContextReusesCompletionWaiterAcrossRetries(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	var completion waitGroupCompletion
	var firstDone chan struct{}

	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := waitGroupContext(ctx, &wg, &completion)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("attempt %d error = %v, want deadline exceeded", attempt+1, err)
		}
		if completion.done == nil {
			t.Fatal("waiter did not retain its completion channel")
		}
		if attempt > 0 && completion.done != firstDone {
			t.Fatal("retry created a new waiter instead of reusing the original completion channel")
		}
		if attempt == 0 {
			firstDone = completion.done
		}
	}

	wg.Done()
	if err := waitGroupContext(context.Background(), &wg, &completion); err != nil {
		t.Fatalf("wait after owner completion: %v", err)
	}
}
