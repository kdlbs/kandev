package messagequeue

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func awaitAdmissionWaiters(t *testing.T, gate *sessionAdmissionGate, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		gate.stateMu.Lock()
		got := len(gate.waiters)
		gate.stateMu.Unlock()
		if got == count {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("gate did not reach %d queued waiters", count)
}

func TestSessionAdmissionFIFOWithCancelledWaiter(t *testing.T) {
	gate := newSessionAdmissionGate()
	require.NoError(t, gate.lock(context.Background()))
	order := make(chan int, 2)
	firstDone := make(chan error, 1)
	go func() {
		err := gate.lock(context.Background())
		if err == nil {
			order <- 1
			gate.unlock()
		}
		firstDone <- err
	}()
	awaitAdmissionWaiters(t, gate, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	go func() { cancelled <- gate.lock(ctx) }()
	awaitAdmissionWaiters(t, gate, 2)
	lastDone := make(chan error, 1)
	go func() {
		err := gate.lock(context.Background())
		if err == nil {
			order <- 3
			gate.unlock()
		}
		lastDone <- err
	}()
	awaitAdmissionWaiters(t, gate, 3)
	cancel()
	require.ErrorIs(t, <-cancelled, context.Canceled)
	gate.unlock()
	require.NoError(t, <-firstDone)
	require.NoError(t, <-lastDone)
	require.Equal(t, 1, <-order)
	require.Equal(t, 3, <-order)
	require.False(t, gate.locked)
	require.Empty(t, gate.waiters)
}

func TestSessionAdmissionCancellationAtGrantHandsOff(t *testing.T) {
	gate := newSessionAdmissionGate()
	require.NoError(t, gate.lock(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- gate.lock(ctx) }()
	awaitAdmissionWaiters(t, gate, 1)
	// Publish cancellation and grant under the state lock. Either select arm
	// must relinquish this grant before a successor can enter.
	gate.stateMu.Lock()
	cancel()
	gate.handoffLocked()
	gate.stateMu.Unlock()
	require.ErrorIs(t, <-done, context.Canceled)
	next, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, gate.lock(next))
	gate.unlock()
	require.False(t, gate.locked)
	require.Empty(t, gate.waiters)
}

func TestSessionAdmissionRechecksImmediateGrant(t *testing.T) {
	gate := newSessionAdmissionGate()
	gate.locked = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, gate.recheckAfterGrant(ctx), context.Canceled)
	require.False(t, gate.locked)
}

func TestSessionAdmissionBudgetAndErrorOwnership(t *testing.T) {
	ctx, cancel, own := admissionWaitBudget(context.Background())
	defer cancel()
	require.True(t, own)
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, time.Second)
	caller, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	child, release, own := admissionWaitBudget(caller)
	defer release()
	require.False(t, own)
	parentDeadline, _ := caller.Deadline()
	childDeadline, _ := child.Deadline()
	require.Equal(t, parentDeadline, childDeadline)
	require.ErrorIs(t, admissionWaitError(context.DeadlineExceeded, true), ErrSessionAdmissionTimeout)
	require.ErrorIs(t, admissionWaitError(context.DeadlineExceeded, false), context.DeadlineExceeded)
	require.ErrorIs(t, admissionWaitError(context.Canceled, true), context.Canceled)
	storageErr := errors.New("storage failure")
	require.ErrorIs(t, admissionWaitError(storageErr, true), storageErr)
}

func TestSessionAdmissionSQLiteTimeoutDoesNotMutateOrLeakReferences(t *testing.T) {
	previous := admissionWaitTimeout
	admissionWaitTimeout = 20 * time.Millisecond
	t.Cleanup(func() { admissionWaitTimeout = previous })
	repo := newTestSQLiteRepo(t)
	svc := NewService(repo, DefaultMaxPerSession, logger.Default())
	held, release, holderDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holderDone <- svc.WithSessionAdmission(context.Background(), "session", func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	ctx := context.Background()
	operations := map[string]func() error{
		"insert": func() error {
			_, err := svc.QueueMessage(ctx, "session", "task", "must not insert", "", QueuedByUser, false, nil)
			return err
		},
		"append": func() error {
			_, _, err := svc.AppendContent(ctx, "session", "task", "must not append", "", QueuedByUser, false, nil)
			return err
		},
		"update": func() error {
			return svc.UpdateMessageWithMetadata(ctx, "session", "entry", "must not update", nil, nil, QueuedByUser)
		},
		"remove":   func() error { return svc.RemoveEntry(ctx, "session", "entry") },
		"clear":    func() error { _, err := svc.CancelAll(ctx, "session"); return err },
		"merge":    func() error { _, err := svc.MergeIntoAbove(ctx, "session", "entry", QueuedByUser); return err },
		"reorder":  func() error { return svc.ReorderEntries(ctx, "session", []string{"entry"}) },
		"auto run": func() error { return svc.SetAutoRun(ctx, "session", false) },
		"auto merge": func() error {
			_, err := svc.SetSessionAutoMerge(ctx, QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation"}, false)
			return err
		},
		"begin edit": func() error { _, err := svc.BeginEdit(ctx, "session", "entry", "connection"); return err },
		"renew edit": func() error { _, err := svc.RenewEdit(ctx, "session", "entry", "lease", "connection"); return err },
		"end edit":   func() error { return svc.EndEdit(ctx, "session", "entry", "lease", "connection") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) { require.ErrorIs(t, operation(), ErrSessionAdmissionTimeout) })
	}
	rows, err := repo.ListBySession(context.Background(), "session")
	require.NoError(t, err)
	require.Empty(t, rows)
	svc.admissionMu.Lock()
	refs := svc.admissions["session"].refs
	svc.admissionMu.Unlock()
	require.Equal(t, 1, refs)
	close(release)
	require.NoError(t, <-holderDone)
	require.Empty(t, svc.admissions)
	_, err = svc.QueueMessage(context.Background(), "session", "task", "positive control", "", QueuedByUser, false, nil)
	require.NoError(t, err)
}

func TestSessionAdmissionCallbackKeepsCallerContextAndDeadlineError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := NewServiceMemory(logger.Default())
		err := svc.WithSessionAdmission(context.Background(), "session", func(ctx context.Context) error {
			<-time.After(31 * time.Second)
			_, deadline := ctx.Deadline()
			require.False(t, deadline)
			require.NoError(t, ctx.Err(), "acquisition budget cancelled an admitted callback")
			return svc.WithSessionAdmission(ctx, "session", func(nested context.Context) error {
				require.Equal(t, ctx, nested)
				return context.DeadlineExceeded
			})
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotErrorIs(t, err, ErrSessionAdmissionTimeout)
		require.Empty(t, svc.admissions)
	})
}

func TestSessionAdmissionLifecycleBudgetOwnsOnlyAcquisitionTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := NewServiceMemory(logger.Default())
		svc.lifecycleMu.Lock()
		defer svc.lifecycleMu.Unlock()
		callback := func(context.Context) error { t.Error("callback ran under lifecycle writer"); return nil }
		require.ErrorIs(t, svc.WithSessionAdmission(context.Background(), "session", callback), ErrSessionAdmissionTimeout)
		caller, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.ErrorIs(t, svc.WithSessionAdmission(caller, "session", callback), context.DeadlineExceeded)
		require.Empty(t, svc.admissions)
	})
}

func TestSessionAdmissionLifecycleBarrierHonorsCancellation(t *testing.T) {
	svc := NewServiceMemory(logger.Default())
	svc.lifecycleMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- svc.WithSessionAdmission(ctx, "session", func(context.Context) error {
			t.Error("cancelled acquisition ran callback")
			return nil
		})
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	svc.lifecycleMu.Unlock()
	require.Empty(t, svc.admissions)
	var mu sync.RWMutex
	require.NoError(t, rlockWithContext(context.Background(), &mu))
	mu.RUnlock()
}
