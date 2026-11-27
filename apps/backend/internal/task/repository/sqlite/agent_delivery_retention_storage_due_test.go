package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageDueDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, engine string
		loseAck      bool
	}{
		{"SQLite", "SQLite", false},
		{"Postgres", "Postgres", false},
		{"SQLite lost commit ack", "SQLite", true},
		{"Postgres lost commit ack", "Postgres", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, tc.engine, "due-discovery", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 600, 1024)
			measured := &retentionStorageCommitAckRepository{Repository: repo, loseAck: tc.loseAck}
			now := repo.nowUTC()
			repo.clockNow = func() time.Time { return now }
			for _, group := range []struct {
				first, last int
				at          time.Time
			}{
				{1, 200, now.Add(-time.Hour - time.Minute)},
				{201, 400, now.Add(-time.Hour + time.Minute)},
				{401, 600, now},
			} {
				if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET projected_at=? WHERE stream_id=? AND sequence BETWEEN ? AND ?`), group.at, events[0].StreamID, group.first, group.last); err != nil {
					t.Fatal(err)
				}
			}
			for attempt := 0; attempt < 2; attempt++ {
				runRetentionStorageBatch(t, measured, 200)
			}
			now = now.Add(2 * time.Minute)
			measured.loseAck = tc.loseAck
			before := measureRetentionStorage(t, repo).RawPayloadBytes
			result, _ := runRetentionStorageBatch(t, measured, 200)
			removedBytes := before - measureRetentionStorage(t, repo).RawPayloadBytes
			if removedBytes <= 0 || removedBytes > 200*1024 || result.Candidates > 200 || result.PayloadBytesRead > 4<<20 {
				t.Fatalf("newly due rows behind the sequence sweep must be found in the next bounded batch: result=%+v durable_removed_bytes=%d", result, removedBytes)
			}
		})
	}
}

func TestDeliveryRetentionStorageProjectedFrontier(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "projected-frontier", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 200, 1024)
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_cursors SET projected_sequence=5 WHERE stream_id=?`), events[0].StreamID); err != nil {
				t.Fatal(err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventPayloadsPruned != 5 || result.EventRowsRetired != 5 {
				t.Fatalf("payload pruning must respect contiguous canonical frontier: result=%+v error=%v", result, err)
			}
			var protected int
			if err := repo.db.QueryRow(repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id=? AND sequence>5 AND payload_pruned_at IS NULL AND LENGTH(payload)>0`), events[0].StreamID).Scan(&protected); err != nil {
				t.Fatal(err)
			}
			if protected != 195 {
				t.Fatalf("out-of-order projected markers lost raw recovery evidence: protected=%d", protected)
			}
		})
	}
}

func TestDeliveryRetentionStorageDiscoveryUpgrade(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "discovery-upgrade", 1, 0)
			before := measureRetentionStorage(t, repo)
			for _, column := range []string{"event_sweep_phase", "last_compaction_at", "last_compaction_stream_id", "last_compaction_sequence"} {
				if _, err := repo.db.Exec("ALTER TABLE agent_delivery_retention_state DROP COLUMN " + column); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.CheckAgentDeliveryRetentionReady(context.Background()); err == nil {
				t.Fatal("missing due-time progress must fail readiness")
			}
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			if err := repo.CheckAgentDeliveryRetentionReady(context.Background()); err != nil {
				t.Fatal(err)
			}
			var format int
			var at sql.NullTime
			if err := repo.db.QueryRow(`SELECT format_version,last_compaction_at FROM agent_delivery_retention_state WHERE id=1`).Scan(&format, &at); err != nil {
				t.Fatal(err)
			}
			if format != 1 || at.Valid {
				t.Fatalf("additive format-1 upgrade marker=%d cursor=%+v", format, at)
			}
			after := measureRetentionStorage(t, repo)
			if after.InboxRows != before.InboxRows || after.RawPayloadBytes != before.RawPayloadBytes || after.GenericEffects != before.GenericEffects {
				t.Fatal("additive cursor upgrade changed delivery evidence")
			}
		})
	}
}

func TestDeliveryRetentionStorageDiscoveryPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "SQLite", "discovery-plan", 1, 0)
	seedRetentionStorageRows(t, repo, events[0], 2000, 1024)
	progress := agentDeliveryRetentionProgress{lastCompactionAt: sql.NullTime{Time: events[0].ReceivedAt, Valid: true}, lastCompactionStreamID: events[0].StreamID, lastCompactionSequence: 1000}
	query, args := agentDeliveryCompactionCandidateQuery(progress, 200, repo.nowUTC().Add(-time.Hour))
	assertRetentionDiscoveryIndex(t, repo, query, args, "idx_agent_delivery_inbox_retention_eligible")
	query, args = agentDeliveryReceiptCandidateQuery(events[0].StreamID, 1000, 200)
	assertRetentionDiscoveryIndex(t, repo, query, args, "idx_agent_delivery_inbox_retention_receipts")
}

func TestDeliveryRetentionStoragePostgresDiscoveryPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "Postgres", "pg-discovery-plan", 1, 0)
	prepareRetentionWindowSubmission(t, repo, events[0])
	seedRetentionStorageRows(t, repo, events[0], 72_000, 1024)
	query, args := agentDeliveryCompactionCandidateQuery(agentDeliveryRetentionProgress{}, 200, repo.nowUTC().Add(-time.Hour))
	plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, args)
	t.Logf("Postgres due-time source plan=%+v", plan)
	assertRetentionPostgresProbes(t, plan, true)
	assertRetentionPostgresEffectProbes(t, plan)
	started := time.Now()
	result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
	t.Logf("Postgres first bounded batch elapsed=%s result=%+v error=%v", time.Since(started), result, err)
	if err != nil {
		t.Fatal(err)
	}
}

func assertRetentionPostgresEffectProbes(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_effects" && plan.Loops > 0 && (plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 400) {
		t.Fatalf("bounded candidate proofs must probe effects by event identity: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionPostgresEffectProbes(t, child)
	}
}

func assertRetentionDiscoveryIndex(t *testing.T, repo *Repository, query string, args []any, indexName string) {
	t.Helper()
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, indexName) && strings.HasPrefix(detail, "SEARCH i ") {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("discovery must seek bounded page using %s", indexName)
	}
}
