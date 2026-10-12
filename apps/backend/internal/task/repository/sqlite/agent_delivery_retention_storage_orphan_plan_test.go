package sqlite

import "testing"

func TestDeliveryRetentionStorageOrphanProofPlan(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "Postgres", "orphan-proof-plan", 1, 0)
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_sessions(id,task_id,queue_incarnation_id,state,started_at,updated_at)
		SELECT 'unrelated-live-'||CAST(value AS TEXT),task_id,'unrelated-live-'||CAST(value AS TEXT),'CREATED',started_at,updated_at
		FROM task_sessions CROSS JOIN generate_series(1,12000) AS value WHERE id=?`), events[0].SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs(session_id,incarnation_id,retired_at,phase,created_at,updated_at)
		SELECT 'unrelated-retired-'||CAST(value AS TEXT),'unrelated-retired-'||CAST(value AS TEXT),?,'pending',?,?
		FROM generate_series(1,12000) AS value`), repo.nowUTC(), repo.nowUTC(), repo.nowUTC()); err != nil {
		t.Fatal(err)
	}
	plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", agentDeliveryOrphanScanQuery, []any{"", "", "", "", 200})
	assertRetentionPostgresOrphanProbes(t, plan)
}

func assertRetentionPostgresOrphanProbes(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if (plan.Relation == "task_sessions" || plan.Relation == "agent_delivery_cleanup_jobs") && plan.Loops > 0 &&
		(plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 200) {
		t.Fatalf("bounded orphan identities must probe live/job evidence by identity: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionPostgresOrphanProbes(t, child)
	}
}
