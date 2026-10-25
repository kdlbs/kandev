package orchestrator

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
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

type pagedRestoreInventory struct {
	taskrepo.SilentRestoreRepository
	nextPage chan struct{}
}

func (r *pagedRestoreInventory) ListSilentRestoreCandidates(_ context.Context, after string, limit int) ([]models.SilentRestoreCandidate, error) {
	if after != "" {
		close(r.nextPage)
		return nil, nil
	}
	rows := make([]models.SilentRestoreCandidate, limit)
	for i := range rows {
		rows[i].SessionID = fmt.Sprintf("pending-%03d", i)
	}
	return rows, nil
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-004.1
func TestSilentRestorePassBoundsInventoryToOnePage(t *testing.T) {
	service, _, _, _, store := newSilentRestoreEligibilityFixture(t, "provider-session", "paged")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	q := service.restartRestoreScheduler(ctx)
	defer q.stop()
	release := make(chan struct{})
	entered := make(chan struct{}, restartRestoreConcurrency)
	for i := range restartRestoreConcurrency {
		q.submit(models.SilentRestoreCandidate{SessionID: fmt.Sprintf("active-%d", i)}, func(ctx context.Context, _ models.SilentRestoreCandidate) error {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		})
	}
	for range restartRestoreConcurrency {
		<-entered
	}
	inventory := &pagedRestoreInventory{SilentRestoreRepository: store, nextPage: make(chan struct{})}
	done := make(chan struct{})
	go func() { service.runSilentRestoreRecoveryPass(ctx, inventory); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case <-inventory.nextPage:
		t.Fatal("read another full metadata page while both workers were blocked")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-inventory.nextPage:
	case <-time.After(3 * time.Second):
		t.Fatal("next page did not become readable after current-page restoration finished")
	}
	<-done
}
