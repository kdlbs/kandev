package telemetrycontract

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
)

// Store owns the telemetry_activations table: creating it, activating
// registered contracts, and reporting their health at boot.
type Store struct {
	writer *sqlx.DB
	reader *sqlx.DB
}

// NewWithDB creates telemetry_activations idempotently and returns a Store.
func NewWithDB(writer, reader *sqlx.DB) (*Store, error) {
	if _, err := writer.Exec(`
		CREATE TABLE IF NOT EXISTS telemetry_activations (
			contract_key     TEXT      NOT NULL,
			contract_version INTEGER   NOT NULL,
			activated_at     TIMESTAMP NOT NULL,
			PRIMARY KEY (contract_key, contract_version)
		)
	`); err != nil {
		return nil, err
	}
	return &Store{writer: writer, reader: reader}, nil
}

// Activate writes one row per registered (key, version) that has none yet,
// stamped with this boot's UTC time. It never rewrites an existing row: a
// version bump appends a second activation row rather than overwriting the
// first, which is the whole point of the composite primary key.
func (s *Store) Activate(ctx context.Context) error {
	now := time.Now().UTC()
	for _, c := range Registry() {
		query := s.writer.Rebind(`
			INSERT INTO telemetry_activations (contract_key, contract_version, activated_at)
			VALUES (?, ?, ?)
			ON CONFLICT (contract_key, contract_version) DO NOTHING
		`)
		if _, err := s.writer.ExecContext(ctx, query, c.Key, c.Version, now); err != nil {
			return err
		}
	}
	return nil
}

// contractHealth is one registered contract's boot-time health snapshot.
type contractHealth struct {
	key         string
	version     int
	exists      bool
	activatedAt *time.Time
	rowCount    int64
	// mostRecentOccurredAt is reported as the driver's raw text rather than a
	// parsed time.Time: it comes from an aggregate MAX(...) over a
	// dialect-declared timestamp column, and SQLite's driver only recognizes
	// a column's declared type (and therefore auto-parses into time.Time) on
	// a direct column read, not through an aggregate function. A log field is
	// observability, not a value consumers parse, so the raw string is enough.
	mostRecentOccurredAt string
}

// LogHealth emits one line per registered contract: object existence,
// activation time, row count, and most recent occurred_at. Never fatal — a
// failure to compute one contract's health logs a warning and the boot
// continues, matching recordSchemaVersion's contract.
func (s *Store) LogHealth(ctx context.Context, log *logger.Logger) {
	if log == nil {
		return
	}
	for _, c := range Registry() {
		health, err := s.contractHealth(ctx, c)
		if err != nil {
			log.Warn("telemetry contract health check failed",
				zap.String("contract_key", c.Key),
				zap.Int("contract_version", c.Version),
				zap.Error(err))
			continue
		}
		fields := []zap.Field{
			zap.String("contract_key", health.key),
			zap.Int("contract_version", health.version),
			zap.Bool("exists", health.exists),
			zap.Int64("row_count", health.rowCount),
		}
		if health.activatedAt != nil {
			fields = append(fields, zap.Time("activated_at", *health.activatedAt))
		}
		if health.mostRecentOccurredAt != "" {
			fields = append(fields, zap.String("most_recent_occurred_at", health.mostRecentOccurredAt))
		}
		log.Info("telemetry.contract.health", fields...)
	}
}

func (s *Store) contractHealth(ctx context.Context, c Contract) (contractHealth, error) {
	health := contractHealth{key: c.Key, version: c.Version}

	exists, err := s.objectsExist(ctx, c)
	if err != nil {
		return health, err
	}
	health.exists = exists

	activatedAt, err := s.activatedAt(ctx, c)
	if err != nil {
		return health, err
	}
	health.activatedAt = activatedAt

	if exists {
		count, mostRecent, err := s.stats(ctx, c)
		if err != nil {
			return health, err
		}
		health.rowCount = count
		health.mostRecentOccurredAt = mostRecent
	}
	return health, nil
}

// objectsExist reports whether a contract's backing objects exist. The
// ExistsQuery is dialect-neutral ("SELECT 1 FROM <table> LIMIT 1"); a missing
// table surfaces as a query error classified by db.IsMissingTableError, and
// an existing-but-empty table surfaces as sql.ErrNoRows, which still means
// the table exists.
func (s *Store) objectsExist(ctx context.Context, c Contract) (bool, error) {
	var discard int
	err := s.reader.QueryRowContext(ctx, c.ExistsQuery).Scan(&discard)
	switch {
	case err == nil, errors.Is(err, sql.ErrNoRows):
		return true, nil
	case dbutil.IsMissingTableError(err):
		return false, nil
	default:
		return false, err
	}
}

func (s *Store) activatedAt(ctx context.Context, c Contract) (*time.Time, error) {
	query := s.reader.Rebind(`
		SELECT activated_at FROM telemetry_activations
		WHERE contract_key = ? AND contract_version = ?
	`)
	var activatedAt time.Time
	err := s.reader.QueryRowContext(ctx, query, c.Key, c.Version).Scan(&activatedAt)
	switch {
	case err == nil:
		return &activatedAt, nil
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	default:
		return nil, err
	}
}

func (s *Store) stats(ctx context.Context, c Contract) (int64, string, error) {
	var count int64
	var mostRecent sql.NullString
	if err := s.reader.QueryRowContext(ctx, c.StatsQuery).Scan(&count, &mostRecent); err != nil {
		return 0, "", err
	}
	if !mostRecent.Valid {
		return count, "", nil
	}
	return count, mostRecent.String, nil
}
