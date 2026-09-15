package alertsource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// ErrReservationAttachedElsewhere and ErrReservationGone are the two
// sentinels AttachReservationTaskID's zero-row diagnostic can return (D3(b),
// A5). ErrReservationGone means a DEFINITE no-live-row observation: T06
// stops retrying and skips ReleaseOrphanedReservation on either sentinel,
// but keeps retrying on a plain (wrapped) store error — so a transient
// query failure must never be reported as either one.
var (
	ErrReservationAttachedElsewhere = errors.New("alertsource: reservation attached to a different task")
	ErrReservationGone              = errors.New("alertsource: reservation released or absent")
)

// Store persists the three tables shared by every registered alert source:
// alert_sources, alert_watches and alert_reservations. Source/watch CRUD is
// T06 (D19); Store exposes only the five reservation-lifecycle operations
// D3 assigns to T04.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("alertsource schema init: %w", err)
	}
	return s, nil
}

// alertSourcesColumns follows the sentry_configs/sentry_issue_watches
// identity convention exactly: TEXT PRIMARY KEY, TEXT NOT NULL references
// (D14). health_status is the D7 closed set, guarded by both a CHECK and Go
// constants; last_checked_at is deliberately absent (R10) — do not add it
// back.
const alertSourcesColumns = `
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	config_json TEXT NOT NULL DEFAULT '{}',
	enabled BOOLEAN NOT NULL DEFAULT 1,
	health_status TEXT NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown','ok','auth_failed','error')),
	last_error TEXT NOT NULL DEFAULT '',
	last_error_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(workspace_id, name)`

// alertWatchesColumns is the 18 columns alert_watches shares with
// sentry_issue_watches/linear's issue-watch table, plus source_id — the
// structural analogue of sentry_instance_id, but NOT NULL: unlike Sentry
// there is no legacy unbound row (D8, R9).
const alertWatchesColumns = `
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	source_id TEXT NOT NULL,
	workflow_id TEXT NOT NULL,
	workflow_step_id TEXT NOT NULL,
	repository_id TEXT NOT NULL DEFAULT '',
	base_branch TEXT NOT NULL DEFAULT '',
	filter_json TEXT NOT NULL DEFAULT '{}',
	agent_profile_id TEXT NOT NULL DEFAULT '',
	executor_profile_id TEXT NOT NULL DEFAULT '',
	prompt TEXT NOT NULL DEFAULT '',
	enabled BOOLEAN NOT NULL DEFAULT 1,
	poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
	max_inflight_tasks INTEGER DEFAULT 5,
	last_polled_at DATETIME,
	last_error TEXT NOT NULL DEFAULT '',
	last_error_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	FOREIGN KEY(source_id) REFERENCES alert_sources(id) ON DELETE RESTRICT`

// alertReservationsColumnsFmt is a format string: %s is the dialect-specific
// COLLATE clause for fingerprint — the one genuinely non-portable concern in
// this schema (D14), so it is the one manual dialect branch rather than a
// dialect.RenderSchema token. alert_reservations deliberately has no
// updated_at (D14): its only two mutations each already have their own
// observable column (task_id, released_at).
const alertReservationsColumnsFmt = `
	id TEXT PRIMARY KEY,
	watch_id TEXT NOT NULL,
	fingerprint TEXT %s NOT NULL CHECK (length(fingerprint) = 64),
	task_id TEXT NOT NULL DEFAULT '',
	alert_raw TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	released_at DATETIME,
	FOREIGN KEY(watch_id) REFERENCES alert_watches(id) ON DELETE CASCADE`

// fingerprintCollate returns fingerprint's dialect-specific COLLATE clause.
// An unqualified Postgres TEXT column inherits the database's default
// collation, which may be case-insensitive or nondeterministic; a
// case-insensitive comparison in the partial unique index would collapse
// two distinct fingerprints and silently suppress an alert (D14, following
// the tasks.external_id precedent).
func fingerprintCollate(driver string) string {
	if dialect.IsPostgres(driver) {
		return `COLLATE "C"`
	}
	return `COLLATE BINARY`
}

// createTablesSQL builds the three-table schema. The reserve path (D1) must
// carry the partial index's predicate in its ON CONFLICT target — against a
// partial index, `ON CONFLICT(watch_id, fingerprint) DO NOTHING` alone is a
// hard parse error in both dialects.
func createTablesSQL(driver string) string {
	reservations := fmt.Sprintf(alertReservationsColumnsFmt, fingerprintCollate(driver))
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS alert_sources (%s
		);
		CREATE TABLE IF NOT EXISTS alert_watches (%s
		);
		CREATE INDEX IF NOT EXISTS idx_alert_watches_source ON alert_watches(source_id);
		CREATE TABLE IF NOT EXISTS alert_reservations (%s
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_alert_reservations_live
			ON alert_reservations(watch_id, fingerprint) WHERE released_at IS NULL;
	`, alertSourcesColumns, alertWatchesColumns, reservations)
}

func (s *Store) initSchema() error {
	rendered, err := dialect.RenderSchema(s.db.DriverName(), createTablesSQL(s.db.DriverName()))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(rendered)
	return err
}

func requireNonEmpty(argName, value string) error {
	if value == "" {
		return fmt.Errorf("alertsource: %s must not be empty", argName)
	}
	return nil
}

func requireFingerprint(fingerprint string) error {
	if !isFingerprint(fingerprint) {
		return fmt.Errorf("alertsource: fingerprint is not a valid Fingerprint output")
	}
	return nil
}

// ReserveFingerprint claims the (watchID, fingerprint) pair for a new
// reservation. It contains no SELECT (D16.1): a single INSERT whose ON
// CONFLICT target carries the partial index's predicate absorbs a
// concurrent duplicate delivery by index rejection alone. reserved=false,
// err=nil means another caller already holds a live reservation for this
// pair — not an error condition.
//
// alertRaw is stored as ” when nil or empty (the column is NOT NULL
// DEFAULT ”), never NULL. Row id and created_at are generated in Go at the
// call site, never CURRENT_TIMESTAMP, so SQLite and Postgres cannot diverge
// on format or clock.
//
// Concurrency note: if a concurrent ReleaseReservationForTask/
// ReleaseOrphanedReservation commits between this statement's conflict
// check and its own commit, a losing caller here still gets reserved=false
// — the index entry it conflicted with was live at check time — and the
// alert is re-reserved on the next delivery. That produces a task one
// delivery later, never zero tasks forever.
func (s *Store) ReserveFingerprint(ctx context.Context, watchID, fingerprint string, alertRaw []byte) (bool, error) {
	if err := requireNonEmpty("watchID", watchID); err != nil {
		return false, err
	}
	if err := requireFingerprint(fingerprint); err != nil {
		return false, err
	}
	raw := ""
	if len(alertRaw) > 0 {
		raw = string(alertRaw)
	}
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO alert_reservations (id, watch_id, fingerprint, task_id, alert_raw, created_at)
		VALUES (?, ?, ?, '', ?, ?)
		ON CONFLICT (watch_id, fingerprint) WHERE released_at IS NULL DO NOTHING`),
		uuid.New().String(), watchID, fingerprint, raw, time.Now().UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// AttachReservationTaskID is a compare-and-set, idempotent for its own task
// ID (D3(b)): the guard `(task_id = ” OR task_id = ?)` means a retry after
// an ambiguous commit (the first UPDATE landed but the response was lost)
// reports the same success as a fresh attach, rather than a spurious
// conflict.
//
// On the zero-row path only, a single diagnostic SELECT (scoped by
// `released_at IS NULL`, so it can return at most one row) classifies the
// cause: a live row owned by a different task returns
// ErrReservationAttachedElsewhere; no live row returns ErrReservationGone.
// A failure of the diagnostic SELECT itself is returned wrapped and is
// NEVER reported as ErrReservationGone (A5) — a transient query failure
// must stay retryable, while ErrReservationGone tells the caller to stop
// retrying and release the reservation.
func (s *Store) AttachReservationTaskID(ctx context.Context, watchID, fingerprint, taskID string) error {
	if err := requireNonEmpty("watchID", watchID); err != nil {
		return err
	}
	if err := requireFingerprint(fingerprint); err != nil {
		return err
	}
	if err := requireNonEmpty("taskID", taskID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE alert_reservations SET task_id = ?
		WHERE watch_id = ? AND fingerprint = ? AND released_at IS NULL AND (task_id = '' OR task_id = ?)`),
		taskID, watchID, fingerprint, taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	return s.classifyAttachFailure(ctx, watchID, fingerprint)
}

func (s *Store) classifyAttachFailure(ctx context.Context, watchID, fingerprint string) error {
	var attachedTaskID string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT task_id FROM alert_reservations WHERE watch_id = ? AND fingerprint = ? AND released_at IS NULL`),
		watchID, fingerprint).Scan(&attachedTaskID)
	switch {
	case err == nil:
		return ErrReservationAttachedElsewhere
	case errors.Is(err, sql.ErrNoRows):
		return ErrReservationGone
	default:
		return fmt.Errorf("alertsource: classify attach failure: %w", err)
	}
}

// DeleteReservation removes a live reservation outright. It runs after task
// creation has already failed (the pipeline's own compensating action for a
// failed CreateIssueTask, before any task ID exists to attach), so a
// zero-row result is a no-op rather than an error — a second error here
// would mask the first.
func (s *Store) DeleteReservation(ctx context.Context, watchID, fingerprint string) error {
	if err := requireNonEmpty("watchID", watchID); err != nil {
		return err
	}
	if err := requireFingerprint(fingerprint); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM alert_reservations WHERE watch_id = ? AND fingerprint = ? AND released_at IS NULL`),
		watchID, fingerprint)
	return err
}

// ReleaseReservationForTask stamps released_at for the reservation attached
// to taskID, called on task close/archive (T06 subscribes). An empty
// taskID is rejected before the statement runs: task_id is NOT NULL
// DEFAULT ”, so an empty argument would otherwise match and release every
// unattached row in the table. Most tasks carry no reservation, so a
// zero-row result is a no-op, not an error; `released_at IS NULL` also
// makes this idempotent against a redelivered close event.
func (s *Store) ReleaseReservationForTask(ctx context.Context, taskID string) error {
	if err := requireNonEmpty("taskID", taskID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE alert_reservations SET released_at = ? WHERE task_id = ? AND released_at IS NULL`),
		time.Now().UTC(), taskID)
	return err
}

// ReleaseOrphanedReservation stamps released_at for a reservation whose
// attach permanently failed after a successful task create (T06 calls this
// once its attach retries are exhausted). The `AND task_id = ”` guard
// makes it structurally incapable of releasing a correctly-attached
// reservation, so it cannot corrupt a concurrent AttachReservationTaskID:
// whichever of the two single-statement conditional updates the database
// serializes first determines whether the row survives attached or ends up
// released with task_id still ” (auditable, and the fingerprint becomes
// reservable again).
func (s *Store) ReleaseOrphanedReservation(ctx context.Context, watchID, fingerprint string) error {
	if err := requireNonEmpty("watchID", watchID); err != nil {
		return err
	}
	if err := requireFingerprint(fingerprint); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE alert_reservations SET released_at = ?
		WHERE watch_id = ? AND fingerprint = ? AND released_at IS NULL AND task_id = ''`),
		time.Now().UTC(), watchID, fingerprint)
	return err
}
