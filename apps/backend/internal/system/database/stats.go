// Package database serves the System -> Database page. It exposes read-only
// database stats plus SQLite maintenance operations: VACUUM, PRAGMA optimize,
// and Factory Reset. Long-running operations are tracked via the jobs.Tracker
// so the frontend can observe progress over the event bus.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/system/jobs"
	"go.uber.org/zap"
)

const (
	databaseDriverPostgres  = "postgres"
	databaseDriverSQLite    = "sqlite"
	databaseMetadataTimeout = 2 * time.Second
)

const logicalStatsErrorScanFailed = "scan_failed"

var errDatabaseMetadataInvalidated = errors.New("database metadata invalidated during read")

// Stats is the read-only database-state payload returned to the frontend.
//
// LastBackupAt is a pointer so the JSON shape is `null` when no backup
// exists. Serialising a zero time.Time as "0001-01-01T00:00:00Z" would
// defeat the frontend's "Never" fallback in database-stats-card.tsx.
type Stats struct {
	Driver                 string     `json:"driver"`
	Path                   string     `json:"path"`
	BackupDirectory        string     `json:"backup_directory"`
	SizeBytes              int64      `json:"size_bytes"`
	WALSizeBytes           int64      `json:"wal_size_bytes"`
	MessageContentBytes    *int64     `json:"message_content_bytes"`
	MessageMetadataBytes   *int64     `json:"message_metadata_bytes"`
	MessagePayloadBytes    *int64     `json:"message_payload_bytes"`
	GitSnapshotBytes       *int64     `json:"git_snapshot_bytes"`
	LogicalStatsState      string     `json:"logical_stats_state"`
	LogicalStatsMeasuredAt *time.Time `json:"logical_stats_measured_at"`
	LogicalStatsError      string     `json:"logical_stats_error,omitempty"`
	MetadataStale          bool       `json:"metadata_stale"`
	MetadataMeasuredAt     *time.Time `json:"metadata_measured_at"`
	SchemaVersion          string     `json:"schema_version"`
	LastBackupAt           *time.Time `json:"last_backup_at"`
}

type databaseMetadata struct {
	driver          string
	path            string
	backupDirectory string
	sizeBytes       int64
	walSizeBytes    int64
	schemaVersion   string
	lastBackupAt    *time.Time
}

type databaseMetadataSnapshot struct {
	metadata   databaseMetadata
	measuredAt time.Time
}

// ResetDirs lists the on-disk directories factory-reset wipes. The Service
// only needs paths to call os.RemoveAll on — the construction site (cmd/kandev)
// fills these in from the resolved data/home dirs.
type ResetDirs struct {
	Worktrees string
	Repos     string
	Sessions  string
	Tasks     string
	QuickChat string
}

// Service is the maintenance + stats facade for the System -> Database page.
//
// FactoryReset does not auto-restart the backend. The job result includes
// restart_required=true; the frontend dialog reads it and asks the user to
// quit and relaunch Kandev. The previous syscall.Exec approach was brittle
// under desktop launchers and `make dev` watchers.
type Service struct {
	pool               *db.Pool
	databasePath       string
	dirs               ResetDirs
	jobs               *jobs.Tracker
	log                *logger.Logger
	logicalStats       *logicalStatsCache
	metadataMu         sync.RWMutex
	metadata           *databaseMetadataSnapshot
	metadataGeneration uint64
	healthy            func() bool

	// PersistenceUnavailable marks required stores unhealthy before a
	// destructive maintenance operation leaves the process awaiting restart.
	PersistenceUnavailable func()

	// OrchestratorShutdown stops the orchestrator and active executions before
	// the factory-reset job runs. Wired by cmd/kandev. Tests pass a no-op.
	OrchestratorShutdown func()
	// DatabaseQuiesce stops database-backed workers before the factory-reset
	// job snapshots or changes the shared schema. Wired by cmd/kandev.
	DatabaseQuiesce func() error
}

// NewService constructs a Service for the configured SQLite database path.
// The sibling backups directory is derived from databasePath. dirs lists the
// on-disk subtrees factory-reset wipes.
func NewService(pool *db.Pool, databasePath string, dirs ResetDirs, j *jobs.Tracker, log *logger.Logger) *Service {
	s := &Service{
		pool:         pool,
		databasePath: databasePath,
		dirs:         dirs,
		jobs:         j,
		log:          log,
	}
	s.logicalStats = newLogicalStatsCache(context.Background(), s.scanLogicalStats, logicalStatsCacheOptions{
		onStart: func() {
			if s.log != nil {
				s.log.Info("database logical stats scan started")
			}
		},
		onCompletion: func(outcome string, duration time.Duration, snapshot *logicalStatsSnapshot, scanErr error) {
			recordLogicalStatsScanOutcome(outcome)
			if snapshot != nil {
				recordLogicalStorageMetrics(snapshot.values, snapshot.measuredAt)
			}
			if s.log == nil {
				return
			}
			fields := []zap.Field{zap.String("outcome", outcome), zap.Duration("duration", duration)}
			if scanErr != nil && !errors.Is(scanErr, errLogicalStatsDeferred) {
				fields = append(fields, zap.Error(scanErr))
			}
			s.log.Info("database logical stats scan completed", fields...)
		},
	})
	return s
}

// SetPersistenceHealthProbe prevents background scans and metadata reads while
// the shared persistence health tracker reports an unhealthy database.
func (s *Service) SetPersistenceHealthProbe(healthy func() bool) {
	s.healthy = healthy
}

// RetryLogicalStats bypasses automatic scan backoff for an explicit request.
func (s *Service) RetryLogicalStats() bool {
	if s == nil || s.logicalStats == nil || !s.persistenceHealthy() {
		return false
	}
	return s.logicalStats.Retry()
}

// StopBackground cancels and joins the process-local logical statistics scan.
func (s *Service) StopBackground() {
	if s.logicalStats != nil {
		s.logicalStats.Close()
	}
}

// InvalidateDatabase drops database-derived state after reset or restore.
func (s *Service) InvalidateDatabase() {
	s.metadataMu.Lock()
	s.metadataGeneration++
	s.metadata = nil
	s.metadataMu.Unlock()
	if s.logicalStats != nil {
		s.logicalStats.Invalidate(true)
	}
}

func (s *Service) backupsDir() string {
	return filepath.Join(filepath.Dir(s.databasePath), "backups")
}

func (s *Service) absoluteBackupsDir() (string, error) {
	backupDir := s.backupsDir()
	absolute, err := filepath.Abs(backupDir)
	if err != nil {
		return "", fmt.Errorf("resolve backup directory %q: %w", backupDir, err)
	}
	return absolute, nil
}

// Stats returns the current database stats without a request cancellation
// context. HTTP handlers should use StatsContext so metadata reads inherit the
// request deadline.
func (s *Service) Stats() (Stats, error) {
	return s.StatsContext(context.Background())
}

// StatsContext reads bounded live metadata and the current logical snapshot.
// Logical totals are refreshed asynchronously by one process-local worker.
func (s *Service) StatsContext(ctx context.Context) (Stats, error) {
	metadata, measuredAt, stale, err := s.readMetadata(ctx)
	if err != nil {
		return Stats{}, err
	}
	var logical logicalStatsRead
	if s.healthy != nil && !s.healthy() {
		logical = s.logicalStats.ReadStale()
	} else {
		logical = s.logicalStats.Read()
	}
	out := Stats{
		Driver: metadata.driver, Path: metadata.path, BackupDirectory: metadata.backupDirectory,
		SizeBytes: metadata.sizeBytes, WALSizeBytes: metadata.walSizeBytes,
		SchemaVersion: metadata.schemaVersion, LastBackupAt: metadata.lastBackupAt,
		LogicalStatsState: logical.state, LogicalStatsError: logical.error,
		MetadataStale: stale, MetadataMeasuredAt: measuredAt,
	}
	if logical.snapshot != nil {
		out.MessageContentBytes = int64Pointer(logical.snapshot.values.messageContent)
		out.MessageMetadataBytes = int64Pointer(logical.snapshot.values.messageMetadata)
		out.MessagePayloadBytes = int64Pointer(logical.snapshot.values.messagePayload)
		out.GitSnapshotBytes = int64Pointer(logical.snapshot.values.gitSnapshot)
		measuredAt := logical.snapshot.measuredAt
		out.LogicalStatsMeasuredAt = &measuredAt
	}
	return out, nil
}

func int64Pointer(value int64) *int64 { return &value }

func (s *Service) readMetadata(ctx context.Context) (databaseMetadata, *time.Time, bool, error) {
	generation := s.metadataGenerationValue()
	metadata, err := s.metadataBase()
	if err != nil {
		return databaseMetadata{}, nil, false, err
	}
	if s.pool == nil || s.pool.Reader() == nil {
		return s.readMetadataWithoutPool(metadata)
	}
	if !s.persistenceHealthy() {
		return s.readCachedMetadata()
	}
	metadata, err = s.measureMetadata(ctx, metadata)
	if err != nil {
		s.warnMetadataReadFailure(err)
		return s.readCachedMetadata()
	}
	measuredAt := time.Now().UTC()
	if !s.storeMetadata(metadata, measuredAt, generation) {
		return databaseMetadata{}, nil, false, errDatabaseMetadataInvalidated
	}
	return metadata, &measuredAt, false, nil
}

func (s *Service) readMetadataWithoutPool(metadata databaseMetadata) (databaseMetadata, *time.Time, bool, error) {
	s.addSQLiteMetadata(&metadata)
	measuredAt := time.Now().UTC()
	return metadata, &measuredAt, false, nil
}

func (s *Service) readCachedMetadata() (databaseMetadata, *time.Time, bool, error) {
	cached, ok := s.cachedMetadata()
	if !ok {
		return databaseMetadata{}, nil, false, errors.New("database metadata unavailable")
	}
	measuredAt := cached.measuredAt
	return cached.metadata, &measuredAt, true, nil
}

func (s *Service) measureMetadata(ctx context.Context, metadata databaseMetadata) (databaseMetadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	queryCtx, cancel := context.WithTimeout(ctx, databaseMetadataTimeout)
	defer cancel()
	reader := s.pool.Reader()
	size, err := readDatabaseSizeContext(queryCtx, reader)
	if err != nil {
		return databaseMetadata{}, err
	}
	metadata.sizeBytes = size
	metadata.schemaVersion, err = readSchemaVersionContext(queryCtx, reader)
	if err != nil {
		return databaseMetadata{}, err
	}
	s.addSQLiteMetadata(&metadata)
	return metadata, nil
}

func (s *Service) addSQLiteMetadata(metadata *databaseMetadata) {
	if metadata.driver != databaseDriverSQLite {
		return
	}
	if wal, err := walSize(s.databasePath); err == nil {
		metadata.walSizeBytes = wal
	}
	if last := lastBackupAt(metadata.backupDirectory); !last.IsZero() {
		metadata.lastBackupAt = &last
	}
}

func (s *Service) metadataGenerationValue() uint64 {
	s.metadataMu.RLock()
	generation := s.metadataGeneration
	s.metadataMu.RUnlock()
	return generation
}

func (s *Service) storeMetadata(metadata databaseMetadata, measuredAt time.Time, generation uint64) bool {
	s.metadataMu.Lock()
	defer s.metadataMu.Unlock()
	if s.metadataGeneration != generation {
		return false
	}
	s.metadata = &databaseMetadataSnapshot{metadata: metadata, measuredAt: measuredAt}
	databaseSizeBytes.Set(metadata.sizeBytes)
	databaseWALSizeBytes.Set(metadata.walSizeBytes)
	return true
}

func (s *Service) warnMetadataReadFailure(err error) {
	if s.log != nil {
		s.log.Warn("database metadata read failed", zap.Error(err))
	}
}

func (s *Service) metadataBase() (databaseMetadata, error) {
	driver := s.databaseDriver()
	metadata := databaseMetadata{driver: driver}
	if driver == databaseDriverSQLite {
		resolved, err := s.absoluteBackupsDir()
		if err != nil {
			return databaseMetadata{}, err
		}
		metadata.path = s.databasePath
		metadata.backupDirectory = resolved
	}
	return metadata, nil
}

func (s *Service) cachedMetadata() (databaseMetadataSnapshot, bool) {
	s.metadataMu.RLock()
	defer s.metadataMu.RUnlock()
	if s.metadata == nil {
		return databaseMetadataSnapshot{}, false
	}
	copy := *s.metadata
	return copy, true
}

type logicalStorageStats struct {
	messageContent  int64
	messageMetadata int64
	messagePayload  int64
	gitSnapshot     int64
}

func (s *Service) databaseDriver() string {
	if s.pool == nil || s.pool.Writer() == nil {
		return databaseDriverSQLite
	}
	switch driver := s.pool.Writer().DriverName(); {
	case dialect.IsPostgres(driver):
		return databaseDriverPostgres
	case driver == dialect.SQLite3:
		return databaseDriverSQLite
	default:
		return driver
	}
}

func readDatabaseSize(d *sqlx.DB) (int64, error) {
	if dialect.IsPostgres(d.DriverName()) {
		return readPostgresDBSize(d)
	}
	return readSQLiteDBSize(d)
}

func readDatabaseSizeContext(ctx context.Context, d *sqlx.DB) (int64, error) {
	if dialect.IsPostgres(d.DriverName()) {
		var size int64
		if err := d.QueryRowxContext(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
			return 0, fmt.Errorf("pg_database_size: %w", err)
		}
		return size, nil
	}
	var pages, pageSize int64
	if err := d.QueryRowxContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return 0, fmt.Errorf("pragma page_count: %w", err)
	}
	if err := d.QueryRowxContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("pragma page_size: %w", err)
	}
	return pages * pageSize, nil
}

// readSQLiteDBSize returns the database size in bytes via PRAGMA page_count *
// page_size. Both PRAGMAs return a single integer row.
func readSQLiteDBSize(d interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}) (int64, error) {
	var pages, pageSize int64
	if err := d.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		return 0, fmt.Errorf("pragma page_count: %w", err)
	}
	if err := d.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("pragma page_size: %w", err)
	}
	return pages * pageSize, nil
}

func readPostgresDBSize(d interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}) (int64, error) {
	var size int64
	if err := d.QueryRow("SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		return 0, fmt.Errorf("pg_database_size: %w", err)
	}
	return size, nil
}

// readSchemaVersion reads the binary version recorded by
// cmd/kandev/storage.go:recordSchemaVersion. Missing key returns "" with
// no error (fresh DB on first boot).
func readSchemaVersion(d *sqlx.DB) (string, error) {
	return readSchemaVersionContext(context.Background(), d)
}

func readSchemaVersionContext(ctx context.Context, d *sqlx.DB) (string, error) {
	var value string
	err := d.QueryRowxContext(ctx, d.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), "kandev_version").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read kandev_version: %w", err)
	}
	return value, nil
}

// walSize stats <dbPath>-wal and returns its size in bytes; a missing WAL
// file is treated as zero (the DB may have been just checkpointed).
func walSize(dbPath string) (int64, error) {
	info, err := os.Stat(dbPath + "-wal")
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

// lastBackupAt returns the mtime of the newest file in backupDir, or the
// zero time if the directory is missing/empty. A directory read error is
// treated as "no backups" — the Stats endpoint is best-effort.
func lastBackupAt(backupDir string) time.Time {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return time.Time{}
	}
	mtimes := make([]time.Time, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		mtimes = append(mtimes, fi.ModTime())
	}
	if len(mtimes) == 0 {
		return time.Time{}
	}
	sort.Slice(mtimes, func(i, j int) bool { return mtimes[i].After(mtimes[j]) })
	return mtimes[0]
}

// HandleStats returns a gin handler for GET /api/v1/system/database.
func HandleStats(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := s.StatsContext(ctxOrBackground(c))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

// HandleRefreshStats starts a background logical scan without waiting for its result.
func HandleRefreshStats(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.RetryLogicalStats() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database statistics unavailable"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// startJobResponse is the canonical 202 payload for POST endpoints that
// spawn a background job.
type startJobResponse struct {
	JobID string `json:"job_id"`
}

func respondAccepted(c *gin.Context, jobID string) {
	c.JSON(http.StatusAccepted, startJobResponse{JobID: jobID})
}

// ctxOrBackground returns the context from the gin request when available,
// falling back to context.Background() for direct service callers.
func ctxOrBackground(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
