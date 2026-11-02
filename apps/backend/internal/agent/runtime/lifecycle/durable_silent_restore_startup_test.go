package lifecycle

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
func TestSilentRestoreStartupAllowsSlowNativeLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		execution, predecessor := silentRestoreDeliveryOwner()
		repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
		repo.generation.Store(predecessor)
		manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
		processor := newStreamEventProcessor(manager, ctx, execution, nil, repo, 0)
		defer func() { processor.cancel(); _ = processor.close() }()
		_, event := retentionBootstrapOwner()
		event.DeliveryHarnessGeneration = 2
		require.True(t, processor.enqueue(event))
		<-repo.firstRead
		// Model a provider whose load response depends on the same open stream.
		// Fake time advances without adding a test-only production timeout seam.
		select {
		case <-processor.ctx.Done():
			t.Fatalf("stream disconnected before native load completed: %v", processor.close())
		case <-time.After(40 * time.Second):
		}
		require.Zero(t, repo.receipts.Load())
		require.Zero(t, repo.projections.Load())
		successor := *predecessor
		successor.Generation = 2
		successor.PredecessorGeneration = 1
		repo.generation.Store(&successor)
		require.NoError(t, processor.close())
		require.EqualValues(t, 1, repo.receipts.Load())
		require.EqualValues(t, 1, repo.projections.Load())
	})
}

func TestSilentRestoreStartupOwnerCancellation(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e, g := silentRestoreDeliveryOwner()
				owner, cancelOwner := context.WithCancel(t.Context())
				defer cancelOwner()
				e.bindSuccessorGenerationOwner(owner, &LaunchRequest{RequiredNativeConversationID: e.RequiredNativeConversationID})
				repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
				repo.generation.Store(g)
				result := make(chan error, 1)
				go func() {
					_, err := awaitSilentRestoreGeneration(t.Context(), e, repo, e.SessionID, e.DeliveryIncarnationID)
					result <- err
				}()
				<-repo.firstRead
				synctest.Wait()
				if committed {
					next := *g
					next.Generation = 2
					repo.generation.Store(&next)
				}
				cancelOwner()
				synctest.Wait()
				select {
				case err := <-result:
					if committed {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, context.Canceled)
					}
				default:
					t.Fatal("pending generation wait outlived its restore owner")
				}
			})
		})
	}
}

func TestSilentRestoreStartupDeadlineRemainsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, g := silentRestoreDeliveryOwner()
		repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
		repo.generation.Store(g)
		start := time.Now()
		_, err := awaitSilentRestoreGeneration(t.Context(), e, repo, e.SessionID, e.DeliveryIncarnationID)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 2*time.Minute, time.Since(start))
	})
}

func TestSilentRestoreStartupCommitRetryUsesNewOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, g := silentRestoreDeliveryOwner()
		previous, cancel := context.WithCancel(t.Context())
		req := &LaunchRequest{RequiredNativeConversationID: e.RequiredNativeConversationID, CandidateExecutionID: e.ID}
		e.bindSuccessorGenerationOwner(previous, req)
		cancel()
		e.bindSuccessorGenerationOwner(t.Context(), req)
		repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
		repo.generation.Store(g)
		result := make(chan error, 1)
		go func() {
			_, err := awaitSilentRestoreGeneration(t.Context(), e, repo, e.SessionID, e.DeliveryIncarnationID)
			result <- err
		}()
		<-repo.firstRead
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatalf("commit retry inherited the expired startup owner: %v", err)
		default:
		}
		next := *g
		next.Generation = 2
		repo.generation.Store(&next)
		require.NoError(t, <-result)
	})
}

// The owner can commit and finish while the stream's SQL read is in flight.
func TestSilentRestoreStartupCommitDuringCancelledRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, g := silentRestoreDeliveryOwner()
		owner, cancel := context.WithCancel(t.Context())
		defer cancel()
		e.bindSuccessorGenerationOwner(owner, &LaunchRequest{RequiredNativeConversationID: e.RequiredNativeConversationID})
		reads := 0
		repo := &restoreGenerationReaderFunc{read: func(ctx context.Context) (*models.HarnessSessionGeneration, error) {
			reads++
			if reads == 1 {
				return g, nil
			}
			if reads == 2 {
				cancel()
				<-ctx.Done()
				return nil, ctx.Err()
			}
			next := *g
			next.Generation = 2
			return &next, nil
		}}
		current, err := awaitSilentRestoreGeneration(t.Context(), e, repo, e.SessionID, e.DeliveryIncarnationID)
		require.NoError(t, err)
		require.EqualValues(t, 2, current.Generation)
	})
}

type restoreGenerationReaderFunc struct {
	read func(context.Context) (*models.HarnessSessionGeneration, error)
}

func (r *restoreGenerationReaderFunc) GetCurrentHarnessSessionGeneration(ctx context.Context, _, _ string) (*models.HarnessSessionGeneration, error) {
	return r.read(ctx)
}
