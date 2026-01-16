package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/user/models"
)

const (
	DefaultUserID    = "default-user"
	DefaultUserEmail = "default@kandev.local"
)

type SQLiteRepository struct {
	db *sql.DB
}

var _ Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	normalizedPath := normalizeSQLitePath(dbPath)
	if err := ensureSQLiteDir(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to prepare database path: %w", err)
	}
	if err := ensureSQLiteFile(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to create database file: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_mode=rwc", normalizedPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	repo := &SQLiteRepository{db: db}
	if err := repo.initSchema(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func ensureSQLiteDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func ensureSQLiteFile(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func normalizeSQLitePath(dbPath string) string {
	if dbPath == "" {
		return dbPath
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return dbPath
	}
	return abs
}

func (r *SQLiteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		settings TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	if err := r.ensureUserSettingsColumn(); err != nil {
		return err
	}

	return r.ensureDefaultUser()
}

func (r *SQLiteRepository) ensureUserSettingsColumn() error {
	exists, err := r.columnExists("users", "settings")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.db.Exec(`ALTER TABLE users ADD COLUMN settings TEXT NOT NULL DEFAULT '{}'`)
	return err
}

func (r *SQLiteRepository) columnExists(table, column string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *SQLiteRepository) ensureDefaultUser() error {
	ctx := context.Background()
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE id = ?", DefaultUserID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := time.Now().UTC()
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO users (id, email, settings, created_at, updated_at)
			VALUES (?, ?, '{}', ?, ?)
		`, DefaultUserID, DefaultUserEmail, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) GetUser(ctx context.Context, id string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, created_at, updated_at
		FROM users WHERE id = ?
	`, id)
	return scanUser(row)
}

func (r *SQLiteRepository) GetDefaultUser(ctx context.Context) (*models.User, error) {
	return r.GetUser(ctx, DefaultUserID)
}

func (r *SQLiteRepository) GetUserSettings(ctx context.Context, userID string) (*models.UserSettings, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT settings, updated_at
		FROM users WHERE id = ?
	`, userID)
	settings, err := scanUserSettings(row, userID)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *SQLiteRepository) UpsertUserSettings(ctx context.Context, settings *models.UserSettings) error {
	settings.UpdatedAt = time.Now().UTC()
	if settings.CreatedAt.IsZero() {
		settings.CreatedAt = settings.UpdatedAt
	}
	settingsPayload, err := json.Marshal(map[string]interface{}{
		"workspace_id":           settings.WorkspaceID,
		"board_id":               settings.BoardID,
		"repository_ids":         settings.RepositoryIDs,
		"initial_setup_complete": settings.InitialSetupComplete,
	})
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET settings = ?, updated_at = ?
		WHERE id = ?
	`, string(settingsPayload), settings.UpdatedAt, settings.UserID)
	if err == nil {
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("user not found: %s", settings.UserID)
		}
	}
	return err
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (*models.User, error) {
	user := &models.User{}
	if err := scanner.Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, err
	}
	return user, nil
}

func scanUserSettings(scanner interface{ Scan(dest ...any) error }, userID string) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	var settingsRaw string
	if err := scanner.Scan(&settingsRaw, &settings.UpdatedAt); err != nil {
		return nil, err
	}
	settings.UserID = userID
	if settingsRaw == "" || settingsRaw == "{}" {
		settings.RepositoryIDs = []string{}
		return settings, nil
	}
	var payload struct {
		WorkspaceID          string   `json:"workspace_id"`
		BoardID              string   `json:"board_id"`
		RepositoryIDs        []string `json:"repository_ids"`
		InitialSetupComplete bool     `json:"initial_setup_complete"`
	}
	if err := json.Unmarshal([]byte(settingsRaw), &payload); err != nil {
		return nil, err
	}
	settings.WorkspaceID = payload.WorkspaceID
	settings.BoardID = payload.BoardID
	settings.RepositoryIDs = payload.RepositoryIDs
	settings.InitialSetupComplete = payload.InitialSetupComplete
	return settings, nil
}
