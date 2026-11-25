package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type continuationUnreadableQueue struct{ messagequeue.Repository }

func (continuationUnreadableQueue) ListBySession(context.Context, string) ([]messagequeue.QueuedMessage, error) {
	return nil, errors.New("queue read unavailable")
}

func TestInterruptionContinuationOwnershipQueuedHumanWorkTakesPriority(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	svc.messageQueue = newAuthoritativeMemoryQueue(svc.repo.(*sqliterepo.Repository), testLogger())
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	queued, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "new human request", "", "", false, nil)
	require.NoError(t, err)
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Empty(t, mgr.capturedPrompts, "automatic continuation cannot jump ahead of human work")
	require.Equal(t, 1, svc.messageQueue.GetStatus(context.Background(), "s1").Count)
	require.Equal(t, queued.ID, svc.messageQueue.GetStatus(context.Background(), "s1").Entries[0].ID)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
}

func TestInterruptionContinuationOwnershipUnknownQueueRefusesDispatch(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	svc.messageQueue = messagequeue.NewService(continuationUnreadableQueue{Repository: messagequeue.NewMemoryRepository()}, messagequeue.DefaultMaxPerSession, testLogger())
	require.Error(t, svc.validateContinuationOwner(context.Background(), "t1", "s1", value.(*transientRetryEntry)), "a failed queue read cannot attest that no human work is waiting")
}

func TestDynamicContinuationKeepsQueueOwnedTranscriptOutOfPrompts(t *testing.T) {
	for _, mode := range []string{"pending", "reserved", "restored", "accepted", "other-session", "unrecorded", "retained"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "held-task", "held-session", "step-1")
			queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
			t.Cleanup(func() { _ = db.Close() })
			sessionID := "held-session"
			if mode == "other-session" {
				sessionID = "another-session"
			}
			metadata := map[string]interface{}{
				metaKeyUserMessageRecorded:                      mode != "unrecorded",
				messagequeue.MetadataDurableTranscriptMessageID: "explicit-held-row",
			}
			if mode == "unrecorded" {
				delete(metadata, messagequeue.MetadataDurableTranscriptMessageID)
			}
			if mode == "retained" {
				metadata[messagequeue.MetadataLifecycleDurable] = true
			}
			queued, err := queue.QueueMessageWithMetadata(ctx, sessionID, "held-task", "queued request", "", "", false, nil, metadata)
			require.NoError(t, err)
			if mode != "pending" && mode != "unrecorded" {
				reserved, ok := queue.ReserveQueued(ctx, sessionID)
				require.True(t, ok)
				require.Equal(t, queued.ID, reserved.ID)
				if mode == "accepted" {
					require.NoError(t, queue.MarkPendingQueueDispatchAccepted(ctx, reserved))
				}
				if mode == "restored" {
					_, err = queue.RestoreMessage(ctx, reserved)
					require.NoError(t, err)
				}
			}
			before, _, err := queue.SnapshotSession(ctx, sessionID)
			require.NoError(t, err)
			claimsBefore, err := queue.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			stableID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("kandev:queued-transcript:held-task:"+sessionID+":"+queued.ID)).String()
			if mode == "unrecorded" {
				stableID = "unrelated-user-row"
			}
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "held-turn", TaskID: "held-task", TaskSessionID: "held-session", StartedAt: now}))
			for index, row := range []struct{ id, content string }{
				{"delivered-row", "delivered request"}, {stableID, "stable queued row"}, {"explicit-held-row", "explicit queued row"},
			} {
				require.NoError(t, repo.CreateMessage(ctx, &models.Message{
					ID: row.id, TaskID: "held-task", TaskSessionID: "held-session", TurnID: "held-turn",
					AuthorType: models.MessageAuthorUser, Content: row.content, CreatedAt: now.Add(time.Duration(index) * time.Second),
				}))
			}
			svc := &Service{repo: repo, messageQueue: queue, logger: testLogger()}
			input := &dynamicruntime.ContinuationInput{UserMessages: []string{"explicit caller prompt"}}
			require.NoError(t, svc.addDynamicConversation(ctx, "held-session", input))
			prompt, ok := svc.dynamicRelaunchPrompt(ctx, "held-session")
			require.True(t, ok)
			if mode == "accepted" || mode == "other-session" || mode == "unrecorded" {
				require.Equal(t, []string{"explicit caller prompt", "delivered request", "stable queued row", "explicit queued row"}, input.UserMessages)
				require.Equal(t, "explicit queued row", prompt.text)
			} else {
				require.Equal(t, []string{"explicit caller prompt", "delivered request"}, input.UserMessages)
				require.Equal(t, "delivered request", prompt.text)
			}
			svc.lastTurnPrompt.Store("held-session", capturedPrompt{text: "authoritative in-memory prompt"})
			prompt, ok = svc.dynamicRelaunchPrompt(ctx, "held-session")
			require.True(t, ok)
			require.Equal(t, "authoritative in-memory prompt", prompt.text)
			after, _, err := queue.SnapshotSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, before, after, "context selection must not mutate queue ownership or content")
			claimsAfter, err := queue.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			require.Equal(t, claimsBefore, claimsAfter)
		})
	}
}

type continuationUnreadableClaims struct{ messagequeue.Repository }

func (continuationUnreadableClaims) ListPendingQueueDispatches(context.Context) ([]messagequeue.PendingQueueDispatch, error) {
	return nil, errors.New("claims read unavailable")
}

func (continuationUnreadableClaims) MarkPendingQueueDispatchAccepted(context.Context, *messagequeue.QueuedMessage) error {
	return nil
}

func (continuationUnreadableClaims) DeletePendingQueueDispatch(context.Context, *messagequeue.QueuedMessage) error {
	return nil
}

func TestDynamicContinuationUnreadableQueueRefusesPromptReconstruction(t *testing.T) {
	for _, kind := range []string{"snapshot", "claims", "optional-store", "nil-queue"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "read-task", "read-session", "step-1")
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "read-turn", TaskID: "read-task", TaskSessionID: "read-session", StartedAt: now}))
			require.NoError(t, repo.CreateMessage(ctx, &models.Message{
				ID: "read-user", TaskID: "read-task", TaskSessionID: "read-session", TurnID: "read-turn",
				AuthorType: models.MessageAuthorUser, Content: "durable request", CreatedAt: now,
			}))
			queueRepo := messagequeue.NewMemoryRepository()
			if kind == "snapshot" {
				queueRepo = continuationUnreadableQueue{Repository: queueRepo}
			}
			if kind == "claims" {
				queueRepo = continuationUnreadableClaims{Repository: queueRepo}
			}
			svc := &Service{repo: repo, messageQueue: messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger()), logger: testLogger()}
			if kind == "nil-queue" {
				svc.messageQueue = nil
			}
			input := &dynamicruntime.ContinuationInput{UserMessages: []string{"explicit prompt"}}
			err := svc.addDynamicConversation(ctx, "read-session", input)
			prompt, ok := svc.dynamicRelaunchPrompt(ctx, "read-session")
			if kind == "snapshot" || kind == "claims" {
				require.Error(t, err)
				require.False(t, ok)
				require.Equal(t, []string{"explicit prompt"}, input.UserMessages)
			} else {
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "durable request", prompt.text)
			}
		})
	}
}

func TestDynamicContinuationExcludesTranscriptRecordedAfterReservation(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(map[bool]string{false: "reserved", true: "restored-from-stale-claim"}[restore], func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "late-task", "late-session", "step-1")
			queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
			t.Cleanup(func() { _ = db.Close() })
			queued, err := queue.QueueMessage(ctx, "late-session", "late-task", "recorded after reservation", "", "", false, nil)
			require.NoError(t, err)
			reserved, ok := queue.ReserveQueued(ctx, queued.SessionID)
			require.True(t, ok)
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "late-turn", TaskID: queued.TaskID, TaskSessionID: queued.SessionID, StartedAt: now}))
			svc := &Service{
				repo: repo, messageQueue: queue, logger: testLogger(),
				messageCreator: &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo},
			}
			svc.activeTurns.Store(queued.SessionID, "late-turn")
			require.NoError(t, svc.recordQueuedUserMessage(ctx, reserved, nil))
			require.Equal(t, true, reserved.Metadata[metaKeyUserMessageRecorded])
			claims, err := queue.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			require.Len(t, claims, 1)
			require.NotEqual(t, true, claims[0].Message.Metadata[metaKeyUserMessageRecorded], "the durable claim predates transcript recording")
			if restore {
				_, err = queue.RestoreMessage(ctx, &claims[0].Message)
				require.NoError(t, err)
			}
			before, _, err := queue.SnapshotSession(ctx, queued.SessionID)
			require.NoError(t, err)
			claimsBefore, err := queue.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			input := &dynamicruntime.ContinuationInput{}
			require.NoError(t, svc.addDynamicConversation(ctx, queued.SessionID, input))
			require.Empty(t, input.UserMessages, "the real writer's row must remain owned by the queued delivery")
			_, ok = svc.dynamicRelaunchPrompt(ctx, queued.SessionID)
			require.False(t, ok, "a queue-only transcript cannot become a recovery prompt")
			after, _, err := queue.SnapshotSession(ctx, queued.SessionID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			claimsAfter, err := queue.ListPendingQueueDispatches(ctx)
			require.NoError(t, err)
			require.Equal(t, claimsBefore, claimsAfter)
		})
	}
}
