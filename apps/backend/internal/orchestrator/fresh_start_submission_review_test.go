package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type initialSubmissionPersistConflictRepo struct {
	*sqliterepo.Repository
	getSessionErr error
}

func (r *initialSubmissionPersistConflictRepo) SetSessionMetadataKeyIfAbsent(
	context.Context,
	string,
	string,
	interface{},
) (bool, error) {
	return false, nil
}

func (r *initialSubmissionPersistConflictRepo) GetTaskSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	if r.getSessionErr != nil {
		return nil, r.getSessionErr
	}
	return r.Repository.GetTaskSession(ctx, sessionID)
}

func TestRejectedLaterPromptPreservesPendingInitialSubmission(t *testing.T) {
	ctx := context.Background()
	const taskID = "fresh-start-rejected-prompt-task"
	const sessionID = "fresh-start-rejected-prompt-session"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	pending, err := models.NewInitialPromptSubmission("original request", false, nil)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, pending,
	))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "initial bootstrap failed", Phase: models.LaunchErrorPhaseBootstrap,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-existing-execution")
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		promptErr:              errors.New("provider rejected prompt"),
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)

	_, err = svc.PromptTask(ctx, taskID, sessionID, "later prompt", "", false, nil, false)
	require.ErrorContains(t, err, "provider rejected prompt")

	storedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	storedSession.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, storedSession))
	available, err := svc.initialSubmissionForFreshStart(ctx, taskID, storedSession)
	require.NoError(t, err)
	require.NotNil(t, available)
	require.Equal(t, models.InitialPromptSubmissionPending, available.State,
		"a provider-rejected later prompt must not retire the original replay")
}

func TestInterruptedLaterPromptAdmissionLeavesReplayUncertain(t *testing.T) {
	ctx := context.Background()
	const taskID = "fresh-start-interrupted-prompt-task"
	const sessionID = "fresh-start-interrupted-prompt-session"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "initial bootstrap failed", Phase: models.LaunchErrorPhaseBootstrap,
	}))
	pending, err := models.NewInitialPromptSubmission("original request", false, nil)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, pending,
	))
	svc := &Service{repo: repo}
	require.NoError(t, svc.beginInitialSubmissionReplayRetirement(
		ctx, sessionID, "later-execution", "later-turn",
	))
	storedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	_, err = svc.initialSubmissionForFreshStart(ctx, taskID, storedSession)
	require.ErrorIs(t, err, ErrInitialSubmissionAcceptanceUncertain,
		"an interrupted in-flight prompt must not authorize an ambiguous replay")
}

func TestPersistLegacyInitialSubmissionReturnsConcurrentWinner(t *testing.T) {
	ctx := context.Background()
	const taskID = "fresh-start-legacy-conflict-task"
	const sessionID = "fresh-start-legacy-conflict-session"
	tests := []struct {
		name        string
		winner      func(*models.InitialPromptSubmission) *models.InitialPromptSubmission
		readErr     error
		wantPending bool
		wantNil     bool
	}{
		{
			name:        "matching pending winner",
			winner:      func(source *models.InitialPromptSubmission) *models.InitialPromptSubmission { return source },
			wantPending: true,
		},
		{
			name: "accepted winner",
			winner: func(source *models.InitialPromptSubmission) *models.InitialPromptSubmission {
				accepted := *source
				accepted.State = models.InitialPromptSubmissionAccepted
				accepted.AcceptedAt = time.Now().UTC().Format(time.RFC3339Nano)
				return &accepted
			},
			wantNil: true,
		},
		{
			name: "mismatched pending winner",
			winner: func(source *models.InitialPromptSubmission) *models.InitialPromptSubmission {
				mismatched := *source
				mismatched.Content = "different request"
				return &mismatched
			},
		},
		{
			name:    "session reread failure",
			readErr: errors.New("session read failed"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
			source, err := models.NewInitialPromptSubmission("original request", false, nil)
			require.NoError(t, err)
			if test.winner != nil {
				require.NoError(t, repo.SetSessionMetadataKey(
					ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, test.winner(source),
				))
			}
			conflictRepo := &initialSubmissionPersistConflictRepo{
				Repository: repo, getSessionErr: test.readErr,
			}
			svc := &Service{repo: conflictRepo, logger: testLogger()}
			got, err := svc.persistLegacyInitialSubmission(ctx, sessionID, source)
			if test.wantPending {
				require.NoError(t, err)
				require.Equal(t, source, got)
				return
			}
			if test.wantNil {
				require.NoError(t, err)
				require.Nil(t, got)
				return
			}
			require.ErrorIs(t, err, ErrInitialSubmissionAcceptanceUncertain)
			require.Nil(t, got)
		})
	}
}

func TestBackfillInitialSubmissionIgnoresUnrelatedSessionMessages(t *testing.T) {
	ctx := context.Background()
	const taskID = "fresh-start-backfill-task"
	const sessionID = "fresh-start-backfill-session"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "later-message-turn", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: now, CreatedAt: now,
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "original-replay-turn", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: now.Add(time.Second), CreatedAt: now.Add(time.Second),
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "later-user-message", TaskID: taskID, TaskSessionID: sessionID, TurnID: "later-message-turn",
		AuthorType: models.MessageAuthorUser, Content: "later prompt",
	}))
	submission, err := models.NewInitialPromptSubmission("original request", false, []v1.MessageAttachment{
		{AttachmentID: "original-file", Type: "resource", Name: "report.txt", MimeType: "text/plain", SizeBytes: 7, DeliveryMode: "path"},
	})
	require.NoError(t, err)
	creator := &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = creator

	svc.backfillInitialSubmissionIfMissing(ctx, taskID, sessionID, "original-replay-turn", submission)
	svc.backfillInitialSubmissionIfMissing(ctx, taskID, sessionID, "original-replay-turn", submission)

	messages, err := repo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2,
		"an unrelated later message must not suppress the original row, and retries must remain idempotent")
	var originalRows []*models.Message
	for _, message := range messages {
		if message.Content == "original request" {
			originalRows = append(originalRows, message)
		}
	}
	require.Len(t, originalRows, 1)
	require.Equal(t, initialSubmissionUserMessageID(taskID, sessionID), originalRows[0].ID)
	require.Equal(t, "original-replay-turn", originalRows[0].TurnID)
	require.Equal(t, 1, len(creator.userMessages))
	require.Equal(t, "original-file", creator.userMessages[0].metadata["attachments"].([]v1.MessageAttachment)[0].AttachmentID)
}
