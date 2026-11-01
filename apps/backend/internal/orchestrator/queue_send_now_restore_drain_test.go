package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// sendNowRestoreDrainFixture seeds one promptable session with a claimed queue
// entry, exactly the state a Send Now replacement is in when its prompt fails.
type sendNowRestoreDrainFixture struct {
	svc        *Service
	agentMgr   *mockAgentManager
	claim      *messagequeue.SendNowClaim
	prompts    atomic.Int32
	workerDone chan struct{}
}

func newSendNowRestoreDrainFixture(
	t *testing.T,
	prompt func(attempt int32) (*executor.PromptResult, error),
	decorate func(messagequeue.Repository) messagequeue.Repository,
) *sendNowRestoreDrainFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	fixture := &sendNowRestoreDrainFixture{workerDone: make(chan struct{})}
	fixture.agentMgr = &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(
			context.Context, string, string, []v1.MessageAttachment, bool,
		) (*executor.PromptResult, error) {
			return prompt(fixture.prompts.Add(1))
		},
	}
	fixture.svc = createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), fixture.agentMgr)
	fixture.svc.executor = executor.NewExecutor(fixture.agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	fixture.svc.onQueuedMessageExecutionComplete = func() { close(fixture.workerDone) }
	queueRepo := newAuthoritativeMemoryRepository(repo)
	if decorate != nil {
		queueRepo = decorate(queueRepo)
	}
	fixture.svc.messageQueue = messagequeue.NewService(
		queueRepo, messagequeue.DefaultMaxPerSession, testLogger(),
	)

	source, err := fixture.svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "queued while the turn runs", "", messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	claim, err := fixture.svc.messageQueue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source})
	require.NoError(t, err)
	fixture.claim = claim
	// Production dispatch registers the handoff before starting the worker.
	fixture.svc.markQueuedDispatchInFlight("session-1", claim.Dispatch.ID)
	return fixture
}

func (f *sendNowRestoreDrainFixture) runWorker(ctx context.Context) {
	f.svc.executeSendNowClaimWithContext(ctx, f.claim, nil)
}

// awaitFIFOWorker waits for the asynchronous ordinary-FIFO dispatch the restore
// scheduled.
func (f *sendNowRestoreDrainFixture) awaitFIFOWorker(t *testing.T) {
	t.Helper()
	select {
	case <-f.workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the restored entry's FIFO dispatch")
	}
}

// TestSendNowRestoreDrainsRestoredEntryThroughFIFO covers the incident where
// readiness was published while the claim was still held: the FIFO drain found
// nothing to deliver, so restoring the claim is itself the delivery trigger.
func TestSendNowRestoreDrainsRestoredEntryThroughFIFO(t *testing.T) {
	fixture := newSendNowRestoreDrainFixture(t, func(attempt int32) (*executor.PromptResult, error) {
		if attempt == 1 {
			return nil, errors.New("prompt superseded by a newer turn")
		}
		return &executor.PromptResult{}, nil
	}, nil)

	fixture.runWorker(context.Background())
	fixture.awaitFIFOWorker(t)

	require.EqualValues(t, 2, fixture.prompts.Load(),
		"restore must trigger exactly one ordinary FIFO delivery")
	require.Len(t, fixture.agentMgr.capturedPrompts, 2)
	require.Contains(t, fixture.agentMgr.capturedPrompts[1], "queued while the turn runs")
	require.Equal(t, 0, fixture.svc.messageQueue.GetStatus(context.Background(), "session-1").Count)
}

// TestSendNowRestoreDrainFailureDoesNotLoop keeps a persistent FIFO failure on
// the existing disposition instead of re-entering Send Now.
func TestSendNowRestoreDrainFailureDoesNotLoop(t *testing.T) {
	fixture := newSendNowRestoreDrainFixture(t, func(int32) (*executor.PromptResult, error) {
		return nil, errors.New("provider rejected the prompt")
	}, nil)

	fixture.runWorker(context.Background())
	fixture.awaitFIFOWorker(t)

	require.EqualValues(t, 2, fixture.prompts.Load(),
		"one Send Now attempt plus one FIFO attempt, then the entry rests")
	// A permanent FIFO failure keeps its existing disposition (the entry is
	// consumed, not retried). The invariant this covers is that the restore does
	// not start a new Send Now or loop the FIFO attempt.
	require.False(t, fixture.svc.isQueuedDispatchInFlight("session-1"))
	require.Nil(t, fixture.svc.currentCancellation("session-1"),
		"the failed FIFO attempt must not reopen a Send Now operation")
}

// failingRestoreQueueRepository keeps every queue operation working except the
// Send Now claim restore.
type failingRestoreQueueRepository struct {
	messagequeue.Repository
}

func (r *failingRestoreQueueRepository) RestoreSendNowClaim(
	context.Context,
	*messagequeue.SendNowClaim,
) error {
	return errors.New("restore unavailable")
}

// TestSendNowFailedRestoreDoesNotMarkDrain pins that only a successful restore
// schedules delivery. A claim still holding its entries must not run them.
func TestSendNowFailedRestoreDoesNotMarkDrain(t *testing.T) {
	fixture := newSendNowRestoreDrainFixture(t, func(int32) (*executor.PromptResult, error) {
		return nil, errors.New("prompt superseded by a newer turn")
	}, func(repository messagequeue.Repository) messagequeue.Repository {
		return &failingRestoreQueueRepository{Repository: repository}
	})

	fixture.runWorker(context.Background())

	require.EqualValues(t, 1, fixture.prompts.Load(), "a failed restore must not trigger FIFO")
	_, pending := fixture.svc.queuedDispatchDrainPending.Load("session-1")
	require.False(t, pending, "a failed restore must not mark a pending drain")
}
