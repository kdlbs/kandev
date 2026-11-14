package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// ChangeRequestState enumerates the lifecycle states a plugin-reported task
// change request can carry. Host consumers only treat "merged" as merged;
// every other value is shown but never counted as a merge.
const (
	ChangeRequestStateOpen   = "open"
	ChangeRequestStateMerged = "merged"
	ChangeRequestStateClosed = "closed"
)

var (
	ErrChangeRequestTaskConflict = errors.New("plugin task change request is associated with a different task")
	// ErrChangeRequestAmbiguous reports that a host-less identity matches rows
	// on more than one provider host. The Host never guesses which object the
	// caller meant and never acts on several of them at once.
	ErrChangeRequestAmbiguous = errors.New("plugin task change request identity has multiple provider hosts")
)

// ChangeRequestRecord is one Host-owned, plugin-reported task change request.
// The row is keyed by the owning installation and workspace plus the plugin's
// host-qualified provider identity (provider id, provider host, repository id,
// number), so two plugins can never collide on the same forge object, two code
// hosts that reuse a provider id stay distinct, and a disabled or uninstalled
// plugin leaves rows that are fenced by owner rather than silently reused. The
// installation id is a Host-minted opaque principal (store.Record): the
// service maps it to the plugin id for uninstall fencing. Timestamps are
// RFC3339 text in the record; an empty terminal instant means "not set" and is
// stored as NULL.
type ChangeRequestRecord struct {
	ID             string `db:"id"`
	InstallationID string `db:"installation_id"`
	WorkspaceID    string `db:"workspace_id"`
	TaskID         string `db:"task_id"`
	ProviderID     string `db:"provider_id"`
	ProviderHost   string `db:"provider_host"`
	RepositoryID   string `db:"repository_id"`
	Number         int64  `db:"number"`
	URL            string `db:"url"`
	Title          string `db:"title"`
	State          string `db:"state"`
	HeadBranch     string `db:"head_branch"`
	BaseBranch     string `db:"base_branch"`
	CreatedAt      string `db:"created_at"`
	MergedAt       string `db:"merged_at"`
	ClosedAt       string `db:"closed_at"`
	UpdatedAt      string `db:"updated_at"`
}

// ChangeRequestStore persists plugin-reported task change requests in the
// plugin_task_change_requests table. The owning installation stamps every
// row; uninstall deletes by that identity, while disable only fences new
// writes, so a re-enable can reuse the same installation's history.
type ChangeRequestStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewChangeRequestStore creates a ChangeRequestStore and initializes the
// plugin_task_change_requests schema if needed.
func NewChangeRequestStore(pool *db.Pool) (*ChangeRequestStore, error) {
	if pool == nil || pool.Writer() == nil || pool.Reader() == nil {
		return nil, errors.New("plugin change request store: database pool is unavailable")
	}
	store := &ChangeRequestStore{db: pool.Writer(), ro: pool.Reader()}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

// changeRequestSchemaStatements is the single source of truth for the table
// shape. Each statement is rendered for the active driver and run with its own
// Exec so both SQLite and Postgres accept it. The unique key includes the
// provider host, terminal instants are nullable timestamps, and the shape is
// the one existing installations already carry, so CREATE TABLE IF NOT EXISTS
// opens such a table unchanged.
var changeRequestSchemaStatements = []string{`
CREATE TABLE IF NOT EXISTS plugin_task_change_requests (
 id TEXT PRIMARY KEY, installation_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
 task_id TEXT NOT NULL, provider_id TEXT NOT NULL, provider_host TEXT NOT NULL DEFAULT '',
 repository_id TEXT NOT NULL, number BIGINT NOT NULL, url TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'open',
 head_branch TEXT NOT NULL DEFAULT '', base_branch TEXT NOT NULL DEFAULT '',
 created_at {{timestamp}} NOT NULL, merged_at {{timestamp}}, closed_at {{timestamp}}, updated_at {{timestamp}} NOT NULL,
 UNIQUE (installation_id, workspace_id, provider_id, provider_host, repository_id, number)
)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_task_change_requests_task ON plugin_task_change_requests(installation_id, workspace_id, task_id)`,
	`CREATE INDEX IF NOT EXISTS idx_plugin_task_change_requests_merged ON plugin_task_change_requests(workspace_id, state, merged_at)`,
}

func (s *ChangeRequestStore) initSchema() error {
	for _, statement := range changeRequestSchemaStatements {
		rendered, err := dialect.RenderSchema(s.db.DriverName(), statement)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(rendered); err != nil {
			return err
		}
	}
	return nil
}

// validChangeRequestState reports whether state is one of the three lifecycle
// values the contract admits. Anything else is rejected at the Host boundary
// before it can reach storage.
func validChangeRequestState(state string) bool {
	switch state {
	case ChangeRequestStateOpen, ChangeRequestStateMerged, ChangeRequestStateClosed:
		return true
	default:
		return false
	}
}

const changeRequestTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

func validateChangeRequestTimestamp(value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return errors.New("plugin task change request: timestamp is invalid")
	}
	return nil
}

func normalizeChangeRequestTimestamp(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(changeRequestTimestampLayout)
}

func validateChangeRequestRecord(record ChangeRequestRecord) error {
	if record.InstallationID == "" || record.WorkspaceID == "" || record.TaskID == "" ||
		record.ProviderID == "" || record.RepositoryID == "" || record.Number <= 0 {
		return errors.New("plugin task change request: required identity fields are missing")
	}
	if !validChangeRequestState(record.State) {
		return errors.New("plugin task change request: state is invalid")
	}
	if err := validateChangeRequestTimestamps(record.CreatedAt, record.MergedAt, record.ClosedAt); err != nil {
		return err
	}
	return validateChangeRequestStateTimestamps(record.State, record.MergedAt, record.ClosedAt)
}

func validateChangeRequestTimestamps(values ...string) error {
	for _, value := range values {
		if err := validateChangeRequestTimestamp(value); err != nil {
			return err
		}
	}
	return nil
}

func validateChangeRequestStateTimestamps(state, mergedAt, closedAt string) error {
	switch state {
	case ChangeRequestStateOpen:
		if mergedAt != "" || closedAt != "" {
			return errors.New("plugin task change request: open state cannot carry terminal timestamps")
		}
	case ChangeRequestStateMerged:
		if mergedAt == "" || closedAt != "" {
			return errors.New("plugin task change request: merged state requires merged_at only")
		}
	case ChangeRequestStateClosed:
		if closedAt == "" || mergedAt != "" {
			return errors.New("plugin task change request: closed state requires closed_at only")
		}
	}
	return nil
}

func normalizeChangeRequestRecord(record ChangeRequestRecord, now string) ChangeRequestRecord {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	record.CreatedAt = normalizeChangeRequestTimestamp(record.CreatedAt)
	record.MergedAt = normalizeChangeRequestTimestamp(record.MergedAt)
	record.ClosedAt = normalizeChangeRequestTimestamp(record.ClosedAt)
	record.UpdatedAt = now
	return record
}

// ReportChangeRequest upserts one plugin-reported task change request. The
// owning installation, workspace, provider, provider host, repository, and
// number identify the row; the latest report updates the descriptive fields.
// A report without a provider host updates the unique existing row of that
// identity and keeps its host; when rows exist on several hosts it fails with
// ErrChangeRequestAmbiguous. The created, merged, and closed instants are
// latched, and merged and closed are terminal states, so a stale poll cannot
// reopen or unmerge a row. The second return reports whether the row already
// existed.
func (s *ChangeRequestStore) ReportChangeRequest(ctx context.Context, record ChangeRequestRecord) (ChangeRequestRecord, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ChangeRequestRecord{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	stored, created, err := s.reportChangeRequestTx(ctx, tx, record)
	if err != nil {
		return ChangeRequestRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ChangeRequestRecord{}, false, err
	}
	return stored, !created, nil
}

func (s *ChangeRequestStore) reportChangeRequestTx(ctx context.Context, tx *sqlx.Tx, record ChangeRequestRecord) (ChangeRequestRecord, bool, error) {
	if err := validateChangeRequestRecord(record); err != nil {
		return ChangeRequestRecord{}, false, err
	}
	now := time.Now().UTC().Format(changeRequestTimestampLayout)
	record = normalizeChangeRequestRecord(record, now)
	existing, err := lookupChangeRequest(ctx, tx, record)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ChangeRequestRecord{}, false, err
	}
	created := errors.Is(err, sql.ErrNoRows)
	if !created {
		if existing.TaskID != record.TaskID {
			return ChangeRequestRecord{}, false, ErrChangeRequestTaskConflict
		}
		// A host-less report may update the unique existing object but never
		// changes that object's host-qualified identity.
		record.ProviderHost = existing.ProviderHost
	}
	merged := changeRequestKnownTimestamp(s.db.DriverName(), "merged_at")
	closed := changeRequestKnownTimestamp(s.db.DriverName(), "closed_at")
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO plugin_task_change_requests (`+changeRequestColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(installation_id, workspace_id, provider_id, provider_host, repository_id, number)
		DO UPDATE SET url = excluded.url, title = excluded.title,
			state = CASE WHEN plugin_task_change_requests.state IN ('merged', 'closed')
				THEN plugin_task_change_requests.state ELSE excluded.state END,
			head_branch = excluded.head_branch, base_branch = excluded.base_branch,
			merged_at = CASE WHEN plugin_task_change_requests.state = 'closed' THEN plugin_task_change_requests.merged_at
				ELSE COALESCE(`+merged+`, excluded.merged_at) END,
			closed_at = CASE WHEN plugin_task_change_requests.state = 'merged' THEN plugin_task_change_requests.closed_at
				ELSE COALESCE(`+closed+`, excluded.closed_at) END,
			updated_at = excluded.updated_at
		WHERE plugin_task_change_requests.task_id = excluded.task_id
	`), record.ID, record.InstallationID, record.WorkspaceID, record.TaskID, record.ProviderID, record.ProviderHost,
		record.RepositoryID, record.Number, record.URL, record.Title, record.State, record.HeadBranch, record.BaseBranch,
		record.CreatedAt, optionalChangeRequestValue(record.MergedAt), optionalChangeRequestValue(record.ClosedAt), record.UpdatedAt)
	if err != nil {
		return ChangeRequestRecord{}, false, err
	}
	stored, err := lookupChangeRequest(ctx, tx, record)
	if err != nil {
		return ChangeRequestRecord{}, false, err
	}
	if stored.TaskID != record.TaskID {
		return ChangeRequestRecord{}, false, ErrChangeRequestTaskConflict
	}
	return stored, created, nil
}

// lookupChangeRequest resolves the caller's identity to at most one row. With
// a provider host it is an exact key lookup; without one it matches the
// identity on any host and rejects a second match as ambiguous.
func lookupChangeRequest(ctx context.Context, tx *sqlx.Tx, record ChangeRequestRecord) (ChangeRequestRecord, error) {
	query := `SELECT ` + changeRequestColumns + ` FROM plugin_task_change_requests
		WHERE installation_id = ? AND workspace_id = ? AND provider_id = ? AND repository_id = ? AND number = ?`
	args := []any{record.InstallationID, record.WorkspaceID, record.ProviderID, record.RepositoryID, record.Number}
	if record.ProviderHost != "" {
		query += ` AND provider_host = ?`
		args = append(args, record.ProviderHost)
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query+` ORDER BY id LIMIT 2`), args...)
	if err != nil {
		return ChangeRequestRecord{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ChangeRequestRecord{}, err
		}
		return ChangeRequestRecord{}, sql.ErrNoRows
	}
	stored, err := scanChangeRequest(rows)
	if err != nil {
		return ChangeRequestRecord{}, err
	}
	if rows.Next() {
		return ChangeRequestRecord{}, ErrChangeRequestAmbiguous
	}
	return stored, rows.Err()
}

// changeRequestKnownTimestamp reads a stored terminal instant as NULL when it
// is unset. SQLite tables carried over from older shapes may hold empty text
// instead of NULL; Postgres timestamp columns can only hold NULL.
func changeRequestKnownTimestamp(driver, column string) string {
	qualified := "plugin_task_change_requests." + column
	if dialect.IsPostgres(driver) {
		return qualified
	}
	return "NULLIF(" + qualified + ", '')"
}

func optionalChangeRequestValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// RemoveChangeRequest deletes one plugin-reported task change request owned
// by the calling installation. Removing a row that does not exist is not an
// error; removing another installation's row is impossible because every
// statement is scoped to the caller's installation id. The identity carries
// no provider host, so when it matches rows on several hosts nothing is
// deleted and ErrChangeRequestAmbiguous is returned.
func (s *ChangeRequestStore) RemoveChangeRequest(ctx context.Context, installationID, workspaceID, providerID, repositoryID string, number int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := removeChangeRequestTx(ctx, tx, installationID, workspaceID, providerID, repositoryID, number); err != nil {
		return err
	}
	return tx.Commit()
}

// ListChangeRequestsByTask returns every change request the installation
// reported for one task, newest first. Reads are scoped to the caller's
// installation and workspace, so one plugin never sees another's rows.
func (s *ChangeRequestStore) ListChangeRequestsByTask(ctx context.Context, installationID, workspaceID, taskID string) ([]ChangeRequestRecord, error) {
	if installationID == "" || workspaceID == "" || taskID == "" {
		return nil, errors.New("plugin task change request: installation, workspace, and task are required")
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(
		`SELECT `+changeRequestColumns+` FROM plugin_task_change_requests WHERE installation_id = ? AND workspace_id = ? AND task_id = ? ORDER BY updated_at DESC, id`),
		installationID, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var records []ChangeRequestRecord
	for rows.Next() {
		record, err := scanChangeRequest(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// DeleteByInstallation removes every change request row for one installation
// id. The uninstall path calls it before the installation record is removed,
// so no row outlives the installation that reported it.
func (s *ChangeRequestStore) DeleteByInstallation(ctx context.Context, installationID string) error {
	if installationID == "" {
		return errors.New("plugin task change request: installation id is required")
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM plugin_task_change_requests WHERE installation_id = ?
	`), installationID)
	return err
}

// DeleteByWorkspace removes every reported row for a deleted workspace, across
// installations. The workspace.deleted handler owns this cleanup.
func (s *ChangeRequestStore) DeleteByWorkspace(ctx context.Context, workspaceID string) error {
	if workspaceID == "" {
		return errors.New("plugin task change request: workspace id is required")
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM plugin_task_change_requests WHERE workspace_id = ?
	`), workspaceID)
	return err
}

const changeRequestColumns = `id, installation_id, workspace_id, task_id, provider_id, provider_host,
	repository_id, number, url, title, state, head_branch, base_branch,
	created_at, merged_at, closed_at, updated_at`

// ChangeRequestResourceVersion is the stable version of one reported row.
func ChangeRequestResourceVersion(record ChangeRequestRecord) string {
	canonical := record.InstallationID + "\x00" + record.WorkspaceID + "\x00" + record.TaskID +
		"\x00" + record.ProviderID + "\x00" + record.ProviderHost + "\x00" + record.RepositoryID +
		"\x00" + record.ID + "\x00" + strconv.FormatInt(record.Number, 10) +
		"\x00" + record.State + "\x00" + record.UpdatedAt
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:])
}
