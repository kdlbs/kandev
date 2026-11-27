package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/db"
)

const retentionFullUnprojectedIndex = `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_unprojected
	ON agent_delivery_inbox(stream_id, projected_at, sequence)`

// @covers AC-PLATFORM-DELIVERY-RETENTION-006.1
func TestDeliveryRetentionStorageUnprojectedIndex(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "pending-index", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 10_000, 1024)
			event := *events[0]
			event.Sequence = 10_001
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, event.Sequence); err != nil || !inserted {
				t.Fatalf("pending admission: %t %v", inserted, err)
			}
			pending, err := repo.ListUnprojectedAgentDeliveryEvents(t.Context(), event.StreamID, 10)
			if err != nil || len(pending) != 1 || pending[0].Sequence != event.Sequence {
				t.Fatalf("pending replay changed: %v %v", pending, err)
			}
			assertRetentionUnprojectedIndex(t, repo, true)
			// Frozen N's same-name additive DDL must preserve the new membership.
			if _, err := repo.db.Exec(retentionFullUnprojectedIndex); err != nil {
				t.Fatal(err)
			}
			assertRetentionUnprojectedIndex(t, repo, true)
			if engine == "Postgres" {
				assertRetentionGenericPendingPlan(t, repo, event.StreamID)
			}
		})
	}
}

func TestDeliveryRetentionStorageUnprojectedIndexUpgrade(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, point := range []string{"", "before_unprojected_index_upgrade", "after_unprojected_index_drop", "after_unprojected_index_create", "cancel"} {
			t.Run(engine+"/"+point, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "pending-index-upgrade", 1, 0)
				event := *events[0]
				event.Sequence = 2
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
					t.Fatalf("pending upgrade fixture: %t %v", inserted, err)
				}
				canonical := retentionCanonicalContent(t, repo, event.SessionID)
				if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_inbox_unprojected`); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.db.Exec(retentionFullUnprojectedIndex); err != nil {
					t.Fatal(err)
				}
				injected := errors.New("unprojected index upgrade interrupted")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				repo.migrate = db.NewRequiredMigrateLoggerContext(repo.db, nil, ctx)
				repo.agentDeliveryRetentionFailpoint = func(got string) error {
					if point == "cancel" && got == "after_unprojected_index_drop" {
						cancel()
						return ctx.Err()
					}
					if point != "" && point == got {
						return injected
					}
					return nil
				}
				err := repo.initAgentDeliveryRetentionSchema()
				if point != "" {
					want := injected
					if point == "cancel" {
						want = context.Canceled
					}
					if !errors.Is(err, want) {
						t.Fatalf("upgrade did not surface interruption: %v", err)
					}
					assertRetentionUnprojectedIndex(t, repo, false)
				} else if err != nil {
					t.Fatal(err)
				}
				repo.migrate = db.NewRequiredMigrateLogger(repo.db, nil)
				repo.agentDeliveryRetentionFailpoint = nil
				reopened := reopenRetentionStorageRepository(t, repo)
				assertRetentionUnprojectedIndex(t, reopened, true)
				pending, err := reopened.ListUnprojectedAgentDeliveryEvents(t.Context(), event.StreamID, 10)
				if err != nil || len(pending) != 1 || pending[0].Sequence != 2 || retentionCanonicalContent(t, reopened, event.SessionID) != canonical {
					t.Fatalf("upgrade/reopen changed pending replay or canonical history: %v %v", pending, err)
				}
				assertRetentionRowCount(t, reopened, "agent_delivery_inbox", "stream_id = ?", event.StreamID, 2)
				if err := reopened.CheckAgentDeliveryRetentionReady(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func assertRetentionUnprojectedIndex(t *testing.T, repo *Repository, partial bool) {
	t.Helper()
	definition, err := db.IndexDefinitionContext(t.Context(), repo.db, "idx_agent_delivery_inbox_unprojected")
	if err != nil || definition == "" || strings.Contains(strings.ToLower(definition), "projected_at is null") != partial {
		t.Fatalf("pending replay index membership partial=%t: %s %v", partial, definition, err)
	}
}

func assertRetentionGenericPendingPlan(t *testing.T, repo *Repository, streamID string) {
	t.Helper()
	for _, query := range []string{`ANALYZE agent_delivery_inbox`, `SET plan_cache_mode=force_generic_plan`, `PREPARE retention_pending_replay(text,integer) AS SELECT stream_id,sequence,session_id,incarnation_id,harness_generation,submission_id,event_type,payload,terminal,received_at,projected_at,content_hash,hash_version,payload_pruned_at,payload_bytes FROM agent_delivery_inbox WHERE stream_id=$1 AND projected_at IS NULL ORDER BY sequence LIMIT $2`} {
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	plan := retentionExplainPostgres(t, repo, "(ANALYZE, FORMAT JSON)", "EXECUTE retention_pending_replay('"+strings.ReplaceAll(streamID, "'", "''")+"',10)", nil)
	assertRetentionPendingPlan(t, plan)
}

func assertRetentionPendingPlan(t *testing.T, plan retentionPostgresPlan) {
	t.Helper()
	if plan.Relation == "agent_delivery_inbox" && (plan.NodeType == "Seq Scan" || plan.Rows*plan.Loops > 1 || plan.Index != "idx_agent_delivery_inbox_unprojected") {
		t.Fatalf("legacy pending replay scanned projected history: %+v", plan)
	}
	for _, child := range plan.Plans {
		assertRetentionPendingPlan(t, child)
	}
}
