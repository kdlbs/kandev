package main

import (
	"github.com/jmoiron/sqlx"

	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/task/repository"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func provideRepositories(cfg *config.Config, log *logger.Logger) (*sqlx.DB, *Repositories, []func() error, error) {
	cleanups := make([]func() error, 0, 6)
	dbConn, cleanup, err := persistence.Provide(cfg, log)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	taskRepoImpl, cleanup, err := repository.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	// Workflow repo must be initialized before analytics repo because
	// analytics creates indexes on the workflow_steps table.
	workflowRepo, err := workflowrepository.NewWithDB(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}

	analyticsRepo, cleanup, err := analyticsrepository.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	agentSettingsRepo, cleanup, err := settingsstore.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	userRepo, cleanup, err := userstore.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	notificationRepo, cleanup, err := notificationstore.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	editorRepo, cleanup, err := editorstore.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	promptRepo, cleanup, err := promptstore.Provide(dbConn)
	if err != nil {
		return nil, nil, nil, err
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
	}
	return dbConn, repos, cleanups, nil
}
