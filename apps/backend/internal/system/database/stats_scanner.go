package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/system/maintenance"
)

const (
	logicalStatsBatchSize    = 128
	logicalStatsBatchTimeout = 2 * time.Second
	logicalStatsBatchYield   = time.Millisecond
)

type logicalStatsTable struct {
	name        string
	key         string
	expressions []string
}

type logicalStatsBatch struct {
	table   logicalStatsTable
	first   bool
	lastID  string
	maxID   string
	size    int
	timeout time.Duration
}

var logicalStatsTables = [...]logicalStatsTable{
	{
		name: "task_session_messages", key: "id",
		expressions: []string{"LENGTH(content)", "LENGTH(metadata)"},
	},
	{
		name: "task_message_payloads", key: "digest",
		expressions: []string{"LENGTH(compressed_content)"},
	},
	{
		name: "task_session_git_snapshots", key: "id",
		expressions: []string{"LENGTH(files) + LENGTH(metadata)"},
	},
}

func (s *Service) scanLogicalStats(ctx context.Context) (logicalStorageStats, error) {
	if s.pool == nil || s.pool.Reader() == nil {
		return logicalStorageStats{}, fmt.Errorf("logical stats: database reader unavailable")
	}
	return scanLogicalStorageStats(ctx, s.pool, logicalStatsBatchSize, logicalStatsBatchTimeout, logicalStatsBatchYield, s.persistenceHealthy)
}

func scanLogicalStorageStats(
	ctx context.Context,
	pool *db.Pool,
	batchSize int,
	batchTimeout time.Duration,
	batchYield time.Duration,
	healthy func() bool,
) (logicalStorageStats, error) {
	if pool == nil || pool.Reader() == nil {
		return logicalStorageStats{}, fmt.Errorf("logical stats: database reader unavailable")
	}
	if batchSize <= 0 {
		return logicalStorageStats{}, fmt.Errorf("logical stats: batch size must be positive")
	}
	var result logicalStorageStats
	for _, table := range logicalStatsTables {
		values, err := scanLogicalStatsTable(ctx, pool, table, batchSize, batchTimeout, batchYield, healthy)
		if err != nil {
			return logicalStorageStats{}, err
		}
		switch table.name {
		case "task_session_messages":
			result.messageContent = values[0]
			result.messageMetadata = values[1]
		case "task_message_payloads":
			result.messagePayload = values[0]
		case "task_session_git_snapshots":
			result.gitSnapshot = values[0]
		}
	}
	return result, nil
}

func scanLogicalStatsTable(
	ctx context.Context,
	pool *db.Pool,
	table logicalStatsTable,
	batchSize int,
	batchTimeout time.Duration,
	batchYield time.Duration,
	healthy func() bool,
) ([]int64, error) {
	values := make([]int64, len(table.expressions))
	maxID, exists, err := readLogicalStatsMaxID(ctx, pool, table, batchTimeout, healthy)
	if err != nil || !exists {
		return values, err
	}
	var lastID string
	first := true
	for {
		count, nextID, err := scanLogicalStatsBatch(ctx, pool, logicalStatsBatch{
			table: table, first: first, lastID: lastID, maxID: maxID,
			size: batchSize, timeout: batchTimeout,
		}, healthy, values)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			break
		}
		lastID = nextID
		first = false
		if count < batchSize {
			break
		}
		if err := waitLogicalStatsYield(ctx, batchYield); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func scanLogicalStatsBatch(
	ctx context.Context,
	pool *db.Pool,
	batch logicalStatsBatch,
	healthy func() bool,
	totals []int64,
) (int, string, error) {
	if healthy != nil && !healthy() {
		return 0, "", errLogicalStatsDeferred
	}
	batchCtx, cancel := context.WithTimeout(ctx, batch.timeout)
	release, err := acquireLogicalStatsAdmission(pool)
	if err != nil {
		cancel()
		return 0, "", err
	}
	query := logicalStatsBatchQuery(batch.table, batch.first)
	args := make([]any, 0, 3)
	if !batch.first {
		args = append(args, batch.lastID)
	}
	args = append(args, batch.maxID, batch.size)
	rows, queryErr := pool.Reader().QueryxContext(batchCtx, pool.Reader().Rebind(query), args...)
	if queryErr != nil {
		cancel()
		release()
		if healthy != nil && !healthy() {
			return 0, "", errLogicalStatsDeferred
		}
		return 0, "", fmt.Errorf("read logical stats from %s: %w", batch.table.name, queryErr)
	}
	count, nextID, scanErr := sumLogicalStatsBatch(rows, totals)
	closeErr := rows.Close()
	cancel()
	release()
	if scanErr != nil {
		return 0, "", fmt.Errorf("scan logical stats from %s: %w", batch.table.name, scanErr)
	}
	if closeErr != nil {
		return 0, "", fmt.Errorf("close logical stats rows from %s: %w", batch.table.name, closeErr)
	}
	return count, nextID, nil
}

func readLogicalStatsMaxID(
	ctx context.Context,
	pool *db.Pool,
	table logicalStatsTable,
	timeout time.Duration,
	healthy func() bool,
) (string, bool, error) {
	if healthy != nil && !healthy() {
		return "", false, errLogicalStatsDeferred
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	release, err := acquireLogicalStatsAdmission(pool)
	if err != nil {
		return "", false, err
	}
	defer release()
	var maxID sql.NullString
	query := fmt.Sprintf("SELECT MAX(%s) FROM %s", table.key, table.name)
	if err := pool.Reader().QueryRowxContext(queryCtx, query).Scan(&maxID); err != nil {
		if healthy != nil && !healthy() {
			return "", false, errLogicalStatsDeferred
		}
		return "", false, fmt.Errorf("read logical stats key from %s: %w", table.name, err)
	}
	return maxID.String, maxID.Valid, nil
}

func logicalStatsBatchQuery(table logicalStatsTable, first bool) string {
	query := "SELECT " + table.key
	for _, expression := range table.expressions {
		query += ", COALESCE(" + expression + ", 0)"
	}
	if first {
		query += " FROM " + table.name + " WHERE " + table.key + " <= ? ORDER BY " + table.key + " LIMIT ?"
	} else {
		query += " FROM " + table.name + " WHERE " + table.key + " > ? AND " + table.key + " <= ? ORDER BY " + table.key + " LIMIT ?"
	}
	return query
}

func sumLogicalStatsBatch(rows *sqlx.Rows, totals []int64) (int, string, error) {
	count := 0
	lastID := ""
	for rows.Next() {
		var id string
		values := make([]int64, len(totals))
		dest := make([]any, 0, len(values)+1)
		dest = append(dest, &id)
		for index := range values {
			dest = append(dest, &values[index])
		}
		if err := rows.Scan(dest...); err != nil {
			return count, lastID, err
		}
		for index, value := range values {
			totals[index] += value
		}
		count++
		lastID = id
	}
	if err := rows.Err(); err != nil {
		return count, lastID, err
	}
	return count, lastID, nil
}

func waitLogicalStatsYield(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func acquireLogicalStatsAdmission(pool *db.Pool) (func(), error) {
	if pool == nil || pool.Reader() == nil {
		return nil, fmt.Errorf("logical stats: database reader unavailable")
	}
	if pool.Writer() == nil || pool.Writer().DriverName() != dialect.SQLite3 {
		return func() {}, nil
	}
	release, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		return nil, errLogicalStatsDeferred
	}
	return release, nil
}

func (s *Service) persistenceHealthy() bool {
	return s.healthy == nil || s.healthy()
}
