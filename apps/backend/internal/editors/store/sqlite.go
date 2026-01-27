package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/sqlite"
	"github.com/kandev/kandev/internal/editors/discovery"
	"github.com/kandev/kandev/internal/editors/models"
)

type sqliteRepository struct {
	db     *sql.DB
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(dbConn *sql.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(dbConn, false)
}

func newSQLiteRepository(dbConn *sql.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: dbConn, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := dbConn.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	if err := repo.ensureDefaults(context.Background()); err != nil {
		if ownsDB {
			if closeErr := dbConn.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after defaults error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to seed editor defaults: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS editors (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		command TEXT NOT NULL,
		scheme TEXT NOT NULL,
		config TEXT,
		installed INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	_, err := r.db.Exec(schema)
	if err != nil {
		return err
	}
	columns := []columnDef{
		{name: "name", typ: "TEXT", defaultValue: "''"},
		{name: "kind", typ: "TEXT", defaultValue: "'built_in'"},
		{name: "config", typ: "TEXT", defaultValue: "NULL"},
		{name: "installed", typ: "INTEGER", defaultValue: "0"},
	}
	for _, column := range columns {
		if err := ensureColumn(r.db, "editors", column); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) ensureDefaults(ctx context.Context) error {
	definitions, err := discovery.LoadDefaults()
	if err != nil {
		return err
	}
	typeSet := make(map[string]struct{}, len(definitions))
	now := time.Now().UTC()
	defaults := make([]*models.Editor, 0, len(definitions))
	for _, def := range definitions {
		if def.Type == "" || def.Name == "" {
			continue
		}
		typeSet[def.Type] = struct{}{}
		installed := false
		if def.Command != "" {
			_, lookupErr := exec.LookPath(def.Command)
			installed = lookupErr == nil
		}
		defaults = append(defaults, &models.Editor{
			ID:        uuid.New().String(),
			Type:      def.Type,
			Name:      def.Name,
			Kind:      "built_in",
			Command:   def.Command,
			Scheme:    def.Scheme,
			Installed: installed,
			Enabled:   def.Enabled,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	if len(defaults) == 0 {
		return fmt.Errorf("no editor defaults loaded")
	}
	if err := r.UpsertEditors(ctx, defaults); err != nil {
		return err
	}
	if err := r.deleteBuiltinsNotIn(ctx, typeSet); err != nil {
		return err
	}
	return nil
}

func (r *sqliteRepository) ListEditors(ctx context.Context) ([]*models.Editor, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, type, name, kind, command, scheme, config, installed, enabled, created_at, updated_at
		FROM editors
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var editors []*models.Editor
	for rows.Next() {
		editor, err := scanEditor(rows)
		if err != nil {
			return nil, err
		}
		editors = append(editors, editor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return editors, nil
}

func (r *sqliteRepository) GetEditorByType(ctx context.Context, editorType string) (*models.Editor, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, name, kind, command, scheme, config, installed, enabled, created_at, updated_at
		FROM editors
		WHERE type = ?
	`, editorType)
	return scanEditor(row)
}

func (r *sqliteRepository) GetEditorByID(ctx context.Context, editorID string) (*models.Editor, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, type, name, kind, command, scheme, config, installed, enabled, created_at, updated_at
		FROM editors
		WHERE id = ?
	`, editorID)
	return scanEditor(row)
}

func (r *sqliteRepository) UpsertEditors(ctx context.Context, editors []*models.Editor) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO editors (id, type, name, kind, command, scheme, config, installed, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			command = excluded.command,
			scheme = excluded.scheme,
			config = excluded.config,
			installed = excluded.installed,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, editor := range editors {
		if editor.ID == "" {
			editor.ID = uuid.New().String()
		}
		if editor.CreatedAt.IsZero() {
			editor.CreatedAt = time.Now().UTC()
		}
		editor.UpdatedAt = time.Now().UTC()
		configValue, err := marshalConfig(editor.Config)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		_, err = stmt.ExecContext(
			ctx,
			editor.ID,
			editor.Type,
			editor.Name,
			editor.Kind,
			editor.Command,
			editor.Scheme,
			configValue,
			sqlite.BoolToInt(editor.Installed),
			sqlite.BoolToInt(editor.Enabled),
			editor.CreatedAt,
			editor.UpdatedAt,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (r *sqliteRepository) CreateEditor(ctx context.Context, editor *models.Editor) error {
	if editor == nil {
		return fmt.Errorf("editor is nil")
	}
	if editor.ID == "" {
		editor.ID = uuid.New().String()
	}
	if editor.CreatedAt.IsZero() {
		editor.CreatedAt = time.Now().UTC()
	}
	editor.UpdatedAt = time.Now().UTC()
	configValue, err := marshalConfig(editor.Config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO editors (id, type, name, kind, command, scheme, config, installed, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, editor.ID, editor.Type, editor.Name, editor.Kind, editor.Command, editor.Scheme, configValue, sqlite.BoolToInt(editor.Installed), sqlite.BoolToInt(editor.Enabled), editor.CreatedAt, editor.UpdatedAt)
	return err
}

func (r *sqliteRepository) UpdateEditor(ctx context.Context, editor *models.Editor) error {
	if editor == nil {
		return fmt.Errorf("editor is nil")
	}
	editor.UpdatedAt = time.Now().UTC()
	configValue, err := marshalConfig(editor.Config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE editors
		SET name = ?, kind = ?, command = ?, scheme = ?, config = ?, installed = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, editor.Name, editor.Kind, editor.Command, editor.Scheme, configValue, sqlite.BoolToInt(editor.Installed), sqlite.BoolToInt(editor.Enabled), editor.UpdatedAt, editor.ID)
	return err
}

func (r *sqliteRepository) DeleteEditor(ctx context.Context, editorID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM editors WHERE id = ?`, editorID)
	return err
}

func scanEditor(scanner interface{ Scan(dest ...any) error }) (*models.Editor, error) {
	editor := &models.Editor{}
	var config sql.NullString
	var installed int
	var enabled int
	if err := scanner.Scan(
		&editor.ID,
		&editor.Type,
		&editor.Name,
		&editor.Kind,
		&editor.Command,
		&editor.Scheme,
		&config,
		&installed,
		&enabled,
		&editor.CreatedAt,
		&editor.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if config.Valid && config.String != "" {
		editor.Config = json.RawMessage(config.String)
	}
	editor.Installed = installed == 1
	editor.Enabled = enabled == 1
	return editor, nil
}

type columnDef struct {
	name         string
	typ          string
	defaultValue string
}

func ensureColumn(db *sql.DB, table string, column columnDef) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return err
		}
		if name == column.name {
			return nil
		}
	}
	alter := fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s %s DEFAULT %s",
		table,
		column.name,
		column.typ,
		column.defaultValue,
	)
	_, err = db.Exec(alter)
	return err
}

func marshalConfig(config json.RawMessage) (string, error) {
	if len(config) == 0 {
		return "", nil
	}
	if !json.Valid(config) {
		return "", fmt.Errorf("invalid editor config json")
	}
	return string(config), nil
}

func (r *sqliteRepository) deleteBuiltinsNotIn(ctx context.Context, types map[string]struct{}) error {
	if len(types) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(types))
	args := make([]any, 0, len(types)+1)
	for editorType := range types {
		placeholders = append(placeholders, "?")
		args = append(args, editorType)
	}
	query := fmt.Sprintf(
		`DELETE FROM editors WHERE kind = 'built_in' AND type NOT IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}
