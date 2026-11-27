package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeliveryRetentionStorageOwnerStatementReuse(t *testing.T) {
	for _, associated := range []bool{false, true} {
		name := "unassociated"
		if associated {
			name = "associated"
		}
		t.Run(name, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "SQLite", "owner-cache", 1, 0)
			event := *events[0]
			prepareRetentionWindowSubmission(t, repo, &event)
			if !associated {
				event.SubmissionID = ""
			}
			var preparations atomic.Int64
			setRetentionReceiptAuthorizer(t, repo, func(operation int, table, _, _ string) int {
				if operation == sqlite3.SQLITE_READ && table == "task_sessions" {
					preparations.Add(1)
				}
				return sqlite3.SQLITE_OK
			})
			defer setRetentionReceiptAuthorizer(t, repo, nil)
			event.Sequence = 2
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
				t.Fatalf("warm owner query: %t %v", inserted, err)
			}
			if preparations.Swap(0) == 0 {
				t.Fatal("positive control: owner query was not prepared")
			}
			event.Sequence = 3
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3); err != nil || !inserted {
				t.Fatalf("reuse owner query: %t %v", inserted, err)
			}
			if count := preparations.Load(); count != 0 {
				t.Fatalf("owner SQL plan must be reused; observed %d native preparation reads", count)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE executors_running SET agent_execution_id='replacement' WHERE session_id=?`), event.SessionID); err != nil {
				t.Fatal(err)
			}
			event.Sequence = 4
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 4); inserted || !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
				t.Fatalf("reused SQL must read the replacement owner: %t %v", inserted, err)
			}
			if count := preparations.Load(); count != 0 {
				t.Fatalf("replacement check recompiled owner SQL: %d", count)
			}
		})
	}
}

func TestDeliveryRetentionStorageOwnerStatementLifecycle(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, associated := range []bool{false, true} {
			name := "unassociated"
			if associated {
				name = "associated"
			}
			t.Run(engine+"/"+name, func(t *testing.T) {
				repo, events := seedRetentionBoundsStream(t, engine, "owner-lifecycle", 1, 0)
				event := *events[0]
				prepareRetentionWindowSubmission(t, repo, &event)
				if !associated {
					event.SubmissionID = ""
				}
				event.Sequence = 2
				holder, err := repo.db.BeginTxx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
				inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 2)
				cancel()
				if inserted || !errors.Is(err, context.DeadlineExceeded) {
					_ = holder.Rollback()
					t.Fatalf("blocked first preparation must cancel before writer entry: %t %v", inserted, err)
				}
				if _, err := repo.agentDeliveryOwnerStatement(associated); err == nil {
					t.Fatal("canceled preparation published a statement")
				}
				if err := holder.Rollback(); err != nil {
					t.Fatal(err)
				}
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
					t.Fatalf("retry after canceled preparation: %t %v", inserted, err)
				}
				if engine == "Postgres" {
					connection, err := repo.db.Conn(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					err = connection.Raw(func(any) error { return driver.ErrBadConn })
					_ = connection.Close()
					if !errors.Is(err, driver.ErrBadConn) {
						t.Fatalf("discard prepared connection: %v", err)
					}
				}
				event.Sequence = 3
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 3); err != nil || !inserted {
					t.Fatalf("prepared query must retain schema on connection replacement: %t %v", inserted, err)
				}
				if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE executors_running SET agent_execution_id='replacement' WHERE session_id=?`), event.SessionID); err != nil {
					t.Fatal(err)
				}
				event.Sequence = 4
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 4); inserted || !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
					t.Fatalf("fresh owner proof after reuse: %t %v", inserted, err)
				}
				if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE executors_running SET agent_execution_id=? WHERE session_id=?`), event.ExecutionID, event.SessionID); err != nil {
					t.Fatal(err)
				}
				if err := repo.Close(); err != nil {
					t.Fatal(err)
				}
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 4); inserted || err == nil {
					t.Fatalf("closed statement owner must reject new admission: %t %v", inserted, err)
				}
				var one int
				if err := repo.db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
					t.Fatalf("statement close must preserve shared database ownership: %v", err)
				}
				reopened := reopenRetentionStorageRepository(t, repo)
				if err := reopened.CheckAgentDeliveryRetentionReady(t.Context()); err != nil {
					t.Fatal(err)
				}
				if inserted, err := reopened.ReceiveAgentDeliveryEvent(t.Context(), &event, 4); err != nil || !inserted {
					t.Fatalf("reopened repository prepares against ready schema: %t %v", inserted, err)
				}
			})
		}
	}
}

func TestDeliveryRetentionStorageOwnerStatementPublication(t *testing.T) {
	for _, closeDuringPrepare := range []bool{false, true} {
		name := "cancel"
		if closeDuringPrepare {
			name = "close"
		}
		t.Run(name, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, "SQLite", "owner-publication", 1, 0)
			event := *events[0]
			prepareRetentionWindowSubmission(t, repo, &event)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			setRetentionReceiptAuthorizer(t, repo, func(operation int, table, _, _ string) int {
				if operation == sqlite3.SQLITE_READ && table == "task_sessions" {
					once.Do(func() { close(entered); <-release })
				}
				return sqlite3.SQLITE_OK
			})
			defer setRetentionReceiptAuthorizer(t, repo, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- repo.prepareAgentDeliveryOwnerStatement(ctx, &event) }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("native preparation did not reach the owner SQL")
			}
			if closeDuringPrepare {
				closeRetentionOwnerDuringPreparation(t, repo, release)
			} else {
				cancel()
				close(release)
			}
			if err := <-done; err == nil || (!closeDuringPrepare && !errors.Is(err, context.Canceled)) {
				t.Fatalf("interrupted native preparation must not publish: %v", err)
			}
			if _, err := repo.agentDeliveryOwnerStatement(true); err == nil {
				t.Fatal("interrupted preparation leaked a published statement")
			}
			if !closeDuringPrepare {
				event.Sequence = 2
				if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
					t.Fatalf("native cancellation poisoned subsequent preparation: %t %v", inserted, err)
				}
			}
		})
	}
}

func closeRetentionOwnerDuringPreparation(t *testing.T, repo *Repository, release chan struct{}) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- repo.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		cache := &repo.agentDeliveryOwnerStatements
		cache.mu.Lock()
		closed := cache.closed
		cache.mu.Unlock()
		if closed {
			break
		}
		select {
		case <-ctx.Done():
			close(release)
			t.Fatal("repository close did not fence owner statement publication")
		case <-ticker.C:
		}
	}
	// Closing another cached plan can wait for the connection held by preparation.
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("repository close did not finish after native preparation was released")
	}
}

func TestDeliveryRetentionStorageOwnerStatementConcurrentPreparation(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "owner-concurrent", 1, 0)
			event := *events[0]
			prepareRetentionWindowSubmission(t, repo, &event)
			start := make(chan struct{})
			done := make(chan error, 16)
			for range 16 {
				go func() {
					<-start
					done <- repo.prepareAgentDeliveryOwnerStatement(t.Context(), &event)
				}()
			}
			close(start)
			for range 16 {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			event.Sequence = 2
			if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, 2); err != nil || !inserted {
				t.Fatalf("duplicate preparation closed the winning plan: %t %v", inserted, err)
			}
		})
	}
}
