package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	schedpkg "github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestMessageTurnCapacityWait_RestoresHeadAndRetriesOnRelease proves the queue
// loop end to end: a capacity refusal restores the exact head, registers one
// wait, and the shared sweep delivers it after the filler releases its slot.
func TestMessageTurnCapacityWait_RestoresHeadAndRetriesOnRelease(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	manager.promptDone = make(chan struct{})
	seedTaskAndSession(t, repo, "filler-task", "filler-session", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "target-session", "target-task", "target-exec")
	markTargetPromptReady(t, svc, "target-task", "target-session", "target-exec")
	filler, refusal := svc.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: "filler-task", sessionID: "filler-session",
		intent: turnCapacityBounded, seam: "filler",
	})
	require.Nil(t, refusal)
	queued, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "target-session", "target-task", "queued follow-up", "", messagequeue.QueuedByUser,
		false, nil, nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, queued.ID)

	require.True(t, svc.drainQueuedMessageForPromptableSession(ctx, "target-session"))
	require.Eventually(t, func() bool {
		return svc.messageCapacityWaitRegistry().len() == 1
	}, 3*time.Second, 10*time.Millisecond, "refusal must register exactly one wait")

	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "target-task", "target-session")
	require.NoError(t, err)
	status, err := svc.messageQueue.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Len(t, status.Entries, 1)
	require.Equal(t, queued.ID, status.Entries[0].ID, "the same accepted entry must be restored")
	require.Equal(t, queued.Content, status.Entries[0].Content)
	require.Equal(t, queued.Position, status.Entries[0].Position, "queue position must be preserved")

	filler.release()
	svc.retryMessageCapacityWaits(ctx)
	select {
	case <-manager.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("capacity release must deliver the waiting entry")
	}
	manager.mu.Lock()
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Equal(t, []string{"queued follow-up"}, prompts)
	require.Eventually(t, func() bool {
		status, err := svc.messageQueue.Snapshot(ctx, identity)
		return err == nil && status.Count == 0
	}, 3*time.Second, 10*time.Millisecond)
	svc.retryMessageCapacityWaits(ctx)
	require.Zero(t, svc.messageCapacityWaitRegistry().len())
}

// TestMessageTurnCapacityWait_RemovesEmptyQueue keeps the registry from
// retaining waits whose accepted head no longer exists.
func TestMessageTurnCapacityWait_RemovesEmptyQueue(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := capacityTestService(t, 1)
	seedTaskAndSession(t, repo, "target-task", "target-session", models.TaskSessionStateWaitingForInput)
	svc.RegisterMessageCapacityWait(ctx, "target-task", "target-session", "missing-entry")
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len())
	svc.retryMessageCapacityWaits(ctx)
	require.Zero(t, svc.messageCapacityWaitRegistry().len())
}

type countingCapacityManager struct {
	*mockAgentManager
	resolves atomic.Int32
	launches atomic.Int32
}

func (m *countingCapacityManager) ResolveAgentProfile(
	ctx context.Context, profileID string,
) (*executor.AgentProfileInfo, error) {
	m.resolves.Add(1)
	return m.mockAgentManager.ResolveAgentProfile(ctx, profileID)
}

func (m *countingCapacityManager) LaunchAgent(
	ctx context.Context, req *executor.LaunchAgentRequest,
) (*executor.LaunchAgentResponse, error) {
	m.launches.Add(1)
	return m.mockAgentManager.LaunchAgent(ctx, req)
}

func useCountingCapacityManager(
	t *testing.T, svc *Service, repo *sqliterepo.Repository,
) *countingCapacityManager {
	t.Helper()
	manager := &countingCapacityManager{mockAgentManager: &mockAgentManager{
		repoForExecutionLookup: repo, isAgentRunning: true,
	}}
	svc.agentManager = manager
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.scheduler = schedpkg.NewScheduler(queue.NewTaskQueue(10), svc.executor, svc.taskRepo, testLogger(), schedpkg.SchedulerConfig{})
	return manager
}

func seedCapacityTaskRepo(t *testing.T, svc *Service, taskIDs ...string) {
	t.Helper()
	repo, ok := svc.taskRepo.(*mockTaskRepo)
	require.True(t, ok)
	for _, taskID := range taskIDs {
		repo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	}
}

func seedCapacityWaitTarget(
	t *testing.T, svc *Service, repo *sqliterepo.Repository,
	taskID, sessionID string, state models.TaskSessionState, prompt string, metadata map[string]interface{},
) messageCapacityWait {
	t.Helper()
	ctx := context.Background()
	seedTaskAndSession(t, repo, taskID, sessionID, state)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	queued, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByUser, false, nil, metadata,
	)
	require.NoError(t, err)
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	require.NoError(t, err)
	wait := messageCapacityWait{
		taskID: taskID, sessionID: sessionID,
		incarnationID: identity.SessionIncarnationID,
		entryID:       queued.ID, queuedAt: queued.QueuedAt,
	}
	svc.messageCapacityWaitRegistry().register(wait)
	return wait
}

func TestMessageTurnCapacityWait_CreatedStartupHonorsAutoRun(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := capacityTestService(t, 1)
	manager := useCountingCapacityManager(t, svc, repo)
	seedCapacityWaitTarget(t, svc, repo, "created-task", "created-session",
		models.TaskSessionStateCreated, "created wait", nil)
	seedCapacityTaskRepo(t, svc, "created-task")
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "created-session", false))

	svc.retryMessageCapacityWaits(ctx)
	require.Zero(t, manager.resolves.Load(), "paused CREATED queue must not start")
	require.Zero(t, manager.launches.Load())
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len())

	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "created-session", true))
	svc.retryMessageCapacityWaits(ctx)
	require.NotZero(t, manager.resolves.Load(), "enabled CREATED queue must attempt its bounded startup")
	require.NotZero(t, manager.launches.Load())
	session, err := repo.GetTaskSession(ctx, "created-session")
	require.NoError(t, err)
	require.Nil(t, session.Metadata[ceilingManualOverrideMetadataKey],
		"bounded startup must not be audited as a manual override")
}

func TestMessageTurnCapacityWait_CompletedResumeHonorsAutoRun(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := capacityTestService(t, 1)
	manager := useCountingCapacityManager(t, svc, repo)
	seedCapacityWaitTarget(t, svc, repo, "completed-task", "completed-session",
		models.TaskSessionStateCompleted, "completed wait",
		map[string]interface{}{MetaKeyCompletedFollowUpIntent: true})
	seedCapacityTaskRepo(t, svc, "completed-task")
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "completed-session", false))

	svc.retryMessageCapacityWaits(ctx)
	require.Zero(t, manager.resolves.Load(), "paused completed queue must not resume")
	require.Zero(t, manager.launches.Load())
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len())

	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "completed-session", true))
	require.Zero(t, manager.launches.Load(),
		"enabling Auto-run alone must not resume outside the capacity sweep")
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len())
}

func TestMessageTurnCapacityWait_StartupRaceTakesOneTicket(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := capacityTestService(t, 1)
	sessions := []struct {
		taskID, sessionID string
		state             models.TaskSessionState
	}{
		{"race-task-a", "race-session-a", models.TaskSessionStateCreated},
		{"race-task-b", "race-session-b", models.TaskSessionStateCreated},
		{"race-task-c", "race-session-c", models.TaskSessionStateCompleted},
	}
	waits := make([]messageCapacityWait, 0, len(sessions))
	for _, seeded := range sessions {
		wait := seedCapacityWaitTarget(t, svc, repo, seeded.taskID, seeded.sessionID,
			seeded.state, "race wait", nil)
		waits = append(waits, wait)
	}
	registry := svc.messageCapacityWaitRegistry()
	var started atomic.Int32
	startedCh := make(chan struct{}, 1)
	release := make(chan struct{})
	startup := func(context.Context) error {
		started.Add(1)
		startedCh <- struct{}{}
		<-release
		return nil
	}
	var wg sync.WaitGroup
	attemptsDone := make(chan struct{}, len(waits))
	startupErrors := make(chan error, len(waits))
	for _, wait := range waits {
		wait := wait
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := repo.GetTaskSession(ctx, wait.sessionID)
			if err != nil {
				startupErrors <- err
				return
			}
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, wait.taskID, wait.sessionID)
			if err != nil {
				startupErrors <- err
				return
			}
			svc.startMessageCapacityWaitWithTicket(ctx, registry, wait, identity, session, startup)
			attemptsDone <- struct{}{}
		}()
	}
	select {
	case <-startedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("no startup contender acquired the final ticket")
	}
	for i := 0; i < len(waits)-1; i++ {
		select {
		case <-attemptsDone:
		case <-time.After(2 * time.Second):
			t.Fatal("losing startup contenders did not return")
		}
	}
	close(release)
	wg.Wait()
	close(startupErrors)
	for err := range startupErrors {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), started.Load(), "exactly one startup may own the final slot")
	for _, seeded := range sessions {
		session, err := repo.GetTaskSession(ctx, seeded.sessionID)
		require.NoError(t, err)
		require.Nil(t, session.Metadata[ceilingManualOverrideMetadataKey],
			"a bounded startup ticket must never emit a manual override")
	}
}

type failingCapacityQueueRepository struct {
	messagequeue.Repository
	failResolve  atomic.Bool
	failSnapshot atomic.Bool
}

func (r *failingCapacityQueueRepository) ResolveSessionIdentity(
	ctx context.Context, taskID, sessionID string,
) (messagequeue.QueueSessionIdentity, error) {
	if r.failResolve.Swap(false) {
		return messagequeue.QueueSessionIdentity{}, errors.New("transient identity read")
	}
	return r.Repository.ResolveSessionIdentity(ctx, taskID, sessionID)
}

func (r *failingCapacityQueueRepository) Snapshot(
	ctx context.Context, identity messagequeue.QueueSessionIdentity,
) (messagequeue.RepositorySnapshot, error) {
	if r.failSnapshot.Swap(false) {
		return messagequeue.RepositorySnapshot{}, errors.New("transient snapshot read")
	}
	return r.Repository.Snapshot(ctx, identity)
}

func newFailingCapacityService(
	t *testing.T,
) (*Service, *mockAgentManager, *failingCapacityQueueRepository, *sqliterepo.Repository) {
	t.Helper()
	svc, manager, repo := capacityTestService(t, 1)
	failing := &failingCapacityQueueRepository{Repository: newAuthoritativeMemoryRepository(repo)}
	svc.messageQueue = messagequeue.NewService(failing, messagequeue.DefaultMaxPerSession, testLogger())
	return svc, manager, failing, repo
}

func waitForCapacityPrompt(t *testing.T, manager *mockAgentManager) {
	t.Helper()
	select {
	case <-manager.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("capacity retry did not deliver the accepted prompt")
	}
}

func TestMessageTurnCapacityWait_TransientIdentityReadKeepsWait(t *testing.T) {
	ctx := context.Background()
	svc, manager, failing, repo := newFailingCapacityService(t)
	seedCapacityWaitTarget(t, svc, repo, "id-task", "id-session",
		models.TaskSessionStateWaitingForInput, "identity retry", nil)
	seedExecutorRunning(t, repo, "id-session", "id-task", "id-exec")
	markTargetPromptReady(t, svc, "id-task", "id-session", "id-exec")
	failing.failResolve.Store(true)
	svc.retryMessageCapacityWaits(ctx)
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len(), "transient identity read must keep the wait")
	manager.promptDone = make(chan struct{})
	svc.retryMessageCapacityWaits(ctx)
	waitForCapacityPrompt(t, manager)
}

func TestMessageTurnCapacityWait_TransientSnapshotReadKeepsWait(t *testing.T) {
	ctx := context.Background()
	svc, manager, failing, repo := newFailingCapacityService(t)
	seedCapacityWaitTarget(t, svc, repo, "snap-task", "snap-session",
		models.TaskSessionStateWaitingForInput, "snapshot retry", nil)
	seedExecutorRunning(t, repo, "snap-session", "snap-task", "snap-exec")
	markTargetPromptReady(t, svc, "snap-task", "snap-session", "snap-exec")
	failing.failSnapshot.Store(true)
	svc.retryMessageCapacityWaits(ctx)
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len(), "transient snapshot read must keep the wait")
	manager.promptDone = make(chan struct{})
	svc.retryMessageCapacityWaits(ctx)
	waitForCapacityPrompt(t, manager)
}

type failingSessionReadStore struct {
	sessionExecutorStore
	fail atomic.Bool
}

func (s *failingSessionReadStore) GetTaskSession(
	ctx context.Context, id string,
) (*models.TaskSession, error) {
	if s.fail.Swap(false) {
		return nil, errors.New("transient session read")
	}
	return s.sessionExecutorStore.GetTaskSession(ctx, id)
}

func TestMessageTurnCapacityWait_TransientSessionReadKeepsWait(t *testing.T) {
	ctx := context.Background()
	svc, manager, repo := capacityTestService(t, 1)
	seedCapacityWaitTarget(t, svc, repo, "sess-task", "sess-session",
		models.TaskSessionStateWaitingForInput, "session retry", nil)
	seedExecutorRunning(t, repo, "sess-session", "sess-task", "sess-exec")
	markTargetPromptReady(t, svc, "sess-task", "sess-session", "sess-exec")
	failing := &failingSessionReadStore{sessionExecutorStore: svc.repo}
	failing.fail.Store(true)
	svc.repo = failing
	svc.retryMessageCapacityWaits(ctx)
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len(), "transient session read must keep the wait")
	manager.promptDone = make(chan struct{})
	svc.retryMessageCapacityWaits(ctx)
	waitForCapacityPrompt(t, manager)
}

func TestNotifyQueuedUserPromptCreatedSessionUsesBoundedSweep(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := capacityTestService(t, 1)
	manager := useCountingCapacityManager(t, svc, repo)
	seedTaskAndSession(t, repo, "notify-task", "notify-session", models.TaskSessionStateCreated)
	session, err := repo.GetTaskSession(ctx, "notify-session")
	require.NoError(t, err)
	session.AgentProfileID = "profile-1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedCapacityTaskRepo(t, svc, "notify-task")
	_, err = svc.messageQueue.QueueMessageWithMetadata(
		ctx, "notify-session", "notify-task", "queued created", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)

	svc.NotifyQueuedUserPrompt(ctx, "notify-task", "notify-session")
	require.Zero(t, manager.launches.Load(), "CREATED startup must wait for the sweep's bounded ticket")
	require.Equal(t, 1, svc.messageCapacityWaitRegistry().len())

	svc.retryMessageCapacityWaits(ctx)
	require.NotZero(t, manager.launches.Load())
}
