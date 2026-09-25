package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
)

// Explicit opt-in: this fixture needs several GiB and retains its DB and backup
// in an operator-selected empty directory for subsequent isolated startup checks.
func TestConversationLargeLegacyUpgrade(t *testing.T) {
	root := os.Getenv("KANDEV_LARGE_UPGRADE_DIR")
	if root == "" {
		t.Skip("set KANDEV_LARGE_UPGRADE_DIR to an empty disposable directory")
	}
	path := filepath.Join(root, "data", "kandev.db")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fixture database must not already exist")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	repo, err := NewWithDB(writer, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	seedLargeLegacy(t, repo)
	stat, _ := os.Stat(path)
	t.Logf("legacy_db_bytes=%d", stat.Size())
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{HomeDir: root}
	cfg.Database.Path = path
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "info", Format: "json", OutputPath: filepath.Join(root, "persistence.log")})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	pool, closePool, err := persistence.Provide(cfg, log, "large-upgrade-verification")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closePool() })
	t.Logf("open_and_backup_ms=%.3f", float64(time.Since(started).Microseconds())/1000)
	repo = &Repository{db: pool.Writer(), ro: pool.Reader()}
	started = time.Now()
	if err := repo.cleanupLegacyConversationJournal(); err != nil {
		t.Fatal(err)
	}
	t.Logf("cleanup_ms=%.3f", float64(time.Since(started).Microseconds())/1000)
	started = time.Now()
	repo, err = NewWithDB(pool.Writer(), pool.Reader(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task_repository_startup_ms=%.3f", float64(time.Since(started).Microseconds())/1000)
	var remaining int
	err = pool.Reader().Get(&remaining, `SELECT count(*) FROM sqlite_master WHERE name IN ('conversation_session_events','conversation_message_versions','conversation_turn_versions','conversation_session_streams','conversation_journal_meta') OR (type='trigger' AND (sql LIKE '%INSERT INTO conversation_message_versions%' OR sql LIKE '%INSERT INTO conversation_session_events%' OR sql LIKE '%INSERT INTO conversation_turn_versions%'))`)
	if err != nil || remaining != 0 {
		t.Fatalf("legacy objects remaining=%d error=%v", remaining, err)
	}
	var count int
	if err = pool.Reader().Get(&count, `SELECT count(*) FROM task_session_messages`); err != nil || count != 50000 {
		t.Fatalf("source count=%d error=%v", count, err)
	}
	var pages, free int64
	if err = pool.Reader().Get(&pages, "PRAGMA page_count"); err != nil {
		t.Fatal(err)
	}
	if err = pool.Reader().Get(&free, "PRAGMA freelist_count"); err != nil {
		t.Fatal(err)
	}
	t.Logf("post_cleanup_pages=%d free_pages=%d", pages, free)
	runLargeUpgradeWriterLoad(t, repo, pool)
}

func seedLargeLegacy(t *testing.T, repo *Repository) {
	t.Helper()
	const stamp = "2026-09-16T12:00:00Z"
	for _, query := range []string{
		`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('large-workspace','Large fixture',?,?)`,
		`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('large-task','large-workspace','large legacy',?,?)`,
		`INSERT INTO task_sessions(id,task_id,queue_incarnation_id,started_at,updated_at) VALUES('large-session','large-task','large-incarnation',?,?)`,
	} {
		if _, err := repo.db.Exec(query, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,created_at,updated_at) VALUES('large-turn','large-session','large-task',?,?,?)`, stamp, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	var triggers []string
	if err := repo.db.Select(&triggers, `SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE 'conversation_source_%'`); err != nil {
		t.Fatal(err)
	}
	for _, name := range triggers {
		if _, err := repo.db.Exec(`DROP TRIGGER "` + name + `"`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`DROP TABLE conversation_session_revisions`); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/conversation-journal-88c6c0fe.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("synthetic-data-", 585)
	for batch := 0; batch < 100; batch++ {
		tx, err := repo.db.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 500; index++ {
			id := fmt.Sprintf("large-%d", batch*500+index)
			if _, err = tx.Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,author_id,content,requests_input,type,metadata,created_at) VALUES(?,'large-session','large-task','large-turn','agent','',?,0,'message','{}',?)`, id, payload, stamp); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if _, err = tx.Exec(`UPDATE task_session_messages SET content=content || 'final' WHERE id=?`, id); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if batch%20 == 0 {
			t.Logf("seeded_messages=%d", (batch+1)*500)
		}
	}
	if _, err = repo.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
}

func runLargeUpgradeWriterLoad(t *testing.T, repo *Repository, pool *db.Pool) {
	t.Helper()
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{ID: "task", OwnerPackage: "internal/task", RequiredTables: []string{"tasks", "task_sessions", "task_session_messages", "task_session_turns", "conversation_session_revisions"}, Sweep: startup.StepStoresRepositories}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tracker.RecordSuccess("task"); err != nil {
		t.Fatal(err)
	}
	if err = tracker.Complete(); err != nil {
		t.Fatal(err)
	}
	health := requiredstores.NewHealth(tracker, pool, nil)
	if err = health.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var updates, failures atomic.Int64
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			msg, readErr := repo.GetMessage(ctx, fmt.Sprintf("large-%d", id))
			if readErr != nil {
				failures.Add(1)
				return
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					msg.Content = strings.Repeat("stream-", 1170) + fmt.Sprint(updates.Load())
					msg.Type = models.MessageTypeMessage
					if _, writeErr := repo.UpdateMessageWithConversationReceipt(ctx, msg); writeErr != nil {
						if ctx.Err() == nil {
							failures.Add(1)
						}
					} else {
						updates.Add(1)
					}
				}
			}
		}(worker)
	}
	probes, probeFailures := 0, 0
	var worst time.Duration
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			probeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			start := time.Now()
			probeErr := health.Check(probeCtx)
			elapsed := time.Since(start)
			stop()
			if ctx.Err() != nil {
				break
			}
			probes++
			if elapsed > worst {
				worst = elapsed
			}
			if probeErr != nil {
				probeFailures++
				t.Logf("probe_failure=%v", probeErr)
			}
		}
	}
	workers.Wait()
	t.Logf("stream_seconds=60 workers=8 target_updates_per_second=800 updates=%d write_failures=%d probes=%d probe_failures=%d worst_probe_ms=%.3f", updates.Load(), failures.Load(), probes, probeFailures, float64(worst.Microseconds())/1000)
	if failures.Load() != 0 || probeFailures != 0 {
		t.Fatal("post-readiness streaming load failed")
	}
}
