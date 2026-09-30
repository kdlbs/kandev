package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

const blockedPollLimit = 10 * time.Second

// openPeerPostgresStore opens a second store on its own connection, in the
// same isolated schema as store, so two statements can contend for row locks.
func openPeerPostgresStore(t *testing.T, store *Store) *Store {
	t.Helper()
	var schema string
	if err := store.db.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	raw, err := internaldb.OpenPostgres(testutil.PostgresDSNFromEnv(t), 1, 1)
	if err != nil {
		t.Fatalf("open peer postgres: %v", err)
	}
	peer := sqlx.NewDb(raw, "pgx")
	peer.SetMaxOpenConns(1)
	peer.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = peer.Close() })
	if _, err := peer.Exec(`SET search_path TO ` + schema); err != nil {
		t.Fatalf("peer search_path: %v", err)
	}
	peerStore, err := NewStore(peer, peer)
	if err != nil {
		t.Fatalf("peer store: %v", err)
	}
	return peerStore
}

// openPostgresObserver opens a third connection used only to read
// PostgreSQL's blocking-lock state.
func openPostgresObserver(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := internaldb.OpenPostgres(testutil.PostgresDSNFromEnv(t), 1, 1)
	if err != nil {
		t.Fatalf("open observer: %v", err)
	}
	obs := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = obs.Close() })
	return obs
}

// waitForBlockedBy polls pg_stat_activity/pg_blocking_pids until some backend
// is waiting on a lock held by blockerPID, failing after a bounded wait.
func waitForBlockedBy(t *testing.T, obs *sqlx.DB, blockerPID int) {
	t.Helper()
	deadline := time.Now().Add(blockedPollLimit)
	for {
		var waiting int
		if err := obs.Get(&waiting, `SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))`, blockerPID); err != nil {
			t.Fatalf("query blocking pids: %v", err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no backend became blocked by pid %d within %s", blockerPID, blockedPollLimit)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func backendPID(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var pid int
	if err := db.Get(&pid, `SELECT pg_backend_pid()`); err != nil {
		t.Fatalf("pg_backend_pid: %v", err)
	}
	return pid
}

// deleteRaceCase names one production delete under test and how to check that
// nothing survived it.
type deleteRaceCase struct {
	name   string
	setup  func(t *testing.T, store *Store) *Coordinator
	remove func(store *Store, c *Coordinator) error
	verify func(t *testing.T, store *Store, c *Coordinator)
}

func deleteRaceCases() []deleteRaceCase {
	return []deleteRaceCase{
		{
			name: "DeleteCoordinator",
			setup: func(t *testing.T, store *Store) *Coordinator {
				return newTestCoordinator(t, store, "ws-1")
			},
			remove: func(store *Store, c *Coordinator) error {
				return store.DeleteCoordinator(context.Background(), "ws-1", c.ID)
			},
			verify: func(t *testing.T, store *Store, c *Coordinator) {
				assertCoordinatorAbsent(t, store, c.ID)
			},
		},
		{
			name: "DeleteWorkspaceState",
			setup: func(t *testing.T, store *Store) *Coordinator {
				seedWorkspaceCoordinatorState(t, store, "ws-2")
				return newTestCoordinator(t, store, "ws-1")
			},
			remove: func(store *Store, _ *Coordinator) error {
				return store.DeleteWorkspaceState(context.Background(), "ws-1")
			},
			verify: func(t *testing.T, store *Store, c *Coordinator) {
				assertCoordinatorAbsent(t, store, c.ID)
				cs, ps, ss := countWorkspaceCoordinatorState(t, store, "ws-1")
				if cs != 0 || ps != 0 || ss != 0 {
					t.Fatalf("ws-1: coordinators=%d proposals=%d stalls=%d, want all 0", cs, ps, ss)
				}
				cs, ps, ss = countWorkspaceCoordinatorState(t, store, "ws-2")
				if cs != 1 || ps != 5 || ss != 1 {
					t.Fatalf("ws-2: coordinators=%d proposals=%d stalls=%d, want 1/5/1 unchanged", cs, ps, ss)
				}
			},
		},
	}
}

func assertCoordinatorAbsent(t *testing.T, store *Store, coordinatorID string) {
	t.Helper()
	var coordinators, proposals int
	if err := store.db.Get(&coordinators, store.db.Rebind(`SELECT COUNT(*) FROM coordinators WHERE id = ?`), coordinatorID); err != nil {
		t.Fatalf("count coordinators: %v", err)
	}
	if err := store.db.Get(&proposals, store.db.Rebind(`SELECT COUNT(*) FROM coordinator_proposals WHERE coordinator_id = ?`), coordinatorID); err != nil {
		t.Fatalf("count proposals: %v", err)
	}
	if coordinators != 0 || proposals != 0 {
		t.Fatalf("coordinator %s left coordinators=%d proposals=%d, want 0/0", coordinatorID, coordinators, proposals)
	}
}

// A proposal inserted under the coordinator row lock and still uncommitted
// must make the delete wait, then be removed with the coordinator.
func TestDelete_Postgres_WaitsForInFlightProposalInsert(t *testing.T) {
	for _, tc := range deleteRaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStorePostgres(t)
			peer := openPeerPostgresStore(t, store)
			obs := openPostgresObserver(t)
			ctx := context.Background()
			c := tc.setup(t, store)

			txA, err := peer.db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatalf("begin tx A: %v", err)
			}
			defer func() { _ = txA.Rollback() }()
			var pidA int
			if err := txA.Get(&pidA, `SELECT pg_backend_pid()`); err != nil {
				t.Fatalf("tx A pid: %v", err)
			}
			p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
			if err := peer.insertProposalBody(ctx, txA, txA.Rebind, p); err != nil {
				t.Fatalf("insert in tx A: %v", err)
			}

			done := make(chan error, 1)
			go func() { done <- tc.remove(store, c) }()

			waitForBlockedBy(t, obs, pidA)
			select {
			case err := <-done:
				t.Fatalf("delete finished while tx A held the lock: %v", err)
			default:
			}

			if err := txA.Commit(); err != nil {
				t.Fatalf("commit tx A: %v", err)
			}
			if err := <-done; err != nil {
				t.Fatalf("delete: %v", err)
			}
			tc.verify(t, store, c)
		})
	}
}

// Once the delete holds the coordinator row lock, a concurrent insert must
// wait for it, then find the coordinator gone and leave no proposal behind.
func TestDelete_Postgres_InsertAfterLockIsNotFound(t *testing.T) {
	for _, tc := range deleteRaceCases() {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStorePostgres(t)
			peer := openPeerPostgresStore(t, store)
			obs := openPostgresObserver(t)
			ctx := context.Background()
			c := tc.setup(t, store)
			deletePID := backendPID(t, store.db)

			locked := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseDelete := func() { releaseOnce.Do(func() { close(release) }) }
			store.beforeCoordinatorRowDelete = func() {
				close(locked)
				<-release
			}

			var wg sync.WaitGroup
			t.Cleanup(wg.Wait)
			t.Cleanup(releaseDelete)

			deleteDone := make(chan error, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				deleteDone <- tc.remove(store, c)
			}()
			select {
			case <-locked:
			case <-time.After(blockedPollLimit):
				t.Fatal("delete never reached the post-lock barrier")
			}

			insertDone := make(chan error, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				insertDone <- peer.InsertProposal(ctx, &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()})
			}()

			waitForBlockedBy(t, obs, deletePID)
			releaseDelete()

			if err := <-deleteDone; err != nil {
				t.Fatalf("delete: %v", err)
			}
			if err := <-insertDone; !errors.Is(err, ErrNotFound) {
				t.Fatalf("insert after delete = %v, want ErrNotFound", err)
			}
			tc.verify(t, store, c)
		})
	}
}
