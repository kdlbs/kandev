package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// newTestStorePostgres opens an isolated PostgreSQL schema for both the
// writer and reader, matching production's single pool for PostgreSQL
// (db.NewPool(pgDB, pgDB)). Skips the test if KANDEV_TEST_POSTGRES_DSN is
// unset.
func newTestStorePostgres(t *testing.T) *Store {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	pg := testutil.OpenIsolatedPostgres(t, dsn)
	store, err := NewStore(pg, pg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

// TestPatchCoordinator_Postgres_ConcurrentDisjointFields is the PostgreSQL
// twin of TestPatchCoordinator_ConcurrentDisjointFields (Build decision 7
// test (a)), exercising the real SELECT ... FOR UPDATE row lock instead of
// SQLite's single-connection writer pool.
func TestPatchCoordinator_Postgres_ConcurrentDisjointFields(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("rep-%d", i), func(t *testing.T) {
			store := newTestStorePostgres(t)
			ctx := context.Background()
			c := &Coordinator{WorkspaceID: "ws-1", Name: "orig", AgentProfileID: "a", ExecutorProfileID: "e", Context: "orig-ctx"}
			if err := store.CreateCoordinator(ctx, c); err != nil {
				t.Fatalf("CreateCoordinator: %v", err)
			}

			start := make(chan struct{})
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				name := "alpha"
				_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
				errs <- err
			}()
			go func() {
				defer wg.Done()
				<-start
				newContext := "beta-ctx"
				_, _, err := store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Context: &newContext}, nil)
				errs <- err
			}()
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("PatchCoordinator: %v", err)
				}
			}

			got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
			if err != nil {
				t.Fatalf("GetCoordinator: %v", err)
			}
			if got.Name != "alpha" || got.Context != "beta-ctx" {
				t.Fatalf("GetCoordinator after concurrent PATCH = %+v, want Name=alpha Context=beta-ctx", got)
			}
		})
	}
}

// TestPatchCoordinator_Postgres_SameFieldDeterministicOrder is the PostgreSQL
// twin of TestPatchCoordinator_SameFieldDeterministicOrder (Build decision 7
// test (b)). On PostgreSQL the lock is acquired by the SELECT ... FOR UPDATE
// itself, so afterLock fires only once A's row read returns; B's own
// SELECT ... FOR UPDATE blocks on the row lock until A commits.
func TestPatchCoordinator_Postgres_SameFieldDeterministicOrder(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("rep-%d", i), func(t *testing.T) {
			store := newTestStorePostgres(t)
			ctx := context.Background()
			c := &Coordinator{WorkspaceID: "ws-1", Name: "orig", AgentProfileID: "a", ExecutorProfileID: "e"}
			if err := store.CreateCoordinator(ctx, c); err != nil {
				t.Fatalf("CreateCoordinator: %v", err)
			}

			// Millisecond-scale ticks: PostgreSQL's timestamp column is
			// microsecond precision, so a nanosecond-scale fake clock (as
			// SQLite tolerates) would round A's and B's timestamps to the
			// same stored value and fail the strict-ordering assertion below.
			var tick int64
			store.now = func() time.Time {
				n := atomic.AddInt64(&tick, 1)
				return time.Unix(0, n*int64(time.Millisecond)).UTC()
			}

			locked := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			store.afterLock = func(_ context.Context) {
				once.Do(func() {
					close(locked)
					<-release
				})
			}

			var wg sync.WaitGroup
			var aResult, bResult *Coordinator
			var aErr, bErr error

			wg.Add(1)
			go func() {
				defer wg.Done()
				name := "alpha"
				aResult, _, aErr = store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
			}()

			<-locked // A's SELECT ... FOR UPDATE returned; A holds the row lock.

			wg.Add(1)
			go func() {
				defer wg.Done()
				name := "beta"
				bResult, _, bErr = store.PatchCoordinator(ctx, "ws-1", c.ID, CoordinatorPatch{Name: &name}, nil)
			}()

			close(release)
			wg.Wait()

			if aErr != nil {
				t.Fatalf("PATCH A: %v", aErr)
			}
			if bErr != nil {
				t.Fatalf("PATCH B: %v", bErr)
			}

			got, err := store.GetCoordinator(ctx, "ws-1", c.ID)
			if err != nil {
				t.Fatalf("GetCoordinator: %v", err)
			}
			if got.Name != "beta" {
				t.Fatalf("GetCoordinator.Name = %q, want %q (B commits second)", got.Name, "beta")
			}
			if !got.UpdatedAt.Equal(bResult.UpdatedAt) {
				t.Fatalf("GetCoordinator.UpdatedAt = %v, want B's returned %v", got.UpdatedAt, bResult.UpdatedAt)
			}
			if !bResult.UpdatedAt.After(aResult.UpdatedAt) {
				t.Fatalf("B.UpdatedAt = %v, want strictly after A.UpdatedAt = %v", bResult.UpdatedAt, aResult.UpdatedAt)
			}
		})
	}
}
