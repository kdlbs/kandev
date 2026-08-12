package lsp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestManagerCrashAfterRunnerReapReportsProcessExit(t *testing.T) {
	manager, processes := newManagerForTest(t, func(_ int) *fakeLSPServer {
		server := newFakeLSPServer()
		server.crashAfterReady = true
		return server
	})
	processes.mu.Lock()
	processes.stopErr = process.ErrProcessNotFound
	processes.mu.Unlock()

	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 4}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	snapshot := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseError)
	if snapshot.ErrorCode != "process_exited" || snapshot.Generation != 4 {
		t.Fatalf("already-reaped crash snapshot = %#v", snapshot)
	}
	slot, err := manager.slotFor("kotlin")
	if err != nil {
		t.Fatal(err)
	}
	slot.opMu.Lock()
	retained := slot.runtime != nil
	slot.opMu.Unlock()
	if retained {
		t.Fatal("already-reaped process retained runtime ownership slot")
	}
}

func TestManagerCrashCleanupFailureRetainsGenerationUntilReapRetry(t *testing.T) {
	manager, processes := newManagerForTest(t, func(start int) *fakeLSPServer {
		server := newFakeLSPServer()
		server.crashAfterReady = start == 1
		return server
	})
	processes.mu.Lock()
	processes.stopErr = errors.New("process tree still alive")
	processes.mu.Unlock()

	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 5}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	snapshot := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseError)
	if snapshot.ErrorCode != "process_cleanup_failed" || snapshot.Generation != 5 {
		t.Fatalf("cleanup failure snapshot = %#v", snapshot)
	}
	slot, err := manager.slotFor("kotlin")
	if err != nil {
		t.Fatal(err)
	}
	slot.opMu.Lock()
	retained := slot.runtime != nil && slot.runtime.generation == 5
	slot.opMu.Unlock()
	if !retained {
		t.Fatal("failed cleanup released the runtime ownership slot")
	}

	processes.mu.Lock()
	processes.stopErr = nil
	processes.mu.Unlock()
	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 6}); err != nil {
		t.Fatalf("retry replacement: %v", err)
	}
	snapshot = waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	started, stopped, overlap := processes.counts()
	if snapshot.Generation != 6 || started != 2 || stopped != 1 || overlap {
		t.Fatalf("snapshot=%#v started=%d stopped=%d overlap=%v", snapshot, started, stopped, overlap)
	}
}

func TestManagerCloseCleanupFailureRetainsRuntimeOwnershipEvidence(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 8}); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	processes.mu.Lock()
	processes.stopErr = errors.New("process tree still alive")
	processes.mu.Unlock()

	if err := manager.Close(context.Background()); err == nil {
		t.Fatal("manager close unexpectedly proved cleanup")
	}
	snapshot := manager.Snapshot("kotlin")
	if snapshot.ErrorCode != "process_cleanup_failed" || snapshot.Generation != 8 {
		t.Fatalf("failed close snapshot = %#v", snapshot)
	}
	slot := manager.slots[taskLanguageRuntimeKey("task-1", "kotlin")]
	slot.opMu.Lock()
	runtime := slot.runtime
	slot.opMu.Unlock()
	if runtime == nil || runtime.generation != 8 {
		t.Fatal("failed close discarded the sole runtime cleanup handle")
	}

	// The outer process manager remains the final cleanup owner. Reap the fake
	// child directly so this regression test leaves no background goroutine.
	processes.mu.Lock()
	processes.stopErr = nil
	processes.mu.Unlock()
	if err := processes.StopProcess(context.Background(), process.StopProcessRequest{
		ProcessID: runtime.process.ID,
	}); err != nil {
		t.Fatal(err)
	}
	manager.closeErr = nil
}
