// Package instances stores scoped plugin instances, immutable release
// metadata, grants, storage reservations, and cleanup inventory. Native
// installed plugins keep their existing filesystem records; this package is
// the durable authority for isolated web-application instances.
//
//revive:disable:file-length-limit // Instance persistence, admission, release, and cleanup share one transaction boundary.
package instances

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

const (
	SourceInstalled   = "installed"
	SourceLocalCanvas = "local_canvas"

	ScopeInstance   = "instance"
	ScopeWorkspace  = "workspace"
	ScopeTask       = "task"
	ScopeSession    = "session"
	ScopeRepository = "repository"

	StatusPending  = "pending"
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusArchived = "archived"
	StatusError    = "error"
	StatusRemoved  = "removed"

	ValidationValid             = "valid"
	ValidationPendingPermission = "pending_permission"
	ValidationInvalid           = "invalid"
	ValidationUnavailable       = "plugin_release_unavailable"

	CleanupPending   = "pending"
	CleanupRunning   = "running"
	CleanupRetryWait = "retry_wait"
	CleanupCompleted = "completed"

	MaxTaskInstances      = 10
	MaxWorkspaceInstances = 100

	WorkspaceArtifactLimitBytes    int64 = 2 << 30
	InstallationArtifactLimitBytes int64 = 10 << 30
)

var (
	ErrNotFound                 = errors.New("plugin instance not found")
	ErrInvalidScope             = errors.New("invalid plugin instance scope")
	ErrTaskCanvasLimit          = errors.New("canvas task limit exceeded")
	ErrWorkspaceCanvasLimit     = errors.New("canvas workspace limit exceeded")
	ErrWorkspaceStorageLimit    = errors.New("canvas workspace storage limit exceeded")
	ErrInstallationStorageLimit = errors.New("canvas installation storage limit exceeded")
	ErrInvalidRelease           = errors.New("invalid plugin release")
	ErrStalePromotionReview     = errors.New("canvas promotion review is stale")
	ErrStaleCanvasEdit          = errors.New("canvas edit base release is stale")
	ErrStaleCanvasPublish       = errors.New("canvas publish authority is stale")
	ErrInvalidLifecycleState    = errors.New("plugin instance lifecycle state does not allow this operation")
)

// ScopeIdentifiers contains only the trusted identifiers required by a scope.
// Empty strings represent SQL NULL for optional identifiers.
type ScopeIdentifiers struct {
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
}

// ValidateScope rejects incomplete and mixed scopes before persistence.
func ValidateScope(kind string, ids ScopeIdentifiers) error {
	allowed := scopeIdentifiersFor(kind)
	if allowed == nil {
		return ErrInvalidScope
	}
	values := map[string]string{
		"workspace":  ids.WorkspaceID,
		"task":       ids.TaskID,
		"session":    ids.SessionID,
		"repository": ids.RepositoryID,
	}
	for name, value := range values {
		if allowed[name] != (value != "") {
			return ErrInvalidScope
		}
	}
	return nil
}

func scopeIdentifiersFor(kind string) map[string]bool {
	switch kind {
	case ScopeInstance:
		return map[string]bool{}
	case ScopeWorkspace:
		return map[string]bool{"workspace": true}
	case ScopeTask:
		return map[string]bool{"workspace": true, "task": true}
	case ScopeSession:
		return map[string]bool{"workspace": true, "session": true}
	case ScopeRepository:
		return map[string]bool{"workspace": true, "repository": true}
	default:
		return nil
	}
}

type Instance struct {
	ID              string
	PluginID        string
	SourceKind      string
	ScopeKind       string
	WorkspaceID     string
	TaskID          string
	SessionID       string
	RepositoryID    string
	Status          string
	ActiveReleaseID string
	GrantGeneration int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PublishAuthority is the durable identity captured when a trusted authoring
// request is authorized. A publish must match every field in this snapshot at
// the release insertion boundary so a scope transition, archive, or competing
// release cannot turn an already-authorized stream into a write for a new
// authority.
type PublishAuthority struct {
	InstanceID      string
	ScopeKind       string
	WorkspaceID     string
	TaskID          string
	SessionID       string
	RepositoryID    string
	Status          string
	ActiveReleaseID string
	GrantGeneration int64
}

func (i Instance) PublishAuthority() PublishAuthority {
	return PublishAuthority{
		InstanceID: i.ID, ScopeKind: i.ScopeKind, WorkspaceID: i.WorkspaceID,
		TaskID: i.TaskID, SessionID: i.SessionID, RepositoryID: i.RepositoryID,
		Status: i.Status, ActiveReleaseID: i.ActiveReleaseID, GrantGeneration: i.GrantGeneration,
	}
}

func (a PublishAuthority) IsZero() bool {
	return a.InstanceID == "" && a.ScopeKind == "" && a.WorkspaceID == "" &&
		a.TaskID == "" && a.SessionID == "" && a.RepositoryID == "" &&
		a.Status == "" && a.ActiveReleaseID == "" && a.GrantGeneration == 0
}

type Release struct {
	ID                      string
	PluginID                string
	InstanceID              string
	PackageDigest           string
	SourceKind              string
	SourceActorKind         string
	SourceUserID            string
	SourceTaskID            string
	SourceSessionID         string
	ManifestJSON            json.RawMessage
	DeclaredPermissionsJSON json.RawMessage
	ArtifactPath            string
	ArtifactBytes           int64
	ProtocolVersion         int
	ValidationStatus        string
	ValidationError         string
	CreatedAt               time.Time
}

type Grant struct {
	InstanceID     string
	PermissionKind string
	Resource       string
	NetworkOrigin  string
	ScopeCeiling   string
	ApprovedBy     string
	ApprovedAt     time.Time
}

type Reservation struct {
	ID          string
	WorkspaceID string
	Bytes       int64
	Status      string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type CleanupJob struct {
	ID            string
	WorkspaceID   string
	InstanceID    string
	ArtifactPath  string
	Status        string
	Attempts      int
	LastError     string
	CreatedAt     time.Time
	NextAttemptAt time.Time
}

// ArtifactCheck is the content-free result returned by startup artifact
// reconciliation.
type ArtifactCheck struct {
	Available bool
	Reason    string
}

type Store struct {
	db        *sqlx.DB
	ro        *sqlx.DB
	admission sync.Mutex
}

func NewStore(pool *db.Pool) (*Store, error) {
	if pool == nil || pool.Writer() == nil || pool.Reader() == nil {
		return nil, errors.New("plugin instances: database pool is nil")
	}
	s := &Store{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("plugin instances schema: %w", err)
	}
	return s, nil
}

// SchemaSQL is exported for migration replay tests and backup tooling. Each
// statement is idempotent and uses only the common SQLite/PostgreSQL subset.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS plugin_instances (
  id TEXT PRIMARY KEY,
  plugin_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  scope_kind TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  repository_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  active_release_id TEXT NOT NULL DEFAULT '',
  grant_generation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plugin_instances_workspace ON plugin_instances(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_plugin_instances_task ON plugin_instances(task_id, status);
CREATE TABLE IF NOT EXISTS plugin_releases (
  id TEXT PRIMARY KEY,
  plugin_id TEXT NOT NULL,
  instance_id TEXT NOT NULL,
  package_digest TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  source_actor_kind TEXT NOT NULL,
  source_user_id TEXT NOT NULL DEFAULT '',
  source_task_id TEXT NOT NULL DEFAULT '',
  source_session_id TEXT NOT NULL DEFAULT '',
  manifest_json TEXT NOT NULL,
  declared_permissions_json TEXT NOT NULL,
  artifact_path TEXT NOT NULL,
  artifact_bytes INTEGER NOT NULL DEFAULT 0,
  protocol_version INTEGER NOT NULL DEFAULT 1,
  validation_status TEXT NOT NULL,
  validation_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plugin_releases_instance ON plugin_releases(instance_id, created_at);
CREATE TABLE IF NOT EXISTS plugin_instance_grants (
  plugin_instance_id TEXT NOT NULL,
  permission_kind TEXT NOT NULL,
  resource TEXT NOT NULL,
  network_origin TEXT NOT NULL DEFAULT '',
  scope_ceiling TEXT NOT NULL,
  approved_by TEXT NOT NULL,
  approved_at TEXT NOT NULL,
  PRIMARY KEY (plugin_instance_id, permission_kind, resource, network_origin)
);
CREATE TABLE IF NOT EXISTS plugin_storage_reservations (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  bytes INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plugin_storage_reservations_scope ON plugin_storage_reservations(workspace_id, status);
CREATE TABLE IF NOT EXISTS plugin_artifact_cleanup_jobs (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  instance_id TEXT NOT NULL DEFAULT '',
  artifact_path TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  next_attempt_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plugin_artifact_cleanup_jobs_due ON plugin_artifact_cleanup_jobs(status, next_attempt_at);
`

func (s *Store) initSchema() error {
	for _, statement := range strings.Split(SchemaSQL, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

// WithTransaction runs one instance lifecycle transaction while holding the
// in-process admission lock. Callers use the Tx helpers below when a canvas
// metadata mutation must commit with its plugin-instance mutation.
func (s *Store) WithTransaction(ctx context.Context, fn func(*sqlx.Tx) error) error {
	if s == nil || s.db == nil || fn == nil {
		return ErrInvalidScope
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Create(ctx context.Context, instance Instance) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.CreateTx(ctx, tx, instance)
	})
}

// CreateTx inserts an instance in an existing lifecycle transaction.
func (s *Store) CreateTx(ctx context.Context, tx *sqlx.Tx, instance Instance) error {
	if instance.ID == "" || instance.PluginID == "" || instance.SourceKind == "" || instance.Status == "" {
		return ErrInvalidScope
	}
	if err := ValidateScope(instance.ScopeKind, ScopeIdentifiers{WorkspaceID: instance.WorkspaceID, TaskID: instance.TaskID, SessionID: instance.SessionID, RepositoryID: instance.RepositoryID}); err != nil {
		return err
	}
	if err := s.checkAdmission(ctx, tx, instance.ScopeKind, instance.WorkspaceID, instance.TaskID); err != nil {
		return err
	}
	now := time.Now().UTC()
	createdAt, updatedAt := instance.CreatedAt, instance.UpdatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`
INSERT INTO plugin_instances (id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), instance.ID, instance.PluginID, instance.SourceKind, instance.ScopeKind, instance.WorkspaceID, instance.TaskID, instance.SessionID, instance.RepositoryID, instance.Status, instance.ActiveReleaseID, instance.GrantGeneration, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return nil
}

// ListBySource returns instances created by a lifecycle source. It is used by
// startup reconciliation to remove instances left behind by a crash before
// their canvas metadata transaction committed.
func (s *Store) ListBySource(ctx context.Context, sourceKind string, includeRemoved bool) ([]Instance, error) {
	if strings.TrimSpace(sourceKind) == "" {
		return nil, ErrInvalidScope
	}
	query := `SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE source_kind = ?`
	args := []any{sourceKind}
	if !includeRemoved {
		query += ` AND status <> ?`
		args = append(args, StatusRemoved)
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []Instance
	for rows.Next() {
		var row instanceRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		instance, err := row.instance()
		if err != nil {
			return nil, err
		}
		result = append(result, instance)
	}
	return result, rows.Err()
}

func (s *Store) checkAdmission(ctx context.Context, tx *sqlx.Tx, scopeKind, workspaceID, taskID string) error {
	return s.checkAdmissionExcept(ctx, tx, scopeKind, workspaceID, taskID, "")
}

// checkAdmissionExcept applies the shared canvas count boundary while
// optionally excluding an existing instance. Promotion changes the scope of
// one existing instance, so its current row must not consume capacity in the
// destination scope when the destination is otherwise full.
func (s *Store) checkAdmissionExcept(ctx context.Context, tx *sqlx.Tx, scopeKind, workspaceID, taskID, excludeID string) error {
	exclude := ""
	argsSuffix := []any{}
	if excludeID != "" {
		exclude = " AND id <> ?"
		argsSuffix = append(argsSuffix, excludeID)
	}
	var count int
	if scopeKind == ScopeTask {
		args := []any{taskID, StatusRemoved}
		args = append(args, argsSuffix...)
		if err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE task_id = ? AND status <> ?`+exclude), args...); err != nil {
			return err
		}
		if count >= MaxTaskInstances {
			return ErrTaskCanvasLimit
		}
	}
	if workspaceID != "" {
		args := []any{workspaceID, StatusRemoved}
		args = append(args, argsSuffix...)
		if err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE workspace_id = ? AND status <> ?`+exclude), args...); err != nil {
			return err
		}
		if count >= MaxWorkspaceInstances {
			return ErrWorkspaceCanvasLimit
		}
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (Instance, error) {
	var row instanceRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	return row.instance()
}

// GetMany loads instance authority in one query for lifecycle projections.
// Missing IDs are omitted so callers can reconcile stale metadata without
// turning an expected orphan into a query error.
func (s *Store) GetMany(ctx context.Context, ids []string) ([]Instance, error) {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return []Instance{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for i, id := range unique {
		args[i] = id
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id IN (`+placeholders+`)`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]Instance, 0, len(unique))
	for rows.Next() {
		var row instanceRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		instance, err := row.instance()
		if err != nil {
			return nil, err
		}
		result = append(result, instance)
	}
	return result, rows.Err()
}

func (s *Store) List(ctx context.Context, workspaceID string, includeRemoved bool) ([]Instance, error) {
	query := `SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE workspace_id = ?`
	args := []any{workspaceID}
	if !includeRemoved {
		query += ` AND status <> ?`
		args = append(args, StatusRemoved)
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []Instance
	for rows.Next() {
		var row instanceRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		instance, err := row.instance()
		if err != nil {
			return nil, err
		}
		result = append(result, instance)
	}
	return result, rows.Err()
}

func (s *Store) CountActive(ctx context.Context, scopeKind, scopeID string) (int, error) {
	column := map[string]string{ScopeTask: "task_id", ScopeWorkspace: "workspace_id", ScopeSession: "session_id", ScopeRepository: "repository_id"}[scopeKind]
	if column == "" || scopeID == "" {
		return 0, ErrInvalidScope
	}
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE `+column+` = ? AND status <> ?`), scopeID, StatusRemoved)
	return count, err
}

func (s *Store) Archive(ctx context.Context, id string) error {
	return s.updateStatus(ctx, id, StatusArchived)
}

func (s *Store) Restore(ctx context.Context, id string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var row instanceRow
	if err := tx.GetContext(ctx, &row, tx.Rebind(`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id = ?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if row.Status != StatusArchived {
		return fmt.Errorf("%w: instance is %s", ErrInvalidScope, row.Status)
	}
	if err := s.checkAdmission(ctx, tx, row.ScopeKind, row.WorkspaceID, row.TaskID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_instances SET status = ?, updated_at = ? WHERE id = ? AND status = ?`), StatusActive, time.Now().UTC().Format(time.RFC3339Nano), id, StatusArchived)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) updateStatus(ctx context.Context, id, status string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE plugin_instances SET status = ?, updated_at = ? WHERE id = ? AND status <> ?`,
	), status, time.Now().UTC().Format(time.RFC3339Nano), id, StatusRemoved)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var exists int
		if err := s.ro.GetContext(ctx, &exists, s.ro.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE id = ?`), id); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetScope(ctx context.Context, id, scopeKind string, ids ScopeIdentifiers) error {
	if err := ValidateScope(scopeKind, ids); err != nil {
		return err
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current instanceRow
	if err := tx.GetContext(ctx, &current, tx.Rebind(`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id = ?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if current.Status == StatusRemoved {
		return ErrNotFound
	}
	if err := s.checkAdmissionExcept(ctx, tx, scopeKind, ids.WorkspaceID, ids.TaskID, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_instances SET scope_kind = ?, workspace_id = ?, task_id = ?, session_id = ?, repository_id = ?, grant_generation = grant_generation + 1, updated_at = ? WHERE id = ? AND status <> ?`), scopeKind, ids.WorkspaceID, ids.TaskID, ids.SessionID, ids.RepositoryID, time.Now().UTC().Format(time.RFC3339Nano), id, StatusRemoved)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetPluginID binds a local instance to the package identity discovered by a
// validated release. Local canvas instances start with a synthetic identity
// because the package does not exist at create time; publishing replaces it
// atomically before the release row is written.
func (s *Store) SetPluginID(ctx context.Context, id, pluginID string) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.SetPluginIDTx(ctx, tx, id, pluginID)
	})
}

// SetPluginIDTx updates the package identity inside an existing lifecycle
// transaction. Pending releases must not call this helper because changing the
// identity also advances the runtime grant generation.
func (s *Store) SetPluginIDTx(ctx context.Context, tx *sqlx.Tx, id, pluginID string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(pluginID) == "" {
		return ErrInvalidScope
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET plugin_id = ?, grant_generation = grant_generation + 1, updated_at = ? WHERE id = ? AND status <> ?`,
	), pluginID, time.Now().UTC().Format(time.RFC3339Nano), id, StatusRemoved)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveRelease(ctx context.Context, instanceID, releaseID string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.GetContext(ctx, &exists, tx.Rebind(`SELECT COUNT(*) FROM plugin_releases WHERE id = ? AND instance_id = ? AND validation_status = ?`), releaseID, instanceID, ValidationValid); err != nil {
		return err
	}
	if exists == 0 {
		return ErrInvalidRelease
	}
	var status string
	if err := tx.GetContext(ctx, &status, tx.Rebind(`SELECT status FROM plugin_instances WHERE id = ?`), instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := activationStateError(status); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET active_release_id = ?, updated_at = ? WHERE id = ? AND status IN (?, ?, ?)`,
	), releaseID, time.Now().UTC().Format(time.RFC3339Nano), instanceID, StatusPending, StatusActive, StatusError)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return activationStateError(status)
	}
	return tx.Commit()
}

// ActivateRelease makes a valid release the active release and moves the
// instance into the active state. It is the publish/governance operation for
// local canvas instances; SetActiveRelease remains the narrow historical
// helper used by migration and compatibility tests.
func (s *Store) ActivateRelease(ctx context.Context, instanceID, releaseID string) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.ActivateReleaseTx(ctx, tx, instanceID, releaseID)
	})
}

// ActivateReleaseTx validates and activates a release inside an existing
// lifecycle transaction. The caller can combine it with release insertion or
// another owner-table mutation without exposing a half-published release.
func (s *Store) ActivateReleaseTx(ctx context.Context, tx *sqlx.Tx, instanceID, releaseID string) error {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(releaseID) == "" {
		return ErrInvalidRelease
	}
	var current instanceRow
	if err := tx.GetContext(ctx, &current, tx.Rebind(
		`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id = ?`,
	), instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if current.Status == StatusRemoved {
		return ErrNotFound
	}
	if err := activationStateError(current.Status); err != nil {
		return err
	}
	var release struct {
		DeclaredPermissionsJSON string `db:"declared_permissions_json"`
		ValidationStatus        string `db:"validation_status"`
	}
	if err := tx.GetContext(ctx, &release, tx.Rebind(
		`SELECT declared_permissions_json, validation_status FROM plugin_releases WHERE id = ? AND instance_id = ?`,
	), releaseID, instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidRelease
		}
		return err
	}
	if release.ValidationStatus != ValidationValid || !declaredGrantsFitInstance(ctx, tx, instanceID, current.ScopeKind, release.DeclaredPermissionsJSON) {
		return ErrInvalidRelease
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET active_release_id = ?, status = ?, updated_at = ? WHERE id = ? AND status IN (?, ?, ?)`,
	), releaseID, StatusActive, time.Now().UTC().Format(time.RFC3339Nano), instanceID, StatusPending, StatusActive, StatusError)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return activationStateError(current.Status)
	}
	return nil
}

func activationStateError(status string) error {
	switch status {
	case StatusPending, StatusActive, StatusError:
		return nil
	case StatusRemoved:
		return ErrNotFound
	default:
		return fmt.Errorf("%w: instance is %s", ErrInvalidLifecycleState, status)
	}
}

type declaredPermissions struct {
	Reads           []string `json:"reads"`
	APIRead         []string `json:"api_read"`
	Writes          []string `json:"writes"`
	APIWrite        []string `json:"api_write"`
	Events          []string `json:"events"`
	SharedState     bool     `json:"shared_state"`
	State           bool     `json:"state"`
	ExternalOrigins []string `json:"external_origins"`
}

func (p declaredPermissions) readResources() []string {
	return append(append([]string(nil), p.Reads...), p.APIRead...)
}

func (p declaredPermissions) writeResources() []string {
	return append(append([]string(nil), p.Writes...), p.APIWrite...)
}

func parseDeclaredPermissions(declaredJSON string) (declaredPermissions, error) {
	var declared declaredPermissions
	if strings.TrimSpace(declaredJSON) == "" {
		return declared, errors.New("declared permissions are empty")
	}
	trimmed := strings.TrimSpace(declaredJSON)
	if strings.HasPrefix(trimmed, "[") {
		var legacy []string
		if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
			return declared, err
		}
		for _, permission := range legacy {
			parts := strings.SplitN(permission, ":", 2)
			if len(parts) != 2 {
				continue
			}
			switch parts[0] {
			case "api_read":
				declared.APIRead = append(declared.APIRead, parts[1])
			case "api_write":
				declared.APIWrite = append(declared.APIWrite, parts[1])
			case "events":
				declared.Events = append(declared.Events, parts[1])
			case "state":
				declared.State = true
			case "network":
				declared.ExternalOrigins = append(declared.ExternalOrigins, parts[1])
			}
		}
		return declared, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &declared); err != nil {
		return declared, err
	}
	return declared, nil
}

func declaredPermissionRequirements(declaredJSON string) ([]Grant, error) {
	declared, err := parseDeclaredPermissions(declaredJSON)
	if err != nil {
		return nil, err
	}
	requirements := make([]Grant, 0, len(declared.readResources())+len(declared.writeResources())+len(declared.Events)+len(declared.ExternalOrigins)+1)
	for _, resource := range declared.readResources() {
		requirements = append(requirements, Grant{PermissionKind: "api_read", Resource: resource})
	}
	for _, resource := range declared.writeResources() {
		requirements = append(requirements, Grant{PermissionKind: "api_write", Resource: resource})
	}
	for _, subject := range declared.Events {
		requirements = append(requirements, Grant{PermissionKind: "events", Resource: subject})
	}
	if declared.SharedState || declared.State {
		requirements = append(requirements, Grant{PermissionKind: "state"})
	}
	for _, origin := range declared.ExternalOrigins {
		requirements = append(requirements, Grant{PermissionKind: "network", NetworkOrigin: origin})
	}
	return requirements, nil
}

func declaredPermissionGrantFits(declaredJSON string, scope string, grant Grant) bool {
	declared, err := parseDeclaredPermissions(declaredJSON)
	if err != nil || !grantScopeFitsInstance(grant.ScopeCeiling, scope) {
		return false
	}
	switch grant.PermissionKind {
	case "api_read":
		return containsString(declared.readResources(), grant.Resource)
	case "api_write":
		return containsString(declared.writeResources(), grant.Resource)
	case "events":
		return containsString(declared.Events, grant.Resource)
	case "state":
		return declared.SharedState || declared.State
	case "network":
		return grant.NetworkOrigin != "" && containsString(declared.ExternalOrigins, grant.NetworkOrigin)
	default:
		return false
	}
}

func grantScopeFitsInstance(ceiling, scope string) bool {
	if ceiling == ScopeInstance {
		return true
	}
	return ceiling == scope
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func grantsCoverDeclaration(declaredJSON string, scope string, grants []Grant) bool {
	requirements, err := declaredPermissionRequirements(declaredJSON)
	if err != nil {
		return false
	}
	for _, requirement := range requirements {
		covered := false
		for _, grant := range grants {
			if grant.PermissionKind != requirement.PermissionKind || grant.Resource != requirement.Resource || grant.NetworkOrigin != requirement.NetworkOrigin {
				continue
			}
			if grantScopeFitsInstance(grant.ScopeCeiling, scope) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func declaredGrantsFitInstance(ctx context.Context, tx *sqlx.Tx, instanceID, scope, declaredJSON string) bool {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(
		`SELECT permission_kind, resource, network_origin, scope_ceiling FROM plugin_instance_grants WHERE plugin_instance_id = ?`,
	), instanceID)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	grants := make([]Grant, 0)
	for rows.Next() {
		var row struct {
			PermissionKind string `db:"permission_kind"`
			Resource       string `db:"resource"`
			NetworkOrigin  string `db:"network_origin"`
			ScopeCeiling   string `db:"scope_ceiling"`
		}
		if err := rows.StructScan(&row); err != nil {
			return false
		}
		grant := Grant{PermissionKind: row.PermissionKind, Resource: row.Resource, NetworkOrigin: row.NetworkOrigin, ScopeCeiling: row.ScopeCeiling}
		grants = append(grants, grant)
	}
	if rows.Err() != nil {
		return false
	}
	for _, grant := range grants {
		// A release may remove a previously declared permission without a new
		// approval. Runtime authorization intersects the release declaration
		// with these grants, so only a grant that exceeds the instance scope
		// must reject activation here.
		if !grantScopeFitsInstance(grant.ScopeCeiling, scope) {
			return false
		}
	}
	return grantsCoverDeclaration(declaredJSON, scope, grants)
}

func validateGrantsForDeclaration(declaredJSON string, scope string, grants []Grant) error {
	for _, grant := range grants {
		if strings.TrimSpace(grant.PermissionKind) == "" || strings.TrimSpace(grant.ScopeCeiling) == "" ||
			!declaredPermissionGrantFits(declaredJSON, scope, grant) {
			return ErrInvalidScope
		}
	}
	if !grantsCoverDeclaration(declaredJSON, scope, grants) {
		return ErrInvalidScope
	}
	return nil
}

func loadInstanceGrants(ctx context.Context, tx *sqlx.Tx, instanceID string) ([]Grant, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(
		`SELECT permission_kind, resource, network_origin, scope_ceiling FROM plugin_instance_grants WHERE plugin_instance_id = ?`,
	), instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	grants := make([]Grant, 0)
	for rows.Next() {
		var row struct {
			PermissionKind string `db:"permission_kind"`
			Resource       string `db:"resource"`
			NetworkOrigin  string `db:"network_origin"`
			ScopeCeiling   string `db:"scope_ceiling"`
		}
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		grants = append(grants, Grant{
			PermissionKind: row.PermissionKind,
			Resource:       row.Resource,
			NetworkOrigin:  row.NetworkOrigin,
			ScopeCeiling:   row.ScopeCeiling,
		})
	}
	return grants, rows.Err()
}

func insertInstanceGrantsTx(ctx context.Context, tx *sqlx.Tx, instanceID, approvedBy string, grants []Grant) error {
	now := time.Now().UTC()
	for _, grant := range grants {
		normalized, err := normalizeGrant(grant, approvedBy, now)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`INSERT INTO plugin_instance_grants (plugin_instance_id, permission_kind, resource, network_origin, scope_ceiling, approved_by, approved_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(plugin_instance_id, permission_kind, resource, network_origin) DO UPDATE SET scope_ceiling = excluded.scope_ceiling, approved_by = excluded.approved_by, approved_at = excluded.approved_at`,
		), instanceID, normalized.PermissionKind, normalized.Resource, normalized.NetworkOrigin, normalized.ScopeCeiling, normalized.ApprovedBy, normalized.ApprovedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return nil
}

func normalizeGrant(grant Grant, approvedBy string, now time.Time) (Grant, error) {
	if strings.TrimSpace(grant.PermissionKind) == "" || strings.TrimSpace(grant.ScopeCeiling) == "" {
		return Grant{}, ErrInvalidScope
	}
	if grant.ApprovedBy == "" {
		grant.ApprovedBy = approvedBy
	}
	if grant.ApprovedAt.IsZero() {
		grant.ApprovedAt = now
	}
	return grant, nil
}

// SetReleaseValidation changes a release's validation state without changing
// the active release. This is used for rejected and pending-permission
// releases, where the currently running application must remain untouched.
func (s *Store) SetReleaseValidation(ctx context.Context, releaseID, validationStatus, validationError string) error {
	if strings.TrimSpace(releaseID) == "" || strings.TrimSpace(validationStatus) == "" {
		return ErrInvalidRelease
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE plugin_releases SET validation_status = ?, validation_error = ? WHERE id = ?`,
	), validationStatus, validationError, releaseID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RejectReleaseAndPrune records a user rejection and transfers cleanup
// ownership for the rejected artifact in the same transaction. This prevents
// a rejected pending release from consuming durable storage until a later
// publish happens to trigger retention.
func (s *Store) RejectReleaseAndPrune(ctx context.Context, instanceID, releaseID string) error {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(releaseID) == "" {
		return ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadReleaseRetentionInstance(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	if err := activationStateError(current.Status); err != nil {
		return err
	}
	var validationStatus string
	if err := tx.GetContext(ctx, &validationStatus, tx.Rebind(
		`SELECT validation_status FROM plugin_releases WHERE id = ? AND instance_id = ?`,
	), releaseID, instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if validationStatus != ValidationPendingPermission {
		return ErrInvalidRelease
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_releases SET validation_status = ?, validation_error = ? WHERE id = ? AND instance_id = ? AND validation_status = ?`,
	), ValidationInvalid, "rejected_by_user", releaseID, instanceID, ValidationPendingPermission); err != nil {
		return err
	}
	if err := s.pruneReleasesTx(ctx, tx, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

// PromoteScopeAndGrants atomically moves an instance to workspace scope and
// records the grants approved by one human. The admission check excludes the
// instance itself, so a full destination workspace can still accept its
// promoted canvas only when it is already counted there (which is not the
// normal task-to-workspace case).
func (s *Store) PromoteScopeAndGrants(ctx context.Context, instanceID, workspaceID, approvedBy string, grants []Grant) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.PromoteScopeAndGrantsTx(ctx, tx, instanceID, workspaceID, approvedBy, grants)
	})
}

// PromoteScopeAndGrantsTx applies workspace scope and grants inside an
// existing transaction. Canvas metadata promotion uses the same transaction
// through this helper to keep the two authorities consistent after a crash.
func (s *Store) PromoteScopeAndGrantsTx(ctx context.Context, tx *sqlx.Tx, instanceID, workspaceID, approvedBy string, grants []Grant) error {
	return s.promoteScopeAndGrantsReviewedTx(ctx, tx, instanceID, workspaceID, approvedBy, grants, "", "", 0)
}

// PromoteScopeAndGrantsReviewedTx applies a human-reviewed promotion only if
// the release, permission declaration, and grant generation are unchanged
// since the review was shown. All checks and mutations share one transaction.
func (s *Store) PromoteScopeAndGrantsReviewedTx(ctx context.Context, tx *sqlx.Tx, instanceID, workspaceID, approvedBy string, grants []Grant, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	return s.promoteScopeAndGrantsReviewedTx(ctx, tx, instanceID, workspaceID, approvedBy, grants, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration)
}

func (s *Store) promoteScopeAndGrantsReviewedTx(ctx context.Context, tx *sqlx.Tx, instanceID, workspaceID, approvedBy string, grants []Grant, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	if err := validatePromotionRequest(instanceID, workspaceID, approvedBy); err != nil {
		return err
	}
	current, declaredJSON, err := loadPromotionState(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	if err := validatePromotionReview(current, declaredJSON, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration); err != nil {
		return err
	}
	if err := validateGrantsForDeclaration(declaredJSON, ScopeWorkspace, grants); err != nil {
		return err
	}
	if err := s.checkAdmissionExcept(ctx, tx, ScopeWorkspace, workspaceID, "", instanceID); err != nil {
		return err
	}
	return s.updatePromotedInstance(ctx, tx, instanceID, workspaceID, approvedBy, grants, expectedReleaseID, expectedPermissionDigest, expectedGrantGeneration)
}

func loadPromotionState(ctx context.Context, tx *sqlx.Tx, instanceID string) (instanceRow, string, error) {
	var current instanceRow
	if err := tx.GetContext(ctx, &current, tx.Rebind(
		`SELECT id, plugin_id, source_kind, scope_kind, workspace_id, task_id, session_id, repository_id, status, active_release_id, grant_generation, created_at, updated_at FROM plugin_instances WHERE id = ?`,
	), instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return instanceRow{}, "", ErrNotFound
		}
		return instanceRow{}, "", err
	}
	if err := activationStateError(current.Status); err != nil {
		return instanceRow{}, "", err
	}
	if current.ActiveReleaseID == "" {
		return instanceRow{}, "", ErrInvalidRelease
	}
	var declaredJSON string
	if err := tx.GetContext(ctx, &declaredJSON, tx.Rebind(
		`SELECT declared_permissions_json FROM plugin_releases WHERE id = ? AND instance_id = ?`,
	), current.ActiveReleaseID, instanceID); err != nil {
		return instanceRow{}, "", ErrInvalidRelease
	}
	return current, declaredJSON, nil
}

func validatePromotionReview(current instanceRow, declaredJSON, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	if expectedReleaseID != "" && current.ActiveReleaseID != expectedReleaseID {
		return ErrStalePromotionReview
	}
	if (expectedReleaseID != "" || expectedPermissionDigest != "") && current.GrantGeneration != expectedGrantGeneration {
		return ErrStalePromotionReview
	}
	if expectedPermissionDigest != "" && permissionDigest(declaredJSON) != expectedPermissionDigest {
		return ErrStalePromotionReview
	}
	return nil
}

func (s *Store) updatePromotedInstance(ctx context.Context, tx *sqlx.Tx, instanceID, workspaceID, approvedBy string, grants []Grant, expectedReleaseID, expectedPermissionDigest string, expectedGrantGeneration int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updateQuery := `UPDATE plugin_instances SET scope_kind = ?, workspace_id = ?, task_id = '', session_id = '', repository_id = '', grant_generation = grant_generation + 1, updated_at = ? WHERE id = ? AND status <> ?`
	updateArgs := []any{ScopeWorkspace, workspaceID, now, instanceID, StatusRemoved}
	reviewed := expectedReleaseID != "" || expectedPermissionDigest != ""
	if reviewed {
		// Recheck the review identity while taking the instance row lock. A
		// release activation or grant change that races this statement causes
		// the conditional update to affect no rows instead of being silently
		// overwritten by the promotion.
		updateQuery += ` AND active_release_id = ? AND grant_generation = ?`
		updateArgs = append(updateArgs, expectedReleaseID, expectedGrantGeneration)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(updateQuery), updateArgs...)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		if reviewed {
			return ErrStalePromotionReview
		}
		return ErrNotFound
	}
	if err := insertInstanceGrantsTx(ctx, tx, instanceID, approvedBy, grants); err != nil {
		return err
	}
	return nil
}

func permissionDigest(declaredJSON string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(declaredJSON)))
	return fmt.Sprintf("%x", digest[:])
}

func validatePromotionRequest(instanceID, workspaceID, approvedBy string) error {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(approvedBy) == "" {
		return ErrInvalidScope
	}
	return ValidateScope(ScopeWorkspace, ScopeIdentifiers{WorkspaceID: workspaceID})
}

// ApproveRelease atomically grants the requested permission rows, marks the
// release valid, and activates it. It is intentionally separate from
// SetReleaseValidation so a human approval cannot expose a release before
// its grants and active pointer commit together.
func (s *Store) ApproveRelease(ctx context.Context, instanceID, releaseID, approvedBy string, grants []Grant) error {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(releaseID) == "" || strings.TrimSpace(approvedBy) == "" {
		return ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	release, err := loadPendingReleaseTx(ctx, tx, instanceID, releaseID)
	if err != nil {
		return err
	}
	if release.ValidationStatus != ValidationPendingPermission {
		return ErrInvalidRelease
	}
	if err := validateGrantsForDeclaration(release.DeclaredPermissionsJSON, release.ScopeKind, grants); err != nil {
		return err
	}
	existingGrants, err := loadInstanceGrants(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	if !grantsCoverDeclaration(release.DeclaredPermissionsJSON, release.ScopeKind, append(existingGrants, grants...)) {
		return ErrInvalidScope
	}
	if err := insertInstanceGrantsTx(ctx, tx, instanceID, approvedBy, grants); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_releases SET validation_status = ?, validation_error = '' WHERE id = ? AND instance_id = ?`,
	), ValidationValid, releaseID, instanceID); err != nil {
		return err
	}
	if err := s.SetPluginIDTx(ctx, tx, instanceID, release.PluginID); err != nil {
		return err
	}
	if err := activateReleaseTx(ctx, tx, instanceID, releaseID); err != nil {
		return err
	}
	return tx.Commit()
}

type pendingReleaseRow struct {
	PluginID                string `db:"plugin_id"`
	DeclaredPermissionsJSON string `db:"declared_permissions_json"`
	ValidationStatus        string `db:"validation_status"`
	ScopeKind               string `db:"scope_kind"`
	Status                  string `db:"status"`
}

func loadPendingReleaseTx(ctx context.Context, tx *sqlx.Tx, instanceID, releaseID string) (pendingReleaseRow, error) {
	var release pendingReleaseRow
	if err := tx.GetContext(ctx, &release, tx.Rebind(
		`SELECT r.plugin_id, r.declared_permissions_json, r.validation_status, i.scope_kind, i.status FROM plugin_releases r JOIN plugin_instances i ON i.id = r.instance_id WHERE r.id = ? AND r.instance_id = ?`,
	), releaseID, instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pendingReleaseRow{}, ErrNotFound
		}
		return pendingReleaseRow{}, err
	}
	if err := activationStateError(release.Status); err != nil {
		return pendingReleaseRow{}, err
	}
	return release, nil
}

func activateReleaseTx(ctx context.Context, tx *sqlx.Tx, instanceID, releaseID string) error {
	var status string
	if err := tx.GetContext(ctx, &status, tx.Rebind(`SELECT status FROM plugin_instances WHERE id = ?`), instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := activationStateError(status); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET active_release_id = ?, status = ?, grant_generation = grant_generation + 1, updated_at = ? WHERE id = ? AND status IN (?, ?, ?)`,
	), releaseID, StatusActive, time.Now().UTC().Format(time.RFC3339Nano), instanceID, StatusPending, StatusActive, StatusError)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return activationStateError(status)
	}
	return nil
}

func (s *Store) CreateRelease(ctx context.Context, release Release) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.CreateReleaseTx(ctx, tx, release)
	})
}

// CreateReleaseTx inserts an immutable release row in an existing lifecycle
// transaction.
func (s *Store) CreateReleaseTx(ctx context.Context, tx *sqlx.Tx, release Release) error {
	if release.ID == "" {
		release.ID = uuid.NewString()
	}
	if release.InstanceID == "" || release.PluginID == "" || release.PackageDigest == "" || release.ArtifactPath == "" || release.ValidationStatus == "" {
		return ErrInvalidRelease
	}
	if len(release.ManifestJSON) == 0 {
		release.ManifestJSON = json.RawMessage(`{}`)
	}
	if len(release.DeclaredPermissionsJSON) == 0 {
		release.DeclaredPermissionsJSON = json.RawMessage(`[]`)
	}
	created := release.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO plugin_releases (id, plugin_id, instance_id, package_digest, source_kind, source_actor_kind, source_user_id, source_task_id, source_session_id, manifest_json, declared_permissions_json, artifact_path, artifact_bytes, protocol_version, validation_status, validation_error, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), release.ID, release.PluginID, release.InstanceID, release.PackageDigest, release.SourceKind, release.SourceActorKind, release.SourceUserID, release.SourceTaskID, release.SourceSessionID, string(release.ManifestJSON), string(release.DeclaredPermissionsJSON), release.ArtifactPath, release.ArtifactBytes, release.ProtocolVersion, release.ValidationStatus, release.ValidationError, created.Format(time.RFC3339Nano))
	return err
}

// CreateReleaseIfActiveReleaseTx inserts a release only when the editor's
// source was materialized from the currently active release. The check and
// insert are intentionally one transaction so two edit sessions cannot
// silently overwrite each other's work.
func (s *Store) CreateReleaseIfActiveReleaseTx(ctx context.Context, tx *sqlx.Tx, instanceID, expectedReleaseID string, release Release) error {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(expectedReleaseID) == "" {
		return ErrStaleCanvasEdit
	}
	// The no-op update both verifies the base release and holds the instance
	// row lock until the surrounding edit publish transaction commits. A
	// separate SELECT would allow an active release to change between the
	// check and the release insert.
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET updated_at = updated_at WHERE id = ? AND active_release_id = ? AND status <> ?`,
	), instanceID, expectedReleaseID, StatusRemoved)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var exists int
		if err := tx.GetContext(ctx, &exists, tx.Rebind(
			`SELECT COUNT(*) FROM plugin_instances WHERE id = ?`,
		), instanceID); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		return ErrStaleCanvasEdit
	}
	return s.CreateReleaseTx(ctx, tx, release)
}

// CreateReleaseIfAuthorityTx inserts a release only when the complete
// authorizing instance snapshot is still current. The conditional update and
// insert share one transaction, so promotion, archive, restore, or another
// release cannot invalidate an already-streamed source between authorization
// and durable release ownership.
func (s *Store) CreateReleaseIfAuthorityTx(ctx context.Context, tx *sqlx.Tx, instanceID string, expected PublishAuthority, release Release) error {
	if strings.TrimSpace(instanceID) == "" || expected.InstanceID != instanceID || expected.ScopeKind == "" || expected.Status == "" {
		return ErrStaleCanvasPublish
	}
	if err := activationStateError(expected.Status); err != nil {
		return err
	}
	if release.InstanceID != instanceID {
		return ErrInvalidRelease
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE plugin_instances SET updated_at = updated_at WHERE id = ? AND scope_kind = ? AND workspace_id = ? AND task_id = ? AND session_id = ? AND repository_id = ? AND status = ? AND active_release_id = ? AND grant_generation = ?`,
	), instanceID, expected.ScopeKind, expected.WorkspaceID, expected.TaskID, expected.SessionID, expected.RepositoryID, expected.Status, expected.ActiveReleaseID, expected.GrantGeneration)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var exists int
		if err := tx.GetContext(ctx, &exists, tx.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE id = ?`), instanceID); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		return ErrStaleCanvasPublish
	}
	return s.CreateReleaseTx(ctx, tx, release)
}

// PruneReleases keeps the active release, the newest non-active valid release,
// and the newest pending release for one instance. Superseded release rows are
// deleted only after cleanup ownership is recorded in the same transaction.
// The active release pointer is never changed by pruning.
func (s *Store) PruneReleases(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.pruneReleasesTx(ctx, tx, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) pruneReleasesTx(ctx context.Context, tx *sqlx.Tx, instanceID string) error {
	current, err := loadReleaseRetentionInstance(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	releases, err := listReleaseRetentionRows(ctx, tx, instanceID)
	if err != nil {
		return err
	}
	retainedPaths, removed := classifyReleaseRetention(current.ActiveReleaseID, releases)
	if err := scheduleRemovedReleaseCleanup(ctx, tx, instanceID, current.WorkspaceID, retainedPaths, removed); err != nil {
		return err
	}
	return deleteRemovedReleases(ctx, tx, instanceID, removed)
}

type releaseRetentionInstance struct {
	WorkspaceID     string `db:"workspace_id"`
	Status          string `db:"status"`
	ActiveReleaseID string `db:"active_release_id"`
}

type releaseRetentionRow struct {
	ID               string `db:"id"`
	ArtifactPath     string `db:"artifact_path"`
	ValidationStatus string `db:"validation_status"`
}

func loadReleaseRetentionInstance(ctx context.Context, tx *sqlx.Tx, instanceID string) (releaseRetentionInstance, error) {
	var current releaseRetentionInstance
	if err := tx.GetContext(ctx, &current, tx.Rebind(
		`SELECT workspace_id, status, active_release_id FROM plugin_instances WHERE id = ?`,
	), instanceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return releaseRetentionInstance{}, ErrNotFound
		}
		return releaseRetentionInstance{}, err
	}
	if current.Status == StatusRemoved {
		return releaseRetentionInstance{}, ErrNotFound
	}
	return current, nil
}

func listReleaseRetentionRows(ctx context.Context, tx *sqlx.Tx, instanceID string) ([]releaseRetentionRow, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(
		`SELECT id, artifact_path, validation_status FROM plugin_releases WHERE instance_id = ? ORDER BY created_at DESC, id DESC`,
	), instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var releases []releaseRetentionRow
	for rows.Next() {
		var release releaseRetentionRow
		if err := rows.StructScan(&release); err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, rows.Err()
}

func classifyReleaseRetention(activeReleaseID string, releases []releaseRetentionRow) (map[string]bool, []releaseRetentionRow) {
	retainedPaths := make(map[string]bool, 3)
	removed := make([]releaseRetentionRow, 0, len(releases))
	priorKept, pendingKept := false, false
	for _, release := range releases {
		if release.ID == activeReleaseID {
			retainedPaths[release.ArtifactPath] = true
			pendingKept = pendingKept || release.ValidationStatus == ValidationPendingPermission
			continue
		}
		keep := false
		switch release.ValidationStatus {
		case ValidationValid:
			keep, priorKept = !priorKept, true
		case ValidationPendingPermission:
			keep, pendingKept = !pendingKept, true
		}
		if keep {
			retainedPaths[release.ArtifactPath] = true
			continue
		}
		removed = append(removed, release)
	}
	return retainedPaths, removed
}

func scheduleRemovedReleaseCleanup(ctx context.Context, tx *sqlx.Tx, instanceID, workspaceID string, retainedPaths map[string]bool, removed []releaseRetentionRow) error {
	scheduled := make(map[string]bool, len(removed))
	for _, release := range removed {
		if retainedPaths[release.ArtifactPath] || scheduled[release.ArtifactPath] {
			continue
		}
		referenced, err := artifactReferencedByOtherInstanceTx(ctx, tx, instanceID, release.ArtifactPath)
		if err != nil {
			return err
		}
		if referenced {
			continue
		}
		if err := enqueueCleanupJobTx(ctx, tx, workspaceID, instanceID, release.ArtifactPath); err != nil {
			return err
		}
		scheduled[release.ArtifactPath] = true
	}
	return nil
}

func deleteRemovedReleases(ctx context.Context, tx *sqlx.Tx, instanceID string, removed []releaseRetentionRow) error {
	for _, release := range removed {
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`DELETE FROM plugin_releases WHERE id = ? AND instance_id = ?`,
		), release.ID, instanceID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListReleases(ctx context.Context, instanceID string) ([]Release, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`SELECT id, plugin_id, instance_id, package_digest, source_kind, source_actor_kind, source_user_id, source_task_id, source_session_id, manifest_json, declared_permissions_json, artifact_path, artifact_bytes, protocol_version, validation_status, validation_error, created_at FROM plugin_releases WHERE instance_id = ? ORDER BY created_at DESC, id DESC`), instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var releases []Release
	for rows.Next() {
		var row releaseRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		release, err := row.release()
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, rows.Err()
}

func (s *Store) GetRelease(ctx context.Context, id string) (Release, error) {
	var row releaseRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`SELECT id, plugin_id, instance_id, package_digest, source_kind, source_actor_kind, source_user_id, source_task_id, source_session_id, manifest_json, declared_permissions_json, artifact_path, artifact_bytes, protocol_version, validation_status, validation_error, created_at FROM plugin_releases WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	if err != nil {
		return Release{}, err
	}
	return row.release()
}

// ReconcileArtifacts checks retained release metadata before runtime routes
// are registered. The checker must not execute package code. Unavailable
// artifacts remain in the database for recovery and are marked with a stable
// validation status.
func (s *Store) ReconcileArtifacts(ctx context.Context, checker func(path, digest string, bytes int64) (ArtifactCheck, error)) (int, error) {
	if checker == nil {
		return 0, errors.New("artifact checker is nil")
	}
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`SELECT id, package_digest, artifact_path, artifact_bytes FROM plugin_releases WHERE validation_status IN (?, ?)`), ValidationValid, ValidationPendingPermission)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		id, digest, path string
		bytes            int64
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.digest, &item.path, &item.bytes); err != nil {
			_ = rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	marked := 0
	for _, item := range candidates {
		check, err := checker(item.path, item.digest, item.bytes)
		if err != nil {
			return marked, err
		}
		if check.Available {
			continue
		}
		status := ValidationUnavailable
		if check.Reason == "" {
			check.Reason = "artifact_unavailable"
		}
		if _, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE plugin_releases SET validation_status = ?, validation_error = ? WHERE id = ? AND validation_status IN (?, ?)`), status, check.Reason, item.id, ValidationValid, ValidationPendingPermission); err != nil {
			return marked, err
		}
		marked++
	}
	return marked, nil
}

func (s *Store) AddGrant(ctx context.Context, grant Grant) error {
	if grant.InstanceID == "" || grant.PermissionKind == "" || grant.ScopeCeiling == "" || grant.ApprovedBy == "" {
		return ErrInvalidScope
	}
	if grant.ApprovedAt.IsZero() {
		grant.ApprovedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.GetContext(ctx, &exists, tx.Rebind(`SELECT COUNT(*) FROM plugin_instances WHERE id = ? AND status <> ?`), grant.InstanceID, StatusRemoved); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO plugin_instance_grants (plugin_instance_id, permission_kind, resource, network_origin, scope_ceiling, approved_by, approved_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(plugin_instance_id, permission_kind, resource, network_origin) DO UPDATE SET scope_ceiling = excluded.scope_ceiling, approved_by = excluded.approved_by, approved_at = excluded.approved_at`), grant.InstanceID, grant.PermissionKind, grant.Resource, grant.NetworkOrigin, grant.ScopeCeiling, grant.ApprovedBy, grant.ApprovedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_instances SET grant_generation = grant_generation + 1, updated_at = ? WHERE id = ? AND status <> ?`), time.Now().UTC().Format(time.RFC3339Nano), grant.InstanceID, StatusRemoved); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListGrants(ctx context.Context, instanceID string) ([]Grant, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`SELECT plugin_instance_id, permission_kind, resource, network_origin, scope_ceiling, approved_by, approved_at FROM plugin_instance_grants WHERE plugin_instance_id = ? ORDER BY permission_kind, resource, network_origin`), instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var grants []Grant
	for rows.Next() {
		var row struct {
			InstanceID     string `db:"plugin_instance_id"`
			PermissionKind string `db:"permission_kind"`
			Resource       string `db:"resource"`
			NetworkOrigin  string `db:"network_origin"`
			ScopeCeiling   string `db:"scope_ceiling"`
			ApprovedBy     string `db:"approved_by"`
			ApprovedAt     string `db:"approved_at"`
		}
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		approvedAt, err := time.Parse(time.RFC3339Nano, row.ApprovedAt)
		if err != nil {
			return nil, err
		}
		grants = append(grants, Grant{InstanceID: row.InstanceID, PermissionKind: row.PermissionKind, Resource: row.Resource, NetworkOrigin: row.NetworkOrigin, ScopeCeiling: row.ScopeCeiling, ApprovedBy: row.ApprovedBy, ApprovedAt: approvedAt})
	}
	return grants, rows.Err()
}

func (s *Store) ReserveBytes(ctx context.Context, workspaceID string, bytes, workspaceLimit, installationLimit int64) (Reservation, error) {
	if strings.TrimSpace(workspaceID) == "" || bytes <= 0 || workspaceLimit <= 0 || installationLimit <= 0 {
		return Reservation{}, ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Reservation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	_, _ = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM plugin_storage_reservations WHERE status = ? AND expires_at < ?`), "reserved", now.Format(time.RFC3339Nano))
	var workspaceBytes, installationBytes int64
	if err := tx.GetContext(ctx, &workspaceBytes, tx.Rebind(`SELECT COALESCE(SUM(bytes), 0) FROM plugin_storage_reservations WHERE workspace_id = ? AND status = ?`), workspaceID, "reserved"); err != nil {
		return Reservation{}, err
	}
	if err := tx.GetContext(ctx, &installationBytes, tx.Rebind(`SELECT COALESCE(SUM(bytes), 0) FROM plugin_storage_reservations WHERE status = ?`), "reserved"); err != nil {
		return Reservation{}, err
	}
	var workspaceReleaseBytes, installationReleaseBytes int64
	if err := tx.GetContext(ctx, &workspaceReleaseBytes, tx.Rebind(
		`SELECT COALESCE(SUM(artifact_bytes), 0) FROM (SELECT r.artifact_path, MAX(r.artifact_bytes) AS artifact_bytes FROM plugin_releases r JOIN plugin_instances i ON i.id = r.instance_id WHERE i.workspace_id = ? GROUP BY r.artifact_path) AS retained_artifacts`,
	), workspaceID); err != nil {
		return Reservation{}, err
	}
	if err := tx.GetContext(ctx, &installationReleaseBytes, tx.Rebind(
		`SELECT COALESCE(SUM(artifact_bytes), 0) FROM (SELECT r.artifact_path, MAX(r.artifact_bytes) AS artifact_bytes FROM plugin_releases r JOIN plugin_instances i ON i.id = r.instance_id GROUP BY r.artifact_path) AS retained_artifacts`,
	)); err != nil {
		return Reservation{}, err
	}
	workspaceBytes += workspaceReleaseBytes
	installationBytes += installationReleaseBytes
	if exceedsStorageLimit(workspaceBytes, bytes, workspaceLimit) {
		return Reservation{}, ErrWorkspaceStorageLimit
	}
	if exceedsStorageLimit(installationBytes, bytes, installationLimit) {
		return Reservation{}, ErrInstallationStorageLimit
	}
	reservation := Reservation{ID: uuid.NewString(), WorkspaceID: workspaceID, Bytes: bytes, Status: "reserved", CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO plugin_storage_reservations (id, workspace_id, bytes, status, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`), reservation.ID, reservation.WorkspaceID, reservation.Bytes, reservation.Status, reservation.CreatedAt.Format(time.RFC3339Nano), reservation.ExpiresAt.Format(time.RFC3339Nano))
	if err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

func exceedsStorageLimit(current, requested, limit int64) bool {
	return current > limit || requested > limit-current
}

func (s *Store) ReleaseBytes(ctx context.Context, id string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE plugin_storage_reservations SET status = ? WHERE id = ? AND status = ?`), "released", id, "reserved")
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResizeReservation adjusts an active reservation after validation has
// measured the exact artifact size. The admission check remains in the same
// transaction, so a concurrent publish cannot consume the released headroom.
func (s *Store) ResizeReservation(ctx context.Context, id string, bytes, workspaceLimit, installationLimit int64) error {
	if strings.TrimSpace(id) == "" || bytes <= 0 || workspaceLimit <= 0 || installationLimit <= 0 {
		return ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	reservation, err := loadReservationTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if reservation.Status != "reserved" {
		return ErrNotFound
	}
	now := time.Now().UTC()
	_, _ = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM plugin_storage_reservations WHERE status = ? AND expires_at < ?`), "reserved", now.Format(time.RFC3339Nano))
	workspaceBytes, installationBytes, workspaceReleaseBytes, installationReleaseBytes, err := reservationUsageTx(ctx, tx, id, reservation.WorkspaceID)
	if err != nil {
		return err
	}
	if exceedsStorageLimit(workspaceBytes+workspaceReleaseBytes, bytes, workspaceLimit) {
		return ErrWorkspaceStorageLimit
	}
	if exceedsStorageLimit(installationBytes+installationReleaseBytes, bytes, installationLimit) {
		return ErrInstallationStorageLimit
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_storage_reservations SET bytes = ? WHERE id = ? AND status = ?`), bytes, id, "reserved")
	if err != nil {
		return err
	}
	return tx.Commit()
}

func loadReservationTx(ctx context.Context, tx *sqlx.Tx, id string) (Reservation, error) {
	var reservation Reservation
	var createdAt, expiresAt string
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT id, workspace_id, bytes, status, created_at, expires_at
		FROM plugin_storage_reservations WHERE id = ?
	`), id).Scan(&reservation.ID, &reservation.WorkspaceID, &reservation.Bytes, &reservation.Status, &createdAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Reservation{}, ErrNotFound
		}
		return Reservation{}, err
	}
	return reservation, nil
}

func reservationUsageTx(ctx context.Context, tx *sqlx.Tx, reservationID, workspaceID string) (int64, int64, int64, int64, error) {
	var workspaceBytes, installationBytes int64
	if err := tx.GetContext(ctx, &workspaceBytes, tx.Rebind(`SELECT COALESCE(SUM(bytes), 0) FROM plugin_storage_reservations WHERE workspace_id = ? AND status = ? AND id <> ?`), workspaceID, "reserved", reservationID); err != nil {
		return 0, 0, 0, 0, err
	}
	if err := tx.GetContext(ctx, &installationBytes, tx.Rebind(`SELECT COALESCE(SUM(bytes), 0) FROM plugin_storage_reservations WHERE status = ? AND id <> ?`), "reserved", reservationID); err != nil {
		return 0, 0, 0, 0, err
	}
	var workspaceReleaseBytes, installationReleaseBytes int64
	if err := tx.GetContext(ctx, &workspaceReleaseBytes, tx.Rebind(
		`SELECT COALESCE(SUM(artifact_bytes), 0) FROM (SELECT r.artifact_path, MAX(r.artifact_bytes) AS artifact_bytes FROM plugin_releases r JOIN plugin_instances i ON i.id = r.instance_id WHERE i.workspace_id = ? GROUP BY r.artifact_path) AS retained_artifacts`,
	), workspaceID); err != nil {
		return 0, 0, 0, 0, err
	}
	if err := tx.GetContext(ctx, &installationReleaseBytes, tx.Rebind(
		`SELECT COALESCE(SUM(artifact_bytes), 0) FROM (SELECT r.artifact_path, MAX(r.artifact_bytes) AS artifact_bytes FROM plugin_releases r JOIN plugin_instances i ON i.id = r.instance_id GROUP BY r.artifact_path) AS retained_artifacts`,
	)); err != nil {
		return 0, 0, 0, 0, err
	}
	return workspaceBytes, installationBytes, workspaceReleaseBytes, installationReleaseBytes, nil
}

func (s *Store) AddCleanupJob(ctx context.Context, job CleanupJob) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.ArtifactPath == "" {
		return ErrInvalidRelease
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.Status == "" {
		job.Status = CleanupPending
	}
	if job.Status != CleanupPending && job.Status != CleanupRetryWait {
		return ErrInvalidRelease
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO plugin_artifact_cleanup_jobs (id, workspace_id, instance_id, artifact_path, status, attempts, last_error, created_at, next_attempt_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`), job.ID, job.WorkspaceID, job.InstanceID, job.ArtifactPath, job.Status, job.Attempts, job.LastError, job.CreatedAt.Format(time.RFC3339Nano), job.NextAttemptAt.Format(time.RFC3339Nano))
	return err
}

// ClaimCleanupJob atomically claims the oldest due cleanup job. The store
// mutex protects callers in one process; the transaction and conditional
// update keep a second backend process from claiming the same row.
func (s *Store) ClaimCleanupJob(ctx context.Context, now time.Time) (CleanupJob, bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return CleanupJob{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var row cleanupRow
	err = tx.GetContext(ctx, &row, tx.Rebind(`
		SELECT id, workspace_id, instance_id, artifact_path, status, attempts,
		       last_error, created_at, next_attempt_at
		FROM plugin_artifact_cleanup_jobs
		WHERE status IN (?, ?) AND next_attempt_at <= ?
		ORDER BY next_attempt_at, created_at, id
		LIMIT 1
	`), CleanupPending, CleanupRetryWait, now.UTC().Format(time.RFC3339Nano))
	if errors.Is(err, sql.ErrNoRows) {
		return CleanupJob{}, false, nil
	}
	if err != nil {
		return CleanupJob{}, false, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE plugin_artifact_cleanup_jobs
		SET status = ?, attempts = attempts + 1
		WHERE id = ? AND status IN (?, ?) AND next_attempt_at <= ?
	`), CleanupRunning, row.ID, CleanupPending, CleanupRetryWait, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return CleanupJob{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CleanupJob{}, false, err
	}
	if affected == 0 {
		return CleanupJob{}, false, nil
	}
	row.Status = CleanupRunning
	row.Attempts++
	job, err := row.cleanupJob()
	if err != nil {
		return CleanupJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return CleanupJob{}, false, err
	}
	return job, true, nil
}

// RetryCleanupJob returns a claimed job to the due queue with durable
// diagnostic text. The next-attempt timestamp is supplied by the worker so
// retry backoff remains explicit and testable.
func (s *Store) RetryCleanupJob(ctx context.Context, id string, nextAttemptAt time.Time, cause error) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC()
	}
	lastError := ""
	if cause != nil {
		lastError = cause.Error()
		if len(lastError) > 4096 {
			lastError = lastError[:4096]
		}
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_artifact_cleanup_jobs
		SET status = ?, last_error = ?, next_attempt_at = ?
		WHERE id = ? AND status = ?
	`), CleanupRetryWait, lastError, nextAttemptAt.UTC().Format(time.RFC3339Nano), id, CleanupRunning)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CompleteCleanupJob marks a claimed artifact cleanup as finished. Completion
// is idempotent so a worker can safely repeat its final acknowledgement.
func (s *Store) CompleteCleanupJob(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_artifact_cleanup_jobs
		SET status = ?
		WHERE id = ? AND status = ?
	`), CleanupCompleted, id, CleanupRunning)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		return nil
	}
	var status string
	if err := s.ro.GetContext(ctx, &status, s.ro.Rebind(`SELECT status FROM plugin_artifact_cleanup_jobs WHERE id = ?`), id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	} else if status == CleanupCompleted {
		return nil
	}
	return ErrNotFound
}

// RemoveArtifactIfUnreferenced rechecks artifact ownership and completes a
// claimed cleanup job in one writer transaction. The removal callback runs
// while the admission lock and SQLite write transaction are held, so a
// concurrent release cannot republish the same artifact path between the
// ownership check and filesystem removal.
func (s *Store) RemoveArtifactIfUnreferenced(ctx context.Context, id, artifactPath string, remove func() error) (bool, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(artifactPath) == "" || remove == nil {
		return false, ErrInvalidRelease
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE plugin_artifact_cleanup_jobs
		SET status = ?
		WHERE id = ? AND artifact_path = ? AND status = ?
	`), CleanupRunning, id, artifactPath, CleanupRunning)
	if err != nil {
		return false, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return false, ErrNotFound
	}
	var references int
	if err := tx.GetContext(ctx, &references, tx.Rebind(`SELECT COUNT(*) FROM plugin_releases WHERE artifact_path = ?`), artifactPath); err != nil {
		return false, err
	}
	if references > 0 {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_artifact_cleanup_jobs SET status = ? WHERE id = ? AND status = ?`), CleanupCompleted, id, CleanupRunning); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := remove(); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_artifact_cleanup_jobs SET status = ? WHERE id = ? AND status = ?`), CleanupCompleted, id, CleanupRunning); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// ListCleanupJobs returns the durable cleanup inventory in stable order.
func (s *Store) ListCleanupJobs(ctx context.Context) ([]CleanupJob, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`
		SELECT id, workspace_id, instance_id, artifact_path, status, attempts,
		       last_error, created_at, next_attempt_at
		FROM plugin_artifact_cleanup_jobs
		ORDER BY created_at, id
	`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]CleanupJob, 0)
	for rows.Next() {
		var row cleanupRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		job, err := row.cleanupJob()
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// RequeueRunningCleanupJobs makes jobs recoverable after a process restart.
func (s *Store) RequeueRunningCleanupJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE plugin_artifact_cleanup_jobs
		SET status = ?, next_attempt_at = ?
		WHERE status = ?
	`), CleanupPending, time.Now().UTC().Format(time.RFC3339Nano), CleanupRunning)
	return err
}

type cleanupRow struct {
	ID            string `db:"id"`
	WorkspaceID   string `db:"workspace_id"`
	InstanceID    string `db:"instance_id"`
	ArtifactPath  string `db:"artifact_path"`
	Status        string `db:"status"`
	Attempts      int    `db:"attempts"`
	LastError     string `db:"last_error"`
	CreatedAt     string `db:"created_at"`
	NextAttemptAt string `db:"next_attempt_at"`
}

func (r cleanupRow) cleanupJob() (CleanupJob, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return CleanupJob{}, fmt.Errorf("parse cleanup created_at: %w", err)
	}
	nextAttemptAt, err := time.Parse(time.RFC3339Nano, r.NextAttemptAt)
	if err != nil {
		return CleanupJob{}, fmt.Errorf("parse cleanup next_attempt_at: %w", err)
	}
	return CleanupJob{
		ID:            r.ID,
		WorkspaceID:   r.WorkspaceID,
		InstanceID:    r.InstanceID,
		ArtifactPath:  r.ArtifactPath,
		Status:        r.Status,
		Attempts:      r.Attempts,
		LastError:     r.LastError,
		CreatedAt:     createdAt,
		NextAttemptAt: nextAttemptAt,
	}, nil
}

func enqueueCleanupJobTx(ctx context.Context, tx *sqlx.Tx, workspaceID, instanceID, artifactPath string) error {
	if strings.TrimSpace(artifactPath) == "" {
		return ErrInvalidRelease
	}
	var existing int
	if err := tx.GetContext(ctx, &existing, tx.Rebind(
		`SELECT COUNT(*) FROM plugin_artifact_cleanup_jobs WHERE instance_id = ? AND artifact_path = ? AND status <> ?`,
	), instanceID, artifactPath, "completed"); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(
		`INSERT INTO plugin_artifact_cleanup_jobs (id, workspace_id, instance_id, artifact_path, status, attempts, last_error, created_at, next_attempt_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	), uuid.NewString(), workspaceID, instanceID, artifactPath, "pending", 0, "", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return err
}

func artifactReferencedByOtherInstanceTx(ctx context.Context, tx *sqlx.Tx, instanceID, artifactPath string) (bool, error) {
	var references int
	if err := tx.GetContext(ctx, &references, tx.Rebind(
		`SELECT COUNT(*) FROM plugin_releases WHERE artifact_path = ? AND instance_id <> ?`,
	), artifactPath, instanceID); err != nil {
		return false, err
	}
	return references > 0, nil
}

func (s *Store) RemoveInstance(ctx context.Context, id string) error {
	return s.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return s.RemoveInstanceTx(ctx, tx, id)
	})
}

// RemoveInstanceTx records cleanup ownership, removes release and grant rows,
// and marks an instance removed in an existing lifecycle transaction.
func (s *Store) RemoveInstanceTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	var workspaceID string
	if err := tx.GetContext(ctx, &workspaceID, tx.Rebind(`SELECT workspace_id FROM plugin_instances WHERE id = ?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var paths []string
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT artifact_path FROM plugin_releases WHERE instance_id = ?`), id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			_ = rows.Close()
			return err
		}
		paths = append(paths, path)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, artifactPath := range paths {
		referenced, err := artifactReferencedByOtherInstanceTx(ctx, tx, id, artifactPath)
		if err != nil {
			return err
		}
		if referenced {
			continue
		}
		if err := enqueueCleanupJobTx(ctx, tx, workspaceID, id, artifactPath); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE plugin_instances SET status = ?, active_release_id = '', updated_at = ? WHERE id = ? AND status <> ?`), StatusRemoved, time.Now().UTC().Format(time.RFC3339Nano), id, StatusRemoved)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM plugin_releases WHERE instance_id = ?`), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM plugin_instance_grants WHERE plugin_instance_id = ?`), id); err != nil {
		return err
	}
	return nil
}

type instanceRow struct {
	ID              string `db:"id"`
	PluginID        string `db:"plugin_id"`
	SourceKind      string `db:"source_kind"`
	ScopeKind       string `db:"scope_kind"`
	WorkspaceID     string `db:"workspace_id"`
	TaskID          string `db:"task_id"`
	SessionID       string `db:"session_id"`
	RepositoryID    string `db:"repository_id"`
	Status          string `db:"status"`
	ActiveReleaseID string `db:"active_release_id"`
	GrantGeneration int64  `db:"grant_generation"`
	CreatedAt       string `db:"created_at"`
	UpdatedAt       string `db:"updated_at"`
}

func (r instanceRow) instance() (Instance, error) {
	created, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return Instance{}, err
	}
	updated, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return Instance{}, err
	}
	return Instance{ID: r.ID, PluginID: r.PluginID, SourceKind: r.SourceKind, ScopeKind: r.ScopeKind, WorkspaceID: r.WorkspaceID, TaskID: r.TaskID, SessionID: r.SessionID, RepositoryID: r.RepositoryID, Status: r.Status, ActiveReleaseID: r.ActiveReleaseID, GrantGeneration: r.GrantGeneration, CreatedAt: created, UpdatedAt: updated}, nil
}

type releaseRow struct {
	ID                      string `db:"id"`
	PluginID                string `db:"plugin_id"`
	InstanceID              string `db:"instance_id"`
	PackageDigest           string `db:"package_digest"`
	SourceKind              string `db:"source_kind"`
	SourceActorKind         string `db:"source_actor_kind"`
	SourceUserID            string `db:"source_user_id"`
	SourceTaskID            string `db:"source_task_id"`
	SourceSessionID         string `db:"source_session_id"`
	ManifestJSON            string `db:"manifest_json"`
	DeclaredPermissionsJSON string `db:"declared_permissions_json"`
	ArtifactPath            string `db:"artifact_path"`
	ArtifactBytes           int64  `db:"artifact_bytes"`
	ProtocolVersion         int    `db:"protocol_version"`
	ValidationStatus        string `db:"validation_status"`
	ValidationError         string `db:"validation_error"`
	CreatedAt               string `db:"created_at"`
}

func (r releaseRow) release() (Release, error) {
	created, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return Release{}, err
	}
	return Release{ID: r.ID, PluginID: r.PluginID, InstanceID: r.InstanceID, PackageDigest: r.PackageDigest, SourceKind: r.SourceKind, SourceActorKind: r.SourceActorKind, SourceUserID: r.SourceUserID, SourceTaskID: r.SourceTaskID, SourceSessionID: r.SourceSessionID, ManifestJSON: json.RawMessage(r.ManifestJSON), DeclaredPermissionsJSON: json.RawMessage(r.DeclaredPermissionsJSON), ArtifactPath: r.ArtifactPath, ArtifactBytes: r.ArtifactBytes, ProtocolVersion: r.ProtocolVersion, ValidationStatus: r.ValidationStatus, ValidationError: r.ValidationError, CreatedAt: created}, nil
}
