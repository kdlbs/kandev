package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type admissionRecoveryBarrierRepository struct {
	messagequeue.Repository
	attempts atomic.Int32
	entered  chan struct{}
	release  chan struct{}
}

func (r *admissionRecoveryBarrierRepository) RequeuePreservingFIFOForSession(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, msg *messagequeue.QueuedMessage,
) error {
	if err := r.beforeCompensation(ctx); err != nil {
		return err
	}
	return r.Repository.RequeuePreservingFIFOForSession(ctx, identity, msg)
}

func (r *admissionRecoveryBarrierRepository) RestoreForSession(
	ctx context.Context, identity messagequeue.QueueSessionIdentity, msg *messagequeue.QueuedMessage, max int,
) error {
	if err := r.beforeCompensation(ctx); err != nil {
		return err
	}
	return r.Repository.RestoreForSession(ctx, identity, msg, max)
}

func (r *admissionRecoveryBarrierRepository) beforeCompensation(ctx context.Context) error {
	if r.attempts.Add(1) == 1 {
		return messagequeue.ErrSessionAdmissionTimeout
	}
	close(r.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func TestQueuedAdmissionRecoveryRetainsSQLiteReservationUntilCompensation(t *testing.T) {
	for _, mode := range []struct {
		name          string
		stop, restore bool
	}{
		{name: "requeue"}, {name: "requeue-service-stop", stop: true},
		{name: "restore", restore: true}, {name: "restore-service-stop", stop: true, restore: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			stop := mode.stop
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
			manager := &mockAgentManager{isAgentRunning: true, promptErr: messagequeue.ErrSessionAdmissionTimeout, repoForExecutionLookup: repo}
			if mode.restore {
				manager.promptErr = errors.New("UsageLimitExceeded")
			}
			paused, resume := make(chan struct{}), make(chan struct{})
			core, _ := observer.New(zapcore.ErrorLevel)
			log, err := logger.NewFromZap(zap.New(core, zap.Hooks(func(entry zapcore.Entry) error {
				if stop && entry.Message == "failed post-claim queue mutation" {
					close(paused)
					<-resume
				}
				return nil
			})))
			require.NoError(t, err)
			bus := eventbus.NewMemoryEventBus(log)
			t.Cleanup(bus.Close)
			svc := NewService(DefaultServiceConfig(), bus, manager, newMockTaskRepo(), repo, nil, nil, nil, log)
			svc.SetTurnService(&repoTurnService{repo: repo})
			queueDB := sqlx.NewDb(repo.DB(), "sqlite3")
			sqlQueue, err := messagequeue.NewSQLiteRepository(queueDB, queueDB)
			require.NoError(t, err)
			barrier := &admissionRecoveryBarrierRepository{Repository: sqlQueue, entered: make(chan struct{}), release: make(chan struct{})}
			svc.messageQueue = messagequeue.NewService(barrier, messagequeue.DefaultMaxPerSession, log)
			svc.messageQueue.SetAutoMergeEnabled(false)
			require.NoError(t, svc.Start(ctx))
			defer func() {
				if svc.running {
					require.NoError(t, svc.Stop())
				}
			}()
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "t1", "s1")
			require.NoError(t, err)
			queued, err := svc.messageQueue.QueueMessageWithMetadataForSession(ctx, identity, "keep exact payload", "", messagequeue.QueuedByUser, false, nil, map[string]interface{}{"evidence": "preserve"})
			require.NoError(t, err)
			taken, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
			require.NoError(t, err)
			require.True(t, ok)
			reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(context.Background(), identity, taken.ID, taken)
			done := make(chan struct{})
			go func() { svc.executeQueuedMessageWithReservation("s1", taken, reservation); close(done) }()
			<-barrier.entered
			select {
			case <-done:
				t.Fatal("worker released its reservation before compensation settled")
			default:
			}
			rows, err := sqlQueue.ListBySession(ctx, "s1")
			require.NoError(t, err)
			require.Empty(t, rows)
			var payload string
			require.NoError(t, repo.DB().QueryRow("SELECT message_json FROM queue_dispatch_claims WHERE entry_id = ?", queued.ID).Scan(&payload))
			var claimed messagequeue.QueuedMessage
			require.NoError(t, json.Unmarshal([]byte(payload), &claimed))
			require.Equal(t, queued.ID, claimed.ID)
			require.Equal(t, queued.Content, claimed.Content)
			if stop {
				require.NoError(t, svc.Stop())
				<-paused
				// Install the fresh callback owner exactly as Start does, while
				// the old caller is paused after compensation cancellation.
				svc.resetReservedPromptCallbacks()
				oldCtx := svc.queuedRecoveryContext(ctx, reservation)
				lateReservation := svc.newQueuedDispatchReservation(oldCtx, "s1", taken.ID, identity, taken)
				require.Same(t, reservation.recoveryOwner, lateReservation.recoveryOwner)
				called := false
				err := svc.retryQueuedRecoveryMutation(svc.queuedRecoveryContext(ctx, reservation), func(context.Context) error {
					called = true
					return nil
				})
				require.ErrorIs(t, err, context.Canceled)
				require.False(t, called, "old reservation borrowed the new recovery owner")
				svc.markQueuedDispatchDrainPending("s1")
				close(resume)
			} else {
				close(barrier.release)
			}
			<-done
			rows, err = sqlQueue.ListBySession(ctx, "s1")
			require.NoError(t, err)
			if stop {
				require.Empty(t, rows)
				_, pending := svc.queuedDispatchDrainPending.Load("s1")
				require.True(t, pending, "stopped-generation worker consumed the new owner's pending drain")
				require.NoError(t, repo.DB().QueryRow("SELECT message_json FROM queue_dispatch_claims WHERE entry_id = ?", queued.ID).Scan(&payload))
				require.NoError(t, json.Unmarshal([]byte(payload), &claimed))
				require.Equal(t, queued.Content, claimed.Content)
				svc.stopReservedPromptCallbacks()
			} else {
				require.Len(t, rows, 1)
				require.Equal(t, queued.ID, rows[0].ID)
				require.Equal(t, queued.Content, rows[0].Content)
				require.Equal(t, "preserve", rows[0].Metadata["evidence"])
				require.False(t, rows[0].IsReservedInFlight())
			}
			require.Equal(t, int32(2), barrier.attempts.Load())
		})
	}
}

type admissionDrainRecordingRepository struct {
	messagequeue.Repository
	reserved chan string
}

func (r *admissionDrainRecordingRepository) ReserveHeadIfAutoRunForSession(
	ctx context.Context, identity messagequeue.QueueSessionIdentity,
) (*messagequeue.QueuedMessage, bool, error) {
	msg, enabled, err := r.Repository.ReserveHeadIfAutoRunForSession(ctx, identity)
	if msg != nil {
		r.reserved <- msg.ID
	}
	return msg, enabled, err
}

func TestQueuedAdmissionRecoveryDrainCannotCrossStopWhileWaitingForCancelGuard(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "live-positive-control", true: "stopped-generation"}[stop], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
			manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			log := logger.Default()
			bus := eventbus.NewMemoryEventBus(log)
			t.Cleanup(bus.Close)
			svc := NewService(DefaultServiceConfig(), bus, manager, newMockTaskRepo(), repo, nil, nil, nil, log)
			svc.SetTurnService(&repoTurnService{repo: repo})
			db := sqlx.NewDb(repo.DB(), "sqlite3")
			sqlQueue, err := messagequeue.NewSQLiteRepository(db, db)
			require.NoError(t, err)
			recording := &admissionDrainRecordingRepository{Repository: sqlQueue, reserved: make(chan string, 2)}
			svc.messageQueue = messagequeue.NewService(recording, messagequeue.DefaultMaxPerSession, log)
			svc.messageQueue.SetAutoMergeEnabled(false)
			require.NoError(t, svc.Start(ctx))
			t.Cleanup(func() {
				if svc.running {
					require.NoError(t, svc.Stop())
				}
				svc.stopReservedPromptCallbacks()
			})
			workerDone := make(chan struct{}, 1)
			svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }
			guard, release := svc.acquireCancelInFlightGuard("s1")
			guard.Lock()
			locked := true
			defer func() {
				if locked {
					guard.Unlock()
				}
				release()
			}()
			oldCtx := svc.queuedRecoveryContext(ctx, nil)
			svc.markQueuedDispatchDrainPending("s1")
			drained := make(chan struct{})
			go func() { svc.drainQueuedDispatchIfPending(oldCtx, "s1"); close(drained) }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				svc.cancelInFlightMu.Lock()
				refs := svc.cancelInFlight["s1"].refs
				svc.cancelInFlightMu.Unlock()
				if refs == 2 {
					break
				}
				require.True(t, time.Now().Before(deadline), "drain did not reach the real cancel guard")
				runtime.Gosched()
			}
			if stop {
				require.NoError(t, svc.Stop())
				select {
				case <-drained:
				case <-time.After(5 * time.Second):
					t.Fatal("stopped drain still waits for the held cancel guard")
				}
				svc.resetReservedPromptCallbacks()
			}
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "t1", "s1")
			require.NoError(t, err)
			fresh, err := svc.messageQueue.QueueMessageWithMetadataForSession(ctx, identity, "fresh generation payload", "", messagequeue.QueuedByUser, false, nil, nil)
			require.NoError(t, err)
			guard.Unlock()
			locked = false
			select {
			case <-drained:
			case <-time.After(5 * time.Second):
				t.Fatal("drain did not settle after the guard was released")
			}
			select {
			case id := <-recording.reserved:
				select {
				case <-workerDone:
				case <-time.After(5 * time.Second):
					t.Fatal("dispatched worker did not settle")
				}
				require.Equal(t, fresh.ID, id)
				require.False(t, stop, "old drain reserved fresh SQL work after Stop while waiting for the cancel guard")
			default:
				require.True(t, stop, "live positive control did not reserve the SQL queue head")
				rows, err := sqlQueue.ListBySession(ctx, "s1")
				require.NoError(t, err)
				require.Len(t, rows, 1)
				require.Equal(t, fresh.ID, rows[0].ID)
			}
		})
	}
}
