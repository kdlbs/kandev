package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const wideRetentionEligibleIndex = `CREATE INDEX idx_agent_delivery_inbox_retention_eligible
	ON agent_delivery_inbox(projected_at, stream_id, sequence) WHERE payload_pruned_at IS NULL`

func TestDeliveryRetentionStorageEligibleIndexUpgrade(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			assertRetentionProjectedIndex(t, repo, true)
			replaceRetentionEligibleIndexWithWide(t, repo)
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			assertRetentionProjectedIndex(t, repo, true)
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			assertRetentionProjectedIndex(t, repo, true)
			// Release N's additive CREATE IF NOT EXISTS must retain the upgraded index.
			if _, err := repo.db.Exec(strings.Replace(wideRetentionEligibleIndex, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1)); err != nil {
				t.Fatal(err)
			}
			assertRetentionProjectedIndex(t, repo, true)
		})
	}
}

func TestDeliveryRetentionStorageEligibleIndexCrash(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, point := range []string{"before_eligible_index_upgrade", "after_eligible_index_drop", "after_eligible_index_create"} {
			t.Run(engine+"/"+point, func(t *testing.T) {
				repo := newRetentionTestRepo(t, engine)
				replaceRetentionEligibleIndexWithWide(t, repo)
				injected := errors.New("index upgrade interrupted")
				repo.agentDeliveryRetentionFailpoint = func(got string) error {
					if got == point {
						return injected
					}
					return nil
				}
				if err := repo.initAgentDeliveryRetentionSchema(); !errors.Is(err, injected) {
					t.Fatalf("upgrade interruption=%v want=%v", err, injected)
				}
				assertRetentionProjectedIndex(t, repo, false)
				repo.agentDeliveryRetentionFailpoint = nil
				if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
					t.Fatal(err)
				}
				assertRetentionProjectedIndex(t, repo, true)
			})
		}
	}
}

func TestDeliveryRetentionStorageEligibleIndexLegacyKey(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_inbox_retention_eligible`); err != nil {
				t.Fatal(err)
			}
			query := strings.Replace(wideRetentionEligibleIndex, "(projected_at, stream_id, sequence)", "(stream_id, sequence)", 1)
			if _, err := repo.db.Exec(query); err != nil {
				t.Fatal(err)
			}
			if err := repo.initAgentDeliveryRetentionSchema(); err != nil {
				t.Fatal(err)
			}
			assertRetentionProjectedIndex(t, repo, true)
		})
	}
}

func TestDeliveryRetentionStorageEligibleIndexUnprojected(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "index-unprojected", 1, 0)
			if _, err := repo.db.Exec(`UPDATE agent_delivery_inbox SET projected_at=NULL`); err != nil {
				t.Fatal(err)
			}
			result, err := repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.Candidates != 0 || result.EventPayloadsPruned != 0 || result.EventRowsRetired != 0 {
				t.Fatalf("unprojected event cleanup=%+v err=%v", result, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND payload_pruned_at IS NULL AND projected_at IS NULL", events[0].StreamID, 1)
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET projected_at=?`), repo.nowUTC().Add(-48*time.Hour)); err != nil {
				t.Fatal(err)
			}
			result, err = repo.RunAgentDeliveryRetentionBatch(context.Background(), models.AgentDeliveryRetentionBatchOptions{})
			if err != nil || result.EventPayloadsPruned != 1 || result.EventRowsRetired != 1 {
				t.Fatalf("projected event cleanup=%+v err=%v stream=%s", result, err, events[0].StreamID)
			}
		})
	}
}

func replaceRetentionEligibleIndexWithWide(t testing.TB, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`DROP INDEX idx_agent_delivery_inbox_retention_eligible`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(wideRetentionEligibleIndex); err != nil {
		t.Fatal(err)
	}
}

func assertRetentionProjectedIndex(t testing.TB, repo *Repository, want bool) {
	t.Helper()
	var definition string
	query := `SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_agent_delivery_inbox_retention_eligible'`
	if dialect.IsPostgres(repo.db.DriverName()) {
		query = `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname='idx_agent_delivery_inbox_retention_eligible'`
	}
	if err := repo.db.QueryRow(query).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if got := strings.Contains(strings.ToLower(definition), "projected_at is not null"); got != want {
		t.Fatalf("projected-only eligible index=%v want=%v definition=%s", got, want, definition)
	}
	if want && !strings.Contains(strings.Join(strings.Fields(definition), ""), "(projected_at,stream_id,sequence)") {
		t.Fatalf("projected-only eligible index has incorrect ordered keys: %s", definition)
	}
}
