package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type freshStartSQLReader struct {
	*tasksqlite.Repository
	firstRead chan struct{}
	once      sync.Once
}

func (r *freshStartSQLReader) GetCurrentHarnessSessionGeneration(ctx context.Context, sessionID, incarnationID string) (*models.HarnessSessionGeneration, error) {
	generation, err := r.Repository.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	r.once.Do(func() { close(r.firstRead) })
	return generation, err
}

func TestFreshStartSQLWaitAdmissionAndCancellation(t *testing.T) {
	for _, outcome := range []string{"committed", "cancelled", "rejected commit", "failed commit"} {
		t.Run(outcome, func(t *testing.T) { exerciseFreshStartSQLWait(t, outcome) })
	}
}

func exerciseFreshStartSQLWait(t *testing.T, outcome string) {
	t.Helper()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	execution, event := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.DeliveryHarnessGeneration, execution.ReplacesNativeConversation = 2, true
	event.DeliveryHarnessGeneration = 2
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: execution.TaskID, Title: "Fresh SQL wait"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: execution.SessionID, TaskID: execution.TaskID, QueueIncarnationID: execution.DeliveryIncarnationID}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{SessionID: execution.SessionID, TaskID: execution.TaskID,
		AgentExecutionID: execution.ID, ResumeToken: "replacement-native", Status: models.ExecutorRunningStatusRunning}))
	predecessor := retentionBootstrapGeneration()
	committed, err := repo.CommitHarnessSessionGeneration(t.Context(), predecessor, 0)
	require.NoError(t, err)
	require.True(t, committed)
	reader := &freshStartSQLReader{Repository: repo, firstRead: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	done, result := make(chan struct{}), make(chan error, 1)
	go func() {
		defer close(done)
		_, err := manager.prepareAgentEvent(ctx, execution, nil, reader, event)
		result <- err
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-reader.firstRead:
	case <-ctx.Done():
		t.Fatal("event did not inspect the real SQL predecessor")
	}
	var receipts int
	require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
	require.Zero(t, receipts, "no receipt may precede the successor commit")
	select {
	case err := <-result:
		t.Fatalf("event returned before its successor commit: %v", err)
	default:
	}
	successor := *predecessor
	successor.Generation, successor.PredecessorGeneration = 2, 1
	successor.NativeSessionID, successor.CreationReason = "replacement-native", "native_replaced"
	if outcome != "cancelled" {
		if outcome == "rejected commit" {
			successor.NativeSessionID = "obsolete-native"
		}
		if outcome == "failed commit" {
			_, triggerErr := repo.DB().Exec(`CREATE TRIGGER reject_fresh_successor BEFORE INSERT ON harness_session_generations WHEN NEW.generation = 2 BEGIN SELECT RAISE(ABORT, 'successor insert rejected'); END`)
			require.NoError(t, triggerErr)
		}
		committed, err = repo.CommitHarnessSessionGenerationForExecution(t.Context(), &successor, 1, execution.TaskID, execution.ID)
		if outcome == "failed commit" {
			require.ErrorContains(t, err, "successor insert rejected")
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, outcome == "committed", committed)
	}
	if outcome != "committed" {
		cancel()
	}
	select {
	case err = <-result:
		if outcome == "committed" {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, context.Canceled)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("successor waiter did not settle")
	}
	<-done
	require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
	if outcome != "committed" {
		require.Zero(t, receipts)
		return
	}
	require.Equal(t, 1, receipts)
	execution.DeliveryHarnessGeneration, event.DeliveryHarnessGeneration = 1, 1
	execution.ReplacesNativeConversation = false
	_, err = manager.prepareAgentEvent(t.Context(), execution, nil, reader, event)
	require.True(t, errors.Is(err, ErrDurableAdoptionIdentityMismatch), "a delayed predecessor event must fail after the successor")
	require.NoError(t, repo.DB().QueryRow("SELECT count(*) FROM agent_delivery_inbox").Scan(&receipts))
	require.Equal(t, 1, receipts, "the stale event must not add a second receipt")
}
