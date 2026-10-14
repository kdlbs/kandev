package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeliveryRetentionStoragePostgresProofPlan(t *testing.T) {
	repo, event, _ := seedRetentionPromptSubmission(t, "Postgres", "bounded-proof-plan")
	putRetentionWorkflowOutcome(t, repo, event, "not_required", retentionPromptTurnID(t, event))
	if settled, err := repo.CompleteAgentDeliveryTerminalSettlement(t.Context(), terminalRetentionEffectKey(event), time.Time{}); err != nil || !settled {
		t.Fatalf("complete proof fixture: %t %v", settled, err)
	}
	seedRetentionProofHistory(t, repo, event.SubmissionID)
	for _, table := range []string{"agent_delivery_submissions", "agent_delivery_effects", "agent_delivery_inbox"} {
		if _, err := repo.db.Exec("ANALYZE " + table); err != nil {
			t.Fatal(err)
		}
	}
	for _, proof := range []struct{ name, alias, expression string }{
		{"submission", "source", agentDeliveryRetainedSubmissionProofSQL},
		{"terminal", "terminal", agentDeliveryRetainedTerminalProofSQL},
	} {
		t.Run(proof.name, func(t *testing.T) {
			query := `WITH candidates AS MATERIALIZED (
 SELECT i.* FROM agent_delivery_inbox i CROSS JOIN generate_series(1, 200)
 WHERE i.stream_id = ? LIMIT 200
) SELECT CASE WHEN ` + proof.expression + ` THEN 1 ELSE 0 END FROM candidates ` + proof.alias
			plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, []any{event.StreamID})
			assertRetentionBoundedProofPlan(t, plan)
			t.Logf("Postgres 12000-row lifetime proof %s plan=%+v", proof.name, plan)
		})
	}
	t.Run("terminal_frontier", func(t *testing.T) {
		query, args := agentDeliveryRetentionCandidateQuery(event.StreamID, 0, []string{event.StreamID}, 200, 4<<20, repo.nowUTC().Add(-time.Hour), true)
		plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, args)
		assertRetentionBoundedProofPlan(t, plan)
		assertRetentionPostgresProbes(t, plan, true)
		t.Logf("Postgres 12000-row terminal frontier plan=%+v", plan)
	})
}

func seedRetentionProofHistory(t *testing.T, repo *Repository, submissionID string) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO agent_delivery_submissions
 (id, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload, state, outcome, created_at, updated_at)
 SELECT 'historical-proof-prompt:' || n, session_id, incarnation_id, harness_generation, owner_generation,
 payload_hash, payload, state, outcome, created_at, updated_at
 FROM agent_delivery_submissions CROSS JOIN generate_series(1, 12000) n WHERE id = ?`,
		`INSERT INTO agent_delivery_effects
 (effect_key, stream_id, sequence, effect_type, state, created_at, completed_at)
 SELECT 'historical-proof-effect:' || n, 'historical-proof-stream', n,
 'agent_delivery.event', 'completed', created_at, updated_at
 FROM agent_delivery_submissions CROSS JOIN generate_series(1, 12000) n WHERE id = ?`,
		`INSERT INTO agent_delivery_inbox
 (stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at, projected_at)
 SELECT 'historical-proof-stream', n, session_id, incarnation_id, harness_generation,
 id, 'complete', payload, 1, created_at, updated_at
 FROM agent_delivery_submissions CROSS JOIN generate_series(1, 12000) n WHERE id = ?`,
	} {
		if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(query), submissionID); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRetentionBoundedProofPlan(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_submissions" || plan.Relation == "agent_delivery_effects" {
		if plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 400 {
			t.Fatalf("bounded candidate proof must probe identities instead of reading lifetime evidence: %+v", plan)
		}
	}
	for _, child := range plan.Plans {
		assertRetentionBoundedProofPlan(t, child)
	}
}

func TestDeliveryRetentionStorageProofPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "SQLite", "proof-plan", 1, 0)
	prepareRetentionWindowSubmission(t, repo, events[0])
	seedRetentionStorageRows(t, repo, events[0], 200, 1024)
	query, args := agentDeliveryRetentionCandidateQuery("", 0, nil, 200, 4<<20, repo.nowUTC().Add(-time.Hour), true)
	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	counts := make(map[string]int)
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(detail, "CORRELATED SCALAR SUBQUERY ") {
			counts[detail]++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for subquery, occurrences := range counts {
		if occurrences != 1 {
			t.Fatalf("bounded candidate proof must be evaluated once per source expression, rather than copied into payload predicates: %s occurs %d times", subquery, occurrences)
		}
	}
	if len(counts) == 0 {
		t.Fatal("candidate proof query plan was not observed")
	}
}
