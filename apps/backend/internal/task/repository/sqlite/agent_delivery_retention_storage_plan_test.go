package sqlite

import (
	"encoding/json"
	"testing"
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
