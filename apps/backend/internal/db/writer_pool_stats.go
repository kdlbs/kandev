package db

import (
	"expvar"
	"sync"
)

var writerPoolStatsOnce sync.Once

// RegisterWriterPoolStats publishes the writer pool's connection-pool
// counters (sql.DBStats: WaitCount, WaitDuration, InUse, Idle, ...) at
// /debug/vars under "db_writer_pool_stats". SQLite's writer pool is a single
// connection (SetMaxOpenConns(1)), so a rising WaitCount/WaitDuration is the
// signature of callers queued behind it -- the mechanism behind a
// synchronous claim seeing `context deadline exceeded` well before SQLite's
// own busy_timeout would report "database is locked". sqlx.DB embeds
// *sql.DB, so Writer().Stats() is available directly.
//
// expvar.Publish panics on a duplicate variable name, and this is wired from
// a devMode-only startup path that must run at most once per process; the
// sync.Once makes a second call a safe no-op instead of a boot-time panic.
func RegisterWriterPoolStats(pool *Pool) {
	writerPoolStatsOnce.Do(func() {
		expvar.Publish("db_writer_pool_stats", expvar.Func(func() any {
			return pool.Writer().Stats()
		}))
	})
}
