package sqlite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

// TestClaimActiveClarificationBundleUsesPendingIDIndex is the regression
// witness for the claim query scanning task_session_messages instead of
// seeking. idx_messages_metadata_pending_id leads with task_session_id, which
// the claim predicate never constrains, so it cannot be used here. The
// message table is seeded past the row count where SQLite's planner would
// otherwise favor a scan, so the assertion reflects the query shape rather
// than an artifact of an empty table.
func TestClaimActiveClarificationBundleUsesPendingIDIndex(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-claim-plan", "session-claim-plan", "turn-claim-plan")
	now := time.Now().UTC()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("msg-claim-plan-%d", i)
		if _, err := repo.db.Exec(repo.db.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'agent', '', 'hello', 0, 'message', '{}', ?, ?)
		`), id, "session-claim-plan", "task-claim-plan", "turn-claim-plan", now, now); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}

	drv := repo.db.DriverName()
	pendingIDExpr := dialect.JSONExtract(drv, "task_session_messages.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "task_session_messages.metadata", "status")
	bundlePendingIDExpr := dialect.JSONExtract(drv, "bundle.metadata", "pending_id")
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	claimQuery := fmt.Sprintf(`
		EXPLAIN QUERY PLAN
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE %s = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s
		  AND turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = task_session_messages.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM task_session_messages bundle
			WHERE %s = ?
			  AND (
				bundle.type != 'clarification_request'
				OR bundle.task_session_id != task_session_messages.task_session_id
				OR bundle.turn_id != task_session_messages.turn_id
			  )
		  )
	`, dialect.JSONSet(drv, "metadata", "status", clarificationStatusResponding), pendingIDExpr, statusExpr,
		nonTerminalSessionPredicate("task_session_messages"),
		predicate, orderBy, bundlePendingIDExpr)

	rows, err := repo.db.Query(repo.db.Rebind(claimQuery), now, "pending-claim-plan", "pending-claim-plan")
	if err != nil {
		t.Fatalf("explain claim query: %v", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("close plan rows: %v", closeErr)
		}
	}()

	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate plan rows: %v", err)
	}

	planText := strings.Join(plan, "\n")
	for _, forbidden := range []string{"SCAN task_session_messages", "SCAN bundle"} {
		if strings.Contains(planText, forbidden) {
			t.Fatalf("claim query plan still scans instead of seeking:\n%s", planText)
		}
	}
	if !strings.Contains(planText, "idx_messages_metadata_pending_id_lookup") {
		t.Fatalf("claim query plan does not use idx_messages_metadata_pending_id_lookup:\n%s", planText)
	}
}
