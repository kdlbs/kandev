package db

import (
	"encoding/json"
	"expvar"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
)

// TestRegisterWriterPoolStatsExposesDBStatsAtDebugVars covers Defect 3's
// diagnostic half: nothing today exposes the writer pool's contention
// counters, so a WaitCount/WaitDuration spike (the actual mechanism behind
// the claim's `context deadline exceeded`, as opposed to a SQLite
// busy_timeout "database is locked") was invisible. RegisterWriterPoolStats
// must publish a JSON-decodable sql.DBStats snapshot under the documented
// expvar name.
func TestRegisterWriterPoolStatsExposesDBStatsAtDebugVars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "writer-pool-stats.db")
	conn, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	writer := sqlx.NewDb(conn, "sqlite3")
	pool := NewPool(writer, writer)

	RegisterWriterPoolStats(pool)

	v := expvar.Get("db_writer_pool_stats")
	if v == nil {
		t.Fatal("db_writer_pool_stats not published")
	}
	var stats struct {
		MaxOpenConnections int
		OpenConnections    int
		InUse              int
		Idle               int
		WaitCount          int64
		WaitDuration       int64
	}
	if err := json.Unmarshal([]byte(v.String()), &stats); err != nil {
		t.Fatalf("unmarshal db_writer_pool_stats: %v (raw: %s)", err, v.String())
	}
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1 (single-writer pool)", stats.MaxOpenConnections)
	}

	// Guards the sync.Once: a second call (e.g. a future second devMode
	// wiring path) must not panic with expvar's "Reuse of exported var name"
	// error. This has to be the same test as the first call above, not a
	// separate one: expvar.Publish has no unpublish, so whichever test in
	// this process registers "db_writer_pool_stats" first permanently
	// consumes the package's sync.Once, and a same-named second test can
	// only ever see it already consumed -- it would pass whether or not the
	// guard actually works.
	RegisterWriterPoolStats(pool)
}
