package sshconnections

import (
	"context"
	"encoding/json"
	"expvar"
	"sync/atomic"
	"time"
)

const (
	migrationCountPending  = "pending"
	migrationCountVerified = "verified"
	migrationCountBlocked  = "blocked"
)

var migrationMetricsStore atomic.Pointer[Store]

func init() {
	expvar.Publish("ssh_migration_imports", expvar.Func(migrationMetricsSnapshot))
}

// ActivateMigrationMetrics exposes durable import counts from this backend's store.
func (s *Store) ActivateMigrationMetrics() {
	migrationMetricsStore.Store(s)
}

func migrationMetricsSnapshot() any {
	if store := migrationMetricsStore.Load(); store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if counts, err := store.migrationCounts(ctx); err == nil {
			return counts
		}
	}
	return newMigrationCounts(-1)
}

func newMigrationCounts(initial int) map[string]int {
	return map[string]int{
		migrationCountPending: initial, migrationCountVerified: initial, migrationCountBlocked: initial,
	}
}

func (s *Store) migrationCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryxContext(ctx, `SELECT m.data, r.connection_id, r.route_stamp
		FROM ssh_migrations m
		JOIN executors e ON e.id=m.executor_id AND e.deleted_at IS NULL
		JOIN ssh_connection_refs r ON r.owner_id=m.executor_id AND r.owner_type='executor'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	counts := newMigrationCounts(0)
	for rows.Next() {
		var data, connectionID, stamp string
		if err := rows.Scan(&data, &connectionID, &stamp); err != nil {
			return nil, err
		}
		var migration sshMigration
		if err := json.Unmarshal([]byte(data), &migration); err != nil {
			return nil, err
		}
		if migration.ConnectionID == connectionID {
			counts[migrationCountState(migration, stamp)]++
		}
	}
	return counts, rows.Err()
}

func migrationCountState(migration sshMigration, stamp string) string {
	switch {
	case migration.Verified || migration.RouteStamp != stamp:
		return migrationCountVerified
	case migration.Blocked:
		return migrationCountBlocked
	default:
		return migrationCountPending
	}
}
