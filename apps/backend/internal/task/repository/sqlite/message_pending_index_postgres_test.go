package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresPendingIDIndexFreshReplayAndPlanner proves the real PostgreSQL
// expression index is created on a fresh schema, can be replayed and restored
// on an existing schema, and is eligible for a pending-ID-only lookup after
// unrelated message history is analyzed.
func TestPostgresPendingIDIndexFreshReplayAndPlanner(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	definition := assertPostgresIndexExists(t, repo, pendingIDLookupIndexName)
	if !strings.Contains(definition, "pending_id") || !strings.Contains(definition, "created_at") || !strings.Contains(definition, "id") {
		t.Fatalf("pending-ID index definition = %q, want pending_id, created_at, and id keys", definition)
	}
	if !strings.Contains(strings.ToUpper(definition), "WHERE") {
		t.Fatalf("pending-ID index definition = %q, want partial-index predicate", definition)
	}
	assertPostgresIndexExists(t, repo, "idx_messages_metadata_pending_id")

	seedPendingActionSession(t, repo, "task-pending-index-pg", "session-pending-index-pg")
	base := time.Date(2026, 9, 5, 22, 0, 0, 0, time.UTC)
	createPendingActionTurn(t, repo, "task-pending-index-pg", "session-pending-index-pg", "turn-pending-index-pg", base, base)
	insertPendingIndexMessage(t, repo, "message-pending-target-pg", "session-pending-index-pg", "task-pending-index-pg", "turn-pending-index-pg", "target-pending-pg", base)
	for i := 0; i < 2000; i++ {
		insertPendingIndexMessage(
			t,
			repo,
			fmt.Sprintf("message-pending-unrelated-pg-%04d", i),
			"session-pending-index-pg",
			"task-pending-index-pg",
			"turn-pending-index-pg",
			fmt.Sprintf("unrelated-pending-pg-%04d", i),
			base.Add(time.Duration(i+1)*time.Microsecond),
		)
	}
	if _, err := repo.db.Exec(`ANALYZE task_session_messages`); err != nil {
		t.Fatalf("analyze messages: %v", err)
	}

	query := "EXPLAIN (COSTS OFF)\n" + findMessagesByPendingIDQuery(repo.db.DriverName())
	rows, err := repo.db.QueryxContext(ctx, repo.db.Rebind(query), "target-pending-pg")
	if err != nil {
		t.Fatalf("explain postgres pending lookup: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan postgres pending lookup plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read postgres pending lookup plan: %v", err)
	}
	if !strings.Contains(strings.Join(plan, "\n"), pendingIDLookupIndexName) {
		t.Fatalf("postgres pending lookup plan = %v, want %s", plan, pendingIDLookupIndexName)
	}

	// Use the production claim statement, including its malformed-bundle guard,
	// so this witness covers the write path rather than a simplified lookup.
	claimQuery := "EXPLAIN (COSTS OFF)\n" + clarificationClaimQuery(repo.db.DriverName())
	claimRows, err := repo.db.QueryxContext(
		ctx,
		repo.db.Rebind(claimQuery),
		base,
		"target-pending-pg",
		"target-pending-pg",
	)
	if err != nil {
		t.Fatalf("explain postgres clarification claim: %v", err)
	}
	var claimPlan []string
	for claimRows.Next() {
		var line string
		if err := claimRows.Scan(&line); err != nil {
			_ = claimRows.Close()
			t.Fatalf("scan postgres clarification claim plan: %v", err)
		}
		claimPlan = append(claimPlan, line)
	}
	if err := claimRows.Err(); err != nil {
		_ = claimRows.Close()
		t.Fatalf("read postgres clarification claim plan: %v", err)
	}
	if err := claimRows.Close(); err != nil {
		t.Fatalf("close postgres clarification claim plan: %v", err)
	}
	claimPlanText := strings.ToUpper(strings.Join(claimPlan, "\n"))
	for _, line := range claimPlan {
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "SEQ SCAN ON TASK_SESSION_MESSAGES") &&
			!strings.Contains(upperLine, "TURN_AUTHORITY_MESSAGE") {
			t.Fatalf("postgres clarification claim plan scans an outer or bundle message path:\n%s", strings.Join(claimPlan, "\n"))
		}
	}
	if !strings.Contains(claimPlanText, "IDX_MESSAGES_METADATA_PENDING_ID_LOOKUP") {
		t.Fatalf("postgres clarification claim plan does not use a pending-ID-leading index:\n%s", strings.Join(claimPlan, "\n"))
	}

	var before int
	if err := repo.db.Get(&before, `SELECT COUNT(*) FROM task_session_messages`); err != nil {
		t.Fatalf("count postgres messages before upgrade replay: %v", err)
	}
	if _, err := repo.db.Exec("DROP INDEX IF EXISTS " + pendingIDLookupIndexName); err != nil {
		t.Fatalf("drop postgres lookup index to simulate legacy database: %v", err)
	}
	if err := repo.ensureMessageMetadataIndexes(); err != nil {
		t.Fatalf("upgrade postgres legacy database: %v", err)
	}
	if err := repo.ensureMessageMetadataIndexes(); err != nil {
		t.Fatalf("replay postgres lookup index creation: %v", err)
	}
	assertPostgresIndexExists(t, repo, pendingIDLookupIndexName)
	var after int
	if err := repo.db.Get(&after, `SELECT COUNT(*) FROM task_session_messages`); err != nil {
		t.Fatalf("count postgres messages after upgrade replay: %v", err)
	}
	if after != before {
		t.Fatalf("postgres message count after index upgrade = %d, want %d", after, before)
	}
}

func assertPostgresIndexExists(t *testing.T, repo *Repository, indexName string) string {
	t.Helper()
	var definition string
	err := repo.db.Get(
		&definition,
		repo.db.Rebind(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`),
		indexName,
	)
	if err != nil {
		t.Fatalf("index %s is missing: %v", indexName, err)
	}
	return definition
}
