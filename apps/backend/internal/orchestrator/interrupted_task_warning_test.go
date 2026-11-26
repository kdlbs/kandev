package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestTaskInterruptionPromptAcceptance(t *testing.T) {
	t.Run("accepted direct prompt clears the captured marker and publishes", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, _, session := newInterruptionPromptFixture(t, "task-direct", "session-direct")
		marker := addInterruptedSessionMarker(t, repo, "task-direct", session)
		snapshot := svc.captureInterruptedTaskWarning(ctx, "task-direct", session, "execution-"+session.ID, "attempt", "turn")
		require.True(t, snapshot.captured, "the direct prompt should capture the scoped warning")
		require.Equal(t, marker.Token, snapshot.token)
		publisher := &recordingTaskUpdatedPublisher{}
		svc.SetTaskEventPublisher(publisher)

		_, err := svc.promptTask(ctx, "task-direct", session.ID, "continue", "", false, nil, false, launchOriginManual, promptTaskOptions{
			onAccepted: func(string) {
				requireTaskInterruptionEntries(t, repo, "task-direct", map[string]models.InterruptedSessionMarker{
					session.ID: marker,
				})
			},
		})
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-direct", nil)
		require.Contains(t, publisher.updatedTaskIDs, "task-direct")
		require.NotEmpty(t, marker.Token)
	})

	t.Run("rejected prompt and queue-only submission keep the marker", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, manager, session := newInterruptionPromptFixture(t, "task-refused", "session-refused")
		marker := addInterruptedSessionMarker(t, repo, "task-refused", session)
		manager.promptErr = errors.New("provider refused prompt")

		_, err := svc.promptTask(ctx, "task-refused", session.ID, "continue", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.Error(t, err)
		requireTaskInterruptionEntries(t, repo, "task-refused", map[string]models.InterruptedSessionMarker{session.ID: marker})

		queue := newAuthoritativeMemoryQueue(repo, testLogger())
		_, err = queue.QueueMessageWithMetadata(ctx, session.ID, "task-refused", "later", "", messagequeue.QueuedByUser, false, nil, nil)
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-refused", map[string]models.InterruptedSessionMarker{session.ID: marker})
	})

	t.Run("accepted queued prompt clears the captured marker", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, _, session := newInterruptionPromptFixture(t, "task-queued", "session-queued")
		addInterruptedSessionMarker(t, repo, "task-queued", session)
		svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
		require.NoError(t, svc.messageQueue.SetAutoRun(ctx, session.ID, true))
		identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-queued", session.ID)
		require.NoError(t, err)
		_, err = svc.messageQueue.QueueMessageWithMetadata(
			ctx, session.ID, "task-queued", "queued continuation", "", messagequeue.QueuedByUser, false, nil, nil,
		)
		require.NoError(t, err)
		queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, queued)
		reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(ctx, identity, queued.ID, queued)
		require.NotNil(t, reservation)
		claimed, err := svc.claimQueuedDispatchForExecution(session.ID, queued.ID, reservation)
		require.NoError(t, err)
		require.True(t, claimed)

		_, err = svc.promptTask(
			ctx, "task-queued", session.ID, "queued continuation", "", false, nil, true, launchOriginAutomatic,
			promptTaskOptions{claimEntryID: queued.ID, expectedSessionIdentity: &identity},
		)
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-queued", nil)
	})

	t.Run("replacement initial prompt clears only after current acceptance", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, _, session := newInterruptionPromptFixture(t, "task-replacement", "session-replacement")
		marker := addInterruptedSessionMarker(t, repo, "task-replacement", session)
		accepted := false
		options := startCreatedSessionOptions{
			acceptInitialPrompt: func(executionID string) bool { return executionID == "replacement-execution" },
			onInitialPromptAccepted: func(string) {
				accepted = true
			},
		}
		var launchOptions executor.LaunchOptions
		require.NoError(t, svc.configureInitialPromptDispatch(
			ctx, "task-replacement", session, "replacement-turn", true,
			"replacement prompt", nil, options, &launchOptions,
		))
		require.NotNil(t, launchOptions.OnExecutionAdmitted)
		require.NotNil(t, launchOptions.OnInitialPromptAccepted)
		launchOptions.OnExecutionAdmitted("replacement-execution")
		launchOptions.OnInitialPromptAccepted("stale-execution")
		require.False(t, accepted, "a stale replacement execution must not accept the prompt")
		requireTaskInterruptionEntries(t, repo, "task-replacement", map[string]models.InterruptedSessionMarker{
			session.ID: marker,
		})

		launchOptions.OnInitialPromptAccepted("replacement-execution")
		require.True(t, accepted)
		requireTaskInterruptionEntries(t, repo, "task-replacement", nil)
	})

	t.Run("healthy siblings preserve both warnings until each accepts", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, _, first := newInterruptionPromptFixture(t, "task-siblings", "session-first")
		second := createInterruptionPromptSession(t, repo, "task-siblings", "session-second")
		healthy := createInterruptionPromptSession(t, repo, "task-siblings", "session-healthy")
		seedExecutorRunning(t, repo, second.ID, "task-siblings", "execution-"+second.ID)
		seedExecutorRunning(t, repo, healthy.ID, "task-siblings", "execution-"+healthy.ID)
		firstMarker := addInterruptedSessionMarker(t, repo, "task-siblings", first)
		secondMarker := addInterruptedSessionMarker(t, repo, "task-siblings", second)
		wantBoth := map[string]models.InterruptedSessionMarker{first.ID: firstMarker, second.ID: secondMarker}

		_, err := svc.promptTask(ctx, "task-siblings", healthy.ID, "healthy prompt", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-siblings", wantBoth)

		_, err = svc.promptTask(ctx, "task-siblings", first.ID, "first continuation", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-siblings", map[string]models.InterruptedSessionMarker{second.ID: secondMarker})

		_, err = svc.promptTask(ctx, "task-siblings", second.ID, "second continuation", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		requireTaskInterruptionEntries(t, repo, "task-siblings", nil)
	})

	t.Run("accepted prompt consumes captured legacy warning and preserves scoped sibling", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, _, session := newInterruptionPromptFixture(t, "task-legacy-scoped", "session-legacy-scoped")
		legacy := "2026-11-24T10:00:00Z"
		require.NoError(t, repo.SetTaskMetadataKey(ctx, "task-legacy-scoped", models.MetaKeyInterruptedAt, legacy))
		sibling := createInterruptionPromptSession(t, repo, "task-legacy-scoped", "session-legacy-sibling")
		targetMarker := addInterruptedSessionMarker(t, repo, "task-legacy-scoped", session)
		siblingMarker := addInterruptedSessionMarker(t, repo, "task-legacy-scoped", sibling)
		requireTaskInterruptionEntries(t, repo, "task-legacy-scoped", map[string]models.InterruptedSessionMarker{
			session.ID: targetMarker, sibling.ID: siblingMarker,
		})

		_, err := svc.promptTask(ctx, "task-legacy-scoped", session.ID, "continue", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		task, err := repo.GetTask(ctx, "task-legacy-scoped")
		require.NoError(t, err)
		require.NotContains(t, task.Metadata, models.MetaKeyLegacyInterruptedAt)
		require.Equal(t, siblingMarker.InterruptedAt, task.Metadata[models.MetaKeyInterruptedAt])
		requireTaskInterruptionEntries(t, repo, "task-legacy-scoped", map[string]models.InterruptedSessionMarker{
			sibling.ID: siblingMarker,
		})
	})

	t.Run("accepted prompt preserves a newer legacy marker", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, manager, session := newInterruptionPromptFixture(t, "task-new-legacy", "session-new-legacy")
		const (
			legacy               = "2026-11-24T10:00:00Z"
			targetInterruptedAt  = "2026-11-25T10:00:00Z"
			siblingInterruptedAt = "2026-11-25T11:00:00Z"
			newer                = "2026-11-26T10:00:00Z"
		)
		require.NoError(t, repo.SetTaskMetadataKey(ctx, "task-new-legacy", models.MetaKeyInterruptedAt, legacy))
		sibling := createInterruptionPromptSession(t, repo, "task-new-legacy", "session-new-legacy-sibling")
		targetMarker := addInterruptedSessionMarkerAt(t, repo, "task-new-legacy", session, targetInterruptedAt)
		siblingMarker := addInterruptedSessionMarkerAt(t, repo, "task-new-legacy", sibling, siblingInterruptedAt)
		manager.promptAgentFunc = func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			require.NoError(t, repo.SetTaskMetadataKey(ctx, "task-new-legacy", models.MetaKeyLegacyInterruptedAt, newer))
			return &executor.PromptResult{}, nil
		}

		_, err := svc.promptTask(ctx, "task-new-legacy", session.ID, "continue", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		task, err := repo.GetTask(ctx, "task-new-legacy")
		require.NoError(t, err)
		require.Equal(t, newer, task.Metadata[models.MetaKeyLegacyInterruptedAt])
		require.Equal(t, newer, task.Metadata[models.MetaKeyInterruptedAt])
		requireTaskInterruptionEntries(t, repo, "task-new-legacy", map[string]models.InterruptedSessionMarker{
			sibling.ID: siblingMarker,
		})
		require.NotEqual(t, targetMarker.Token, siblingMarker.Token)
	})

	t.Run("stale callback cannot clear a newer incarnation", func(t *testing.T) {
		ctx := context.Background()
		repo, svc, manager, session := newInterruptionPromptFixture(t, "task-stale", "session-stale")
		oldMarker := addInterruptedSessionMarker(t, repo, "task-stale", session)
		newIncarnation := "new-interruption-incarnation"
		newMarker := models.InterruptedSessionMarker{
			IncarnationID: newIncarnation,
			InterruptedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Token:         "new-interruption-token",
		}
		manager.promptAgentFunc = func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			_, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`, newIncarnation, session.ID)
			require.NoError(t, err)
			changed, err := repo.SetTaskInterruptedSessionIfNotArchived(ctx, "task-stale", session.ID, newMarker)
			require.NoError(t, err)
			require.True(t, changed)
			return &executor.PromptResult{}, nil
		}

		_, err := svc.promptTask(ctx, "task-stale", session.ID, "continue", "", false, nil, false, launchOriginManual, promptTaskOptions{})
		require.NoError(t, err)
		require.NotEqual(t, oldMarker.Token, newMarker.Token)
		requireTaskInterruptionEntries(t, repo, "task-stale", map[string]models.InterruptedSessionMarker{session.ID: newMarker})
	})
}

func newInterruptionPromptFixture(
	t *testing.T,
	taskID, sessionID string,
) (*sqliterepo.Repository, *Service, *mockAgentManager, *models.TaskSession) {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, session.QueueIncarnationID)
	executionID := "execution-" + sessionID
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = withTestCapabilities(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
	svc.turnService = &repoTurnService{repo: repo}
	return repo, svc, manager, session
}

func createInterruptionPromptSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string) *models.TaskSession {
	t.Helper()
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreateTaskSession(context.Background(), session))
	reloaded, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, reloaded.QueueIncarnationID)
	return reloaded
}

func addInterruptedSessionMarker(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	session *models.TaskSession,
) models.InterruptedSessionMarker {
	t.Helper()
	return addInterruptedSessionMarkerAt(t, repo, taskID, session, time.Now().UTC().Format(time.RFC3339Nano))
}

func addInterruptedSessionMarkerAt(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	session *models.TaskSession,
	interruptedAt string,
) models.InterruptedSessionMarker {
	t.Helper()
	marker := models.InterruptedSessionMarker{
		IncarnationID: session.QueueIncarnationID,
		InterruptedAt: interruptedAt,
		Token:         "interruption-" + session.ID,
	}
	changed, err := repo.SetTaskInterruptedSessionIfNotArchived(context.Background(), taskID, session.ID, marker)
	require.NoError(t, err)
	require.True(t, changed)
	return marker
}

func requireTaskInterruptionEntries(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID string,
	want map[string]models.InterruptedSessionMarker,
) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	got, _, err := models.LoadInterruptedSessionMarkers(task.Metadata)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
