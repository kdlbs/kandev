package sqlite

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionStorageFreshReceiptRead(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		name := "fresh"
		if duplicate {
			name = "duplicate_above_frontier"
		}
		t.Run(name, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "SQLite", "fresh-authorizer", 1, 0)
			event := *events[0]
			prepareRetentionWindowSubmission(t, repo, &event)
			event.Sequence = 2
			if duplicate {
				event.Sequence = 3
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3); err != nil || !inserted {
					t.Fatalf("seed out-of-order receipt: %t %v", inserted, err)
				}
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3); err != nil || inserted {
					t.Fatalf("positive duplicate control: %t %v", inserted, err)
				}
			}
			var denied atomic.Int64
			setRetentionReceiptAuthorizer(t, repo, func(operation int, table, column, _ string) int {
				if operation == sqlite3.SQLITE_READ && table == "agent_delivery_inbox" && column == "content_hash" {
					denied.Add(1)
					return sqlite3.SQLITE_DENY
				}
				return sqlite3.SQLITE_OK
			})
			defer setRetentionReceiptAuthorizer(t, repo, nil)
			inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, event.Sequence)
			if duplicate {
				if err == nil || inserted || denied.Load() == 0 {
					t.Fatalf("duplicate above frontier must read and independently validate its receipt: %t %v denied=%d", inserted, err, denied.Load())
				}
			} else if err != nil || !inserted || denied.Load() != 0 {
				t.Fatalf("fresh authenticated event must insert without a redundant receipt payload/hash read: %t %v denied=%d", inserted, err, denied.Load())
			}
		})
	}
}

func TestDeliveryRetentionStorageFutureReceiptEvidence(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, scenario := range []string{"duplicate", "conflict", "corrupt_hash", "missing_received", "missing_cursor", "cursorless_orphan"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "future-evidence", 1, 0)
				event := *events[0]
				prepareRetentionWindowSubmission(t, repo, &event)
				event.Sequence = 3
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3); err != nil || !inserted {
					t.Fatalf("seed future receipt: %t %v", inserted, err)
				}
				var want error
				switch scenario {
				case "conflict":
					event.Payload = []byte(`{"type":"message_chunk","text":"different output"}`)
					want = repoerrors.ErrAgentDeliveryEventConflict
				case "corrupt_hash":
					if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET content_hash='corrupt' WHERE stream_id=? AND sequence=3`), event.StreamID); err != nil {
						t.Fatal(err)
					}
					want = repoerrors.ErrAgentDeliveryReceiptCorrupt
				case "missing_received":
					if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM agent_delivery_inbox WHERE stream_id=? AND sequence=1`), event.StreamID); err != nil {
						t.Fatal(err)
					}
					event.Sequence = 1
					want = repoerrors.ErrAgentDeliveryCheckpointMissing
				case "missing_cursor", "cursorless_orphan":
					if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM agent_delivery_cursors WHERE stream_id=?`), event.StreamID); err != nil {
						t.Fatal(err)
					}
					if scenario == "cursorless_orphan" {
						event.Sequence = 4
					}
					want = repoerrors.ErrAgentDeliveryCheckpointMissing
				}
				inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3)
				if inserted || !errors.Is(err, want) {
					t.Fatalf("future receipt evidence %s: inserted=%t error=%v want=%v", scenario, inserted, err, want)
				}
				assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id=? AND sequence=3", event.StreamID, 1)
			})
		}
	}
}

func setRetentionReceiptAuthorizer(t *testing.T, repo *Repository, authorize func(int, string, string, string) int) {
	t.Helper()
	connection, err := repo.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(authorize)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
