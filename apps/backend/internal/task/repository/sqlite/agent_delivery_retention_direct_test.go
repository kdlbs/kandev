package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionStorageDirectExpiredPrefix(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy_%t", engine, legacy), func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "direct-prefix", 1, 0)
				seedRetentionStorageRows(t, repo, events[0], 3, 1024)
				if legacy {
					if _, err := repo.db.Exec(`UPDATE agent_delivery_inbox SET hash_version=0,content_hash='',payload_bytes=0 WHERE sequence=1`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := repo.db.Exec(`UPDATE agent_delivery_effects SET state='pending',completed_at=NULL WHERE sequence=2`); err != nil {
					t.Fatal(err)
				}
				before := retentionCanonicalContent(t, repo, events[0].SessionID)
				tx, err := repo.db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				replay, _, err := repo.loadAgentDeliveryEventTx(t.Context(), tx, events[0].StreamID, 3)
				_ = tx.Rollback()
				if err != nil {
					t.Fatal(err)
				}
				guardDirectPrefixUpdates(t, repo, events[0].StreamID, 1)
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				if err != nil || result.EventRowsRetired != 1 || result.EventPayloadsPruned != 2 || result.PayloadBytesPruned != int64(len(events[0].Payload)+len(replay.Payload)) {
					t.Fatalf("expired validated prefix must delete directly while compacting above protected gap: %+v %v", result, err)
				}
				cursor, err := repo.GetAgentDeliveryCursor(t.Context(), events[0].StreamID)
				if err != nil || cursor.RetiredSequence != 1 {
					t.Fatalf("only contiguous expired prefix can retire: %+v %v", cursor, err)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=1", events[0].StreamID, 0)
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=2 AND payload_pruned_at IS NULL AND LENGTH(payload)=1024", events[0].StreamID, 1)
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=3 AND payload_pruned_at IS NOT NULL AND hash_version=1 AND content_hash<>'' AND LENGTH(payload)=0", events[0].StreamID, 1)
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &replay, 3); err != nil || inserted {
					t.Fatalf("compacted row above gap must authenticate replay: %t %v", inserted, err)
				}
				if got := retentionCanonicalContent(t, repo, events[0].SessionID); got != before {
					t.Fatal("direct prefix deletion changed canonical output")
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageDirectValidation(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, failure := range []string{"hash", "version", "bytes", "grace"} {
			t.Run(engine+"/"+failure, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "direct-validation", 1, 0)
				var mutation string
				want := repoerrors.ErrAgentDeliveryReceiptCorrupt
				options := models.AgentDeliveryRetentionBatchOptions{}
				switch failure {
				case "hash":
					mutation = `UPDATE agent_delivery_inbox SET content_hash='corrupt'`
				case "version":
					mutation = `UPDATE agent_delivery_inbox SET hash_version=2`
					want = repoerrors.ErrAgentDeliveryHashVersion
				case "bytes":
					mutation = `UPDATE agent_delivery_inbox SET payload_bytes=99999`
				case "grace":
					options.PayloadGrace = 72 * time.Hour
					options.ReplayEvents = 1
					want = nil
				}
				if mutation != "" {
					if _, err := repo.db.Exec(mutation); err != nil {
						t.Fatal(err)
					}
				}
				guardDirectPrefixUpdates(t, repo, events[0].StreamID, 1)
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), options)
				if !errors.Is(err, want) || result.EventRowsRetired != 0 || result.EventPayloadsPruned != 0 {
					t.Fatalf("direct deletion must preserve raw validation and grace: %+v %v want=%v", result, err, want)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=?", events[0].StreamID, 1)
			})
		}
	}
}

func TestDeliveryRetentionStorageDirectMixedPrefix(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "direct-mixed", 1, 0)
			seedRetentionStorageRows(t, repo, events[0], 4, 1024)
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			first, _, err := repo.loadAgentDeliveryEventTx(t.Context(), tx, events[0].StreamID, 1)
			if err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			third, _, err := repo.loadAgentDeliveryEventTx(t.Context(), tx, events[0].StreamID, 3)
			_ = tx.Rollback()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET payload=?,payload_pruned_at=?,content_hash=?,hash_version=1,payload_bytes=? WHERE sequence=1`), []byte{}, repo.nowUTC(), agentDeliveryEventHash(&first), len(first.Payload)); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET content_hash=?,hash_version=1,payload_bytes=? WHERE sequence=3`), agentDeliveryEventHash(&third), len(third.Payload)); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET projected_at=? WHERE sequence=4`), repo.nowUTC()); err != nil {
				t.Fatal(err)
			}
			guardDirectPrefixUpdates(t, repo, events[0].StreamID, 3)
			result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{ReplayEvents: 1})
			if err != nil || result.EventRowsRetired != 3 || result.EventPayloadsPruned != 2 || result.PayloadBytesPruned != 2048 {
				t.Fatalf("mixed compact/legacy/v1 prefix must retire once and preserve recent raw output: %+v %v", result, err)
			}
			cursor, err := repo.GetAgentDeliveryCursor(t.Context(), events[0].StreamID)
			if err != nil || cursor.RetiredSequence != 3 {
				t.Fatalf("mixed prefix cursor: %+v %v", cursor, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=4 AND payload_pruned_at IS NULL AND LENGTH(payload)=1024", events[0].StreamID, 1)
		})
	}
}

func guardDirectPrefixUpdates(t *testing.T, repo *Repository, streamID string, last int64) {
	t.Helper()
	stream := "'" + strings.ReplaceAll(streamID, "'", "''") + "'"
	condition := fmt.Sprintf("OLD.stream_id=%s AND OLD.sequence<=%d", stream, last)
	if repo.db.DriverName() == "pgx" {
		if _, err := repo.db.Exec(`CREATE FUNCTION reject_direct_prefix_update() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'expired prefix must not be rewritten'; END $$`); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.db.Exec(`CREATE TRIGGER reject_direct_prefix_update BEFORE UPDATE OF content_hash,payload ON agent_delivery_inbox FOR EACH ROW WHEN (` + condition + `) EXECUTE FUNCTION reject_direct_prefix_update()`); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER reject_direct_prefix_update BEFORE UPDATE OF content_hash,payload ON agent_delivery_inbox WHEN ` + condition + ` BEGIN SELECT RAISE(FAIL,'expired prefix must not be rewritten'); END`); err != nil {
		t.Fatal(err)
	}
}
