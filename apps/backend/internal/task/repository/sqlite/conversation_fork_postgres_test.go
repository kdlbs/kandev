package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestConversationForkPostgresSnapshotAndDurableDraft(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-fork-pg"
		sessionID = "session-fork-pg"
		turnID    = "turn-fork-pg"
		workspace = "workspace-fork-pg"
	)
	seedPostgresConversationSource(t, repo, taskID, sessionID, turnID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces(id, name, owner_id, created_at, updated_at) VALUES (?, 'Fork PG', 'owner-pg', ?, ?)
	`), workspace, now, now); err != nil {
		t.Fatalf("create postgres workspace: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET workspace_id = ?, title = ? WHERE id = ?`), workspace, "PG fork", taskID); err != nil {
		t.Fatalf("bind postgres source task: %v", err)
	}
	insertPostgresConversationMessage(t, repo, "fork-pg-c", sessionID, taskID, turnID, "user", "third", now, 0)
	insertPostgresConversationMessage(t, repo, "fork-pg-a", sessionID, taskID, turnID, "user", "first", now, 0)
	insertPostgresConversationMessage(t, repo, "fork-pg-b", sessionID, taskID, turnID, "user", "selected cutoff", now, 0)
	source, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{
		SessionID: sessionID, CutoffMessageID: "fork-pg-b",
	})
	if err != nil {
		t.Fatalf("read postgres fork source: %v", err)
	}
	if source.TaskID != taskID || source.WorkspaceID != workspace || source.TaskTitle != "PG fork" || len(source.Messages) != 2 || source.Messages[0].ID != "fork-pg-a" || source.Messages[1].ID != "fork-pg-b" {
		t.Fatalf("postgres fork source = %+v", source)
	}
	if _, err := repo.readConversationForkSourceWithLimits(ctx, models.ConversationForkSourceRequest{SessionID: sessionID, CutoffMessageID: "fork-pg-b"}, 1, 1<<20); !errors.Is(err, models.ErrConversationForkLimitExceeded) {
		t.Fatalf("postgres row bound error = %v, want limit exceeded", err)
	}
	if _, err := repo.readConversationForkSourceWithLimits(ctx, models.ConversationForkSourceRequest{SessionID: sessionID, CutoffMessageID: "fork-pg-b"}, 10, 1); !errors.Is(err, models.ErrConversationForkLimitExceeded) {
		t.Fatalf("postgres byte bound error = %v, want limit exceeded", err)
	}
	secondDB := openSecondPostgresConnection(t, dsn, db)
	readTx, err := repo.beginConversationSourceRead(ctx)
	if err != nil {
		t.Fatalf("begin fork repeatable-read: %v", err)
	}
	snapshot, err := repo.readConversationForkSourceTx(ctx, readTx, models.ConversationForkSourceRequest{SessionID: sessionID, CutoffMessageID: "fork-pg-b"}, 10, 1<<20, func() {
		_, err = secondDB.Exec(secondDB.Rebind(`UPDATE task_session_messages SET content = ? WHERE id = ?`), "mutated after fork snapshot", "fork-pg-a")
	})
	if err != nil {
		_ = readTx.Rollback()
		t.Fatalf("read concurrent fork snapshot: %v", err)
	}
	if err := readTx.Commit(); err != nil {
		t.Fatalf("commit fork repeatable-read: %v", err)
	}
	if err != nil {
		t.Fatalf("mutate source after revision read: %v", err)
	}
	if snapshot.Revision != source.Revision || snapshot.Messages[0].Content != "first" {
		t.Fatalf("mixed postgres fork source = revision %d, first message %q; want revision %d and original text", snapshot.Revision, snapshot.Messages[0].Content, source.Revision)
	}
	current, err := repo.ReadConversationForkSource(ctx, models.ConversationForkSourceRequest{SessionID: sessionID, CutoffMessageID: "fork-pg-b"})
	if err != nil || current.Revision <= snapshot.Revision || current.Messages[0].Content != "mutated after fork snapshot" {
		t.Fatalf("post-snapshot source = revision %d, content %q, err %v", current.Revision, current.Messages[0].Content, err)
	}

	draft := &models.ConversationForkDraft{
		OwnerID: "owner-pg", WorkspaceID: workspace, CompiledText: "PG frozen context",
		DraftRequestID: "fork-pg-request", RequestFingerprint: "pg-fingerprint",
		Descriptor: models.ConversationForkDescriptor{
			SourceTaskID: taskID, SourceSessionID: sessionID, SourceMessageID: "fork-pg-b",
			SourceRevision: source.Revision, CompilerVersion: "conversation-fork-v1", ContentHash: "pg-hash",
			Omissions: map[string]int{}, ExpiresAt: now.Add(24 * time.Hour), State: "draft",
		},
	}
	created, err := repo.CreateConversationForkDraft(ctx, draft)
	if err != nil {
		t.Fatalf("create postgres fork draft: %v", err)
	}
	reloaded, err := repo.GetConversationForkDraft(ctx, "owner-pg", created.Descriptor.ID, now)
	if err != nil || reloaded.CompiledText != "PG frozen context" || reloaded.Descriptor.SourceRevision != source.Revision {
		t.Fatalf("reload postgres fork draft = %+v, %v", reloaded, err)
	}
	retry, err := repo.CreateConversationForkDraft(ctx, draft)
	if err != nil || retry.Descriptor.ID != created.Descriptor.ID {
		t.Fatalf("postgres idempotent draft retry = %q, %v; want %q", retry.Descriptor.ID, err, created.Descriptor.ID)
	}
	destination := &models.Task{ID: "task-fork-pg-destination", WorkspaceID: workspace, Title: "Destination task"}
	if err := repo.CreateTaskWithConversationFork(ctx, destination, models.ConversationForkAdmission{
		OwnerID: "owner-pg", WorkspaceID: workspace, ForkID: created.Descriptor.ID,
		DestinationKind:      "task",
		DestinationRequestID: "fork-pg-destination-request", RequestFingerprint: "destination-pg-fingerprint",
		DestinationTaskID: destination.ID,
	}); err != nil {
		t.Fatalf("create postgres task with attached fork: %v", err)
	}
	attached, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-pg", "fork-pg-destination-request")
	if err != nil || attached.Descriptor.State != "attached" || attached.Descriptor.DestinationTaskID != destination.ID {
		t.Fatalf("postgres attached destination = %+v, %v", attached.Descriptor, err)
	}
	if err := repo.MarkConversationForkTaskDestinationComplete(ctx, "owner-pg", created.Descriptor.ID, destination.ID); err != nil {
		t.Fatalf("complete postgres task destination: %v", err)
	}
	completed, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-pg", "fork-pg-destination-request")
	if err != nil || !completed.Descriptor.DestinationComplete {
		t.Fatalf("postgres completed destination = %+v, %v", completed.Descriptor, err)
	}

	rollbackDraft := *draft
	rollbackDraft.Descriptor.ID = ""
	rollbackDraft.DraftRequestID = "fork-pg-rollback-request"
	rollbackDraft.RequestFingerprint = "pg-rollback-fingerprint"
	rollbackDraft.Descriptor.ContentHash = "pg-rollback-hash"
	rollbackFork, err := repo.CreateConversationForkDraft(ctx, &rollbackDraft)
	if err != nil {
		t.Fatalf("create postgres rollback draft: %v", err)
	}
	rollbackTask := &models.Task{ID: "task-fork-pg-rollback", WorkspaceID: workspace, Title: "Rollback task"}
	if err := repo.CreateTaskWithConversationFork(ctx, rollbackTask, models.ConversationForkAdmission{
		OwnerID: "owner-pg", WorkspaceID: workspace, ForkID: rollbackFork.Descriptor.ID,
		DestinationKind: "task", DestinationRequestID: "fork-pg-rollback-destination",
		RequestFingerprint: "pg-rollback-destination-fingerprint", DestinationTaskID: rollbackTask.ID,
	}); err != nil {
		t.Fatalf("create postgres rollback destination: %v", err)
	}
	if err := repo.RestoreConversationForkTaskDestinationForRollback(ctx, rollbackTask.ID); err != nil {
		t.Fatalf("restore postgres fork after task rollback: %v", err)
	}
	rollbackRestored, err := repo.GetConversationForkDraft(ctx, "owner-pg", rollbackFork.Descriptor.ID, time.Now().UTC())
	if err != nil || rollbackRestored.Descriptor.State != "draft" || rollbackRestored.Descriptor.DestinationComplete {
		t.Fatalf("postgres restored rollback draft = %+v, %v", rollbackRestored.Descriptor, err)
	}
	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("delete postgres source task: %v", err)
	}
	retained, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-pg", "fork-pg-destination-request")
	if err != nil || retained.CompiledText != "PG frozen context" {
		t.Fatalf("postgres fork after source deletion = %+v, %v", retained, err)
	}
	const destinationSessionID = "session-fork-pg-destination"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: destinationSessionID, TaskID: destination.ID, State: models.TaskSessionStateCreated, ConversationForkPendingTask: true,
	}); err != nil {
		t.Fatalf("bind postgres task fork to its first session: %v", err)
	}
	firstSession, err := repo.GetTaskSession(ctx, destinationSessionID)
	if err != nil || firstSession.Metadata[models.MetaKeyConversationForkID] != created.Descriptor.ID {
		t.Fatalf("postgres first-session fork delivery metadata = %+v, %v", firstSession, err)
	}
	bound, err := repo.GetConversationForkByDestinationSession(ctx, "owner-pg", destination.ID, destinationSessionID)
	if err != nil || bound.Descriptor.DestinationSessionID != destinationSessionID {
		t.Fatalf("postgres first session binding = %+v, %v", bound.Descriptor, err)
	}
	for _, create := range []struct {
		name string
		call func(*models.TaskSession) error
	}{
		{name: "ordinary additional session", call: func(session *models.TaskSession) error { return repo.CreateTaskSession(ctx, session) }},
		{name: "workflow-created session", call: func(session *models.TaskSession) error {
			return repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, session, nil)
		}},
	} {
		t.Run(create.name, func(t *testing.T) {
			later := &models.TaskSession{
				ID:     "session-fork-pg-later-" + strings.ReplaceAll(create.name, " ", "-"),
				TaskID: destination.ID, State: models.TaskSessionStateCreated, ConversationForkPendingTask: true,
			}
			if err := create.call(later); err != nil {
				t.Fatalf("create later postgres session: %v", err)
			}
			stored, err := repo.GetTaskSession(ctx, later.ID)
			if err != nil {
				t.Fatalf("get later postgres session: %v", err)
			}
			if _, found := stored.Metadata[models.MetaKeyConversationForkID]; found {
				t.Fatalf("later postgres session inherited consumed fork delivery metadata: %#v", stored.Metadata)
			}
		})
	}
	if err := repo.DeleteTask(ctx, destination.ID); err != nil {
		t.Fatalf("delete postgres destination task: %v", err)
	}
	if _, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-pg", "fork-pg-destination-request"); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("postgres deleted destination fork read = %v, want not found", err)
	}
}
