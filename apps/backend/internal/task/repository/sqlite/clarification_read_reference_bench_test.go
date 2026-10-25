package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const clarificationReferenceHistoryMessages = 100_000

func BenchmarkClarificationInboxReference(b *testing.B) {
	path := filepath.Join(b.TempDir(), "clarification-reference.db")
	raw, err := internaldb.OpenSQLite(path)
	if err != nil {
		b.Fatalf("open SQLite: %v", err)
	}
	db := sqlx.NewDb(raw, "sqlite3")
	b.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		b.Fatalf("initialize repository: %v", err)
	}
	seedClarificationReferenceHistory(b, repo, clarificationReferenceHistoryMessages)
	listOptions := models.ListClarificationBundlesOptions{
		Unscoped:    true,
		WorkspaceID: "workspace-reference",
		Limit:       50,
		Sidecar:     &models.ClarificationSidecarFilter{UserID: "reference-user", Now: time.Now().UTC()},
	}
	countOptions := models.ListClarificationBundlesOptions{
		Unscoped:    true,
		WorkspaceID: "workspace-reference",
		Limit:       1,
		Sidecar:     &models.ClarificationSidecarFilter{UserID: "reference-user", Only: true, Now: listOptions.Sidecar.Now},
	}
	logClarificationReferencePlans(b, repo, listOptions, countOptions)
	b.Logf("machine go=%s os=%s arch=%s cpu_count=%d fixture_messages=%d clarification_bundles=1 unrelated_workspace_history=true",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), clarificationReferenceHistoryMessages+1)

	b.Run("list", func(b *testing.B) {
		b.ReportAllocs()
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := repo.ListUnresolvedClarificationBundles(ctx, listOptions); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("hidden_count", func(b *testing.B) {
		b.ReportAllocs()
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := repo.CountHiddenClarificationBundles(ctx, countOptions); err != nil {
				b.Fatal(err)
			}
		}
	})

}

func seedClarificationReferenceHistory(tb testing.TB, repo *Repository, historyMessages int) {
	tb.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-reference", WorkspaceID: "workspace-reference", Title: "reference task",
	}); err != nil {
		tb.Fatalf("create reference task: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-unrelated", WorkspaceID: "workspace-unrelated", Title: "unrelated task",
	}); err != nil {
		tb.Fatalf("create unrelated task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-reference", TaskID: "task-reference"}); err != nil {
		tb.Fatalf("create reference session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-unrelated", TaskID: "task-unrelated"}); err != nil {
		tb.Fatalf("create unrelated session: %v", err)
	}
	started := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)
	`),
		"turn-reference", "session-reference", "task-reference", started, started, started,
		"turn-unrelated", "session-unrelated", "task-unrelated", started, started, started,
	); err != nil {
		tb.Fatalf("create reference turns: %v", err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		tb.Fatalf("begin history seed: %v", err)
	}
	stmt, err := tx.PreparexContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, 'user', 'history', 0, 'message', '{}', ?)
	`))
	if err != nil {
		_ = tx.Rollback()
		tb.Fatalf("prepare history insert: %v", err)
	}
	for i := 0; i < historyMessages; i++ {
		sessionID, taskID, turnID := "session-unrelated", "task-unrelated", "turn-unrelated"
		if i%4 == 0 {
			sessionID, taskID, turnID = "session-reference", "task-reference", "turn-reference"
		}
		if _, err := stmt.ExecContext(ctx, fmt.Sprintf("history-%06d", i), sessionID, taskID, turnID, started.Add(time.Duration(i)*time.Millisecond)); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			tb.Fatalf("insert history message %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		tb.Fatalf("close history insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		tb.Fatalf("commit history: %v", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"pending_id":  "pending-reference",
		"question_id": "question-reference",
		"status":      "pending",
	})
	if err != nil {
		tb.Fatalf("marshal clarification metadata: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, 'agent', 'question', 1, 'clarification_request', ?, ?)
	`), "clarification-reference", "session-reference", "task-reference", "turn-reference", string(metadata), started.Add(time.Duration(historyMessages+1)*time.Millisecond)); err != nil {
		tb.Fatalf("insert reference clarification: %v", err)
	}
}

func logClarificationReferencePlans(
	b *testing.B,
	repo *Repository,
	listOptions, countOptions models.ListClarificationBundlesOptions,
) {
	b.Helper()
	listQuery, listArgs := clarificationBundleQueryForOptions(repo.ro.DriverName(), listOptions, listOptions.Limit+1)
	countQuery, countArgs := clarificationBundleQueryForOptions(repo.ro.DriverName(), countOptions, 0)
	for _, operation := range []struct {
		name  string
		query string
		args  []interface{}
	}{
		{name: "list", query: listQuery, args: listArgs},
		{name: "hidden_count", query: countQuery, args: countArgs},
	} {
		rows, err := repo.ro.QueryxContext(context.Background(), "EXPLAIN QUERY PLAN "+operation.query, operation.args...)
		if err != nil {
			b.Fatalf("explain %s query: %v", operation.name, err)
		}
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				_ = rows.Close()
				b.Fatalf("scan %s query plan: %v", operation.name, err)
			}
			details = append(details, detail)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			b.Fatalf("read %s query plan: %v", operation.name, err)
		}
		if err := rows.Close(); err != nil {
			b.Fatalf("close %s query plan: %v", operation.name, err)
		}
		b.Logf("query_plan operation=%s details=%q", operation.name, details)
	}
}

func clarificationBundleQueryForOptions(
	driver string,
	opts models.ListClarificationBundlesOptions,
	limit int,
) (string, []interface{}) {
	join, joinArgs := clarificationSidecarJoin(opts.Sidecar)
	where, whereArgs := clarificationBundleWhereClause(opts)
	args := append(append([]interface{}{}, joinArgs...), whereArgs...)
	if limit > 0 {
		args = append(args, limit)
		return clarificationBundleQuery(driver, join, where), args
	}
	return clarificationBundleCountQuery(driver, join, where), args
}
