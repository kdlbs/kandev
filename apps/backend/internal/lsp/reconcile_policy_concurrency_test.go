package lsp

import (
	"context"
	"sync"
	"testing"
	"time"
)

const concurrentPolicyEnableReason = "test_concurrent_policy_enable"

func TestReconcileTaskReloadsPolicyInsideLanguageLane(t *testing.T) {
	baseStore := newMemoryLSPStore()
	scannedAt := time.Unix(100, 0).UTC()
	for _, language := range registeredLanguages() {
		state := DefaultTaskLanguageState("task-1", language)
		state.DetectionState = DetectionComplete
		state.DetectionScannedAt = &scannedAt
		if language == "go" {
			state.Policy = PolicyDisabled
		}
		seedLSPState(t, baseStore, state)
	}
	store := &reconcileSnapshotStore{
		Store: baseStore, taskListBlockCall: 2,
		taskListBlocked: make(chan struct{}), taskListRelease: make(chan struct{}),
	}
	host := newFakeLSPHost()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{}, Store: store, Settings: &fakeLSPSettings{},
		Runtimes: &fakeLSPRuntimes{host: host}, Capacity: NewCapacity(8),
		Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})

	reconcileDone := make(chan error, 1)
	go func() {
		reconcileDone <- controller.ReconcileTask(context.Background(), "task-1")
	}()
	awaitReconcileSnapshot(t, store.taskListBlocked)

	snapshot, err := controller.SetPolicy(
		context.Background(), "task-1", "go", PolicyKeepWarm,
		Origin{Initiator: InitiatorUser, Reason: concurrentPolicyEnableReason},
	)
	if err != nil || snapshot == nil || snapshot.Phase != PhaseReady {
		t.Fatalf("enable while reconcile is queued = %#v, %v", snapshot, err)
	}
	close(store.taskListRelease)
	if err := <-reconcileDone; err != nil {
		t.Fatalf("reconcile task: %v", err)
	}

	state := storedLSPState(t, baseStore, "task-1", "go")
	if state.Policy != PolicyKeepWarm || state.Phase != PhaseReady || host.stopCalls != 0 {
		t.Fatalf("post-reconcile state=%#v stop calls=%d, want enabled server preserved",
			state, host.stopCalls)
	}
}

func TestApplySettingsReconcileReloadsPolicyInsideLanguageLane(t *testing.T) {
	baseStore := newMemoryLSPStore()
	seedLSPState(t, baseStore, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyDisabled,
		DetectionState: DetectionComplete, Phase: PhaseOff,
	})
	store := &reconcileSnapshotStore{
		Store: baseStore, allListBlockCall: 2,
		allListBlocked: make(chan struct{}), allListRelease: make(chan struct{}),
	}
	host := newFakeLSPHost()
	controller := NewController(ControllerConfig{
		Tasks: &fakeControllerTasks{}, Store: store, Settings: &fakeLSPSettings{},
		Runtimes: &fakeLSPRuntimes{host: host}, Capacity: NewCapacity(8),
		Clock: func() time.Time { return time.Unix(200, 0).UTC() },
	})

	applyDone := make(chan error, 1)
	go func() {
		applyDone <- controller.ApplySettings(context.Background())
	}()
	awaitReconcileSnapshot(t, store.allListBlocked)

	snapshot, err := controller.SetPolicy(
		context.Background(), "task-1", "go", PolicyKeepWarm,
		Origin{Initiator: InitiatorUser, Reason: concurrentPolicyEnableReason},
	)
	if err != nil || snapshot == nil || snapshot.Phase != PhaseReady {
		t.Fatalf("enable while settings reconcile is queued = %#v, %v", snapshot, err)
	}
	close(store.allListRelease)
	if err := <-applyDone; err != nil {
		t.Fatalf("apply settings: %v", err)
	}

	state := storedLSPState(t, baseStore, "task-1", "go")
	if state.Policy != PolicyKeepWarm || state.Phase != PhaseReady || host.stopCalls != 0 {
		t.Fatalf("post-settings state=%#v stop calls=%d, want enabled server preserved",
			state, host.stopCalls)
	}
}

type reconcileSnapshotStore struct {
	Store
	mu                sync.Mutex
	taskListCalls     int
	taskListBlockCall int
	taskListBlocked   chan struct{}
	taskListRelease   chan struct{}
	allListCalls      int
	allListBlockCall  int
	allListBlocked    chan struct{}
	allListRelease    chan struct{}
}

func (s *reconcileSnapshotStore) ListTaskLSPLanguages(
	ctx context.Context,
	taskID string,
) ([]TaskLanguageState, error) {
	states, err := s.Store.ListTaskLSPLanguages(ctx, taskID)
	s.mu.Lock()
	s.taskListCalls++
	shouldBlock := s.taskListCalls == s.taskListBlockCall
	s.mu.Unlock()
	if shouldBlock {
		close(s.taskListBlocked)
		<-s.taskListRelease
	}
	return states, err
}

func (s *reconcileSnapshotStore) ListAllTaskLSPLanguages(
	ctx context.Context,
) ([]TaskLanguageState, error) {
	states, err := s.Store.ListAllTaskLSPLanguages(ctx)
	s.mu.Lock()
	s.allListCalls++
	shouldBlock := s.allListCalls == s.allListBlockCall
	s.mu.Unlock()
	if shouldBlock {
		close(s.allListBlocked)
		<-s.allListRelease
	}
	return states, err
}

func awaitReconcileSnapshot(t *testing.T, blocked <-chan struct{}) {
	t.Helper()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not capture its stale state snapshot")
	}
}
