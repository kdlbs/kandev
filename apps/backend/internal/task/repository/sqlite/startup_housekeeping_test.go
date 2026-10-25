package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.1
func TestPromptSequenceReplaySkipsHistoricalAggregation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	repo.db.SetMaxOpenConns(1)
	seedForMsgTest(t, repo, "history-task", "history-session", "history-turn")
	insertPromptRow(t, repo, "history-message", "history-session", "history-turn", "user", "prior", time.Now().UTC())
	backfillPromptSeqForTest(t, repo)

	setRetentionReceiptAuthorizer(t, repo, func(op int, _, function, _ string) int {
		if op == sqlite3.SQLITE_FUNCTION && strings.EqualFold(function, "max") {
			return sqlite3.SQLITE_DENY
		}
		return sqlite3.SQLITE_OK
	})
	defer setRetentionReceiptAuthorizer(t, repo, nil)
	if _, err := repo.db.Exec(`SELECT MAX(prompt_seq) FROM task_session_messages`); err == nil {
		t.Fatal("positive control: historical aggregation must be denied")
	}
	if err := repo.backfillPromptSeq(t.Context()); err != nil {
		t.Fatalf("completed replay must not prepare a historical aggregation: %v", err)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.1
func TestPromptSequenceOutstandingLookupUsesPartialIndex(t *testing.T) {
	repo := newRepoForSessionTests(t)
	rows, err := repo.db.Query(`EXPLAIN QUERY PLAN SELECT 1 FROM task_session_messages WHERE author_type = 'user' AND prompt_seq = 0 LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "idx_messages_pending_prompt_seq") {
		t.Fatalf("outstanding lookup must use its partial index: %s", plan.String())
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.2
func TestPromptSequenceReplayRepairsOutstandingState(t *testing.T) {
	testPromptSequenceOutstandingState(t, newRepoForSessionTests(t))
}

func TestPostgresPromptSequenceReplayRepairsOutstandingState(t *testing.T) {
	testPromptSequenceOutstandingState(t, openPostgresRepo(t))
}

func testPromptSequenceOutstandingState(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	seedPostgresSession(t, repo, "repair-task", "repair-session", "repair-turn", time.Now().UTC())
	base := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	insertPromptRow(t, repo, "repair-first", "repair-session", "repair-turn", "user", "first", base)
	backfillPromptSeqForTest(t, repo)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_prompt_seq SET last_seq = 50 WHERE task_session_id = ?`), "repair-session"); err != nil {
		t.Fatal(err)
	}
	insertPromptRow(t, repo, "repair-second", "repair-session", "repair-turn", "user", "second", base.Add(time.Second))
	backfillPromptSeqForTest(t, repo)
	assertPromptOrdinals(t, ctx, repo, "repair-session", map[string]int{"repair-first": 1, "repair-second": 2})
	var counter int
	if err := repo.db.Get(&counter, repo.db.Rebind(`SELECT last_seq FROM task_session_prompt_seq WHERE task_session_id = ?`), "repair-session"); err != nil || counter != 50 {
		t.Fatalf("existing counter decreased: %d, %v", counter, err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM task_session_prompt_seq WHERE task_session_id = ?`), "repair-session"); err != nil {
		t.Fatal(err)
	}
	backfillPromptSeqForTest(t, repo)
	if err := repo.db.Get(&counter, repo.db.Rebind(`SELECT last_seq FROM task_session_prompt_seq WHERE task_session_id = ?`), "repair-session"); err != nil || counter != 2 {
		t.Fatalf("missing counter not repaired: %d, %v", counter, err)
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET workspace_id = (SELECT id FROM workspaces ORDER BY id LIMIT 1) WHERE id = 'repair-task'`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithDB(repo.db, repo.ro, nil); err != nil {
		t.Fatalf("schema replay: %v", err)
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-002.2
func TestPromptSequenceRepairAdvancesExistingCounter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedPostgresSession(t, repo, "advance-task", "advance-session", "advance-turn", time.Now().UTC())
	first := time.Now().UTC()
	insertPromptRow(t, repo, "advance-1", "advance-session", "advance-turn", "user", "first", first)
	backfillPromptSeqForTest(t, repo)
	insertPromptRow(t, repo, "advance-2", "advance-session", "advance-turn", "user", "second", first.Add(time.Second))
	backfillPromptSeqForTest(t, repo)
	var counter int
	if err := repo.db.Get(&counter, `SELECT last_seq FROM task_session_prompt_seq WHERE task_session_id='advance-session'`); err != nil || counter != 2 {
		t.Fatalf("repaired ordinals left counter behind: %d %v", counter, err)
	}
}
