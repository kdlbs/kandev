package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/repository"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/office"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func provideRepositories(cfg *config.Config, log *logger.Logger, version string) (*db.Pool, *Repositories, []func() error, error) {
	cleanups := make([]func() error, 0, 12)
	pool, cleanup, err := persistence.Provide(cfg, log, version)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	writer := pool.Writer()
	reader := pool.Reader()

	taskRepoImpl, cleanup, err := repository.Provide(writer, reader, log)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	// Workflow repo must be initialized before analytics repo because
	// analytics creates indexes on the workflow_steps table.
	workflowRepo, err := workflowrepository.NewWithDB(writer, reader, log)
	if err != nil {
		return nil, nil, nil, err
	}

	analyticsRepo, cleanup, err := analyticsrepository.Provide(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	agentSettingsRepo, cleanup, err := settingsstore.Provide(writer, reader, log)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, cleanup)

	supportRepos, supportCleanups, err := provideSupportRepos(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, supportCleanups...)

	officeRepo, officeCleanup, err := office.Provide(writer, reader, log)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("office repo: %w", err)
	}
	cleanups = append(cleanups, officeCleanup)

	masterKeyProvider, err := secrets.NewMasterKeyProvider(cfg.ResolvedDataDir())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("master key: %w", err)
	}
	secretStore, cleanup, err := secrets.Provide(writer, reader, masterKeyProvider)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("secret store: %w", err)
	}
	cleanups = append(cleanups, cleanup)

	// All repositories have finished their initSchema calls. Record the
	// current binary version so the next boot can detect upgrades correctly.
	recordSchemaVersion(writer, cfg.Database.Driver, version, log)

	repos := &Repositories{
		Task:          taskRepoImpl,
		Analytics:     analyticsRepo,
		AgentSettings: agentSettingsRepo,
		User:          supportRepos.user,
		Notification:  supportRepos.notification,
		Editor:        supportRepos.editor,
		Prompts:       supportRepos.prompts,
		Utility:       supportRepos.utility,
		Workflow:      workflowRepo,
		Secrets:       secretStore,
		Office:        officeRepo,
	}
	return pool, repos, cleanups, nil
}

// supportRepositorySet groups the lighter-weight support repositories
// that share a common (writer, reader) wire-up pattern.
type supportRepositorySet struct {
	user         userstore.Repository
	notification notificationstore.Repository
	editor       editorstore.Repository
	prompts      promptstore.Repository
	utility      utilitystore.Repository
}

// provideSupportRepos wires up user, notification, editor, prompt, and utility
// repositories. Extracted from provideRepositories to keep its statement count
// within the funlen limit.
func provideSupportRepos(writer, reader *sqlx.DB) (supportRepositorySet, []func() error, error) {
	var cleanups []func() error
	var repos supportRepositorySet

	userRepo, cleanup, err := userstore.Provide(writer, reader)
	if err != nil {
		return repos, nil, err
	}
	cleanups = append(cleanups, cleanup)
	repos.user = userRepo

	notificationRepo, cleanup, err := notificationstore.Provide(writer, reader)
	if err != nil {
		return repos, nil, err
	}
	cleanups = append(cleanups, cleanup)
	repos.notification = notificationRepo

	editorRepo, cleanup, err := editorstore.Provide(writer, reader)
	if err != nil {
		return repos, nil, err
	}
	cleanups = append(cleanups, cleanup)
	repos.editor = editorRepo

	promptRepo, cleanup, err := promptstore.Provide(writer, reader)
	if err != nil {
		return repos, nil, err
	}
	cleanups = append(cleanups, cleanup)
	repos.prompts = promptRepo

	utilityRepo, cleanup, err := utilitystore.Provide(writer, reader)
	if err != nil {
		return repos, nil, err
	}
	cleanups = append(cleanups, cleanup)
	repos.utility = utilityRepo

	return repos, cleanups, nil
}

// recordSchemaVersion writes the current binary version into kandev_meta so the
// next boot can detect upgrades. A failure here is non-fatal: the stored
// version stays at the previous value and the next boot will take a fresh
// backup (idempotent). Skipped for postgres, which has no kandev_meta table.
func recordSchemaVersion(writer *sqlx.DB, driver, version string, log *logger.Logger) {
	if version == "" || driver == "postgres" {
		return
	}
	if err := persistence.WriteVersion(writer, version); err != nil {
		if log != nil {
			log.Warn("failed to record schema version", zap.Error(err))
		}
		return
	}
	if log != nil {
		log.Info("schema version recorded", zap.String("version", version))
	}
}
