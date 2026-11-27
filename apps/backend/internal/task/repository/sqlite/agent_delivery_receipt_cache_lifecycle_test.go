package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
)

func TestDeliveryRetentionStorageReceiptStatementLifecycle(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, operation := range []string{"receive", "canonical", "noncanonical"} {
			t.Run(engine+"/"+operation, func(t *testing.T) {
				seeded, events := seedRetentionBoundsStream(t, engine, "receipt-lifecycle", 1, 0)
				repo := coldRetentionReceiptRepository(t, seeded)
				holder, err := repo.db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
				err = runRetentionReceiptOperation(ctx, repo, events[0], operation)
				cancel()
				if !errors.Is(err, context.DeadlineExceeded) {
					_ = holder.Rollback()
					t.Fatalf("first preparation must cancel before writer entry: %v", err)
				}
				if statement, err := repo.agentDeliveryReceiptStatement(); err != nil || statement != nil {
					t.Fatalf("canceled first preparation must leave an open cold cache: %v %v", statement, err)
				}
				if err := holder.Rollback(); err != nil {
					t.Fatal(err)
				}
				if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err != nil {
					t.Fatalf("retry after canceled preparation: %v", err)
				}
				discardRetentionReceiptConnection(t, repo)
				if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err != nil {
					t.Fatalf("replacement connection must preserve prepared plan and schema: %v", err)
				}
				if err := repo.Close(); err != nil {
					t.Fatal(err)
				}
				if err := runRetentionReceiptOperation(t.Context(), repo, events[0], operation); err == nil {
					t.Fatal("closed repository must reject receipt operations")
				}
				var one int
				if err := repo.db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
					t.Fatalf("statement close must preserve shared database ownership: %v", err)
				}
				reopened := reopenRetentionStorageRepository(t, repo)
				if err := reopened.CheckAgentDeliveryRetentionReady(t.Context()); err != nil {
					t.Fatal(err)
				}
				if err := runRetentionReceiptOperation(t.Context(), reopened, events[0], operation); err != nil {
					t.Fatalf("reopened ready schema must prepare and read retained evidence: %v", err)
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageReceiptStatementPublication(t *testing.T) {
	for _, closeDuringPrepare := range []bool{false, true} {
		name := "cancel"
		if closeDuringPrepare {
			name = "close"
		}
		t.Run(name, func(t *testing.T) {
			seeded, _ := seedRetentionBoundsStream(t, "SQLite", "receipt-publication", 1, 0)
			repo := coldRetentionReceiptRepository(t, seeded)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			setRetentionReceiptAuthorizer(t, repo, func(kind int, table, column, _ string) int {
				if kind == sqlite3.SQLITE_READ && table == "agent_delivery_inbox" && column == "content_hash" {
					once.Do(func() { close(entered); <-release })
				}
				return sqlite3.SQLITE_OK
			})
			defer setRetentionReceiptAuthorizer(t, repo, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- repo.prepareAgentDeliveryReceiptStatement(ctx) }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("native receipt preparation was not observed")
			}
			if closeDuringPrepare {
				if err := repo.Close(); err != nil {
					close(release)
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			close(release)
			if err := <-done; err == nil || (!closeDuringPrepare && !errors.Is(err, context.Canceled)) {
				t.Fatalf("interrupted native preparation must not publish: %v", err)
			}
			statement, err := repo.agentDeliveryReceiptStatement()
			if statement != nil || (closeDuringPrepare && err == nil) || (!closeDuringPrepare && err != nil) {
				t.Fatalf("publication state must distinguish closed from cold: %v %v", statement, err)
			}
			if !closeDuringPrepare {
				if err := repo.prepareAgentDeliveryReceiptStatement(t.Context()); err != nil {
					t.Fatalf("canceled native preparation poisoned retry: %v", err)
				}
			}
		})
	}
}

func TestDeliveryRetentionStorageReceiptStatementConcurrentPreparation(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			seeded, events := seedRetentionBoundsStream(t, engine, "receipt-duplicate", 1, 0)
			repo := coldRetentionReceiptRepository(t, seeded)
			start, done := make(chan struct{}), make(chan error, 16)
			for range 16 {
				go func() { <-start; done <- repo.prepareAgentDeliveryReceiptStatement(t.Context()) }()
			}
			close(start)
			for range 16 {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			if err := runRetentionReceiptOperation(t.Context(), repo, events[0], "canonical"); err != nil {
				t.Fatalf("duplicate publication closed the winning SQL plan: %v", err)
			}
		})
	}
}

func TestDeliveryRetentionStorageReceiptStatementPrivateFallback(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, warm := range []bool{false, true} {
			name := "cold"
			if warm {
				name = "warm"
			}
			t.Run(engine+"/"+name, func(t *testing.T) {
				seeded, events := seedRetentionBoundsStream(t, engine, "receipt-private", 1, 0)
				repo := coldRetentionReceiptRepository(t, seeded)
				if warm {
					if err := repo.prepareAgentDeliveryReceiptStatement(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				tx, err := repo.db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				event := events[0]
				if err := lockAgentDeliveryEventWritesMode(t.Context(), tx, repo.db.DriverName(), []string{event.SessionID}, []string{event.StreamID}, true); err != nil {
					t.Fatal(err)
				}
				if _, _, err := repo.loadAgentDeliveryEventTx(t.Context(), tx, event.StreamID, event.Sequence); err != nil {
					t.Fatalf("cold/open private helper must retain its transaction-local fallback: %v", err)
				}
				if err := repo.Close(); err != nil {
					t.Fatal(err)
				}
				if _, _, err := repo.loadAgentDeliveryEventTx(t.Context(), tx, event.StreamID, event.Sequence); err == nil {
					t.Fatal("closed cache must not fall back to SQL in an already-started transaction")
				}
			})
		}
	}
}

func coldRetentionReceiptRepository(t *testing.T, seeded *Repository) *Repository {
	t.Helper()
	repo := NewWithInitializedDB(seeded.db, seeded.ro, nil)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func discardRetentionReceiptConnection(t *testing.T, repo *Repository) {
	t.Helper()
	connection, err := repo.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	err = connection.Raw(func(any) error { return driver.ErrBadConn })
	_ = connection.Close()
	if !errors.Is(err, driver.ErrBadConn) {
		t.Fatalf("native connection replacement was not forced: %v", err)
	}
}
