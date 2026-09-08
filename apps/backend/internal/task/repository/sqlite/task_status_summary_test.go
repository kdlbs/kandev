package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestTaskStatusSummarySchemaReplayAndCascade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "task-status-summary.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	var tableName string
	if err := db.Get(&tableName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'task_status_summaries'
	`); err != nil {
		t.Fatalf("task status summary table is missing: %v", err)
	}
	if tableName != "task_status_summaries" {
		t.Fatalf("summary table name = %q", tableName)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-summary-schema", "workspace-summary-schema", "Summary schema", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-summary-schema", "workspace-summary-schema", 1, `{"revision":1}`, now); err != nil {
		t.Fatalf("insert summary: %v", err)
	}

	if _, err := db.Exec(db.Rebind(`DELETE FROM tasks WHERE id = ?`), "task-summary-schema"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM task_status_summaries WHERE task_id = ?`), "task-summary-schema"); err != nil {
		t.Fatalf("count cascade rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("summary rows after task deletion = %d, want 0", count)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}
}

func TestTaskStatusSummaryBatchCompareAndUpdate(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	ctx := context.Background()
	seedTaskForStatusSummary(t, db, "task-summary-repository", "workspace-summary-repository")

	first := statussummary.StoredTaskStatusSummary{
		TaskID:      "task-summary-repository",
		WorkspaceID: "workspace-summary-repository",
		Summary: statussummary.TaskStatusSummary{
			Revision:           1,
			ForegroundActivity: "background",
			Git:                &statussummary.GitSummary{ChangedFiles: 3, Additions: 4},
		},
	}
	changed, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &first)
	if err != nil {
		t.Fatalf("insert summary: %v", err)
	}
	if !changed {
		t.Fatal("first summary insert reported no change")
	}

	loaded, err := repo.LoadTaskStatusSummaries(ctx, []string{"task-summary-repository", "missing-summary"})
	if err != nil {
		t.Fatalf("batch load summaries: %v", err)
	}
	got := loaded["task-summary-repository"]
	if got == nil || got.Revision != 1 || got.ForegroundActivity != "background" || got.Git == nil || got.Git.ChangedFiles != 3 {
		t.Fatalf("loaded summary = %#v", got)
	}
	if _, ok := loaded["missing-summary"]; ok {
		t.Fatal("missing summary should be omitted from the batch result")
	}

	noOp := first
	noOp.Summary.UpdatedAt = time.Now().UTC().Add(time.Hour)
	changed, err = repo.CompareAndUpdateTaskStatusSummary(ctx, &noOp)
	if err != nil {
		t.Fatalf("semantic no-op update: %v", err)
	}
	if changed {
		t.Fatal("semantic no-op created a new row revision")
	}

	stale := first
	stale.Summary.Revision = 0
	if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &stale); err == nil {
		t.Fatal("revision zero should be rejected")
	}

	next := first
	next.Summary.Revision = 2
	next.Summary.ForegroundActivity = "generating"
	changed, err = repo.CompareAndUpdateTaskStatusSummary(ctx, &next)
	if err != nil {
		t.Fatalf("newer summary update: %v", err)
	}
	if !changed {
		t.Fatal("newer semantic summary reported no change")
	}
	loaded, err = repo.LoadTaskStatusSummaries(ctx, []string{"task-summary-repository"})
	if err != nil {
		t.Fatalf("reload summary: %v", err)
	}
	if loaded["task-summary-repository"].Revision != 2 || loaded["task-summary-repository"].ForegroundActivity != "generating" {
		t.Fatalf("newer summary was not retained: %#v", loaded["task-summary-repository"])
	}
	if err := repo.DeleteTaskStatusSummary(ctx, "task-summary-repository"); err != nil {
		t.Fatalf("DeleteTaskStatusSummary: %v", err)
	}
	if err := repo.DeleteTaskStatusSummary(ctx, ""); err != nil {
		t.Fatalf("DeleteTaskStatusSummary(empty): %v", err)
	}
	loaded, err = repo.LoadTaskStatusSummaries(ctx, []string{"task-summary-repository"})
	if err != nil || len(loaded) != 0 {
		t.Fatalf("summaries after delete = %+v, %v", loaded, err)
	}
}

func TestTaskLastActivityBatch(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 17, 10, 0, 0, 0, time.UTC)
	seedTaskAt := func(taskID string, createdAt, updatedAt time.Time) {
		t.Helper()
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`), taskID, "workspace-activity", taskID, createdAt, updatedAt); err != nil {
			t.Fatalf("seed task %q: %v", taskID, err)
		}
	}
	seedTaskAt("task-activity-turn", base, base.Add(time.Hour))
	seedTaskAt("task-activity-active", base.Add(2*time.Hour), base.Add(3*time.Hour))
	seedTaskAt("task-activity-no-session", base.Add(4*time.Hour), base.Add(5*time.Hour))
	seedTaskAt("task-activity-queued", base.Add(4*time.Hour), base.Add(5*time.Hour))

	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("initialize queue repository: %v", err)
	}
	queuedAt := base.Add(9 * time.Hour)
	queuedTargetAt := queuedAt.Add(-time.Minute)
	queuedTarget := &messagequeue.QueuedMessage{
		ID: "queued-activity-user-target", SessionID: "session-activity-queued", TaskID: "task-activity-queued",
		Content: "queued target", QueuedAt: queuedTargetAt, QueuedBy: "user-1",
	}
	if err := queueRepo.Insert(ctx, queuedTarget, 10); err != nil {
		t.Fatalf("insert queued user target: %v", err)
	}
	queuedSource := &messagequeue.QueuedMessage{
		ID: "queued-activity-user", SessionID: "session-activity-queued", TaskID: "task-activity-queued",
		Content: "queued prompt", QueuedAt: queuedAt, QueuedBy: "user-1",
	}
	if err := queueRepo.Insert(ctx, queuedSource, 10); err != nil {
		t.Fatalf("insert queued user prompt: %v", err)
	}
	merged, err := queueRepo.MergeIntoAbove(ctx, queuedSource.SessionID, queuedSource.ID, queuedSource.QueuedBy)
	if err != nil {
		t.Fatalf("merge queued user prompts: %v", err)
	}
	if merged == nil || !merged.QueuedAt.Equal(queuedAt) {
		t.Fatalf("merged queued activity = %#v, want %s", merged, queuedAt)
	}
	if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
		ID: "queued-activity-agent", SessionID: "session-activity-queued", TaskID: "task-activity-queued",
		Content: "queued agent prompt", QueuedAt: base.Add(12 * time.Hour), QueuedBy: messagequeue.QueuedByAgent,
	}, 10); err != nil {
		t.Fatalf("insert queued agent prompt: %v", err)
	}

	for _, session := range []*models.TaskSession{
		{ID: "session-activity-turn", TaskID: "task-activity-turn"},
		{ID: "session-activity-active", TaskID: "task-activity-active"},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create session %q: %v", session.ID, err)
		}
	}
	completedAt := base.Add(8 * time.Hour)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-activity-completed",
		TaskSessionID: "session-activity-turn",
		TaskID:        "task-activity-turn",
		StartedAt:     base.Add(6 * time.Hour),
		CompletedAt:   &completedAt,
	}); err != nil {
		t.Fatalf("create completed turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-activity-user",
		TaskSessionID: "session-activity-turn",
		TaskID:        "task-activity-turn",
		TurnID:        "turn-activity-completed",
		AuthorType:    models.MessageAuthorUser,
		Content:       "user prompt",
		CreatedAt:     base.Add(7 * time.Hour),
	}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-activity-agent",
		TaskSessionID: "session-activity-turn",
		TaskID:        "task-activity-turn",
		TurnID:        "turn-activity-completed",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "agent output",
		CreatedAt:     base.Add(12 * time.Hour),
	}); err != nil {
		t.Fatalf("create agent message: %v", err)
	}
	lifecycleCompletedAt := base.Add(14 * time.Hour)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-activity-lifecycle",
		TaskSessionID: "session-activity-turn",
		TaskID:        "task-activity-turn",
		StartedAt:     base.Add(13 * time.Hour),
		CompletedAt:   &lifecycleCompletedAt,
		Metadata:      map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	}); err != nil {
		t.Fatalf("create lifecycle turn: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-activity-active",
		TaskSessionID: "session-activity-active",
		TaskID:        "task-activity-active",
		StartedAt:     base.Add(10 * time.Hour),
	}); err != nil {
		t.Fatalf("create active turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID:            "message-activity-agent-active",
		TaskSessionID: "session-activity-active",
		TaskID:        "task-activity-active",
		TurnID:        "turn-activity-active",
		AuthorType:    models.MessageAuthorAgent,
		Content:       "streamed output",
		CreatedAt:     base.Add(11 * time.Hour),
	}); err != nil {
		t.Fatalf("create active agent message: %v", err)
	}

	got, err := repo.LoadTaskLastActivity(ctx, []string{
		"task-activity-turn",
		"task-activity-active",
		"task-activity-no-session",
		"task-activity-queued",
		"missing-task",
	})
	if err != nil {
		t.Fatalf("load task activity: %v", err)
	}
	want := map[string]time.Time{
		"task-activity-turn":       completedAt,
		"task-activity-active":     base.Add(10 * time.Hour),
		"task-activity-no-session": base.Add(5 * time.Hour),
		"task-activity-queued":     queuedAt,
	}
	if len(got) != len(want) {
		t.Fatalf("activity rows = %#v, want %#v", got, want)
	}
	for taskID, wantAt := range want {
		gotAt, ok := got[taskID]
		if !ok || !gotAt.Equal(wantAt) {
			t.Errorf("activity[%q] = %v, %v; want %v", taskID, gotAt, ok, wantAt)
		}
	}
}

func TestTaskStatusSummaryConcurrentUpdatesRemainMonotonic(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	ctx := context.Background()
	seedTaskForStatusSummary(t, db, "task-summary-concurrent", "workspace-summary-concurrent")

	const updates = 8
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	for revision := 1; revision <= updates; revision++ {
		revision := revision
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
				TaskID:      "task-summary-concurrent",
				WorkspaceID: "workspace-summary-concurrent",
				Summary: statussummary.TaskStatusSummary{
					Revision:           uint64(revision),
					ForegroundActivity: "revision-" + string(rune('a'+revision)),
				},
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}

	loaded, err := repo.LoadTaskStatusSummaries(ctx, []string{"task-summary-concurrent"})
	if err != nil {
		t.Fatalf("load concurrent result: %v", err)
	}
	if got := loaded["task-summary-concurrent"]; got == nil || got.Revision != updates {
		t.Fatalf("concurrent summary = %#v, want revision %d", got, updates)
	}
}

func TestTaskStatusSummaryRejectsMalformedStoredJSON(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	seedTaskForStatusSummary(t, db, "task-summary-malformed", "workspace-summary-malformed")
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-summary-malformed", "workspace-summary-malformed", 1, "{not-json", now); err != nil {
		t.Fatalf("insert malformed summary: %v", err)
	}
	if _, err := repo.LoadTaskStatusSummaries(context.Background(), []string{"task-summary-malformed"}); err == nil {
		t.Fatal("malformed summary should be observable to the caller")
	}
}

func newTaskStatusSummaryTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-status-summary-repository.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo, db
}

func seedTaskForStatusSummary(t *testing.T, db *sqlx.DB, taskID, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, workspaceID, "Status summary task", now, now); err != nil {
		t.Fatalf("seed task %q: %v", taskID, err)
	}
}
