package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageDiscoverySelection(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "discovery-selection", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 200, 1024)
			query, args := agentDeliveryCompactionCandidateQuery(agentDeliveryRetentionProgress{}, 200, repo.nowUTC().Add(-time.Hour))
			if engine == "Postgres" {
				plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, args)
				assertRetentionDiscoveryRelations(t, plan)
			} else {
				assertRetentionSQLiteDiscoveryOnly(t, repo, query, args)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_effects
 SET state='pending',completed_at=NULL WHERE stream_id=? AND sequence=1`), events[0].StreamID); err != nil {
				t.Fatal(err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{MaxPayloadBytes: 2048})
			if err != nil || result.Candidates != 200 || result.EventPayloadsPruned != 2 || result.PayloadBytesRead != 2048 || result.EventRowsRetired != 0 {
				t.Fatalf("locked authority must retain pending prefix and enforce actual byte budget: %+v %v", result, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=1 AND payload_pruned_at IS NULL", events[0].StreamID, 1)
		})
	}
}

func assertRetentionDiscoveryRelations(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation != "" && plan.Relation != "agent_delivery_inbox" {
		t.Fatalf("pre-lock discovery must select identities without repeating locked evidence proofs: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionDiscoveryRelations(t, child)
	}
}

func assertRetentionSQLiteDiscoveryOnly(t *testing.T, repo *Repository, query string, args []any) {
	t.Helper()
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "CORRELATED SCALAR SUBQUERY") || strings.Contains(detail, "SEARCH c ") {
			t.Fatalf("pre-lock discovery must select identities without repeating locked evidence proofs: %s", detail)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
