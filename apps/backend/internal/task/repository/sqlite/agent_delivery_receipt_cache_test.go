package sqlite

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionStorageReceiptStatementReuse(t *testing.T) {
	for _, operation := range []string{"receive", "canonical", "noncanonical"} {
		t.Run(operation, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "SQLite", "receipt-reuse", 1, 0)
			var prepares atomic.Int64
			setRetentionReceiptAuthorizer(t, repo, func(kind int, table, column, _ string) int {
				if kind == sqlite3.SQLITE_READ && table == "agent_delivery_inbox" && column == "content_hash" {
					prepares.Add(1)
				}
				return sqlite3.SQLITE_OK
			})
			defer setRetentionReceiptAuthorizer(t, repo, nil)
			if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err != nil {
				t.Fatal(err)
			}
			if prepares.Swap(0) == 0 {
				t.Fatal("positive control: native receipt query preparation was not observed")
			}
			if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err != nil {
				t.Fatal(err)
			}
			if count := prepares.Load(); count != 0 {
				t.Fatalf("receipt SQL plan must be reused; observed %d native prepares", count)
			}
		})
	}
}

func TestDeliveryRetentionStorageReceiptStatementFreshEvidence(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, operation := range []string{"receive", "canonical", "noncanonical"} {
			for _, corrupt := range []bool{false, true} {
				name := "replacement"
				if corrupt {
					name = "corrupt_hash"
				}
				t.Run(engine+"/"+operation+"/"+name, func(t *testing.T) {
					repo, events := seedRetentionBoundsStream(t, engine, "receipt-fresh", 1, 0)
					if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err != nil {
						t.Fatal(err)
					}
					query := `UPDATE agent_delivery_inbox SET incarnation_id='replacement' WHERE stream_id=? AND sequence=1`
					want := repoerrors.ErrAgentDeliveryEventConflict
					if corrupt {
						query = `UPDATE agent_delivery_inbox SET content_hash='corrupt' WHERE stream_id=? AND sequence=1`
						want = repoerrors.ErrAgentDeliveryReceiptCorrupt
					}
					if _, err := repo.db.Exec(repo.db.Rebind(query), events[0].StreamID); err != nil {
						t.Fatal(err)
					}
					if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); !errors.Is(err, want) {
						t.Fatalf("reused plan must read fresh receipt %s: got %v want %v", name, err, want)
					}
				})
			}
		}
	}
}

func runRetentionReceiptOperation(ctx context.Context, repo *Repository, event *models.AgentDeliveryEvent, operation string) error {
	switch operation {
	case "receive":
		_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, event.Sequence)
		return err
	case "canonical":
		_, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, nil)
		return err
	default:
		_, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil)
		return err
	}
}
