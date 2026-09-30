package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestManagedBindingReopen(t *testing.T) {
	repo, db, path := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-reopen"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, create := reserveManagedAgentStart(t, repo, sessionID, "turn-create")
	accepted := transitionManagedAgentOperation(t, repo, create, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-initial", "")
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding after create: %v", err)
	}
	settled := transitionManagedAgentOperation(t, repo, accepted, binding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	committed, err := repo.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: settled.ID, RemoteRunID: settled.RemoteRunID,
		EventID: "event-1", EventType: "run_completed", Cursor: "cursor-1",
		DispatchGeneration: settled.DispatchGeneration, TerminalEventType: "completed",
	})
	if err != nil || !committed {
		t.Fatalf("commit checkpoint: committed=%t err=%v", committed, err)
	}
	binding, err = repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload binding: %v", err)
	}
	followup := &models.ManagedAgentOperation{
		ID: "operation-followup-unknown", BindingID: binding.ID, PromptTurnID: "turn-followup-unknown",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-followup-unknown",
		RequestSnapshot: managedAgentRequestSnapshot("follow up"), PreSubmitRunID: "run-initial",
	}
	followupBinding, pending, replayed, err := repo.ReserveManagedAgentOperation(
		ctx, followup, binding.Revision, "worker-2", time.Now().Add(time.Minute),
	)
	if err != nil || replayed {
		t.Fatalf("reserve follow-up: replayed=%t err=%v", replayed, err)
	}
	unknown := transitionManagedAgentOperation(t, repo, pending, followupBinding.Revision, "worker-2",
		models.ManagedAgentSubmissionUnknown, "", "submission could not be confirmed")

	rawToken := "callback-bearer-canary"
	hash := sha256.Sum256([]byte(rawToken))
	grant := &models.ManagedAgentToolGrant{
		ID: "grant-reopen", BindingID: binding.ID, OperationID: settled.ID,
		TokenHash: hex.EncodeToString(hash[:]), Scope: `{"session":"` + sessionID + `"}`,
		Generation: settled.DispatchGeneration, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.CreateManagedAgentToolGrant(ctx, grant); err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	conn, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	reopenedDB := sqlx.NewDb(conn, "sqlite3")
	reopened, err := NewWithDB(reopenedDB, reopenedDB, nil)
	if err != nil {
		_ = reopenedDB.Close()
		t.Fatalf("reinitialize repository: %v", err)
	}
	t.Cleanup(func() { _ = reopenedDB.Close() })

	storedBinding, err := reopened.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil || storedBinding.RemoteAgentID != binding.RemoteAgentID || storedBinding.Launch != binding.Launch {
		t.Fatalf("reopened binding = %+v err=%v", storedBinding, err)
	}
	storedUnknown, err := reopened.GetManagedAgentOperationByPromptTurnID(ctx, unknown.PromptTurnID)
	if err != nil || storedUnknown.State != models.ManagedAgentSubmissionUnknown || storedUnknown.RequestDigest != unknown.RequestDigest {
		t.Fatalf("reopened unknown operation = %+v err=%v", storedUnknown, err)
	}
	checkpoint, err := reopened.GetManagedAgentStreamCheckpoint(ctx, binding.ID, "run-initial")
	if err != nil || checkpoint.Cursor != "cursor-1" || checkpoint.TerminalEventType != "completed" {
		t.Fatalf("reopened checkpoint = %+v err=%v", checkpoint, err)
	}
	storedGrant, err := reopened.GetManagedAgentToolGrantByHash(ctx, grant.TokenHash)
	if err != nil || storedGrant.ID != grant.ID {
		t.Fatalf("reopened grant = %+v err=%v", storedGrant, err)
	}
	var rawCredential, rawBearer int
	if err := reopened.db.GetContext(ctx, &rawCredential, `SELECT COUNT(*) FROM managed_agent_bindings WHERE credential_ref = ?`, "provider-api-key-canary"); err != nil {
		t.Fatalf("check credential persistence: %v", err)
	}
	if err := reopened.db.GetContext(ctx, &rawBearer, `SELECT COUNT(*) FROM managed_agent_tool_grants WHERE token_hash = ?`, rawToken); err != nil {
		t.Fatalf("check bearer persistence: %v", err)
	}
	if rawCredential != 0 || rawBearer != 0 {
		t.Fatalf("raw credentials persisted: api-key=%d bearer=%d", rawCredential, rawBearer)
	}
}

func TestManagedMigrationFailurePreservesSessionsAndReplays(t *testing.T) {
	repo, db, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-migration-replay"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	for _, table := range []string{
		"managed_agent_tool_grants", "managed_agent_stream_events", "managed_agent_streams",
		"managed_agent_operations", "managed_agent_bindings",
	} {
		if _, err := repo.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatalf("drop managed table %s: %v", table, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, `CREATE TABLE managed_agent_operations (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create broken prior table: %v", err)
	}
	if err := repo.initManagedAgentSchema(); err == nil {
		t.Fatal("managed agent migration succeeded against a malformed prior operations table")
	}
	var taskCount, sessionCount int
	if err := repo.db.GetContext(ctx, &taskCount, `SELECT COUNT(*) FROM tasks WHERE id = ?`, "task-"+sessionID); err != nil {
		t.Fatalf("count task after failed migration: %v", err)
	}
	if err := repo.db.GetContext(ctx, &sessionCount, `SELECT COUNT(*) FROM task_sessions WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("count session after failed migration: %v", err)
	}
	if taskCount != 1 || sessionCount != 1 {
		t.Fatalf("failed migration changed prior rows: tasks=%d sessions=%d", taskCount, sessionCount)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE managed_agent_operations`); err != nil {
		t.Fatalf("remove malformed table: %v", err)
	}
	retry, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("retry managed agent repository initialization: %v", err)
	}
	if err := retry.initManagedAgentSchema(); err != nil {
		t.Fatalf("retry managed agent migration: %v", err)
	}
	if err := retry.initManagedAgentSchema(); err != nil {
		t.Fatalf("replay managed agent migration: %v", err)
	}
	if err := retry.db.GetContext(ctx, &taskCount, `SELECT COUNT(*) FROM tasks WHERE id = ?`, "task-"+sessionID); err != nil {
		t.Fatalf("count task after replay: %v", err)
	}
	if err := retry.db.GetContext(ctx, &sessionCount, `SELECT COUNT(*) FROM task_sessions WHERE id = ?`, sessionID); err != nil {
		t.Fatalf("count session after replay: %v", err)
	}
	if taskCount != 1 || sessionCount != 1 {
		t.Fatalf("migration replay changed prior rows: tasks=%d sessions=%d", taskCount, sessionCount)
	}
}

func TestManagedListActiveBindingsTracksOnlyUnsettledOperations(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-active-bindings"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-create")
	active, err := repo.ListActiveManagedAgentBindings(ctx)
	if err != nil || len(active) != 1 || active[0].ID != binding.ID {
		t.Fatalf("active bindings = %#v err=%v, want reserved binding", active, err)
	}
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-initial", "")
	refreshedBinding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	transitionManagedAgentOperation(t, repo, accepted, refreshedBinding.Revision, "",
		models.ManagedAgentSubmissionSucceeded, accepted.RemoteRunID, "")
	active, err = repo.ListActiveManagedAgentBindings(ctx)
	if err != nil || len(active) != 0 {
		t.Fatalf("active bindings after terminal settlement = %#v err=%v, want empty", active, err)
	}
}

func TestManagedStreamAppendsTextAtomicallyAndDeduplicatesReplay(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-stream-append"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, create := reserveManagedAgentStart(t, repo, sessionID, "turn-create")
	accepted := transitionManagedAgentOperation(t, repo, create, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-initial", "")
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-message", TaskSessionID: sessionID, TaskID: "task-" + sessionID}); err != nil {
		t.Fatalf("create message turn: %v", err)
	}
	commit := func(eventID, content string) (bool, error) {
		return repo.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
			BindingID: binding.ID, OperationID: accepted.ID, RemoteRunID: accepted.RemoteRunID,
			EventID: eventID, EventType: "assistant", Cursor: eventID,
			DispatchGeneration: accepted.DispatchGeneration, AppendMessage: true,
			Message: &models.Message{
				ID: "assistant-" + accepted.ID, TaskSessionID: sessionID, TaskID: "task-" + sessionID,
				TurnID: "turn-message", AuthorType: models.MessageAuthorAgent,
				Type: models.MessageTypeMessage, Content: content,
			},
		})
	}
	if recorded, err := commit("event-1", "hel"); err != nil || !recorded {
		t.Fatalf("commit first chunk: recorded=%t err=%v", recorded, err)
	}
	if recorded, err := commit("event-1", "hel"); err != nil || recorded {
		t.Fatalf("replay first chunk: recorded=%t err=%v, want deduplicated", recorded, err)
	}
	if recorded, err := commit("event-2", "lo"); err != nil || !recorded {
		t.Fatalf("commit second chunk: recorded=%t err=%v", recorded, err)
	}
	var content string
	if err := repo.db.GetContext(ctx, &content, `SELECT content FROM task_session_messages WHERE id = ?`, "assistant-"+accepted.ID); err != nil {
		t.Fatalf("read accumulated assistant message: %v", err)
	}
	if content != "hello" {
		t.Fatalf("assistant content = %q, want hello", content)
	}
	checkpoint, err := repo.GetManagedAgentStreamCheckpoint(ctx, binding.ID, accepted.RemoteRunID)
	if err != nil || checkpoint.LastEventID != "event-2" || !checkpoint.AssistantMessageStarted {
		t.Fatalf("checkpoint = %+v err=%v, want last event and started assistant message", checkpoint, err)
	}
}

func TestManagedCheckpointTransaction(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-checkpoint-tx"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-checkpoint")
	accepted := transitionManagedAgentOperation(t, repo, operation, binding.Revision, "worker-1",
		models.ManagedAgentSubmissionAccepted, "run-checkpoint", "")

	failed, err := repo.CommitManagedAgentStreamEvent(ctx, models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: accepted.ID, RemoteRunID: accepted.RemoteRunID,
		EventID: "event-rollback", EventType: "assistant_message", Cursor: "cursor-failed",
		DispatchGeneration: accepted.DispatchGeneration,
		Message:            &models.Message{ID: "message-invalid", TaskSessionID: "missing-session", TaskID: "missing-task", AuthorType: models.MessageAuthorAgent},
	})
	if err == nil || failed {
		t.Fatalf("invalid message commit: applied=%t err=%v, want transaction failure", failed, err)
	}
	var eventCount, checkpointCount int
	if err := repo.db.GetContext(ctx, &eventCount, `SELECT COUNT(*) FROM managed_agent_stream_events WHERE binding_id = ?`, binding.ID); err != nil {
		t.Fatalf("count event receipts: %v", err)
	}
	if err := repo.db.GetContext(ctx, &checkpointCount, `SELECT COUNT(*) FROM managed_agent_streams WHERE binding_id = ?`, binding.ID); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if eventCount != 0 || checkpointCount != 0 {
		t.Fatalf("failed transaction left event/checkpoint rows: %d/%d", eventCount, checkpointCount)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-checkpoint", TaskSessionID: sessionID, TaskID: "task-" + sessionID}); err != nil {
		t.Fatalf("create message turn: %v", err)
	}

	event := models.ManagedAgentStreamEvent{
		BindingID: binding.ID, OperationID: accepted.ID, RemoteRunID: accepted.RemoteRunID,
		EventID: "event-1", EventType: "run_completed", Cursor: "cursor-1",
		DispatchGeneration: accepted.DispatchGeneration, TerminalEventType: "completed", HistoryGap: true,
		Message: &models.Message{
			ID: "message-agent-final", TaskSessionID: sessionID, TaskID: "task-" + sessionID, TurnID: "turn-checkpoint",
			AuthorType: models.MessageAuthorAgent, Content: "Done.", Type: models.MessageTypeMessage,
		},
	}
	applied, err := repo.CommitManagedAgentStreamEvent(ctx, event)
	if err != nil || !applied {
		t.Fatalf("commit event: applied=%t err=%v", applied, err)
	}
	replayed, err := repo.CommitManagedAgentStreamEvent(ctx, event)
	if err != nil || replayed {
		t.Fatalf("duplicate event: applied=%t err=%v, want idempotent duplicate", replayed, err)
	}
	secondTerminalType := event
	secondTerminalType.EventType = "assistant_message"
	secondTerminalType.Cursor = "cursor-2"
	applied, err = repo.CommitManagedAgentStreamEvent(ctx, secondTerminalType)
	if err != nil || !applied {
		t.Fatalf("same SSE ID with distinct event type: applied=%t err=%v", applied, err)
	}
	checkpoint, err := repo.GetManagedAgentStreamCheckpoint(ctx, binding.ID, accepted.RemoteRunID)
	if err != nil || checkpoint.Cursor != "cursor-2" || checkpoint.TerminalEventType != "completed" || !checkpoint.HistoryGap {
		t.Fatalf("checkpoint = %+v err=%v", checkpoint, err)
	}
	message, err := repo.GetMessage(ctx, "message-agent-final")
	if err != nil || message.Content != "Done." {
		t.Fatalf("message = %+v err=%v", message, err)
	}
}

func TestManagedStartReplayRequiresSameRequest(t *testing.T) {
	repo, _, _ := newManagedAgentTestRepo(t)
	ctx := context.Background()
	const sessionID = "session-start-replay"
	seedManagedAgentSession(t, repo, "task-"+sessionID, sessionID)
	binding, operation := reserveManagedAgentStart(t, repo, sessionID, "turn-start")
	inputBinding := *binding
	inputBinding.Revision = 1
	inputBinding.DispatchGeneration = 0
	inputBinding.DispatchOwner = ""
	inputBinding.DispatchLeaseUntil = nil
	inputOperation := *operation
	inputOperation.ID = "new-local-id"
	inputOperation.State = ""
	_, _, replayed, err := repo.ReserveManagedAgentStart(ctx, &inputBinding, &inputOperation, "worker-retry", time.Now().Add(time.Minute))
	if err != nil || !replayed {
		t.Fatalf("identical start replay: replayed=%t err=%v", replayed, err)
	}
	inputOperation.RequestDigest = "different-digest"
	if _, _, _, err := repo.ReserveManagedAgentStart(ctx, &inputBinding, &inputOperation, "worker-retry", time.Now().Add(time.Minute)); !errors.Is(err, ErrManagedAgentBindingConflict) {
		t.Fatalf("changed start digest error = %v, want binding conflict", err)
	}
}
