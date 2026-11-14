package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// e2eSendNowMessageCreator implements only the message operations the release
// step uses. The embedded interface satisfies the rest of MessageCreator
// without wiring the whole orchestrator into the fixture process.
type e2eSendNowMessageCreator struct {
	MessageCreator
	svc *taskservice.Service
}

func (m e2eSendNowMessageCreator) CreateUserMessageIdempotent(
	ctx context.Context,
	messageID, taskID, content, sessionID, turnID string,
	metadata map[string]interface{},
) error {
	_, err := m.svc.CreateMessageIdempotent(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	return err
}

func (m e2eSendNowMessageCreator) SetUserMessageDeliveryStatus(
	ctx context.Context,
	messageID, sessionID, status string,
) (bool, error) {
	return m.svc.SetUserMessageDeliveryStatus(ctx, messageID, sessionID, status)
}

func (m e2eSendNowMessageCreator) CreateCompletedUserMessageIdempotent(
	ctx context.Context,
	messageID, taskID, content, sessionID string,
	metadata map[string]interface{},
) error {
	_, err := m.svc.CreateCompletedUserMessageIdempotent(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	return err
}

// TestSendNowNotDeliveredFixture seeds a pending Send Now claim whose source
// row is missing, then runs the same release entry point a resolved
// unknown_send_now_outcome recovery block uses. The managed browser tests read
// the marked history row afterwards.
func TestSendNowNotDeliveredFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_SEND_NOW_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed browser fixture")
	}
	require.True(t, strings.HasPrefix(filepath.Base(root), "kandev-e2e-"))
	sessionID := os.Getenv("KANDEV_E2E_SEND_NOW_SESSION")
	require.NotEmpty(t, sessionID)
	ctx := context.Background()

	connection, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	sqlConnection := sqlx.NewDb(connection, "sqlite3")
	taskRepo, err := tasksqlite.NewWithDB(sqlConnection, sqlConnection, nil)
	require.NoError(t, err)
	queueRepo, err := messagequeue.NewSQLiteRepository(sqlConnection, sqlConnection)
	require.NoError(t, err)
	log := logger.Default()
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)

	session, err := taskRepo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	if session.State != models.TaskSessionStateWaitingForInput {
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, taskRepo.UpdateTaskSession(ctx, session))
	}

	// A durable plan comment keeps its queued row while the claim owns it, so
	// deleting that row is exactly the missing-source restore failure.
	source, err := queue.QueueMessageWithMetadata(
		ctx, session.ID, session.TaskID, "Fix the button spacing (2 notes)", "",
		messagequeue.QueuedByUser, false, nil, map[string]interface{}{
			plancomments.MetadataClientQueueID:      "e2e-send-now-plan-comment",
			plancomments.MetadataRequestFingerprint: "e2e-send-now-" + session.ID,
		},
	)
	require.NoError(t, err)
	claim, err := queue.ClaimSendNow(ctx, session.ID, []messagequeue.QueuedMessage{*source})
	require.NoError(t, err)
	require.NotEmpty(t, claim.ClaimID)
	expectedMessageID := sendNowClaimTranscriptMessageID(claim)
	_, err = sqlConnection.ExecContext(ctx,
		`DELETE FROM queued_messages WHERE id = ? AND session_id = ?`, source.ID, session.ID)
	require.NoError(t, err)

	svc := &Service{
		logger:       log,
		messageQueue: queue,
		messageCreator: e2eSendNowMessageCreator{
			svc: taskServiceForSendNowFixture(t, sqlConnection),
		},
	}
	require.NoError(t, svc.restorePendingSendNowClaimsForRecovery(ctx, session.ID))

	pending, err := queue.ListPendingSendNowClaims(ctx)
	require.NoError(t, err)
	require.Empty(t, pending, "the unrestorable claim must be released")
	messages, err := taskRepo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	found := false
	for _, message := range messages {
		if message.ID == expectedMessageID &&
			message.AuthorType == models.MessageAuthorUser &&
			message.Metadata[models.MessageMetaKeyDeliveryStatus] == models.MessageDeliveryStatusNotDelivered {
			found = true
		}
	}
	require.True(t, found, "history must contain the not-delivered user message")
	fmt.Println("KANDEV_E2E_SEND_NOW_FIXTURE:released")
}

func taskServiceForSendNowFixture(t *testing.T, database *sqlx.DB) *taskservice.Service {
	t.Helper()
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	log := logger.Default()
	return taskservice.NewService(taskservice.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
		ResourceCleanups: repo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{})
}
