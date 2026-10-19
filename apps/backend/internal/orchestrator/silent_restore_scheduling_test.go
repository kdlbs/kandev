package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
func TestSilentRestorePassAdmitsIndependentCandidateWhileOneIsBlocked(t *testing.T) {
	service, repo, _, _, store := newSilentRestoreEligibilityFixture(t, "provider-session", "independent")
	seedBlockedSilentRestoreCandidate(t, context.Background(), repo)
	blocked := make(chan struct{})
	admitted := make(chan struct{})
	release := make(chan struct{})
	var blockedOnce, admittedOnce sync.Once
	service.SetStartupRecoveryOwnerContextResolver(func(ctx context.Context, owner, org string) (context.Context, error) {
		if owner == "foreign-owner" {
			blockedOnce.Do(func() { close(blocked) })
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			admittedOnce.Do(func() { close(admitted) })
		}
		return ctx, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { service.runSilentRestoreRecoveryPass(ctx, store); close(done) }()
	defer func() { close(release); cancel(); <-done }()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("first candidate never reached its boundary")
	}
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("one blocked candidate serialized an independent restoration")
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
func TestSilentRestoreQueuePrioritizesFocusAndDeduplicates(t *testing.T) {
	q := newRestartRestoreQueue(t.Context())
	defer q.stop()
	entered := make(chan string, 5)
	release := make(chan struct{})
	defer close(release)
	work := func(ctx context.Context, c models.SilentRestoreCandidate) error {
		entered <- c.SessionID
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}
	submit := func(id string) *restartRestoreJob {
		return q.submit(models.SilentRestoreCandidate{SessionID: id}, work)
	}
	first := submit("one")
	_ = submit("two")
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("two independent operations did not start")
		}
	}
	_ = submit("three")
	focused := submit("focused")
	q.prioritize("focused")
	require.Same(t, first, submit("one"))
	require.Same(t, focused, submit("focused"))
	select {
	case id := <-entered:
		t.Fatalf("exceeded two-operation limit: %s", id)
	default:
	}
	// Release exactly one worker by cancellation, without changing the queue owner.
	first.cancel()
	select {
	case id := <-entered:
		require.Equal(t, "focused", id)
	case <-time.After(time.Second):
		t.Fatal("pending focus never took the available slot")
	}
}

func TestSilentRestoreQueueStopsPendingAndJoinsWorkers(t *testing.T) {
	q := newRestartRestoreQueue(t.Context())
	entered := make(chan struct{}, 2)
	work := func(ctx context.Context, _ models.SilentRestoreCandidate) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	q.submit(models.SilentRestoreCandidate{SessionID: "one"}, work)
	q.submit(models.SilentRestoreCandidate{SessionID: "two"}, work)
	for range 2 {
		<-entered
	}
	pending := q.submit(models.SilentRestoreCandidate{SessionID: "pending"}, work)
	q.stop()
	select {
	case <-pending.done:
		require.ErrorIs(t, pending.err, context.Canceled)
	default:
		t.Fatal("pending restoration survived shutdown")
	}
	select {
	case <-entered:
		t.Fatal("queued restoration started during shutdown")
	default:
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
func TestSilentRestoreFocusUsesTheSharedBoundedQueue(t *testing.T) {
	service, _, _, session, _ := newSilentRestoreEligibilityFixture(t, "provider-session", "focused")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service.recoveryLifecycleCtx = ctx
	q := service.restartRestoreScheduler(ctx)
	defer q.stop()
	entered := make(chan string, 4)
	release := make(chan struct{})
	defer close(release)
	work := func(ctx context.Context, c models.SilentRestoreCandidate) error {
		entered <- c.SessionID
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}
	first := q.submit(models.SilentRestoreCandidate{SessionID: "one"}, work)
	q.submit(models.SilentRestoreCandidate{SessionID: "two"}, work)
	for range 2 {
		<-entered
	}
	q.submit(models.SilentRestoreCandidate{SessionID: "three"}, work)
	service.SetStartupRecoveryOwnerContextResolver(func(ctx context.Context, _, _ string) (context.Context, error) {
		entered <- session.ID
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ctx, ctx.Err()
	})
	for range 2 {
		response, err := service.LaunchSession(ctx, &LaunchSessionRequest{TaskID: session.TaskID, SessionID: session.ID, Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionFocus})
		require.NoError(t, err)
		require.Equal(t, "restart_restore_pending", response.ActivationReason)
	}
	first.cancel()
	select {
	case id := <-entered:
		require.Equal(t, session.ID, id)
	case <-time.After(time.Second):
		t.Fatal("focused pending conversation did not take next restoration slot")
	}
}
