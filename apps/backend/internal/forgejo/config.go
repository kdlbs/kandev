package forgejo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrWorkspaceRequired = errors.New("forgejo: workspace_id required")
	ErrNotConfigured     = errors.New("forgejo: workspace not configured")
)

type Config struct {
	WorkspaceID   string     `json:"workspace_id" db:"workspace_id"`
	Origin        string     `json:"origin" db:"origin"`
	Username      string     `json:"username" db:"username"`
	HasSecret     bool       `json:"has_secret" db:"-"`
	LastOK        bool       `json:"last_ok" db:"last_ok"`
	LastError     string     `json:"last_error,omitempty" db:"last_error"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	Revision      int64      `json:"revision" db:"revision"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type SetConfigRequest struct {
	Origin string `json:"origin"`
	Token  string `json:"token,omitempty"`
}

type TestConnectionResult struct {
	OK       bool   `json:"ok"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

type WorkspaceSecretStore interface {
	Reveal(context.Context, string) (string, error)
	Set(context.Context, string, string, string) error
	Delete(context.Context, string) error
	Exists(context.Context, string) (bool, error)
}

func SecretKeyForWorkspace(workspaceID string) string {
	return "forgejo:" + strings.TrimSpace(workspaceID) + ":token"
}

type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func NewStore(db, ro *sqlx.DB) (*Store, error) {
	if db == nil || ro == nil {
		return nil, errors.New("forgejo store requires database handles")
	}
	store := &Store{db: db, ro: ro}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_configs (
		workspace_id TEXT PRIMARY KEY,
		origin TEXT NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		revision INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`)
	return store, err
}

func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*Config, error) {
	var config Config
	err := s.ro.GetContext(ctx, &config, `SELECT workspace_id, origin, username, last_ok, last_error, last_checked_at, revision, created_at, updated_at FROM forgejo_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) SaveConfig(ctx context.Context, config *Config) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_configs (workspace_id, origin, username, last_ok, last_error, last_checked_at, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET origin = excluded.origin, username = excluded.username, last_ok = excluded.last_ok, last_error = excluded.last_error, last_checked_at = excluded.last_checked_at, revision = forgejo_configs.revision + 1, updated_at = excluded.updated_at`,
		config.WorkspaceID, config.Origin, config.Username, config.LastOK, config.LastError, config.LastCheckedAt, now, now)
	return err
}

func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

type Service struct {
	store   *Store
	secrets WorkspaceSecretStore
}

func NewService(store *Store, secrets WorkspaceSecretStore) *Service {
	return &Service{store: store, secrets: secrets}
}

func (s *Service) GetConfig(ctx context.Context, workspaceID string) (*Config, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil || config == nil || s.secrets == nil {
		return config, err
	}
	config.HasSecret, err = s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	return config, err
}

func (s *Service) SetConfig(ctx context.Context, workspaceID string, request *SetConfigRequest) (*Config, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	if request == nil {
		return nil, errors.New("forgejo configuration required")
	}
	token := strings.TrimSpace(request.Token)
	if token == "" {
		if s.secrets == nil {
			return nil, errors.New("Forgejo secret store unavailable")
		}
		var err error
		token, err = s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
		if err != nil {
			return nil, errors.New("Forgejo token required")
		}
	}
	client, err := NewPATClient(request.Origin, token)
	if err != nil {
		return nil, err
	}
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("test Forgejo connection: %w", err)
	}
	config := &Config{WorkspaceID: workspaceID, Origin: client.origin.String(), Username: user.Login, LastOK: true}
	now := time.Now().UTC()
	config.LastCheckedAt = &now
	if s.secrets == nil {
		return nil, errors.New("Forgejo secret store unavailable")
	}
	if strings.TrimSpace(request.Token) != "" {
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Forgejo token", token); err != nil {
			return nil, err
		}
	}
	if err := s.store.SaveConfig(ctx, config); err != nil {
		return nil, err
	}
	return s.GetConfig(ctx, workspaceID)
}

func (s *Service) TestConfig(ctx context.Context, request *SetConfigRequest) *TestConnectionResult {
	if request == nil {
		return &TestConnectionResult{Error: "Forgejo configuration required"}
	}
	client, err := NewPATClient(request.Origin, request.Token)
	if err != nil {
		return &TestConnectionResult{Error: "invalid Forgejo origin"}
	}
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return &TestConnectionResult{Error: "Forgejo connection test failed"}
	}
	return &TestConnectionResult{OK: true, Username: user.Login}
}

func (s *Service) DeleteConfig(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			return err
		}
	}
	return s.store.DeleteConfig(ctx, workspaceID)
}
