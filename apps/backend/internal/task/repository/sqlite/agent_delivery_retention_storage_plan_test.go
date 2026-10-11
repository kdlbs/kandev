package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type retentionPostgresPlan struct {
	NodeType  string                  `json:"Node Type"`
	Relation  string                  `json:"Relation Name"`
	Alias     string                  `json:"Alias"`
	Index     string                  `json:"Index Name"`
	Condition string                  `json:"Index Cond"`
	Rows      float64                 `json:"Actual Rows"`
	Removed   float64                 `json:"Rows Removed by Filter"`
	Loops     float64                 `json:"Actual Loops"`
	Plans     []retentionPostgresPlan `json:"Plans"`
}

func logRetentionPostgresQueryPlans(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent) {
	t.Helper()
	for _, table := range []string{"agent_delivery_inbox", "agent_delivery_effects"} {
		if _, err := repo.db.Exec("ANALYZE " + table); err != nil {
			t.Fatal(err)
		}
	}
	for _, sequence := range []int64{0, 500_000} {
		progress := agentDeliveryRetentionProgress{}
		if sequence > 0 {
			progress.lastCompactionAt = sql.NullTime{Time: event.ReceivedAt, Valid: true}
			progress.lastCompactionStreamID = event.StreamID
			progress.lastCompactionSequence = sequence
		}
		query, args := agentDeliveryCompactionCandidateQuery(progress, 200, repo.nowUTC().Add(-time.Hour))
		plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, args)
		assertRetentionPostgresProbes(t, plan, true)
		assertRetentionPostgresEffectProbes(t, plan)
		t.Logf("Postgres million-row due discovery sequence=%d plan=%+v", sequence, plan)
	}
	query, args := agentDeliveryReceiptCandidateQuery(event.StreamID, 500_000, 200)
	plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", query, args)
	assertRetentionPostgresProbes(t, plan, true)
	assertRetentionPostgresEffectProbes(t, plan)
	t.Logf("Postgres million-row empty-receipt discovery plan=%+v", plan)
	var compact []*agentDeliveryRetentionCandidate
	for sequence := int64(1); sequence <= 200; sequence++ {
		copy := *event
		copy.Sequence = sequence
		compact = append(compact, &agentDeliveryRetentionCandidate{event: copy})
	}
	values, args := agentDeliveryCompactionValues(compact)
	query = strings.Replace(agentDeliveryCompactHashesSQL, "/* values */", values, 1)
	plan = retentionExplainPostgres(t, repo, "(FORMAT JSON)", query, args)
	assertRetentionPostgresProbes(t, plan, false)
	t.Logf("Postgres million-row compaction plan=%+v", plan)
}

func assertRetentionMillionLiveOrphanPhase(t *testing.T, repo *Repository) {
	t.Helper()
	started := time.Now()
	result, err := repo.RunAgentDeliveryOrphanBatch(t.Context(), models.AgentDeliveryOrphanBatchOptions{})
	if err != nil || result.Candidates > 200 || result.JobsScheduled != 0 {
		t.Fatalf("million live-row orphan phase must remain bounded and schedule no deletion: %+v %v", result, err)
	}
	t.Logf("million live-row orphan phase elapsed=%s result=%+v", time.Since(started), result)
}

func retentionExplainPostgres(t *testing.T, repo *Repository, options, query string, args []any) retentionPostgresPlan {
	t.Helper()
	var raw []byte
	if err := repo.db.QueryRow(repo.db.Rebind("EXPLAIN "+options+" "+query), args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan retentionPostgresPlan `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode PostgreSQL plan: %v", err)
	}
	return plans[0].Plan
}

func assertRetentionPostgresProbes(t *testing.T, plan retentionPostgresPlan, bounded bool) {
	t.Helper()
	if plan.Relation == "agent_delivery_inbox" {
		if plan.NodeType == "Seq Scan" || (bounded && plan.Rows*plan.Loops > 400) {
			t.Fatalf("bounded retention needs inbox index probes: %+v", plan)
		}
	}
	for _, child := range plan.Plans {
		assertRetentionPostgresProbes(t, child, bounded)
	}
}
