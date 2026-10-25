package messagequeue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestSQLiteQueueAdmissionReplaysAfterQueueRowRemoval(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()

	first, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-1", "prompt", "model", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "client-1", first.ID)

	removed, err := repo.DeleteByIDForSession(ctx, identity, first.ID)
	require.NoError(t, err)
	require.Len(t, removed.Removed, 1)

	replayed, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-1", "prompt", "model", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, first.ID, replayed.ID)
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Empty(t, entries)

	_, replay, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-1", "changed", "model", QueuedByUser, false, nil, nil, nil,
	)
	require.ErrorIs(t, err, ErrQueueIDConflict)
	require.False(t, replay)
}

func TestQueueWorkflowAdmissionRejectsStaleExactTaskVersion(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "exact-task", SessionID: "exact-session", SessionIncarnationID: "exact-incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	prepareWorkflowEntryAuthority(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	entry := currentWorkflowEntry(t, repo, identity)
	entry.ExpectedTaskResourceVersion = "2000-01-01T00:00:00Z"

	_, err := service.QueueMessageWithMetadataForSessionAtWorkflowEntry(
		context.Background(), identity, entry, "exact body", "", QueuedByUser, false, nil, nil,
	)
	require.ErrorIs(t, err, ErrWorkflowEntryMismatch)
}

func TestExactQueueAdmissionRecoversAndFencesWorkflowEntry(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "exact-task", SessionID: "exact-session", SessionIncarnationID: "exact-incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	prepareWorkflowEntryAuthority(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	entry := currentWorkflowEntry(t, repo, identity)
	entry.RejectPendingMove = true
	ctx := context.Background()

	first, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, entry, "queue-exact-1", "accepted prompt", "", QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "queue-exact-1", first.ID)

	if _, err := repo.db.Exec(`UPDATE tasks SET updated_at = '2026-09-26 10:30:00' WHERE id = ?`, identity.TaskID); err != nil {
		t.Fatalf("advance task version: %v", err)
	}
	replayed, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, entry, "queue-exact-1", "accepted prompt", "", QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, first.ID, replayed.ID)

	_, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, entry, "queue-exact-stale", "different prompt", "", QueuedByUser, false, nil, nil,
	)
	require.ErrorIs(t, err, ErrWorkflowEntryMismatch)

	current := currentWorkflowEntry(t, repo, identity)
	current.RejectPendingMove = true
	current.ExpectedSessionResourceVersion = "2000-01-01T00:00:00Z"
	_, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, current, "queue-exact-stale-session", "different prompt", "", QueuedByUser, false, nil, nil,
	)
	require.ErrorIs(t, err, ErrWorkflowEntryMismatch)

	current = currentWorkflowEntry(t, repo, identity)
	current.RejectPendingMove = true
	require.NoError(t, repo.SetPendingMove(ctx, identity.SessionID, &PendingMove{
		MoveID: "move-pending", TaskID: identity.TaskID, WorkflowID: current.WorkflowID,
		WorkflowStepID: "destination-step",
	}))
	_, _, err = service.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		ctx, identity, current, "queue-exact-pending-move", "different prompt", "", QueuedByUser, false, nil, nil,
	)
	require.ErrorIs(t, err, ErrWorkflowEntryMismatch)
}

func TestExactQueueMessageCannotDispatchAfterManagerChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		update string
		args   []any
		stale  bool
	}{
		{name: "same manager generation", stale: false},
		{name: "transferred to another plugin", update: `UPDATE task_management_claims SET installation_id = ?, instance_key = ?, generation = ? WHERE task_id = ?`, args: []any{"new-manager", "instance-2", 5, "exact-task"}, stale: true},
		{name: "transferred to a human", update: `UPDATE task_management_claims SET owner_kind = 'human', installation_id = '', instance_key = '', generation = ? WHERE task_id = ?`, args: []any{5, "exact-task"}, stale: true},
		{name: "released after admission", update: `UPDATE task_management_claims SET owner_kind = '', installation_id = '', instance_key = '', generation = ? WHERE task_id = ?`, args: []any{5, "exact-task"}, stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestSQLiteRepo(t).(*sqliteRepository)
			identity := QueueSessionIdentity{TaskID: "exact-task", SessionID: "exact-session", SessionIncarnationID: "exact-incarnation"}
			seedQueueSessionIdentity(t, repo, identity)
			prepareWorkflowEntryAuthority(t, repo, identity)
			_, err := repo.db.Exec(`CREATE TABLE task_management_claims (
				task_id TEXT PRIMARY KEY, owner_kind TEXT NOT NULL, installation_id TEXT NOT NULL,
				instance_key TEXT NOT NULL, generation INTEGER NOT NULL
			)`)
			require.NoError(t, err)
			_, err = repo.db.Exec(`INSERT INTO task_management_claims (task_id, owner_kind, installation_id, instance_key, generation)
				VALUES (?, 'plugin', 'old-manager', 'instance-1', 4)`, identity.TaskID)
			require.NoError(t, err)

			entry := currentWorkflowEntry(t, repo, identity)
			entry.EnforceTaskManagementClaim = true
			entry.ManagementInstallationID = "old-manager"
			entry.ManagementInstanceKey = "instance-1"
			entry.ExpectedClaimGeneration = 4
			queue := NewService(repo, 10, logger.Default())
			queued, replayed, err := queue.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
				context.Background(), identity, entry, "managed-message", "old instruction", "", QueuedByUser, false, nil, nil,
			)
			require.NoError(t, err)
			require.False(t, replayed)
			require.NotNil(t, queued)

			if tc.update != "" {
				_, err := repo.db.Exec(tc.update, tc.args...)
				require.NoError(t, err)
			}
			taken, found, err := queue.TakeQueuedEntryForSession(context.Background(), identity, queued.ID)
			require.NoError(t, err)
			if tc.stale {
				require.False(t, found)
				require.Nil(t, taken)
				entries, err := repo.ListBySession(context.Background(), identity.SessionID)
				require.NoError(t, err)
				require.Empty(t, entries, "stale managed input is removed rather than left at the FIFO head")
				return
			}
			require.True(t, found)
			require.Equal(t, queued.ID, taken.ID)
		})
	}
}

func TestExactQueueAdmissionSupportsVersionFencedWorkflowlessTask(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "workflowless-task", SessionID: "workflowless-session", SessionIncarnationID: "workflowless-incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	prepareWorkflowEntryAuthority(t, repo, identity)
	_, err := repo.db.Exec(`UPDATE tasks SET workflow_id = '', workflow_step_id = '' WHERE id = ?`, identity.TaskID)
	require.NoError(t, err)
	entry := currentWorkflowEntry(t, repo, identity)
	entry.WorkflowID = ""
	entry.WorkflowStepID = ""
	entry.TransitionID = 0
	entry.RejectPendingMove = true

	queued, replayed, err := NewService(repo, 10, logger.Default()).QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
		context.Background(), identity, entry, "workflowless-message", "accepted", "", QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "workflowless-message", queued.ID)
}

func prepareWorkflowEntryAuthority(t *testing.T, repo *sqliteRepository, identity QueueSessionIdentity) {
	t.Helper()
	statements := []string{
		`ALTER TABLE tasks ADD COLUMN workflow_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN workflow_step_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE task_sessions ADD COLUMN updated_at TIMESTAMP NOT NULL DEFAULT '2026-09-26 10:00:00'`,
		`CREATE TABLE task_step_transitions (id INTEGER PRIMARY KEY, task_id TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Fatalf("prepare workflow admission authority: %v", err)
		}
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET workflow_id = 'workflow', workflow_step_id = 'step' WHERE id = ?`, identity.TaskID); err != nil {
		t.Fatalf("set task workflow entry: %v", err)
	}
	if _, err := repo.db.Exec(`INSERT INTO task_step_transitions(id, task_id) VALUES(1, ?)`, identity.TaskID); err != nil {
		t.Fatalf("seed task workflow transition: %v", err)
	}
	repo.taskStepTransitionsPresent = true
}

func currentWorkflowEntry(t *testing.T, repo *sqliteRepository, identity QueueSessionIdentity) WorkflowEntryIdentity {
	t.Helper()
	var taskVersion, sessionVersion time.Time
	if err := repo.db.Get(&taskVersion, `SELECT updated_at FROM tasks WHERE id = ?`, identity.TaskID); err != nil {
		t.Fatalf("read task resource version: %v", err)
	}
	if err := repo.db.Get(&sessionVersion, `SELECT updated_at FROM task_sessions WHERE id = ?`, identity.SessionID); err != nil {
		t.Fatalf("read session resource version: %v", err)
	}
	return WorkflowEntryIdentity{
		WorkflowID: "workflow", WorkflowStepID: "step", TransitionID: 1,
		ExpectedTaskResourceVersion:    taskVersion.UTC().Format(time.RFC3339Nano),
		ExpectedSessionResourceVersion: sessionVersion.UTC().Format(time.RFC3339Nano),
	}
}

// @covers AC-TASKS-QUEUE-ADMISSION-001.2
func TestSQLiteQueueAdmissionReceiptSurvivesRepositoryRestart(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqliteRepo := repo.(*sqliteRepository)
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()
	first, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-restart", "prompt", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	_, err = repo.DeleteByIDForSession(ctx, identity, first.ID)
	require.NoError(t, err)

	restarted, err := NewSQLiteRepository(sqliteRepo.db, sqliteRepo.ro)
	require.NoError(t, err)
	restartedService := NewService(restarted, 10, logger.Default())
	replayed, replay, err := restartedService.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-restart", "prompt", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, first.ID, replayed.ID)
}

// @covers AC-TASKS-QUEUE-ADMISSION-001.3
func TestMemoryQueueAdmissionBindsReceiptToSessionIncarnation(t *testing.T) {
	repo := NewMemoryRepository()
	firstIdentity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "memory:session",
	}
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()
	first, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, firstIdentity, "client-reset", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	_, err = repo.DeleteByIDForSession(ctx, firstIdentity, first.ID)
	require.NoError(t, err)

	secondIdentity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "memory:session-v2",
	}
	memoryRepo := repo.(*memoryRepository)
	memoryRepo.mu.Lock()
	memoryRepo.identities[secondIdentity.SessionID] = secondIdentity
	_, oldReceipt := memoryRepo.admissionReceipts[queueAdmissionKey{
		TaskID: firstIdentity.TaskID, SessionID: firstIdentity.SessionID,
		SessionIncarnationID: firstIdentity.SessionIncarnationID, ClientQueueID: "client-reset",
	}]
	memoryRepo.mu.Unlock()
	require.True(t, oldReceipt, "changing the incarnation must retain the old receipt until task/session deletion")
	second, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, secondIdentity, "client-reset", "second", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "second", second.Content)
}

func TestMemoryQueueAdmissionConflictIsNotReplay(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{
		TaskID: "conflict-task", SessionID: "conflict-session", SessionIncarnationID: "memory:conflict-session",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()
	_, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-conflict", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)

	_, replay, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-conflict", "changed", "", QueuedByUser, false, nil, nil, nil,
	)
	require.ErrorIs(t, err, ErrQueueIDConflict)
	require.False(t, replay)
}

// @covers AC-TASKS-QUEUE-ADMISSION-001.2
func TestQueueAdmissionConcurrentReplayIsAtMostOnce(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqliteRepo := repo.(*sqliteRepository)
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	seedQueueSessionIdentity(t, repo, identity)
	otherRepo, err := NewSQLiteRepository(sqliteRepo.db, sqliteRepo.ro)
	require.NoError(t, err)
	services := []*Service{
		NewService(repo, 10, logger.Default()),
		NewService(otherRepo, 10, logger.Default()),
	}
	ctx := context.Background()
	start := make(chan struct{})
	type result struct {
		message *QueuedMessage
		replay  bool
		err     error
	}
	results := make(chan result, 8)
	var group sync.WaitGroup
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			message, replay, err := services[index%len(services)].QueueMessageWithMetadataForSessionWithClientQueueID(
				ctx, identity, "client-concurrent", "prompt", "", QueuedByUser, false, nil, nil, nil,
			)
			results <- result{message: message, replay: replay, err: err}
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	var first *QueuedMessage
	initialAdmissions := 0
	for item := range results {
		require.NoError(t, item.err)
		require.NotNil(t, item.message)
		if !item.replay {
			initialAdmissions++
			first = item.message
		}
	}
	require.Equal(t, 1, initialAdmissions)
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, first.ID, entries[0].ID)
}

func TestSQLiteQueueAdmissionAutoMergeStoresSurvivingQueueID(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqliteRepo := repo.(*sqliteRepository)
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 1, logger.Default())
	service.SetAutoMergePolicy(true, 1)
	ctx := context.Background()
	first, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-first", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	merged, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-second", "second", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, first.ID, merged.ID)
	require.Equal(t, "first\n\nsecond", merged.Content)
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	oldQueuedAt := time.Now().UTC().Add(-time.Hour)
	_, err = sqliteRepo.db.Exec(
		`UPDATE queued_messages SET queued_at = ? WHERE id = ?`, oldQueuedAt, first.ID,
	)
	require.NoError(t, err)
	entries, err = repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	oldQueuedAt = entries[0].QueuedAt

	merged, replay, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-third", "third", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.True(t, merged.QueuedAt.After(oldQueuedAt))
	entries, err = repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, merged.QueuedAt, entries[0].QueuedAt)

	_, err = repo.DeleteByIDForSession(ctx, identity, first.ID)
	require.NoError(t, err)
	replayed, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-third", "third", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, merged.ID, replayed.ID)
	require.Equal(t, merged.QueuedAt, replayed.QueuedAt)
}

func createTestMessageAttachmentsTable(t *testing.T, sqliteRepo *sqliteRepository) {
	t.Helper()
	_, err := sqliteRepo.db.Exec(`
			CREATE TABLE task_message_attachments (
				id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT '',
			queue_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT '',
			delivery_mode TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL,
			storage_key TEXT NOT NULL,
			state TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL
			)`)
	require.NoError(t, err)
}

func TestQueueAdmissionStagedAttachmentFullQueueRejects(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqliteRepo := repo.(*sqliteRepository)
	createTestMessageAttachmentsTable(t, sqliteRepo)
	identity := QueueSessionIdentity{
		TaskID: "staged-task", SessionID: "staged-session", SessionIncarnationID: "staged-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 1, logger.Default())
	service.SetAutoMergePolicy(true, 1)
	ctx := context.Background()
	first, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-first", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	entriesBefore, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entriesBefore, 1)

	now := time.Now().UTC()
	_, err = sqliteRepo.db.Exec(sqliteRepo.db.Rebind(`
			INSERT INTO task_message_attachments
			(id, owner_id, workspace_id, size_bytes, storage_key, state, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "attachment-full", "owner", "workspace", 5, "storage-full", "staged", now.Add(time.Hour), now, now)
	require.NoError(t, err)
	attachments := []MessageAttachment{{
		Type: "resource", Data: "inline-data", AttachmentID: "attachment-full",
		Name: "full.txt", MimeType: "text/plain", SizeBytes: 5, DeliveryMode: "path",
	}}
	claim := &QueueAttachmentClaim{OwnerID: "owner", WorkspaceID: "workspace", IDs: []string{"attachment-full"}}

	_, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-full", "second", "", QueuedByUser, false, attachments, nil, claim,
	)
	require.ErrorIs(t, err, ErrQueueFull)
	require.False(t, replay)
	entriesAfter, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, entriesBefore, entriesAfter)

	var state, taskID, sessionID string
	require.NoError(t, sqliteRepo.db.QueryRowx(
		`SELECT state, task_id, session_id FROM task_message_attachments WHERE id = ?`,
		"attachment-full",
	).Scan(&state, &taskID, &sessionID))
	require.Equal(t, "staged", state)
	require.Empty(t, taskID)
	require.Empty(t, sessionID)

	var receipts int
	require.NoError(t, sqliteRepo.db.Get(&receipts, sqliteRepo.db.Rebind(`
		SELECT COUNT(*) FROM queue_admission_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ? AND client_queue_id = ?
	`), identity.TaskID, identity.SessionID, identity.SessionIncarnationID, "client-full"))
	require.Zero(t, receipts)
	require.Equal(t, first.ID, entriesAfter[0].ID)
}

func TestMemoryQueueAdmissionStagedAttachmentFullQueueRejects(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{
		TaskID: "staged-task", SessionID: "staged-session", SessionIncarnationID: "staged-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 1, logger.Default())
	service.SetAutoMergePolicy(true, 1)
	ctx := context.Background()
	first, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-first", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	entriesBefore, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)

	claim := &QueueAttachmentClaim{OwnerID: "owner", WorkspaceID: "workspace", IDs: []string{"attachment-full"}}
	_, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-full", "second", "", QueuedByUser, false,
		[]MessageAttachment{{Type: "resource", AttachmentID: "attachment-full"}}, nil, claim,
	)
	require.ErrorIs(t, err, ErrQueueFull)
	require.False(t, replay)
	entriesAfter, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, entriesBefore, entriesAfter)
	require.Equal(t, first.ID, entriesAfter[0].ID)

	memoryRepo := repo.(*memoryRepository)
	memoryRepo.mu.Lock()
	_, stored := memoryRepo.admissionReceipts[queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID, ClientQueueID: "client-full",
	}]
	memoryRepo.mu.Unlock()
	require.False(t, stored)
}

func TestMemoryQueueAdmissionRejectsStagedClaimWithoutAttachmentStore(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{
		TaskID: "staged-memory-task", SessionID: "staged-memory-session", SessionIncarnationID: "memory:staged-memory-session",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()
	attachments := []MessageAttachment{{Type: "resource", AttachmentID: "attachment-memory"}}
	claim := &QueueAttachmentClaim{OwnerID: "owner", WorkspaceID: "workspace", IDs: []string{"attachment-memory"}}

	for range 2 {
		_, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
			ctx, identity, "client-staged-memory", "second", "", QueuedByUser, false, attachments, nil, claim,
		)
		require.ErrorIs(t, err, ErrQueueAdmissionUnavailable)
		require.False(t, replay)
	}
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
	memoryRepo := repo.(*memoryRepository)
	memoryRepo.mu.Lock()
	_, stored := memoryRepo.admissionReceipts[queueAdmissionKey{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID, ClientQueueID: "client-staged-memory",
	}]
	memoryRepo.mu.Unlock()
	require.False(t, stored)
}

func TestMemoryQueueAdmissionAutoMergeAdvancesQueuedAtAndReplayKeepsIt(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{
		TaskID: "memory-task", SessionID: "memory-session", SessionIncarnationID: "memory-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 1, logger.Default())
	service.SetAutoMergePolicy(true, 1)
	ctx := context.Background()
	first, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-first", "first", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)

	oldQueuedAt := time.Now().UTC().Add(-time.Hour)
	memoryRepo := repo.(*memoryRepository)
	memoryRepo.mu.Lock()
	memoryRepo.entries[identity.SessionID][0].QueuedAt = oldQueuedAt
	memoryRepo.mu.Unlock()

	merged, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-second", "second", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, first.ID, merged.ID)
	require.True(t, merged.QueuedAt.After(oldQueuedAt))
	entries, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, merged.QueuedAt, entries[0].QueuedAt)

	_, err = repo.DeleteByIDForSession(ctx, identity, first.ID)
	require.NoError(t, err)
	replayed, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-second", "second", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, merged.QueuedAt, replayed.QueuedAt)
}

func TestQueueAdmissionAttachmentReplay(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	sqliteRepo := repo.(*sqliteRepository)
	createTestMessageAttachmentsTable(t, sqliteRepo)
	identity := QueueSessionIdentity{
		TaskID: "attachment-task", SessionID: "attachment-session", SessionIncarnationID: "attachment-incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	now := time.Now().UTC()
	_, err := sqliteRepo.db.Exec(sqliteRepo.db.Rebind(`
		INSERT INTO task_message_attachments
			(id, owner_id, workspace_id, size_bytes, storage_key, state, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "attachment-replay", "owner", "workspace", 5, "storage-replay", "staged", now.Add(time.Hour), now, now)
	require.NoError(t, err)
	service := NewService(repo, 10, logger.Default())
	attachments := []MessageAttachment{{
		Type: "resource", Data: "inline-data", AttachmentID: "attachment-replay",
		Name: "replay.txt", MimeType: "text/plain", SizeBytes: 5, DeliveryMode: "path",
	}}
	claim := &QueueAttachmentClaim{OwnerID: "owner", WorkspaceID: "workspace", IDs: []string{"attachment-replay"}}
	ctx := context.Background()
	first, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-attachment", "prompt", "", QueuedByUser, false, attachments, nil, claim,
	)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "client-attachment", first.ID)
	var state, taskID, sessionID string
	var updatedAt time.Time
	require.NoError(t, sqliteRepo.db.QueryRowx(
		`SELECT state, task_id, session_id, updated_at FROM task_message_attachments WHERE id = ?`,
		"attachment-replay",
	).Scan(&state, &taskID, &sessionID, &updatedAt))
	require.Equal(t, "claimed", state)
	require.Equal(t, identity.TaskID, taskID)
	require.Equal(t, identity.SessionID, sessionID)

	_, err = repo.DeleteByIDForSession(ctx, identity, first.ID)
	require.NoError(t, err)
	replayed, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-attachment", "prompt", "", QueuedByUser, false, attachments, nil, claim,
	)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, first.ID, replayed.ID)
	require.Empty(t, replayed.Attachments[0].Data)
	var replayUpdatedAt time.Time
	require.NoError(t, sqliteRepo.db.QueryRowx(
		`SELECT updated_at FROM task_message_attachments WHERE id = ?`,
		"attachment-replay",
	).Scan(&replayUpdatedAt))
	require.Equal(t, updatedAt, replayUpdatedAt)
}

func TestQueueAdmissionLifecycleCleanup(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	ctx := context.Background()
	_, _, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-lifecycle", "prompt", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	_, err = repo.DeleteAllBySessionForIdentity(ctx, identity)
	require.NoError(t, err)
	_, replay, err := service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-lifecycle", "prompt", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.True(t, replay)

	_, err = repo.PurgeSession(ctx, identity.SessionID)
	require.NoError(t, err)
	_, replay, err = service.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "client-lifecycle", "new prompt", "", QueuedByUser, false, nil, nil, nil,
	)
	require.NoError(t, err)
	require.False(t, replay)
}
