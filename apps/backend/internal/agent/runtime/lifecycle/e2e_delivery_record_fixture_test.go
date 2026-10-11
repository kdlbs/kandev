package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const (
	deliveryRecordFixtureBlockID = "block-e2e-delivery-record"
)

// TestDeliveryRecordRecoveryFixture seeds and checks the isolated SQL and
// retained-journal incident used by managed browser tests.
func TestDeliveryRecordRecoveryFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_RECORD_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed browser fixture")
	}
	require.True(t, strings.HasPrefix(filepath.Base(root), "kandev-e2e-"))
	sessionID := os.Getenv("KANDEV_E2E_RECORD_SESSION")
	require.NotEmpty(t, sessionID)
	action := os.Getenv("KANDEV_E2E_RECORD_ACTION")
	connection, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	sqlConnection := sqlx.NewDb(connection, "sqlite3")
	repo, err := tasksqlite.NewWithDB(sqlConnection, sqlConnection, nil)
	require.NoError(t, err)

	switch action {
	case "seed", "seed_ambiguous", "seed_unblocked":
		seedDeliveryRecordIncident(t, repo, root, sessionID, action == "seed_ambiguous", action != "seed_unblocked")
	case "assert_reconstructed":
		assertDeliveryRecordReconstructed(t, repo, sessionID)
	case "assert_ambiguous_unrepaired":
		assertDeliveryRecordAmbiguousUnrepaired(t, repo, sessionID)
	default:
		t.Fatalf("unknown KANDEV_E2E_RECORD_ACTION %q", action)
	}
}

func seedDeliveryRecordIncident(t *testing.T, repo *tasksqlite.Repository, root, sessionID string, ambiguous, blocked bool) {
	t.Helper()
	ctx := context.Background()
	streamID := "stream-e2e-delivery-record-" + sessionID
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	if !blocked {
		require.NotEmpty(t, session.WorkspacePath)
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, repo.UpdateTaskSession(ctx, session))
	}
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	const generation = int64(7)
	nativeSessionID := "native-e2e-record-" + session.ID
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: generation,
		NativeSessionID: nativeSessionID, CreationReason: "initial",
	}))
	now := time.Now().UTC()
	turn := &models.Turn{
		ID: "turn-e2e-record-" + session.ID, TaskID: session.TaskID,
		TaskSessionID: session.ID, StartedAt: now,
	}
	if !blocked {
		turn.CompletedAt = &now
	}
	require.NoError(t, repo.CreateTurn(ctx, turn))
	block := &models.SessionRecoveryBlock{
		ID: deliveryRecordFixtureBlockID + "-" + session.ID, SessionID: session.ID,
		IncarnationID: incarnationID, ExpectedGeneration: generation,
		Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliveryOutcome: "unknown",
		CreatedAt: now, UpdatedAt: now,
	}
	if blocked {
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	}

	messageCount := 1
	if ambiguous {
		messageCount = 2
	}
	capability := journal.CheckStorage(filepath.Join(root, ".kandev"), session.ID)
	retained, err := journal.Open(journal.Config{Path: capability.Path})
	require.NoError(t, err)
	defer func() { require.NoError(t, retained.Close()) }()
	for index := range messageCount {
		messageID := fmt.Sprintf("message-e2e-record-%s-%d", session.ID, index+1)
		content := "Inspect the saved work and continue from the same conversation"
		if ambiguous {
			content = fmt.Sprintf("Ambiguous saved instruction %d", index+1)
		}
		message := &models.Message{
			ID: messageID, TaskID: session.TaskID, TaskSessionID: session.ID,
			TurnID:     "turn-e2e-record-" + session.ID,
			AuthorType: models.MessageAuthorUser, Content: content,
			Metadata:  map[string]interface{}{models.MessageMetaKeyDeliveryStatus: models.MessageDeliveryStatusBlocked},
			CreatedAt: now.Add(time.Duration(index) * time.Second), UpdatedAt: now.Add(time.Duration(index) * time.Second),
		}
		require.NoError(t, repo.CreateMessage(ctx, message))
		payload, marshalErr := json.Marshal(map[string]string{"text": content})
		require.NoError(t, marshalErr)
		_, err = retained.PutSubmission(ctx, journal.Submission{
			ID: "prompt:" + messageID, StreamID: streamID,
			SessionID: session.ID, IncarnationID: incarnationID,
			HarnessGeneration: uint64(generation), Hash: journal.SubmissionHash(payload), Payload: payload,
			State: journal.SubmissionDispatching, CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
		})
		require.NoError(t, err)
	}
	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: "prompt:completed-e2e-record-" + session.ID, StreamID: "stream-e2e-record-history",
		SessionID: session.ID, IncarnationID: incarnationID, HarnessGeneration: uint64(generation - 1),
		Hash:    journal.SubmissionHash([]byte(`{"text":"completed history"}`)),
		Payload: []byte(`{"text":"completed history"}`), State: journal.SubmissionCompleted,
		TerminalEventRetained: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: session.ID, IncarnationID: incarnationID, HarnessGeneration: uint64(generation),
		StreamID:     streamID,
		SubmissionID: "prompt:message-e2e-record-" + session.ID + "-1",
		Type:         "assistant_message", Payload: []byte(`{"text":"Retained output from the interrupted turn"}`),
	})
	require.NoError(t, err)
	require.NoError(t, retained.Acknowledge(ctx, streamID, 1))
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{
		SessionID: session.ID, IncarnationID: incarnationID, HarnessGeneration: generation,
		StreamID: streamID, Sequence: 1,
		SubmissionID: "prompt:message-e2e-record-" + session.ID + "-1",
		EventType:    "assistant_message", Payload: []byte(`{"text":"Retained output from the interrupted turn"}`),
	}, 1)
	require.NoError(t, err)

	var canonicalCount int
	require.NoError(t, repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = ?`, session.ID).Scan(&canonicalCount))
	require.Zero(t, canonicalCount)
	fmt.Println("KANDEV_E2E_RECORD_FIXTURE:seeded")
}

func assertDeliveryRecordReconstructed(t *testing.T, repo *tasksqlite.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	var count int
	require.NoError(t, repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = ?`, sessionID).Scan(&count))
	require.Equal(t, 1, count, "Retry must create exactly one canonical submission")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Empty(t, recovery.AgentExecutionID)
	require.Zero(t, recovery.PromptGeneration)
	require.NotNil(t, recovery.Reconstruction)
	require.False(t, recovery.Reconstruction.ProcessIdentityKnown)
	require.Equal(t, models.AgentDeliveryRecoveryUncertain, recovery.Phase)
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, recovery.IncarnationID, recovery.HarnessGeneration)
	require.NoError(t, err)
	require.Equal(t, recovery.SubmissionID, block.DeliverySubmissionID)
	require.Equal(t, recovery.StreamID, block.DeliveryStreamID)
	submission, err := repo.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	userMessages := 0
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorUser {
			userMessages++
		}
	}
	require.Equal(t, 1, userMessages, "Retry must not duplicate the saved user instruction")
	fmt.Println("KANDEV_E2E_RECORD_FIXTURE:reconstructed_once")
}

func assertDeliveryRecordAmbiguousUnrepaired(t *testing.T, repo *tasksqlite.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	var count int
	require.NoError(t, repo.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = ?`, sessionID).Scan(&count))
	require.Zero(t, count, "ambiguous retained evidence must not create a canonical submission")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	_, hasRecovery := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.False(t, hasRecovery)
	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	userMessages := 0
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorUser {
			userMessages++
		}
	}
	require.Equal(t, 2, userMessages)
	fmt.Println("KANDEV_E2E_RECORD_FIXTURE:ambiguous_unrepaired")
}
