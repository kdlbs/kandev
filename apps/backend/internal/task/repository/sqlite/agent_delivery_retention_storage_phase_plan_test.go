package sqlite

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

const retentionIdentitySubmissionsPhase = "submissions"

func TestDeliveryRetentionStorageRetirementIdentityPhasePlan(t *testing.T) {
	for _, phase := range []string{"inbox", retentionIdentitySubmissionsPhase} {
		t.Run(phase, func(t *testing.T) {
			repo := newRetentionTestRepo(t, "Postgres")
			seedRetentionIdentityPhaseRows(t, repo, phase, true)
			seedRetentionIdentityPhaseRows(t, repo, phase, false)
			table := "agent_delivery_" + phase
			if _, err := repo.db.Exec("ANALYZE " + table); err != nil {
				t.Fatal(err)
			}
			repo.db.SetMaxOpenConns(2)
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })
			var backendPID int
			if err := tx.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&backendPID); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(t.Context(), `SAVEPOINT observe_identity_phase`); err != nil {
				t.Fatal(err)
			}
			job := &agentDeliveryRetirementJob{sessionID: "zz-retired-session", incarnationID: "old-incarnation", phase: phase}
			batch, err := repo.deleteAgentDeliveryRetirementPhaseTx(t.Context(), tx, phase, job, 10)
			if err != nil || batch.deleted != 10 || batch.scanned > 10 {
				t.Fatalf("actual bounded %s phase: %+v %v", phase, batch, err)
			}
			var query string
			if err := repo.ro.QueryRowContext(t.Context(), `SELECT query FROM pg_stat_activity WHERE pid=$1`, backendPID).Scan(&query); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(t.Context(), `ROLLBACK TO SAVEPOINT observe_identity_phase`); err != nil {
				t.Fatal(err)
			}
			plan := explainObservedRetentionPurge(t, tx, query)
			t.Logf("actual %s phase plan: %+v", phase, plan)
			assertRetentionIdentityPhaseProbes(t, table, plan)
		})
	}
}

func seedRetentionIdentityPhaseRows(t *testing.T, repo *Repository, phase string, owned bool) {
	t.Helper()
	prefix, sessionID := "aa-unrelated", "other-session"
	if owned {
		prefix, sessionID = "zz-retired", "zz-retired-session"
	}
	query := `INSERT INTO agent_delivery_inbox
		(stream_id,sequence,session_id,incarnation_id,harness_generation,event_type,payload,received_at)
		SELECT ? || '-' || LPAD(CAST(n AS TEXT),6,'0'),1,?,
		 CASE WHEN n%3=0 THEN '' ELSE 'incarnation-' || CAST(n%3 AS TEXT) END,1,'message_chunk',?,?
		 FROM generate_series(1,100000) AS n`
	if phase == retentionIdentitySubmissionsPhase {
		query = `INSERT INTO agent_delivery_submissions
			(id,session_id,incarnation_id,harness_generation,owner_generation,payload_hash,payload,state,created_at,updated_at)
			SELECT ? || '-' || LPAD(CAST(n AS TEXT),6,'0'),?,
			 CASE WHEN n%3=0 THEN '' ELSE 'incarnation-' || CAST(n%3 AS TEXT) END,1,1,'fixture-hash',?,'prepared',?,?
			 FROM generate_series(1,100000) AS n`
	}
	args := []any{prefix, sessionID, []byte(`{"type":"message_chunk","text":"retained"}`), repo.nowUTC()}
	if phase == retentionIdentitySubmissionsPhase {
		args = append(args, repo.nowUTC())
	}
	if _, err := repo.db.Exec(repo.db.Rebind(query), args...); err != nil {
		t.Fatal(err)
	}
}

func assertRetentionIdentityPhaseProbes(t *testing.T, table string, plan retentionPurgePlan) {
	t.Helper()
	if plan.Relation == table && plan.Loops > 0 {
		examined := (plan.Rows + plan.Filtered + plan.Joined) * plan.Loops
		if plan.NodeType == "Seq Scan" || examined > 400 {
			t.Fatalf("bounded %s phase scanned lifetime identity history: examined=%g plan=%+v", table, examined, plan)
		}
	}
	for _, child := range plan.Plans {
		assertRetentionIdentityPhaseProbes(t, table, child)
	}
}

func TestDeliveryRetentionStorageRetirementIdentityPhaseScope(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, phase := range []string{"inbox", retentionIdentitySubmissionsPhase} {
			t.Run(engine+"/"+phase, func(t *testing.T) {
				repo := newRetentionTestRepo(t, engine)
				seedRetentionIdentityPhaseScope(t, repo, phase)
				_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
					(session_id,incarnation_id,retired_at,phase,created_at,updated_at)
					VALUES ('zz-retired-session','old-incarnation',?,?,?,?)`), repo.nowUTC(), phase, repo.nowUTC(), repo.nowUTC())
				if err != nil {
					t.Fatal(err)
				}
				completed := false
				for range 12 {
					result, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 3})
					if err != nil || result.RowsPurged > 3 {
						t.Fatalf("bounded identity purge: %+v %v", result, err)
					}
					if result.Completed == 1 {
						completed = true
						break
					}
				}
				if !completed {
					t.Fatal("mixed-incarnation identity purge did not finish")
				}
				table := "agent_delivery_" + phase
				assertRetentionRowCount(t, repo, table, "session_id = ?", "zz-retired-session", 0)
				assertRetentionRowCount(t, repo, table, "session_id = ?", "other-session", 10)
			})
		}
	}
}

func seedRetentionIdentityPhaseScope(t *testing.T, repo *Repository, phase string) {
	t.Helper()
	query := `INSERT INTO agent_delivery_inbox
		(stream_id,sequence,session_id,incarnation_id,harness_generation,event_type,payload,received_at)
		VALUES (?,1,?,?,1,'message_chunk',?,?)`
	if phase == retentionIdentitySubmissionsPhase {
		query = `INSERT INTO agent_delivery_submissions
			(id,session_id,incarnation_id,harness_generation,owner_generation,payload_hash,payload,state,created_at,updated_at)
			VALUES (?,?,?,1,1,'fixture-hash',?,'prepared',?,?)`
	}
	for n := range 30 {
		sessionID := "zz-retired-session"
		if n >= 20 {
			sessionID = "other-session"
		}
		incarnation := ""
		if n%3 != 0 {
			incarnation = fmt.Sprintf("incarnation-%d", n%3)
		}
		args := []any{fmt.Sprintf("identity-phase-%02d", n), sessionID, incarnation, []byte(`{"type":"message_chunk","text":"retained"}`), repo.nowUTC()}
		if phase == retentionIdentitySubmissionsPhase {
			args = append(args, repo.nowUTC())
		}
		if _, err := repo.db.Exec(repo.db.Rebind(query), args...); err != nil {
			t.Fatal(err)
		}
	}
}
