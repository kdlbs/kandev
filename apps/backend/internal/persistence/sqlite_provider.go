package persistence

import (
	"database/sql"
	"fmt"

	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agent/worktree"
	"github.com/kandev/kandev/internal/db"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/task/repository"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type SQLiteProvider struct {
	db                *sql.DB
	taskRepo          repository.Repository
	agentSettingsRepo agentsettingsstore.Repository
	userRepo          userstore.Repository
	notificationRepo  notificationstore.Repository
	editorRepo        editorstore.Repository
	worktreeStore     worktree.Store
}

func NewSQLiteProvider(dbPath string) (*SQLiteProvider, error) {
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	return &SQLiteProvider{db: dbConn}, nil
}

func (p *SQLiteProvider) TaskRepo() (repository.Repository, error) {
	if p.taskRepo != nil {
		return p.taskRepo, nil
	}
	repo, err := repository.NewSQLiteRepositoryWithDB(p.db)
	if err != nil {
		return nil, err
	}
	p.taskRepo = repo
	return repo, nil
}

func (p *SQLiteProvider) AgentSettingsRepo() (agentsettingsstore.Repository, error) {
	if p.agentSettingsRepo != nil {
		return p.agentSettingsRepo, nil
	}
	repo, err := agentsettingsstore.NewSQLiteRepositoryWithDB(p.db)
	if err != nil {
		return nil, err
	}
	p.agentSettingsRepo = repo
	return repo, nil
}

func (p *SQLiteProvider) UserRepo() (userstore.Repository, error) {
	if p.userRepo != nil {
		return p.userRepo, nil
	}
	repo, err := userstore.NewSQLiteRepositoryWithDB(p.db)
	if err != nil {
		return nil, err
	}
	p.userRepo = repo
	return repo, nil
}

func (p *SQLiteProvider) NotificationRepo() (notificationstore.Repository, error) {
	if p.notificationRepo != nil {
		return p.notificationRepo, nil
	}
	repo, err := notificationstore.NewSQLiteRepositoryWithDB(p.db)
	if err != nil {
		return nil, err
	}
	p.notificationRepo = repo
	return repo, nil
}

func (p *SQLiteProvider) EditorRepo() (editorstore.Repository, error) {
	if p.editorRepo != nil {
		return p.editorRepo, nil
	}
	repo, err := editorstore.NewSQLiteRepositoryWithDB(p.db)
	if err != nil {
		return nil, err
	}
	p.editorRepo = repo
	return repo, nil
}

func (p *SQLiteProvider) WorktreeStore() (worktree.Store, error) {
	if p.worktreeStore != nil {
		return p.worktreeStore, nil
	}
	store, err := worktree.NewSQLiteStore(p.db)
	if err != nil {
		return nil, err
	}
	p.worktreeStore = store
	return store, nil
}

func (p *SQLiteProvider) Close() error {
	if p.db == nil {
		return nil
	}
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}
	return nil
}
