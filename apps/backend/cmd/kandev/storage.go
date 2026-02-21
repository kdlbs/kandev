package main

import (
	"fmt"
	"os"
	"path/filepath"

	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/repository"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func provideRepositories(cfg *config.Config, log *logger.Logger) (*db.Pool, *Repositories, []func() error, error) {
	cleanups := make([]func() error, 0, 6)
	pool, cleanup, err := persistence.Provide(cfg, log)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	writer := pool.Writer()
	reader := pool.Reader()

	taskRepoImpl, cleanup, err := repository.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	// Workflow repo must be initialized before analytics repo because
	// analytics creates indexes on the workflow_steps table.
	workflowRepo, err := workflowrepository.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}

	analyticsRepo, cleanup, err := analyticsrepository.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	agentSettingsRepo, cleanup, err := settingsstore.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	userRepo, cleanup, err := userstore.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	notificationRepo, cleanup, err := notificationstore.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	editorRepo, cleanup, err := editorstore.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	promptRepo, cleanup, err := promptstore.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	// Initialize master key and secrets store
	kandevDir := filepath.Join(os.Getenv("HOME"), ".kandev")
	masterKeyProvider, err := secrets.NewMasterKeyProvider(kandevDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("master key: %w", err)
	}

	secretStore, cleanup, err := secrets.Provide(writer, reader, masterKeyProvider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("secret store: %w", err)
	}
	cleanups = append(cleanups, cleanup)

	repos := &Repositories{
		Task:          taskRepoImpl,
		Analytics:     analyticsRepo,
		AgentSettings: agentSettingsRepo,
		User:          userRepo,
		Notification:  notificationRepo,
		Editor:        editorRepo,
		Prompts:       promptRepo,
		Workflow:      workflowRepo,
		Secrets:       secretStore,
	}
	return pool, repos, cleanups, nil
}
