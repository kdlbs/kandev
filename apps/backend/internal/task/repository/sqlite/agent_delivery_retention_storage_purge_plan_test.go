package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageRetirementCursorCheckpointRollback(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "cursor-checkpoint-rollback", 1, 0)
			seedRetentionPurgePlanEffects(t, repo, "cursor")
			if _, err := repo.db.Exec(`DELETE FROM agent_delivery_effects WHERE effect_key >= 'zz-retired-key-00002'`); err != nil {
				t.Fatal(err)
			}
			for _, stream := range []string{"empty-00", "empty-01"} {
				_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cursors
					(stream_id, session_id, incarnation_id, harness_generation, updated_at)
					VALUES (?, 'zz-retired-session', '', 1, ?)`), stream, repo.nowUTC())
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
				(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
				VALUES ('zz-retired-session', 'old-incarnation', ?, 'effects', ?, ?)`), repo.nowUTC(), repo.nowUTC(), repo.nowUTC())
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("deleted effects and frontier crash")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "retirement_after_phase_progress" {
					return injected
				}
				return nil
			}
			options := models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 3}
			if _, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), options); !errors.Is(err, injected) {
				t.Fatalf("checkpoint rollback: %v", err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key LIKE ?", "zz-retired-key-%", 2)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "phase = 'effects' AND last_stream_id = ?", "", 1)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := repo.RunAgentDeliveryRetirementBatch(ctx, options); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled checkpoint: %v", err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key LIKE ?", "zz-retired-key-%", 2)
			result, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), options)
			if err != nil || result.RowsPurged != 2 || result.Completed != 0 {
				t.Fatalf("atomic page retry: %+v %v", result, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "phase = 'effects' AND last_stream_id = ?", "zz-retired-stream", 1)
			assertRetentionRowCount(t, repo, "agent_delivery_cursors", "session_id = ?", "zz-retired-session", 3)
		})
	}
}

func TestDeliveryRetentionStorageRetirementEmptyCursorProgress(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, _ := seedRetentionBoundsStream(t, engine, "empty-cursor-progress", 1, 0)
			seedRetentionPurgePlanEffects(t, repo, "cursor")
			for n := range 10 {
				_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cursors
					(stream_id, session_id, incarnation_id, harness_generation, updated_at)
					VALUES (?, 'zz-retired-session', 'old-incarnation', 1, ?)`), fmt.Sprintf("empty-%02d", n), repo.nowUTC())
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
				(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
				VALUES ('zz-retired-session', 'old-incarnation', ?, 'effects', ?, ?)`), repo.nowUTC(), repo.nowUTC(), repo.nowUTC())
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("cursor checkpoint crash")
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "retirement_after_phase_progress" {
					return injected
				}
				return nil
			}
			if _, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 3}); !errors.Is(err, injected) {
				t.Fatalf("checkpoint crash: %v", err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "last_stream_id <> ?", "", 0)
			result, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 3})
			if err != nil || result.RowsPurged != 0 || result.Completed != 0 {
				t.Fatalf("empty cursor page: %+v %v", result, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "phase = 'effects' AND last_stream_id = ?", "empty-02", 1)
			assertRetentionRowCount(t, repo, "agent_delivery_cursors", "session_id = ?", "zz-retired-session", 11)
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key LIKE ?", "zz-retired-key-%", 20)
			repo = reopenRetentionStorageRepository(t, repo)
			completed := false
			for range 50 {
				result, err = repo.RunAgentDeliveryRetirementBatch(t.Context(), models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 1})
				if err != nil || result.RowsPurged > 1 {
					t.Fatalf("resume bounded cursor page: %+v %v", result, err)
				}
				if result.Completed == 1 {
					completed = true
					break
				}
			}
			if !completed {
				t.Fatal("reopened cursor pagination did not finish")
			}
			assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key LIKE ?", "zz-retired-key-%", 0)
			assertRetentionRowCount(t, repo, "agent_delivery_cursors", "session_id = ?", "zz-retired-session", 0)
		})
	}
}

type retentionPurgePlan struct {
	NodeType string               `json:"Node Type"`
	Relation string               `json:"Relation Name"`
	Rows     float64              `json:"Actual Rows"`
	Loops    float64              `json:"Actual Loops"`
	Filtered float64              `json:"Rows Removed by Filter"`
	Joined   float64              `json:"Rows Removed by Join Filter"`
	Plans    []retentionPurgePlan `json:"Plans"`
}

// The plan comes from the actual phase statement, observed before its
// transaction rolls back. Unrelated generic receipts must not become purge
// candidates merely because their keys precede the retired session's keys.
func TestDeliveryRetentionStorageRetirementEffectPurgePlan(t *testing.T) {
	for _, ownership := range []string{"owned", "cursor", "cursor_empty_history"} {
		t.Run(ownership, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "Postgres", "effect-purge-"+ownership, 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 100_000, 64)
			if ownership == "cursor_empty_history" {
				seedRetentionPurgeEmptyCursors(t, repo)
			}
			seedRetentionPurgePlanEffects(t, repo, ownership)
			seedRetentionPurgePlanTail(t, repo, ownership)
			for _, table := range []string{"agent_delivery_effects", "agent_delivery_cursors"} {
				if _, err := repo.db.Exec("ANALYZE " + table); err != nil {
					t.Fatal(err)
				}
			}
			// The plan observer needs an independent connection while the writer is held.
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
			if _, err := tx.ExecContext(t.Context(), `SAVEPOINT observe_purge`); err != nil {
				t.Fatal(err)
			}
			job := &agentDeliveryRetirementJob{sessionID: "zz-retired-session", phase: "effects"}
			batch, err := repo.deleteAgentDeliveryRetirementPhaseTx(t.Context(), tx, "effects", job, 10)
			wantDeleted := 10
			if ownership == "cursor_empty_history" {
				wantDeleted = 0
			}
			if err != nil || batch.deleted != wantDeleted || batch.scanned > 10 {
				t.Fatalf("actual phase exceeded its bounded page: %+v wantDeleted=%d error=%v", batch, wantDeleted, err)
			}
			var query string
			if err := repo.ro.QueryRowContext(t.Context(), `SELECT query FROM pg_stat_activity WHERE pid = $1`, backendPID).Scan(&query); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(t.Context(), `ROLLBACK TO SAVEPOINT observe_purge`); err != nil {
				t.Fatal(err)
			}
			plan := explainObservedRetentionPurge(t, tx, query)
			t.Logf("actual %s retirement phase plan: %+v", ownership, plan)
			assertRetentionPurgeCandidateProbes(t, plan)
		})
	}
}

func TestDeliveryRetentionStorageRetirementEffectPurgeScope(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, ownership := range []string{"owned", "cursor"} {
			t.Run(engine+"/"+ownership, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "purge-scope-"+ownership, 1, 0)
				seedRetentionPurgePlanEffects(t, repo, ownership)
				_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cleanup_jobs
					(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
					VALUES ('zz-retired-session', 'old-incarnation', ?, 'effects', ?, ?)`), repo.nowUTC(), repo.nowUTC(), repo.nowUTC())
				if err != nil {
					t.Fatal(err)
				}
				completed := false
				for range 12 {
					result, err := repo.RunAgentDeliveryRetirementBatch(t.Context(), models.AgentDeliveryRetirementBatchOptions{MaxCandidates: 3})
					if err != nil || result.RowsPurged > 3 {
						t.Fatalf("bounded retirement changed scope: %+v %v", result, err)
					}
					if result.Completed == 1 {
						completed = true
						break
					}
				}
				if !completed {
					t.Fatal("retirement did not complete bounded attributed-effect purge")
				}
				assertRetentionRowCount(t, repo, "agent_delivery_effects", "effect_key LIKE ?", "zz-retired-key-%", 0)
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", events[0].StreamID, 1)
				assertRetentionRowCount(t, repo, "agent_delivery_effects", "stream_id = ?", events[0].StreamID, 1)
			})
		}
	}
}

func seedRetentionPurgePlanTail(t *testing.T, repo *Repository, ownership string) {
	t.Helper()
	sessionID, streamID := "zz-retired-session", "unknown-stream"
	if ownership != "owned" {
		sessionID, streamID = "", "zz-retired-stream"
	}
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, created_at)
		SELECT 'zz-retired-tail-' || CAST(value AS TEXT), ?, value+20,
		'unknown-effect', 'pending', ?, 'old-incarnation', ? FROM generate_series(1,100000) AS value`),
		streamID, sessionID, repo.nowUTC())
	if err != nil {
		t.Fatal(err)
	}
}

func seedRetentionPurgePlanEffects(t *testing.T, repo *Repository, ownership string) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, updated_at)
		VALUES ('zz-retired-stream', 'zz-retired-session', 'old-incarnation', 1, ?)`), repo.nowUTC()); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 20; n++ {
		sessionID, streamID, incarnationID := "zz-retired-session", "unknown-stream", ""
		if n%2 == 0 {
			incarnationID = "old-incarnation"
		}
		if ownership != "owned" {
			sessionID, streamID = "", "zz-retired-stream"
			if n%2 == 0 {
				sessionID = "other-explicit-owner"
			}
		}
		_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
			(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, created_at)
			VALUES (?, ?, ?, 'unknown-effect', 'pending', ?, ?, ?)`),
			fmt.Sprintf("zz-retired-key-%05d", n), streamID, n+1, sessionID, incarnationID, repo.nowUTC())
		if err != nil {
			t.Fatal(err)
		}
	}
}

func explainObservedRetentionPurge(t *testing.T, tx *sqlx.Tx, query string) retentionPurgePlan {
	t.Helper()
	parameters := regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(query, -1)
	count := 0
	for _, parameter := range parameters {
		position, err := strconv.Atoi(parameter[1])
		if err != nil {
			t.Fatal(err)
		}
		count = max(count, position)
	}
	if count < 2 || count > 3 {
		t.Fatalf("unexpected observed phase parameters: %s", query)
	}
	args := make([]any, count)
	for n := range count - 1 {
		args[n] = "zz-retired-session"
	}
	args[count-1] = 10
	if count == 3 && strings.Contains(query, "cursor_page") {
		args[1] = ""
	}
	var raw []byte
	if err := tx.QueryRowContext(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var plans []struct {
		Plan retentionPurgePlan `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode actual purge plan: %v", err)
	}
	return plans[0].Plan
}

func assertRetentionPurgeCandidateProbes(t *testing.T, plan retentionPurgePlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_effects" && plan.Loops > 0 {
		examined := (plan.Rows + plan.Filtered + plan.Joined) * plan.Loops
		if plan.NodeType == "Seq Scan" || examined > 400 {
			t.Fatalf("bounded effect purge scanned unrelated lifetime receipts: examined=%g plan=%+v", examined, plan)
		}
	}
	if plan.Relation == "agent_delivery_cursors" && plan.Loops > 0 &&
		(plan.Rows+plan.Filtered)*plan.Loops > 400 {
		t.Fatalf("bounded effect purge scanned empty lifetime cursors: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionPurgeCandidateProbes(t, child)
	}
}

func seedRetentionPurgeEmptyCursors(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, updated_at)
		SELECT 'empty-retired-stream-' || CAST(value AS TEXT), 'zz-retired-session',
		'old-incarnation', 1, ? FROM generate_series(1,1000) AS value`), repo.nowUTC())
	if err != nil {
		t.Fatal(err)
	}
}
