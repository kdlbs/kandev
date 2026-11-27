package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/db"
)

func TestDeliveryRetentionStorageSparseIdentityIndex(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			assertRetentionSparseIdentityIndex(t, repo)
		})
	}
}

func TestDeliveryRetentionStorageSparseIdentityOrphanPlan(t *testing.T) {
	repo := newRetentionTestRepo(t, "Postgres")
	queries := []string{
		`INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,created_at)
		 SELECT 'blank-'||CAST(n AS TEXT),'blank-stream',n,'agent_delivery.event','completed','','',CURRENT_TIMESTAMP FROM generate_series(1,10000) n`,
		`INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,created_at)
		 SELECT 'owned-'||CAST(n AS TEXT),'owned-stream',n,'unknown-effect','pending','owned-'||CAST(n AS TEXT),
		 CASE WHEN n=1 THEN '' ELSE 'incarnation-'||CAST(n AS TEXT) END,CURRENT_TIMESTAMP FROM generate_series(1,10) n`,
		`DROP INDEX idx_agent_delivery_effects_retirement`,
		`CREATE INDEX idx_agent_delivery_effects_retirement ON agent_delivery_effects(session_id,incarnation_id) WHERE session_id<>''`,
		`ANALYZE agent_delivery_effects`,
		`SET plan_cache_mode=force_generic_plan`,
		`PREPARE retention_sparse_orphans(text,text,text,text,bigint) AS ` + repo.db.Rebind(agentDeliveryOrphanScanQuery),
	}
	for _, query := range queries {
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	plan := retentionExplainPostgres(t, repo, "(ANALYZE, BUFFERS, FORMAT JSON)", `EXECUTE retention_sparse_orphans('','','','',10)`, nil)
	assertRetentionSparseOrphanPlan(t, plan)
	t.Logf("actual forced-generic sparse orphan query plan=%+v", plan)
}

func assertRetentionSparseOrphanPlan(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_effects" && plan.Loops > 0 && (plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 20) {
		t.Fatalf("bounded orphan seeks must not scan unowned receipt history: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionSparseOrphanPlan(t, child)
	}
}

func TestDeliveryRetentionStorageSparseIdentityUpgrade(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, old := range []struct{ name, query string }{{"full_two", retentionOwnershipPrefixIndex}, {"full_three", retentionWideOwnershipIndex}} {
			for _, point := range []string{"", "before_ownership_index_upgrade", "after_ownership_index_drop", "after_ownership_index_create"} {
				t.Run(engine+"/"+old.name+"/"+point, func(t *testing.T) {
					repo := newRetentionTestRepo(t, engine)
					if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_effects_retirement`); err != nil {
						t.Fatal(err)
					}
					if _, err := repo.db.Exec(old.query); err != nil {
						t.Fatal(err)
					}
					injected := errors.New("identity index repair interrupted")
					repo.agentDeliveryRetentionFailpoint = func(got string) error {
						if point != "" && point == got {
							return injected
						}
						return nil
					}
					err := repo.initAgentDeliveryRetentionSchema()
					if point != "" {
						if !errors.Is(err, injected) {
							t.Fatalf("repair must surface interruption: %v", err)
						}
						definition, err := db.IndexDefinitionContext(t.Context(), repo.db, agentDeliveryOwnershipIndexName)
						if err != nil || strings.Contains(definition, "WHERE") {
							t.Fatalf("failed repair must retain full old index atomically: %s %v", definition, err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
					repo.agentDeliveryRetentionFailpoint = nil
					if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
						t.Fatal(err)
					}
					assertRetentionSparseIdentityIndex(t, repo)
					// Frozen N's exact same-name additive index DDL must preserve the repair.
					if _, err := repo.db.Exec(strings.Replace(retentionWideOwnershipIndex, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1)); err != nil {
						t.Fatal(err)
					}
					if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
						t.Fatal(err)
					}
					assertRetentionSparseIdentityIndex(t, repo)
					assertRetentionSparseIdentityIndex(t, reopenRetentionStorageRepository(t, repo))
				})
			}
		}
	}
}

func TestDeliveryRetentionStorageSparseIdentityCancel(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			for _, query := range []string{`DROP INDEX idx_agent_delivery_effects_retirement`, retentionOwnershipPrefixIndex} {
				if _, err := repo.db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo.migrate = db.NewRequiredMigrateLoggerContext(repo.db, nil, ctx)
			repo.agentDeliveryRetentionFailpoint = func(point string) error {
				if point == "after_ownership_index_drop" {
					cancel()
				}
				return nil
			}
			if err := repo.upgradeAgentDeliveryOwnershipIndex(); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled repair must fail closed: %v", err)
			}
			definition, err := db.IndexDefinitionContext(t.Context(), repo.db, agentDeliveryOwnershipIndexName)
			if err != nil || strings.Contains(definition, "WHERE") {
				t.Fatalf("canceled repair must retain old full index: %s %v", definition, err)
			}
			repo.agentDeliveryRetentionFailpoint = nil
			repo.migrate = db.NewRequiredMigrateLogger(repo.db, nil)
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			assertRetentionSparseIdentityIndex(t, reopenRetentionStorageRepository(t, repo))
		})
	}
}

func assertRetentionSparseIdentityIndex(t *testing.T, repo *Repository) {
	t.Helper()
	definition, err := db.IndexDefinitionContext(t.Context(), repo.db, agentDeliveryOwnershipIndexName)
	if err != nil || !strings.Contains(strings.Join(strings.Fields(definition), ""), "(session_id,incarnation_id)") || !strings.Contains(definition, "session_id <>") {
		t.Fatalf("identity seek index must exclude blank owners while retaining unknown incarnations: %s %v", definition, err)
	}
}
