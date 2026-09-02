package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWatchLossPersistenceFailureStillSchedulesRecovery(t *testing.T) {
	store := newMemoryLSPStore()
	seedLSPState(t, store, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	store.compareErrAt = store.compareCalls + 1
	store.compareErr = errors.New("persistence unavailable")
	host := newFailFirstWatchHost()
	host.snapshots["go"] = RuntimeSnapshot{
		Language: "go", Generation: 1, Phase: PhaseReady,
	}
	runtimes := newReconcileRuntimes()
	runtimes.existing["env-task-1"] = host
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(store, runtimes, scheduler)
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.ensureWatch(key)
	<-host.firstFailed
	timer := scheduler.next(t)
	waitForWatchRemoval(t, controller, key)
	if timer.delay != time.Second {
		t.Fatalf("watch persistence recovery delay = %s, want 1s", timer.delay)
	}
	state := storedLSPState(t, store, key.TaskID, key.Language)
	if state.Phase != PhaseReady {
		t.Fatalf("failed persistence unexpectedly changed state: %#v", state)
	}
	timer.Fire()
	select {
	case <-host.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("recovery did not register a replacement watch")
	}
	state = storedLSPState(t, store, key.TaskID, key.Language)
	if state.Generation != 1 || state.Phase != PhaseReady || host.startCalls != 0 {
		t.Fatalf("rewatch changed generation or relaunched: state=%#v starts=%d", state, host.startCalls)
	}
}

func TestReadyResetReadFailureDoesNotSurviveQueuedCrash(t *testing.T) {
	baseStore := newMemoryLSPStore()
	seedLSPState(t, baseStore, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	store := &blockingFailLanguageReadStore{
		Store: baseStore, err: errors.New("persistence unavailable"),
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(baseStore, newReconcileRuntimes(), scheduler)
	controller.store = store
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.lifecycleMu.Lock()
	controller.recoveries[key] = &recoveryState{attempts: 2}
	controller.lifecycleMu.Unlock()
	controller.scheduleReadyReset(key, 1)
	readyTimer := scheduler.next(t)
	resetDone := make(chan struct{})
	go func() {
		readyTimer.Fire()
		close(resetDone)
	}()
	<-store.entered

	crashDone := make(chan error, 1)
	go func() {
		_, err := controller.commands.submitExclusive(
			context.Background(), key, ActionReconcile,
			func(workCtx context.Context) (*LanguageSnapshot, error) {
				return nil, controller.observeRuntimeSnapshot(workCtx, key, RuntimeSnapshot{
					Language: "go", Generation: 1, Phase: PhaseError,
					ErrorCode: errorCodeProcessExited,
				})
			},
		)
		crashDone <- err
	}()
	close(store.release)
	<-resetDone
	if err := <-crashDone; err != nil {
		t.Fatal(err)
	}

	staleRetry := scheduler.next(t)
	recovery := scheduler.next(t)
	if !staleRetry.Stopped() {
		t.Fatal("queued crash left the failed-read reset retry active")
	}
	if recovery.delay != 30*time.Second {
		t.Fatalf("post-crash recovery delay = %s, want 30s", recovery.delay)
	}
}

func TestReadyBudgetResetReadFailureRearmsReset(t *testing.T) {
	baseStore := newMemoryLSPStore()
	seedLSPState(t, baseStore, TaskLanguageState{
		TaskID: "task-1", Language: "go", Policy: PolicyKeepWarm,
		DetectionState: DetectionComplete, Phase: PhaseReady, Generation: 1,
		LastInitiator: InitiatorAutomatic,
	})
	store := &failNextLanguageReadStore{
		Store: baseStore, remaining: 4, err: errors.New("persistence unavailable"),
	}
	scheduler := newFakeScheduler()
	controller := newReconcileControllerWithScheduler(baseStore, newReconcileRuntimes(), scheduler)
	controller.store = store
	installTestLifecycle(controller)
	t.Cleanup(func() { _ = controller.Close(context.Background()) })

	key := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	controller.lifecycleMu.Lock()
	controller.recoveries[key] = &recoveryState{attempts: 2}
	controller.lifecycleMu.Unlock()
	controller.scheduleReadyReset(key, 1)
	scheduler.next(t).Fire()

	controller.lifecycleMu.Lock()
	attemptsAfterFailure := controller.recoveries[key].attempts
	controller.lifecycleMu.Unlock()
	if attemptsAfterFailure != 2 {
		t.Fatalf("attempts after failed reset read = %d, want 2", attemptsAfterFailure)
	}
	for _, delay := range []time.Duration{
		time.Second, 5 * time.Second, 30 * time.Second, 30 * time.Second,
	} {
		retry := scheduler.next(t)
		if retry.delay != delay {
			t.Fatalf("ready reset read retry delay = %s, want %s", retry.delay, delay)
		}
		retry.Fire()
	}
	controller.lifecycleMu.Lock()
	attemptsAfterRetry := controller.recoveries[key].attempts
	controller.lifecycleMu.Unlock()
	if attemptsAfterRetry != 0 {
		t.Fatalf("attempts after reset retry = %d, want 0", attemptsAfterRetry)
	}
}

type failNextLanguageReadStore struct {
	Store
	mu        sync.Mutex
	remaining int
	err       error
}

type blockingFailLanguageReadStore struct {
	Store
	mu      sync.Mutex
	failed  bool
	err     error
	entered chan struct{}
	release chan struct{}
}

func (s *blockingFailLanguageReadStore) GetTaskLSPLanguage(
	ctx context.Context,
	taskID, language string,
) (*TaskLanguageState, bool, error) {
	s.mu.Lock()
	if !s.failed {
		s.failed = true
		close(s.entered)
		s.mu.Unlock()
		<-s.release
		return nil, false, s.err
	}
	s.mu.Unlock()
	return s.Store.GetTaskLSPLanguage(ctx, taskID, language)
}

type failFirstWatchHost struct {
	*fakeLSPHost
	mu            sync.Mutex
	calls         int
	firstFailed   chan struct{}
	secondStarted chan struct{}
}

func newFailFirstWatchHost() *failFirstWatchHost {
	return &failFirstWatchHost{
		fakeLSPHost: newFakeLSPHost(),
		firstFailed: make(chan struct{}), secondStarted: make(chan struct{}),
	}
}

func (h *failFirstWatchHost) WatchTaskLSP(
	ctx context.Context,
	language string,
	onSnapshot func(RuntimeSnapshot) error,
) error {
	h.mu.Lock()
	h.calls++
	call := h.calls
	h.mu.Unlock()
	if call == 1 {
		close(h.firstFailed)
		return errors.New("watch failed")
	}
	close(h.secondStarted)
	snapshot, err := h.TaskLSPSnapshot(ctx, language)
	if err != nil {
		return err
	}
	if err := onSnapshot(*snapshot); err != nil {
		return err
	}
	<-ctx.Done()
	return context.Cause(ctx)
}

func (s *failNextLanguageReadStore) GetTaskLSPLanguage(
	ctx context.Context,
	taskID, language string,
) (*TaskLanguageState, bool, error) {
	s.mu.Lock()
	if s.remaining > 0 {
		s.remaining--
		err := s.err
		s.mu.Unlock()
		return nil, false, err
	}
	s.mu.Unlock()
	return s.Store.GetTaskLSPLanguage(ctx, taskID, language)
}
