package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// releaseTestMessageCreator delegates the two history operations the release
// step uses to a real task service, so the test observes actual turn and
// message persistence.
type releaseTestMessageCreator struct {
	MessageCreator
	svc            *taskservice.Service
	failSetStatus  int
	setStatusCalls int
}

func (m *releaseTestMessageCreator) SetUserMessageDeliveryStatus(
	ctx context.Context,
	messageID, sessionID, status string,
) (bool, error) {
	m.setStatusCalls++
	if m.failSetStatus > 0 {
		m.failSetStatus--
		return false, errors.New("transient message status write failure")
	}
	return m.svc.SetUserMessageDeliveryStatus(ctx, messageID, sessionID, status)
}

func (m *releaseTestMessageCreator) CreateCompletedUserMessageIdempotent(
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

type sendNowUnrecoverableFixture struct {
	svc     *Service
	queue   *messagequeue.Service
	db      *sqlx.DB
	repo    *sqliterepo.Repository
	taskSvc *taskservice.Service
	claim   *messagequeue.SendNowClaim
	creator *releaseTestMessageCreator
}

// newSendNowUnrecoverableFixture seeds a durable plan-comment source whose
// stored reservation token no longer matches the claim, which is the exact
// restore failure the reporter's database showed. The message creator and turn
// service are real, so tests observe whether release starts a turn.
func newSendNowUnrecoverableFixture(t *testing.T) *sendNowUnrecoverableFixture {
	t.Helper()
	ctx := context.Background()
	raw, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "queue.db"))
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	source, err := queue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "queued while the turn runs", "", messagequeue.QueuedByUser,
		false, nil, map[string]interface{}{
			plancomments.MetadataClientQueueID:      "client-plan-comment",
			plancomments.MetadataRequestFingerprint: "fingerprint-plan-comment",
		},
	)
	require.NoError(t, err)
	claim, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source})
	require.NoError(t, err)

	// The readiness drain took the row with a new token while the lease
	// expired; the claim's own token no longer matches.
	var metadataJSON string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT metadata_json FROM queued_messages WHERE id = ? AND session_id = ?`,
		source.ID, source.SessionID).Scan(&metadataJSON))
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(metadataJSON), &metadata))
	metadata[messagequeue.MetadataDeliveryReservationToken] = "readiness-drain-token"
	updated, err := json.Marshal(metadata)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`UPDATE queued_messages SET metadata_json = ? WHERE id = ? AND session_id = ?`,
		string(updated), source.ID, source.SessionID)
	require.NoError(t, err)

	taskSvc := newReleaseTaskService(t, repo)
	creator := &releaseTestMessageCreator{svc: taskSvc}
	return &sendNowUnrecoverableFixture{
		queue:   queue,
		db:      db,
		repo:    repo,
		taskSvc: taskSvc,
		claim:   claim,
		creator: creator,
		svc: &Service{
			logger:         testLogger(),
			messageQueue:   queue,
			messageCreator: creator,
			turnService:    taskSvc,
		},
	}
}

func newReleaseTaskService(t *testing.T, repo *sqliterepo.Repository) *taskservice.Service {
	t.Helper()
	log := testLogger()
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

func (f *sendNowUnrecoverableFixture) userMessages(t *testing.T) []*models.Message {
	t.Helper()
	messages, err := f.repo.ListMessages(context.Background(), "session-1")
	require.NoError(t, err)
	var userMessages []*models.Message
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorUser {
			userMessages = append(userMessages, message)
		}
	}
	return userMessages
}

func (f *sendNowUnrecoverableFixture) requireReleased(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	pending, err := f.queue.ListPendingSendNowClaims(ctx)
	require.NoError(t, err)
	require.Empty(t, pending, "unrecoverable claim must be released")
	entries, _, err := f.queue.SnapshotSession(ctx, "session-1")
	require.NoError(t, err)
	require.Empty(t, entries, "surviving claimed sources must not stay queued for FIFO")

	userMessages := f.userMessages(t)
	require.Len(t, userMessages, 1, "not-delivered message must be recorded once")
	require.Equal(t, models.MessageDeliveryStatusNotDelivered,
		userMessages[0].Metadata[models.MessageMetaKeyDeliveryStatus])

	require.NotEmpty(t, userMessages[0].TurnID)
	messageTurn, err := f.repo.GetTurn(ctx, userMessages[0].TurnID)
	require.NoError(t, err)
	require.NotNil(t, messageTurn.CompletedAt, "the message must not adopt an open turn")
	require.Equal(t, true, messageTurn.Metadata[models.TurnMetaKeyLifecycleOnly],
		"the message must live in a lifecycle-only turn")

	turns, err := f.repo.ListTurnsBySession(ctx, "session-1")
	require.NoError(t, err)
	require.Len(t, turns, 1, "release must not start an extra turn")
	active, err := f.repo.GetActiveTurnBySessionID(ctx, "session-1")
	require.True(t, errors.Is(err, sql.ErrNoRows) && active == nil,
		"release must not leave an active turn (turn=%v err=%v)", active, err)
}

func TestSendNowExecuteUnrecoverableClaimMarksNotDelivered(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)

	fixture.svc.executeSendNowClaimWithContext(context.Background(), fixture.claim, nil)

	fixture.requireReleased(t)
}

func TestSendNowRecoveryUnrecoverableClaimMarksNotDelivered(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)

	require.NoError(t, fixture.svc.restorePendingSendNowClaimsForRecovery(context.Background(), "session-1"))

	fixture.requireReleased(t)
}

func TestSendNowStartupUnrecoverableClaimMarksNotDelivered(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)

	require.NoError(t, fixture.svc.reconcilePendingSendNowClaimsOnStartup(context.Background()))

	fixture.requireReleased(t)
}

// TestSendNowReleaseUnrecoverableClaimIsIdempotent covers a retry after the
// claim row is already gone. The exact message status is reapplied without a
// second history row or turn.
func TestSendNowReleaseUnrecoverableClaimIsIdempotent(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)

	require.NoError(t, fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim))
	fixture.requireReleased(t)

	require.NoError(t, fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim))
	fixture.requireReleased(t)
}

// TestSendNowReleaseRetriesTransientFailure covers the worker path where a
// transient history write must not leave the claim blocking the queue.
func TestSendNowReleaseRetriesTransientFailure(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	fixture.creator.failSetStatus = 1

	fixture.svc.executeSendNowClaimWithContext(context.Background(), fixture.claim, nil)

	fixture.requireReleased(t)
	require.GreaterOrEqual(t, fixture.creator.setStatusCalls, 2)
}

// TestSendNowReleaseUnrecoverableClaimMarksExistingMessage covers the normal
// path: the transcript already recorded the message before the prompt failed,
// so only its delivery status changes and no second turn is created.
func TestSendNowReleaseUnrecoverableClaimMarksExistingMessage(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	messageID := sendNowClaimTranscriptMessageID(fixture.claim)
	_, err := fixture.taskSvc.CreateCompletedUserMessageIdempotent(
		context.Background(), messageID, &taskservice.CreateMessageRequest{
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			Content:       "queued while the turn runs",
			AuthorType:    "user",
		},
	)
	require.NoError(t, err)

	require.NoError(t, fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim))

	fixture.requireReleased(t)
}

// TestSendNowUnrecoverableClaimForReplacedSessionKeepsExistingHandling pins
// that a claim invalidated by the session identity keeps the previous discard
// behavior: no message is marked.
func TestSendNowUnrecoverableClaimForReplacedSessionKeepsExistingHandling(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	fixture.claim.Identity = messagequeue.QueueSessionIdentity{
		TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "replaced-incarnation",
	}

	require.NoError(t, fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim))

	pending, err := fixture.queue.ListPendingSendNowClaims(context.Background())
	require.NoError(t, err)
	require.Empty(t, pending, "the exact claim is still discarded")
	require.Empty(t, fixture.userMessages(t))
}

// TestSendNowReleaseSkipsMarkingWhenSuccessorOwnsSource pins the ambiguity
// guard: a different claim now owns the session, so release must leave the
// message unmarked even though this claim could not be restored.
func TestSendNowReleaseSkipsMarkingWhenSuccessorOwnsSource(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	_, err := fixture.db.ExecContext(context.Background(),
		`UPDATE queue_send_now_claims SET claim_id = ? WHERE session_id = ?`,
		"successor-claim-id", "session-1")
	require.NoError(t, err)

	require.NoError(t, fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim))

	require.Empty(t, fixture.userMessages(t))
	pending, err := fixture.queue.ListPendingSendNowClaims(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1, "the successor claim must stay in place")
}

// TestSendNowAcceptedClaimNeverMarksNotDelivered pins the release guard: an
// accepted claim was delivered, so a superseded recovery record is discarded
// without touching the message.
func TestSendNowAcceptedClaimNeverMarksNotDelivered(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	pending := &messagequeue.PendingSendNowClaim{Claim: *fixture.claim, Accepted: true}
	pending.Claim.ClaimID = "superseded-claim-id"

	require.NoError(t, fixture.svc.reconcilePendingSendNowClaimOnStartup(context.Background(), pending))
	require.Empty(t, fixture.userMessages(t),
		"an accepted claim must not be marked not delivered")
}

// TestSendNowReleaseStatusFailureKeepsClaimForRetry pins the ordering: the
// claim remains available when the not-delivered status cannot be written.
func TestSendNowReleaseStatusFailureKeepsClaimForRetry(t *testing.T) {
	fixture := newSendNowUnrecoverableFixture(t)
	fixture.creator.failSetStatus = 10

	err := fixture.svc.releaseUnrecoverableSendNowClaim(context.Background(), fixture.claim)
	require.Error(t, err)
	require.Empty(t, fixture.userMessages(t))
	pending, listErr := fixture.queue.ListPendingSendNowClaims(context.Background())
	require.NoError(t, listErr)
	require.Len(t, pending, 1, "the claim must remain for a later retry")
}
