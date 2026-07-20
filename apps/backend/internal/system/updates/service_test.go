package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
)

// newTestPool builds an in-memory SQLite pool with kandev_meta migrated.
// Both Writer/Reader share the same connection because we never need WAL
// snapshots in these tests.
func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// kandev_meta needs to exist before Write/Read; reuse persistence's
	// public roundtrip on a fresh DB by calling WriteLatestVersion with
	// zero values to lazily create the table is not possible — meta.go
	// keeps ensureMetaTable internal. Create the table directly with the
	// same DDL.
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	return db.NewPool(conn, conn)
}

// newStubGitHub stands up an httptest.Server that returns the given tag/url
// payload on every request and records the call count via the returned
// atomic counter.
func newStubGitHub(t *testing.T, tag, url string) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&count, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"` + url + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func TestService_Get_FreshDB_ReturnsZeroValues(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())

	resp, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Current != "v1.0.0" {
		t.Errorf("current=%q", resp.Current)
	}
	if resp.Latest != "" || resp.LatestURL != "" {
		t.Errorf("expected zero latest, got %+v", resp)
	}
	if !resp.LatestCheckedAt.IsZero() {
		t.Errorf("expected zero checkedAt, got %v", resp.LatestCheckedAt)
	}
	if resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=false on fresh DB")
	}
}

func TestService_Check_PersistsToMeta(t *testing.T) {
	pool := newTestPool(t)
	srv, count := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)

	resp, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.Latest != "v1.0.1" || resp.LatestURL != "https://example/v1.0.1" {
		t.Errorf("unexpected resp %+v", resp)
	}
	if !resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=true (v1.0.1 > v1.0.0)")
	}
	if atomic.LoadInt32(count) != 1 {
		t.Errorf("expected 1 github call, got %d", *count)
	}

	// Persistence side-effect: a fresh Get should return the same values.
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Latest != "v1.0.1" {
		t.Errorf("Get latest=%q", got.Latest)
	}
}

func TestService_Check_RateLimited(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("first Check: %v", err)
	}
	_, err := svc.Check(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestService_Check_FailurePreservesPersisted(t *testing.T) {
	pool := newTestPool(t)
	// First a successful stub to seed kandev_meta.
	okSrv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", okSrv.Client(), logger.Default())
	svc.SetReleaseURL(okSrv.URL)
	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("seed Check: %v", err)
	}
	// Drain limiter so the next Check is allowed.
	svc.limiter = NewLimiter(ManualCheckWindow)

	// Now swap in a failing stub.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	svc.SetReleaseURL(failSrv.URL)

	resp, err := svc.Check(context.Background())
	if err == nil {
		t.Fatalf("expected error on failing fetch")
	}
	if resp.Latest != "v1.0.1" {
		t.Errorf("expected preserved latest=v1.0.1, got %q", resp.Latest)
	}

	// kandev_meta itself must remain unchanged.
	v, u, _, rerr := persistence.ReadLatestVersion(pool.Reader())
	if rerr != nil {
		t.Fatalf("read meta: %v", rerr)
	}
	if v != "v1.0.1" || u != "https://example/v1.0.1" {
		t.Errorf("kandev_meta mutated: v=%q u=%q", v, u)
	}
}

func TestService_UpdateAvailable_Cases(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer", "v1.0.0", "v1.0.1", true},
		{"equal", "v1.0.1", "v1.0.1", false},
		{"older", "v1.0.1", "v1.0.0", false},
		{"dev_current", "dev", "v1.0.1", false},
		{"empty_current", "", "v1.0.1", false},
		{"invalid_latest", "v1.0.0", "not-a-version", false},
		{"no_prefix", "1.0.0", "1.0.1", true},
		// git-describe build 60 commits ahead of the latest release: not behind.
		{"ahead_of_latest", "v0.79.0-60-g8fae44fb1", "v0.79.0", false},
		{"ahead_of_latest_dirty", "v0.79.0-60-g8fae44fb1-dirty", "v0.79.0", false},
		// A dev build of an older tag is still behind a newer release.
		{"ahead_but_older_tag", "v0.79.0-60-g8fae44fb1", "v0.80.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil, tc.current, nil, logger.Default())
			if got := svc.updateAvailable(tc.latest); got != tc.want {
				t.Errorf("updateAvailable(%q,%q)=%v want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestService_StartPoller_TicksEvery6h(t *testing.T) {
	// We use an injected Fetcher rather than an httptest.Server because
	// real HTTP I/O sits outside the synctest fake-time bubble and would
	// prevent synctest.Wait from settling on parked goroutines.
	synctest.Test(t, func(t *testing.T) {
		pool := newTestPool(t)
		var count int32
		svc := NewService(pool, "v1.0.0", nil, logger.Default())
		svc.SetFetcher(func(_ context.Context) (string, string, error) {
			atomic.AddInt32(&count, 1)
			return "v1.0.1", "https://example/v1.0.1", nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		svc.StartPoller(ctx)
		defer svc.StopPoller()

		// Initial probe runs immediately after Start.
		synctest.Wait()
		if got := atomic.LoadInt32(&count); got != 1 {
			t.Fatalf("expected 1 initial call, got %d", got)
		}

		// Advance fake time by one full interval; one extra tick should fire.
		time.Sleep(PollInterval)
		synctest.Wait()
		if got := atomic.LoadInt32(&count); got != 2 {
			t.Fatalf("expected 2 calls after 6h, got %d", got)
		}

		// And one more interval = one more call.
		time.Sleep(PollInterval)
		synctest.Wait()
		if got := atomic.LoadInt32(&count); got != 3 {
			t.Fatalf("expected 3 calls after 12h, got %d", got)
		}
	})
}

func TestService_tickOnce_PersistsAndSurvivesFailures(t *testing.T) {
	pool := newTestPool(t)
	srv, count := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)

	svc.tickOnce(context.Background())
	if atomic.LoadInt32(count) != 1 {
		t.Fatalf("expected 1 github call, got %d", *count)
	}
	v, _, _, err := persistence.ReadLatestVersion(pool.Reader())
	if err != nil {
		t.Fatalf("ReadLatestVersion: %v", err)
	}
	if v != "v1.0.1" {
		t.Errorf("expected persisted v1.0.1, got %q", v)
	}
}
