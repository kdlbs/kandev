package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

// BenchmarkConversationSourceStorage provides disposable 1 KiB source-write
// and restart measurements for the storage replacement. Run it once when
// recording upgrade notes, for example with -benchtime=1x.
func BenchmarkConversationSourceStorage(b *testing.B) {
	for _, messageCount := range []int{10_000, 100_000} {
		b.Run(fmt.Sprintf("messages_%d", messageCount), func(b *testing.B) {
			dbPath := filepath.Join(b.TempDir(), "conversation-source.db")
			dbConn, err := db.OpenSQLite(dbPath)
			if err != nil {
				b.Fatal(err)
			}
			sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
			repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
			if err != nil {
				b.Fatal(err)
			}
			seedConversationSourceBenchmark(b, repo)
			payload := strings.Repeat("x", 1024)
			insert := sqlxDB.Rebind(`
				INSERT INTO task_session_messages
					(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
				VALUES (?, ?, ?, ?, 'agent', '', ?, 0, 'message', '{}', ?)
			`)

			b.ResetTimer()
			for run := 0; run < b.N; run++ {
				for index := 0; index < messageCount; index++ {
					id := fmt.Sprintf("benchmark-message-%d-%d", run, index)
					if _, err := sqlxDB.Exec(insert, id, "conversation-source-benchmark-session", "conversation-source-benchmark-task", "conversation-source-benchmark-turn", payload, time.Now().UTC()); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.StopTimer()

			stat, err := fileSize(dbPath)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(stat), "db-bytes")

			if err := sqlxDB.Close(); err != nil {
				b.Fatal(err)
			}
			start := time.Now()
			startupPath := dbPath
			reopen, err := db.OpenSQLite(startupPath)
			if err != nil {
				b.Fatal(err)
			}
			reopenDB := sqlx.NewDb(reopen, "sqlite3")
			if _, err := NewWithDB(reopenDB, reopenDB, nil); err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(time.Since(start).Nanoseconds()), "startup-ns")
			if err := reopenDB.Close(); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func seedConversationSourceBenchmark(b *testing.B, repo *Repository) {
	b.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('conversation-source-benchmark-workspace', 'benchmark', ?, ?)`), now, now); err != nil {
		b.Fatal(err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES ('conversation-source-benchmark-task', 'conversation-source-benchmark-workspace', 'benchmark', ?, ?)`), now, now); err != nil {
		b.Fatal(err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_sessions (id, task_id, queue_incarnation_id, started_at, updated_at) VALUES ('conversation-source-benchmark-session', 'conversation-source-benchmark-task', 'benchmark-incarnation', ?, ?)`), now, now); err != nil {
		b.Fatal(err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at) VALUES ('conversation-source-benchmark-turn', 'conversation-source-benchmark-session', 'conversation-source-benchmark-task', ?, ?, ?)`), now, now, now); err != nil {
		b.Fatal(err)
	}
}

func fileSize(path string) (int64, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}
