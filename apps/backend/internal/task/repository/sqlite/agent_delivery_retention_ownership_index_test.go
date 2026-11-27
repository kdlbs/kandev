package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/db"
)

const retentionWideOwnershipIndex = `CREATE INDEX idx_agent_delivery_effects_retirement
	ON agent_delivery_effects(session_id, incarnation_id, effect_key)`

const retentionOwnershipPrefixIndex = `CREATE INDEX idx_agent_delivery_effects_retirement
	ON agent_delivery_effects(session_id, incarnation_id)`
const retentionOwnedOrderedIndex = `CREATE INDEX idx_agent_delivery_effects_owned_retirement
	ON agent_delivery_effects(session_id, effect_key) WHERE session_id <> ''`

func TestDeliveryRetentionStorageOwnershipIndex(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			definition, err := db.IndexDefinitionContext(context.Background(), repo.db, "idx_agent_delivery_effects_retirement")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,incarnation_id)") {
				t.Fatalf("Release N identity prefix must avoid per-event effect key: %s", definition)
			}
			definition, err = db.IndexDefinitionContext(context.Background(), repo.db, "idx_agent_delivery_effects_owned_retirement")
			if err != nil || !strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,effect_key)") || !strings.Contains(definition, "session_id <>") {
				t.Fatalf("owned ordered index must retain unknown incarnations and exclude unowned generic receipts: %s %v", definition, err)
			}
		})
	}
}

func TestDeliveryRetentionStorageOwnershipIndexPostgresCost(t *testing.T) {
	repo, events := seedRetentionBoundsStream(t, "Postgres", "ownership-index-cost", 1, 0)
	seedRetentionStorageRows(t, repo, events[0], 100_000, 1024)
	if _, err := repo.db.Exec(`CREATE INDEX retention_test_wide_identity ON agent_delivery_effects(session_id, incarnation_id, effect_key)`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`CREATE INDEX retention_test_identity_prefix ON agent_delivery_effects(session_id, incarnation_id)`); err != nil {
		t.Fatal(err)
	}
	var wide, prefix int64
	if err := repo.db.QueryRow(`SELECT pg_relation_size('retention_test_wide_identity'), pg_relation_size('retention_test_identity_prefix')`).Scan(&wide, &prefix); err != nil {
		t.Fatal(err)
	}
	t.Logf("100000 actual unowned generic receipts ownership index bytes: full_three_key=%d identity_prefix=%d", wide, prefix)
	if prefix >= wide {
		t.Fatalf("constant ownership prefix did not reduce measured generic receipt index bytes: full=%d prefix=%d", wide, prefix)
	}
	for _, query := range []string{`DROP INDEX retention_test_wide_identity`, `DROP INDEX retention_test_identity_prefix`, `DROP INDEX idx_agent_delivery_effects_retirement`, retentionWideOwnershipIndex} {
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	wideWAL := measureRetentionOwnershipWAL(t, repo, func() {
		seedRetentionStorageRange(t, repo, events[0], 100_001, 110_000, make([]byte, 1024))
	})
	if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_effects_retirement`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(retentionOwnershipPrefixIndex); err != nil {
		t.Fatal(err)
	}
	prefixWAL := measureRetentionOwnershipWAL(t, repo, func() {
		seedRetentionStorageRange(t, repo, events[0], 110_001, 120_000, make([]byte, 1024))
	})
	t.Logf("paired 10000 unowned generic inbox+receipt inserts cluster-scoped WAL bytes (includes other server work, diagnostic): wide=%d prefix=%d", wideWAL, prefixWAL)
}

func measureRetentionOwnershipWAL(t *testing.T, repo *Repository, insert func()) int64 {
	t.Helper()
	var before string
	if err := repo.db.QueryRow(`SELECT pg_current_wal_insert_lsn()::text`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	insert()
	var bytes int64
	if err := repo.db.QueryRow(repo.db.Rebind(`SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(), CAST(? AS pg_lsn))::bigint`), before).Scan(&bytes); err != nil {
		t.Fatal(err)
	}
	return bytes
}

func TestDeliveryRetentionStorageOwnershipIndexUpgrade(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, point := range []string{"", "before_ownership_index_upgrade", "after_ownership_index_drop", "after_ownership_index_create"} {
			t.Run(engine+"/"+point, func(t *testing.T) {
				repo := newRetentionTestRepo(t, engine)
				if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_effects_retirement`); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.db.Exec(retentionWideOwnershipIndex); err != nil {
					t.Fatal(err)
				}
				injected := errors.New("ownership index upgrade interrupted")
				repo.agentDeliveryRetentionFailpoint = func(got string) error {
					if point != "" && point == got {
						return injected
					}
					return nil
				}
				err := repo.initAgentDeliveryRetentionSchema()
				if point != "" {
					if !errors.Is(err, injected) {
						t.Fatalf("upgrade must surface interruption: %v", err)
					}
					definition, err := db.IndexDefinitionContext(t.Context(), repo.db, "idx_agent_delivery_effects_retirement")
					if err != nil || !strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,incarnation_id,effect_key)") {
						t.Fatalf("interrupted upgrade must retain wide index: %s %v", definition, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				repo.agentDeliveryRetentionFailpoint = nil
				if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
					t.Fatal(err)
				}
				// A rollback binary's same-name additive DDL must preserve the prefix.
				if _, err := repo.db.Exec(strings.Replace(retentionWideOwnershipIndex, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1)); err != nil {
					t.Fatal(err)
				}
				definition, err := db.IndexDefinitionContext(t.Context(), repo.db, "idx_agent_delivery_effects_retirement")
				if err != nil || !strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,incarnation_id)") {
					t.Fatalf("upgrade did not preserve identity prefix: %s %v", definition, err)
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageOwnershipIndexSeek(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "owned-index-seek", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 100_000, 1024)
			for _, query := range []string{`DROP INDEX idx_agent_delivery_effects_retirement`, agentDeliveryOwnershipIndexQuery, strings.Replace(retentionOwnedOrderedIndex, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1)} {
				if _, err := repo.db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			for n := 0; n < 30; n++ {
				incarnation := ""
				if n%2 == 0 {
					incarnation = "old-incarnation"
				}
				_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, created_at) VALUES (?, 'owned-stream', ?, 'unknown-effect', 'pending', 'owned-session', ?, ?)`), fmt.Sprintf("owned-%05d", n), n, incarnation, repo.nowUTC())
				if err != nil {
					t.Fatal(err)
				}
			}
			// Distinct unrelated owned identities expose the parameterized prefix
			// access path without disabling sequential scans in the planner.
			_, err := repo.db.Exec(repo.db.Rebind(`WITH RECURSIVE identities(n) AS (
				SELECT 1 UNION ALL SELECT n+1 FROM identities WHERE n < 1000
			) INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, created_at)
			SELECT 'other-owned-' || CAST(n AS TEXT), 'other-owned-stream', n, 'unknown-effect', 'pending',
			'other-owned-session-' || CAST(n AS TEXT), 'other-incarnation-' || CAST(n AS TEXT), ? FROM identities`), repo.nowUTC())
			if err != nil {
				t.Fatal(err)
			}
			query := `SELECT effect_key FROM agent_delivery_effects WHERE session_id = ? AND session_id <> '' AND effect_key > ? ORDER BY effect_key LIMIT 10`
			rows, err := repo.db.Query(repo.db.Rebind(query), "owned-session", "owned-00009")
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for rows.Next() {
				var key string
				if err := rows.Scan(&key); err != nil {
					t.Fatal(err)
				}
				keys = append(keys, key)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil || len(keys) != 10 || keys[0] != "owned-00010" || keys[9] != "owned-00019" {
				t.Fatalf("owned keyset must include unknown incarnation: %v %v", keys, err)
			}
			if engine == "Postgres" {
				for _, query := range []string{`ANALYZE agent_delivery_effects`, `SET plan_cache_mode=force_generic_plan`, `PREPARE retention_current_identity(text,text) AS SELECT 1 FROM agent_delivery_effects WHERE session_id=$1 AND session_id<>'' AND incarnation_id=$2 LIMIT 1`, `PREPARE retention_owned_keys(text,text) AS SELECT effect_key FROM agent_delivery_effects WHERE session_id=$1 AND session_id<>'' AND effect_key>$2 ORDER BY effect_key LIMIT 10`} {
					if _, err := repo.db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
				for _, prepared := range []string{`EXECUTE retention_current_identity('owned-session','old-incarnation')`, `EXECUTE retention_owned_keys('owned-session','owned-00009')`} {
					plan := retentionExplainPostgres(t, repo, "(ANALYZE, FORMAT JSON)", prepared, nil)
					assertRetentionOwnedIndexProbe(t, plan)
					t.Logf("forced generic current owned identity plan=%+v", plan)
				}
			}
		})
	}
}

func assertRetentionOwnedIndexProbe(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_effects" && (plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 10) {
		t.Fatalf("current owned identity probe scanned unrelated receipts: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionOwnedIndexProbe(t, child)
	}
}
