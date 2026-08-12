package lsp

import (
	"context"
	"errors"
	"sync"
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

func TestManagerPurgeTaskRemovesOnlyTargetTaskState(t *testing.T) {
	manager := NewManager(Config{
		OwnerID: "owner-task", WorkDir: "/workspace", WorkspaceURI: "file:///workspace",
	}, nil, nil, testLogger())
	targetKey := taskLanguageRuntimeKey("borrower-task", "kotlin")
	otherKey := taskLanguageRuntimeKey("other-task", "go")
	targetUpdates := make(chan Snapshot)
	otherUpdates := make(chan Snapshot)
	manager.mu.Lock()
	manager.slots[targetKey] = &languageSlot{lastGeneration: 3}
	manager.slots[otherKey] = &languageSlot{lastGeneration: 7}
	manager.snapshots[targetKey] = Snapshot{
		Language: "kotlin", Generation: 3, WorkspacePath: "/workspace/borrower",
	}
	manager.snapshots[otherKey] = Snapshot{
		Language: "go", Generation: 7, WorkspacePath: "/workspace/other",
	}
	manager.subscribers[targetKey] = map[uint64]chan Snapshot{1: targetUpdates}
	manager.subscribers[otherKey] = map[uint64]chan Snapshot{2: otherUpdates}
	manager.taskConfigs["borrower-task"] = Config{OwnerID: "borrower-task", WorkDir: "/workspace/borrower"}
	manager.taskConfigs["other-task"] = Config{OwnerID: "other-task", WorkDir: "/workspace/other"}
	manager.mu.Unlock()

	if err := manager.PurgeTask("borrower-task"); err != nil {
		t.Fatalf("purge borrower task: %v", err)
	}
	manager.mu.RLock()
	_, targetSlotExists := manager.slots[targetKey]
	_, targetSnapshotExists := manager.snapshots[targetKey]
	_, targetSubscribersExist := manager.subscribers[targetKey]
	_, targetConfigExists := manager.taskConfigs["borrower-task"]
	_, otherSlotExists := manager.slots[otherKey]
	_, otherSnapshotExists := manager.snapshots[otherKey]
	_, otherSubscribersExist := manager.subscribers[otherKey]
	_, otherConfigExists := manager.taskConfigs["other-task"]
	manager.mu.RUnlock()
	if targetSlotExists || targetSnapshotExists || targetSubscribersExist || targetConfigExists {
		t.Fatal("purged task retained task-host state")
	}
	if !otherSlotExists || !otherSnapshotExists || !otherSubscribersExist || !otherConfigExists {
		t.Fatal("purge removed another task's state from the shared host")
	}
	if _, open := <-targetUpdates; open {
		t.Fatal("purge left target task subscriber open")
	}
	select {
	case <-otherUpdates:
		t.Fatal("purge closed another task's subscriber")
	default:
	}
}

func TestManagerPurgeTaskRejectsLiveRuntimeOwnership(t *testing.T) {
	manager := NewManager(Config{OwnerID: "owner-task"}, nil, nil, testLogger())
	key := taskLanguageRuntimeKey("borrower-task", "kotlin")
	slot := &languageSlot{runtime: &runtime{taskID: "borrower-task", language: "kotlin", generation: 4}}
	manager.mu.Lock()
	manager.slots[key] = slot
	manager.snapshots[key] = Snapshot{Language: "kotlin", Generation: 4}
	manager.taskConfigs["borrower-task"] = Config{OwnerID: "borrower-task", WorkDir: "/workspace/borrower"}
	manager.mu.Unlock()

	if err := manager.PurgeTask("borrower-task"); !errors.Is(err, ErrTaskRuntimeActive) {
		t.Fatalf("purge live task error = %v, want %v", err, ErrTaskRuntimeActive)
	}
	manager.mu.RLock()
	_, slotExists := manager.slots[key]
	_, snapshotExists := manager.snapshots[key]
	_, configExists := manager.taskConfigs["borrower-task"]
	manager.mu.RUnlock()
	if !slotExists || !snapshotExists || !configExists {
		t.Fatal("rejected purge discarded live runtime evidence")
	}
	// Avoid leaving a synthetic live runtime for any later cleanup.
	slot.runtime = nil
}

func TestManagerPurgeTaskPreventsStartUsingSlotAcquiredBeforePurge(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	key := taskLanguageRuntimeKey("task-1", "kotlin")
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	manager.mu.Lock()
	manager.slots[key] = &languageSlot{beforeStartRegistration: func() {
		enteredOnce.Do(func() { close(entered) })
		<-release
	}}
	manager.mu.Unlock()

	startDone := make(chan error, 1)
	go func() {
		_, err := manager.Start(context.Background(), StartRequest{
			TaskID: "task-1", Language: "kotlin", Generation: 1,
		})
		startDone <- err
	}()
	<-entered
	if err := manager.PurgeTask("task-1"); err != nil {
		t.Fatalf("purge task: %v", err)
	}
	close(release)
	if err := <-startDone; !errors.Is(err, ErrTaskStatePurging) {
		t.Fatalf("start after purge error = %v, want %v", err, ErrTaskStatePurging)
	}
	started, _, _ := processes.counts()
	if started != 0 {
		t.Fatalf("language-server processes started after purge = %d, want 0", started)
	}
}
