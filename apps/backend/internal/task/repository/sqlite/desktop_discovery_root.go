package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const desktopDiscoverySchemaDDL = `
	CREATE TABLE IF NOT EXISTS desktop_discovery_roots (
		id TEXT PRIMARY KEY,
		path TEXT NOT NULL UNIQUE,
		display_path TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'connected',
		last_scan_at TIMESTAMP,
		last_failure_at TIMESTAMP,
		last_failure_code TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_desktop_discovery_roots_state
		ON desktop_discovery_roots(state);
	CREATE TABLE IF NOT EXISTS desktop_discovery_migration (
		id INTEGER PRIMARY KEY,
		home_confirmation_required INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL
	);
`

func (r *Repository) initDesktopDiscoverySchema() error {
	legacyInstallation, err := r.tableExists("workspaces")
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(desktopDiscoverySchemaDDL); err != nil {
		return err
	}
	required := 0
	if legacyInstallation {
		required = 1
	}
	_, err = r.db.Exec(r.db.Rebind(`
		INSERT INTO desktop_discovery_migration (id, home_confirmation_required, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), required, time.Now().UTC())
	return err
}

const desktopDiscoveryRootColumns = `id, path, display_path, state, last_scan_at, last_failure_at, last_failure_code, created_at, updated_at`

func (r *Repository) scanDesktopDiscoveryRoot(scanner interface{ Scan(...any) error }) (*models.DesktopDiscoveryRoot, error) {
	root := &models.DesktopDiscoveryRoot{}
	var state string
	var lastScanAt, lastFailureAt sql.NullTime
	if err := scanner.Scan(
		&root.ID, &root.Path, &root.DisplayPath, &state,
		&lastScanAt, &lastFailureAt, &root.LastFailureCode,
		&root.CreatedAt, &root.UpdatedAt,
	); err != nil {
		return nil, err
	}
	root.State = models.DesktopDiscoveryRootState(state)
	if lastScanAt.Valid {
		value := lastScanAt.Time
		root.LastScanAt = &value
	}
	if lastFailureAt.Valid {
		value := lastFailureAt.Time
		root.LastFailureAt = &value
	}
	return root, nil
}

func (r *Repository) ListDesktopDiscoveryRoots(ctx context.Context) ([]*models.DesktopDiscoveryRoot, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+desktopDiscoveryRootColumns+` FROM desktop_discovery_roots
		ORDER BY LOWER(display_path), path
	`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	roots := make([]*models.DesktopDiscoveryRoot, 0)
	for rows.Next() {
		root, err := r.scanDesktopDiscoveryRoot(rows)
		if err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

func (r *Repository) GetDesktopDiscoveryRoot(ctx context.Context, path string) (*models.DesktopDiscoveryRoot, error) {
	root, err := r.scanDesktopDiscoveryRoot(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+desktopDiscoveryRootColumns+` FROM desktop_discovery_roots WHERE path = ?`,
	), path))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return root, err
}

func (r *Repository) CreateDesktopDiscoveryRoot(ctx context.Context, root *models.DesktopDiscoveryRoot) error {
	if root.ID == "" {
		root.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	root.CreatedAt = now
	root.UpdatedAt = now
	if root.State == "" {
		root.State = models.DesktopDiscoveryRootConnected
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO desktop_discovery_roots (
			id, path, display_path, state, last_scan_at, last_failure_at,
			last_failure_code, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), root.ID, root.Path, root.DisplayPath, root.State, root.LastScanAt,
		root.LastFailureAt, root.LastFailureCode, root.CreatedAt, root.UpdatedAt)
	return err
}

func (r *Repository) UpdateDesktopDiscoveryRoot(ctx context.Context, root *models.DesktopDiscoveryRoot) error {
	root.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE desktop_discovery_roots
		SET path = ?, display_path = ?, state = ?, last_scan_at = ?,
			last_failure_at = ?, last_failure_code = ?, updated_at = ?
		WHERE id = ?
	`), root.Path, root.DisplayPath, root.State, root.LastScanAt, root.LastFailureAt,
		root.LastFailureCode, root.UpdatedAt, root.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return repoerrors.ErrRepositoryNotFound
	}
	return nil
}

func (r *Repository) DeleteDesktopDiscoveryRoot(ctx context.Context, path string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM desktop_discovery_roots WHERE path = ?`), path)
	return err
}

func (r *Repository) GetDesktopDiscoveryMigration(ctx context.Context) (*models.DesktopDiscoveryMigration, error) {
	migration := &models.DesktopDiscoveryMigration{}
	var required int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT home_confirmation_required, updated_at
		FROM desktop_discovery_migration WHERE id = 1
	`)).Scan(&required, &migration.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return migration, nil
	}
	if err != nil {
		return nil, err
	}
	migration.HomeConfirmationRequired = required != 0
	return migration, nil
}

func (r *Repository) SetDesktopDiscoveryMigration(ctx context.Context, migration *models.DesktopDiscoveryMigration) error {
	migration.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO desktop_discovery_migration (id, home_confirmation_required, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			home_confirmation_required = excluded.home_confirmation_required,
			updated_at = excluded.updated_at
	`), dialect.BoolToInt(migration.HomeConfirmationRequired), migration.UpdatedAt)
	return err
}
