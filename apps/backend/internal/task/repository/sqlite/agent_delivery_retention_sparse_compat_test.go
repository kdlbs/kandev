package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"
)

const retentionNIdentityAdmission = `SELECT EXISTS (
	SELECT 1 FROM agent_delivery_submissions WHERE session_id = $1
	UNION ALL SELECT 1 FROM agent_delivery_inbox WHERE session_id = $2
	UNION ALL SELECT 1 FROM agent_delivery_cursors WHERE session_id = $3
	UNION ALL SELECT 1 FROM agent_delivery_effects e LEFT JOIN agent_delivery_cursors c ON c.stream_id = e.stream_id WHERE e.session_id = $4 OR c.session_id = $5
)`

const retentionNTerminalRecovery = `SELECT stream_id, sequence, turn_id, outcome FROM agent_delivery_effects
	WHERE effect_type = $1 AND state IN ($2, $3) AND session_id = $4 AND incarnation_id = $5
	AND harness_generation = $6 AND submission_id = $7 ORDER BY sequence DESC LIMIT 1`

func TestDeliveryRetentionStorageSparseIdentityNReads(t *testing.T) {
	repo := newRetentionTestRepo(t, "Postgres")
	seedRetentionSparseIdentityEffects(t, repo)
	queries := []string{
		`INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at)
		 VALUES ('terminal','terminal-stream',1,'turn.complete','completed','owned-session','old-incarnation',42,'old-submission','old-turn','accepted',CURRENT_TIMESTAMP)`,
		`INSERT INTO agent_delivery_cursors(stream_id,session_id,incarnation_id,harness_generation,updated_at)
		 VALUES ('blank-stream','attributed-session','attributed-incarnation',1,CURRENT_TIMESTAMP)`,
		`ANALYZE agent_delivery_effects`, `SET plan_cache_mode=force_generic_plan`,
		`PREPARE retention_n_identity(text,text,text,text,text) AS ` + retentionNIdentityAdmission,
		`PREPARE retention_n_terminal(text,text,text,text,text,bigint,text) AS ` + retentionNTerminalRecovery,
	}
	for _, query := range queries {
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	var fullAdmissionWork float64
	for _, full := range []bool{true, false} {
		if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_effects_retirement`); err != nil {
			t.Fatal(err)
		}
		index := agentDeliveryOwnershipIndexQuery
		if full {
			index = retentionOwnershipPrefixIndex
		}
		if _, err := repo.db.Exec(index); err != nil {
			t.Fatal(err)
		}
		for _, session := range []string{"owned-session", "attributed-session", "absent-session"} {
			var exists bool
			if err := repo.db.QueryRow(fmt.Sprintf("EXECUTE retention_n_identity('%s','%s','%s','%s','%s')", session, session, session, session, session)).Scan(&exists); err != nil || exists != (session != "absent-session") {
				t.Fatalf("actual N admission scope full=%t session=%s exists=%t error=%v", full, session, exists, err)
			}
		}
		terminal := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", `EXECUTE retention_n_terminal('turn.complete','completed','pending','owned-session','old-incarnation',42,'old-submission')`, nil)
		assertRetentionOwnedIndexProbe(t, terminal)
		admission := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", `EXECUTE retention_n_identity('absent-session','absent-session','absent-session','absent-session','absent-session')`, nil)
		work := retentionEffectPlanWork(admission)
		if full {
			fullAdmissionWork = work
		} else if work > fullAdmissionWork {
			t.Fatalf("sparse index increased actual N OR-admission work: full=%g sparse=%g", fullAdmissionWork, work)
		}
		t.Logf("actual frozen-N-shaped forced-generic reads full=%t terminal=%+v admission_effect_rows=%g (OR admission has no bounded-plan claim)", full, terminal, work)
	}
}

func retentionEffectPlanWork(plan retentionPostgresPlan) float64 {
	work := float64(0)
	if plan.Relation == "agent_delivery_effects" {
		work = (plan.Rows + plan.Removed) * plan.Loops
	}
	for _, child := range plan.Plans {
		work += retentionEffectPlanWork(child)
	}
	return work
}

func TestDeliveryRetentionStorageSparseIdentityWriteCost(t *testing.T) {
	for _, full := range []bool{true, false} {
		t.Run(fmt.Sprint("full=", full), func(t *testing.T) {
			repo := newRetentionTestRepo(t, "Postgres")
			if full {
				for _, query := range []string{`DROP INDEX idx_agent_delivery_effects_retirement`, retentionOwnershipPrefixIndex} {
					if _, err := repo.db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
			}
			seedRetentionSparseIdentityEffects(t, repo)
			var indexBytes int64
			if err := repo.db.QueryRow(`SELECT pg_relation_size('idx_agent_delivery_effects_retirement')`).Scan(&indexBytes); err != nil {
				t.Fatal(err)
			}
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			var raw []byte
			if err := tx.QueryRow(`EXPLAIN (ANALYZE, WAL, FORMAT JSON) INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,created_at)
			 SELECT 'new-blank-'||CAST(n AS TEXT),'new-blank-stream',n,'agent_delivery.event','completed',CURRENT_TIMESTAMP FROM generate_series(1,1000) n`).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plan []struct {
				Plan struct {
					WALBytes int64 `json:"WAL Bytes"`
				} `json:"Plan"`
			}
			if err := json.Unmarshal(raw, &plan); err != nil || len(plan) != 1 || plan[0].Plan.WALBytes <= 0 {
				t.Fatalf("actual generic insert must expose nonzero statement WAL: %s %v", raw, err)
			}
			t.Logf("10000 unowned receipts: full_identity_index=%t index_bytes=%d; actual 1000 generic inserts rollback-only statement_WAL_bytes=%d", full, indexBytes, plan[0].Plan.WALBytes)
		})
	}
}

func seedRetentionSparseIdentityEffects(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,created_at)
	 SELECT 'blank-'||CAST(n AS TEXT),'blank-stream',n,'agent_delivery.event','completed',CURRENT_TIMESTAMP FROM generate_series(1,10000) n`); err != nil {
		t.Fatal(err)
	}
}
