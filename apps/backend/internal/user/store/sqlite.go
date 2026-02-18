package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/user/models"
)

const (
	DefaultUserID    = "default-user"
	DefaultUserEmail = "default@kandev.local"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

func newSQLiteRepository(writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := dbConn.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		settings TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	return r.ensureDefaultUser()
}

func (r *sqliteRepository) ensureDefaultUser() error {
	ctx := context.Background()
	var count int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind("SELECT COUNT(1) FROM users WHERE id = ?"), DefaultUserID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := time.Now().UTC()
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO users (id, email, settings, created_at, updated_at)
			VALUES (?, ?, '{}', ?, ?)
		`), DefaultUserID, DefaultUserEmail, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) GetUser(ctx context.Context, id string) (*models.User, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, email, created_at, updated_at
		FROM users WHERE id = ?
	`), id)
	return scanUser(row)
}

func (r *sqliteRepository) GetDefaultUser(ctx context.Context) (*models.User, error) {
	return r.GetUser(ctx, DefaultUserID)
}

func (r *sqliteRepository) GetUserSettings(ctx context.Context, userID string) (*models.UserSettings, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT settings, updated_at
		FROM users WHERE id = ?
	`), userID)
	settings, err := scanUserSettings(row, userID)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *sqliteRepository) UpsertUserSettings(ctx context.Context, settings *models.UserSettings) error {
	settings.UpdatedAt = time.Now().UTC()
	if settings.CreatedAt.IsZero() {
		settings.CreatedAt = settings.UpdatedAt
	}
	lspAutoStart := settings.LspAutoStartLanguages
	if lspAutoStart == nil {
		lspAutoStart = []string{}
	}
	lspAutoInstall := settings.LspAutoInstallLanguages
	if lspAutoInstall == nil {
		lspAutoInstall = []string{}
	}
	lspServerConfigs := settings.LspServerConfigs
	if lspServerConfigs == nil {
		lspServerConfigs = map[string]map[string]interface{}{}
	}
	settingsPayload, err := json.Marshal(map[string]interface{}{
		"workspace_id":               settings.WorkspaceID,
		"kanban_view_mode":           settings.KanbanViewMode,
		"workflow_filter_id":         settings.WorkflowFilterID,
		"repository_ids":             settings.RepositoryIDs,
		"initial_setup_complete":     settings.InitialSetupComplete,
		"preferred_shell":            settings.PreferredShell,
		"default_editor_id":          settings.DefaultEditorID,
		"enable_preview_on_click":    settings.EnablePreviewOnClick,
		"chat_submit_key":            settings.ChatSubmitKey,
		"review_auto_mark_on_scroll": settings.ReviewAutoMarkOnScroll,
		"lsp_auto_start_languages":   lspAutoStart,
		"lsp_auto_install_languages": lspAutoInstall,
		"lsp_server_configs":         lspServerConfigs,
	})
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE users
		SET settings = ?, updated_at = ?
		WHERE id = ?
	`), string(settingsPayload), settings.UpdatedAt, settings.UserID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found: %s", settings.UserID)
	}
	return nil
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
		WorkspaceID             string   `json:"workspace_id"`
		KanbanViewMode          string   `json:"kanban_view_mode"`
		WorkflowFilterID        string   `json:"workflow_filter_id"`
		RepositoryIDs           []string `json:"repository_ids"`
		InitialSetupComplete    bool     `json:"initial_setup_complete"`
		PreferredShell          string   `json:"preferred_shell"`
		DefaultEditorID         string   `json:"default_editor_id"`
		EnablePreviewOnClick    bool     `json:"enable_preview_on_click"`
		ChatSubmitKey           string   `json:"chat_submit_key"`
		ReviewAutoMarkOnScroll  *bool    `json:"review_auto_mark_on_scroll"`
		LspAutoStartLanguages   []string                          `json:"lsp_auto_start_languages"`
		LspAutoInstallLanguages []string                          `json:"lsp_auto_install_languages"`
		LspServerConfigs        map[string]map[string]interface{} `json:"lsp_server_configs"`
	}
	if err := json.Unmarshal([]byte(settingsRaw), &payload); err != nil {
		return nil, err
	}
	settings.WorkspaceID = payload.WorkspaceID
	settings.KanbanViewMode = payload.KanbanViewMode
	settings.WorkflowFilterID = payload.WorkflowFilterID
	settings.RepositoryIDs = payload.RepositoryIDs
	settings.InitialSetupComplete = payload.InitialSetupComplete
	settings.PreferredShell = payload.PreferredShell
	settings.DefaultEditorID = payload.DefaultEditorID
	settings.EnablePreviewOnClick = payload.EnablePreviewOnClick
	settings.ChatSubmitKey = payload.ChatSubmitKey
	// Default to "cmd_enter" if empty
	if settings.ChatSubmitKey == "" {
		settings.ChatSubmitKey = "cmd_enter"
	}
	// Default to true when not set
	if payload.ReviewAutoMarkOnScroll != nil {
		settings.ReviewAutoMarkOnScroll = *payload.ReviewAutoMarkOnScroll
	} else {
		settings.ReviewAutoMarkOnScroll = true
	}
	settings.LspAutoStartLanguages = payload.LspAutoStartLanguages
	if settings.LspAutoStartLanguages == nil {
		settings.LspAutoStartLanguages = []string{}
	}
	settings.LspAutoInstallLanguages = payload.LspAutoInstallLanguages
	if settings.LspAutoInstallLanguages == nil {
		settings.LspAutoInstallLanguages = []string{}
	}
	settings.LspServerConfigs = payload.LspServerConfigs
	return settings, nil
}
