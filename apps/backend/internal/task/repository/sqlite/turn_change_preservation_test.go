package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTurnChangePreservationSourceAndHistoryRetention(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ensureHistoryEligibilitySchema(t, repo)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "history", time.Now().UTC().Add(24*time.Hour))
	if err := repo.StoreTurnChangeFiles(ctx, repositoryChangeID, []models.TurnChangeFileContent{
		turnChangeFileContent("file-history", repositoryChangeID, "src/app.go", []byte("patch"), []byte("old"), []byte("new")),
	}); err != nil {
		t.Fatalf("store file summary: %v", err)
	}
	receipt, err := json.Marshal(map[string]any{
		"source_id": "repository-history", "hash_algorithm": "sha1", "pack_sha256": "digest",
		"start_ref": "refs/kandev/history/a/start", "end_ref": "refs/kandev/history/a/end",
		"start_commit_oid": "abc123", "end_commit_oid": "def456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTurnRepositoryPreservation(ctx, repositoryChangeID,
		models.TurnChangeSourceGitArchive, models.TurnChangePreservationReady, "", 2, 4096, string(receipt), true); err != nil {
		t.Fatalf("record preservation: %v", err)
	}
	complete, err := repo.MarkTurnChangeFilesGitBacked(ctx, repositoryChangeID)
	if err != nil || !complete {
		t.Fatalf("mark Git-backed files: complete=%t err=%v", complete, err)
	}
	source, err := repo.GetTurnFileContentSource(ctx, changeSetID, "file-history")
	if err != nil {
		t.Fatalf("read file source: %v", err)
	}
	if source.Repository.ContentSource != models.TurnChangeSourceGitArchive ||
		source.Repository.PreservationState != models.TurnChangePreservationReady ||
		source.Repository.PreservationReceipt != string(receipt) ||
		source.ChangeSet.ID != changeSetID || string(source.File.PathBytes) != "src/app.go" {
		t.Fatalf("file source = %+v", source)
	}

	cutoff := time.Now().UTC().Add(-time.Hour)
	candidates, err := repo.ListTurnHistoryRetentionCandidates(ctx, cutoff, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("list history candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ChangeSetID != changeSetID || candidates[0].PreservationBytes != 4096 {
		t.Fatalf("history candidates = %+v", candidates)
	}
	result, err := repo.ExpireTurnHistory(ctx, []string{changeSetID}, time.Now().UTC().Add(time.Hour), models.TurnChangeReasonExpiredAge, time.Now().UTC())
	if err != nil || result.ExpiredChangeSets != 1 {
		t.Fatalf("expire history = %+v, %v", result, err)
	}
	changeSet, err := repo.GetTurnChangeSet(ctx, "task-history", "session-history", changeSetID)
	if err != nil || changeSet.Availability != models.TurnChangeAvailabilityExpired || changeSet.ContentBytes != 0 {
		t.Fatalf("expired change set = %+v, %v", changeSet, err)
	}
	source, err = repo.GetTurnFileContentSource(ctx, changeSetID, "file-history")
	if err != nil || source.File.ContentAvailability != models.TurnChangeAvailabilityExpired {
		t.Fatalf("expired file source = %+v, %v", source, err)
	}
	expired, err := repo.ListExpiredTurnHistoryRepositories(ctx, 10)
	if err != nil || len(expired) != 1 || expired[0].PreservationReceipt == "" {
		t.Fatalf("expired archive cleanup intent = %+v, %v", expired, err)
	}
	if err := repo.ClearTurnRepositoryPreservationReceipt(ctx, repositoryChangeID); err != nil {
		t.Fatalf("acknowledge ref cleanup: %v", err)
	}
	expired, err = repo.ListExpiredTurnHistoryRepositories(ctx, 10)
	if err != nil || len(expired) != 0 {
		t.Fatalf("archive cleanup intent after acknowledgement = %+v, %v", expired, err)
	}
	// Summaries and file rows survive expiry.
	if _, _, err := repo.ListTurnFileChanges(ctx, changeSetID, repositoryChangeID, 0, 10); err != nil {
		t.Fatalf("list retained file summaries: %v", err)
	}
}

func TestTurnHistoryRetentionProtectsPendingPreservation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ensureHistoryEligibilitySchema(t, repo)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "pending", time.Now().UTC().Add(24*time.Hour))
	if err := repo.UpdateTurnRepositoryPreservation(ctx, repositoryChangeID,
		models.TurnChangeSourceGitArchive, models.TurnChangePreservationPending,
		models.TurnChangeReasonContentUnavailable, 1, 0, "", false); err != nil {
		t.Fatalf("record pending preservation: %v", err)
	}
	candidates, err := repo.ListTurnHistoryRetentionCandidates(ctx, time.Now().UTC().Add(time.Hour), time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("list history candidates: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.ChangeSetID == changeSetID {
			t.Fatal("pending preservation was eligible for destructive retention")
		}
	}
}

func ensureHistoryEligibilitySchema(t *testing.T, repo *Repository) {
	t.Helper()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS queued_messages(task_id TEXT, session_id TEXT)`,
		`CREATE TABLE IF NOT EXISTS pending_moves(task_id TEXT, session_id TEXT)`,
		`CREATE TABLE IF NOT EXISTS runs(status TEXT, payload TEXT, session_id TEXT, requested_at TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_run_status_requested ON runs(status, requested_at)`,
		`CREATE TABLE IF NOT EXISTS task_session_messages(id TEXT PRIMARY KEY, task_session_id TEXT, task_id TEXT, created_at TIMESTAMP, updated_at TIMESTAMP, type TEXT, metadata TEXT, requests_input INTEGER, author_type TEXT DEFAULT 'agent', turn_id TEXT DEFAULT 'turn')`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session_updated ON task_session_messages(task_session_id, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session_created ON task_session_messages(task_session_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_task_author_created ON task_session_messages(task_id, author_type, type, created_at DESC)`,
	} {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Fatalf("ensure history eligibility schema: %v", err)
		}
	}
}

func TestTurnHistoryRetentionRequiresInactiveTaskAndRevalidatesMutation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, repo *Repository)
	}{
		{name: "recent task activity", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`UPDATE tasks SET updated_at=?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "running session", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`UPDATE task_sessions SET state='RUNNING'`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "queued work", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`INSERT INTO queued_messages(task_id, session_id) VALUES('task-guard','session-guard')`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "pending input", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,created_at,updated_at,type,metadata,requests_input,author_type,turn_id,content)
				VALUES('message-guard','session-guard','task-guard','2020-01-01T00:00:00Z','2020-01-01T00:00:00Z','permission_request','{"status":"pending"}',1,'agent','turn-guard','')`); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ensureHistoryEligibilitySchema(t, repo)
			ctx := context.Background()
			changeSetID, _ := seedTurnChangeContentOwner(t, repo, "guard", time.Now().UTC().Add(24*time.Hour))
			testCase.mutate(t, repo)
			cutoff := time.Now().UTC().Add(-time.Hour)
			candidates, err := repo.ListTurnHistoryRetentionCandidates(ctx, cutoff, time.Now().UTC(), 10)
			if err != nil {
				t.Fatalf("list guarded history: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("protected task was eligible for history expiry: %+v", candidates)
			}
			result, err := repo.ExpireTurnHistory(ctx, []string{changeSetID}, cutoff, models.TurnChangeReasonExpiredAge, time.Now().UTC())
			if err != nil || result.ExpiredChangeSets != 0 {
				t.Fatalf("protected expiry = %+v, %v", result, err)
			}
			changeSet, err := repo.GetTurnChangeSet(ctx, "task-guard", "session-guard", changeSetID)
			if err != nil || changeSet.Availability == models.TurnChangeAvailabilityExpired {
				t.Fatalf("protected change set expired: %+v, %v", changeSet, err)
			}
		})
	}
}

func TestTurnHistoryRetentionRevalidatesEligibilityBetweenInventoryAndMutation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ensureHistoryEligibilitySchema(t, repo)
	ctx := context.Background()
	changeSetID, _ := seedTurnChangeContentOwner(t, repo, "barrier", time.Now().UTC().Add(24*time.Hour))
	cutoff := time.Now().UTC().Add(-time.Hour)
	candidates, err := repo.ListTurnHistoryRetentionCandidates(ctx, cutoff, time.Now().UTC(), 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("initial inventory = %d, %v", len(candidates), err)
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET updated_at=?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := repo.ExpireTurnHistory(ctx, []string{changeSetID}, cutoff, models.TurnChangeReasonExpiredAge, time.Now().UTC())
	if err != nil || result.ExpiredChangeSets != 0 {
		t.Fatalf("barrier expiry = %+v, %v", result, err)
	}
	changeSet, err := repo.GetTurnChangeSet(ctx, "task-barrier", "session-barrier", changeSetID)
	if err != nil || changeSet.Availability == models.TurnChangeAvailabilityExpired {
		t.Fatalf("barrier change set expired: %+v, %v", changeSet, err)
	}
}

func TestDeleteTaskStagesTurnHistoryCleanupIntent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ensureHistoryEligibilitySchema(t, repo)
	ctx := context.Background()
	changeSetID, repositoryChangeID := seedTurnChangeContentOwner(t, repo, "delete-intent", time.Now().UTC().Add(24*time.Hour))
	receipt, _ := json.Marshal(map[string]any{
		"source_id": "repository-delete-intent", "start_ref": "refs/kandev/history/start",
		"end_ref": "refs/kandev/history/end", "start_commit_oid": "abc", "end_commit_oid": "def",
	})
	if err := repo.UpdateTurnRepositoryPreservation(ctx, repositoryChangeID,
		models.TurnChangeSourceGitArchive, models.TurnChangePreservationReady, "", 1, 1, string(receipt), true); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTask(ctx, "task-delete-intent"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if _, err := repo.GetTurnChangeSet(ctx, "task-delete-intent", "session-delete-intent", changeSetID); err == nil {
		t.Fatal("deleted task retained its change set")
	}
	intents, err := repo.ListTurnHistoryCleanupIntents(ctx, 10)
	if err != nil || len(intents) != 1 {
		t.Fatalf("deletion cleanup intents = %+v, %v", intents, err)
	}
	intent := intents[0]
	if intent.SourceID != "repository-delete-intent" ||
		!strings.HasPrefix(intent.StartRef, "refs/kandev/history/") || !strings.HasPrefix(intent.EndRef, "refs/kandev/history/") ||
		intent.StartCommitOID != "abc123" || intent.EndCommitOID != "def456" {
		t.Fatalf("deletion intent = %+v", intent)
	}
	if err := repo.DeleteTurnHistoryCleanupIntent(ctx, intent.ID); err != nil {
		t.Fatalf("acknowledge deletion intent: %v", err)
	}
	intents, err = repo.ListTurnHistoryCleanupIntents(ctx, 10)
	if err != nil || len(intents) != 0 {
		t.Fatalf("intent survived acknowledgement: %+v, %v", intents, err)
	}
}

func TestTurnRepositoryPreservationReadyPaginatesPastOneThousand(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTurnChangeContentOwner(t, repo, "pagination", time.Now().UTC().Add(24*time.Hour))
	changeSetID := "change-set-pagination"
	statement, err := repo.db.PrepareContext(ctx, `INSERT INTO turn_repository_changes(
		id, change_set_id, checkout_id, repository_id, content_source, preservation_state, preservation_receipt,
		start_commit_oid, start_tree_oid, end_commit_oid, end_tree_oid, hash_algorithm, availability, created_at, updated_at)
		VALUES(?, ?, ?, ?, 'git_archive', 'ready', ?, 'start', 'start-tree', 'end', 'end-tree', 'sha1', 'ready', ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statement.Close() }()
	now := time.Now().UTC()
	for index := 0; index < 1001; index++ {
		id := fmt.Sprintf("repo-page-%04d", index)
		if _, err := statement.ExecContext(ctx, id, changeSetID, id, "repository-"+id, `{"source_id":"`+id+`"}`, now, now); err != nil {
			t.Fatalf("insert repository page row: %v", err)
		}
	}
	after := ""
	total := 0
	for page := 0; page < 10; page++ {
		rows, next, err := repo.ListTurnRepositoryPreservationReadyPage(ctx, after, 200)
		if err != nil {
			t.Fatalf("page preservation inventory: %v", err)
		}
		if len(rows) > 200 {
			t.Fatalf("page returned %d rows", len(rows))
		}
		total += len(rows)
		if next == "" {
			break
		}
		after = next
	}
	if total != 1001 {
		t.Fatalf("paginated inventory = %d, want 1001", total)
	}
}
