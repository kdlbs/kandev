package backendapp

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// provideAuthService wires the opt-in authentication service: mode state from
// the system settings store, credentials from the auth store, accounts from
// the user store, and the setup-wizard ownership backfills.
func provideAuthService(
	ctx context.Context,
	cfg *config.Config,
	dbPool *db.Pool,
	repos *Repositories,
	log *logger.Logger,
) (*auth.Service, error) {
	settingsStore, err := systemsettings.NewStore(dbPool)
	if err != nil {
		return nil, err
	}
	return auth.NewService(ctx, auth.Deps{
		Cfg:      cfg,
		Store:    repos.Auth,
		Users:    repos.UserAccounts,
		Settings: settingsStore,
		Backfills: []auth.BackfillFunc{
			// Pre-auth workspaces (owner_id='') become the admin's at setup.
			repos.Task.ClaimUnownedWorkspaces,
		},
		Log: log,
	})
}

// warnIfExposedWithoutAuth surfaces the fail-closed nudge promised by
// ServerConfig.NonLoopbackBinds: a server reachable off-box without
// authentication gets a prominent startup warning.
func warnIfExposedWithoutAuth(cfg *config.Config, svc *auth.Service, log *logger.Logger) {
	if svc == nil || svc.Mode() != auth.ModeDisabled {
		return
	}
	binds, err := cfg.Server.NonLoopbackBinds()
	if err != nil || len(binds) == 0 {
		return
	}
	log.Warn("server is reachable on non-loopback interfaces WITHOUT authentication; "+
		"enable it in Settings > System > Authentication or set KANDEV_AUTH_REQUIRED=true",
		zap.Strings("binds", binds))
}
