package backendapp

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	authhttpapi "github.com/kandev/kandev/internal/auth/httpapi"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// pluginSSOBridge adapts the auth service to plugins.AuthLoginBridge: it
// authenticates a plugin-asserted external identity (OIDC/SAML) and sets the
// session cookie (name derived from the request host), so an auth-capable
// plugin can complete SSO login without ever holding the raw session token.
// AuthenticateExternal enforces that auth is enabled, so this returns an error
// (surfaced as 403 by the plugin webhook relay) when it is not.
type pluginSSOBridge struct {
	auth *auth.Service
}

func (b pluginSSOBridge) LoginExternal(c *gin.Context, provider, subject, email, displayName string) error {
	_, token, err := b.auth.AuthenticateExternal(c.Request.Context(), auth.ExternalIdentity{
		Provider:    provider,
		Subject:     subject,
		Email:       email,
		DisplayName: displayName,
	}, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		return err
	}
	authhttpapi.SetSessionCookie(c, b.auth.CookieNameForRequest(c.Request), token, b.auth.SessionTTL())
	return nil
}

// provideAuthService wires the opt-in authentication service. Whether auth is
// enforced is read from cfg.Features.Auth (the `features.auth` runtime flag,
// already resolved at startup); credentials come from the auth store, accounts
// from the user store, and the setup-wizard ownership backfills from task +
// secrets. dbPool is retained for signature symmetry with the other providers.
func provideAuthService(
	ctx context.Context,
	cfg *config.Config,
	_ *db.Pool,
	repos *Repositories,
	log *logger.Logger,
) (*auth.Service, error) {
	backfills := []auth.BackfillFunc{
		// Pre-auth workspaces (owner_id='') become the admin's at setup.
		repos.Task.ClaimUnownedWorkspaces,
	}
	// Pre-auth secrets (user_id='') are claimed the same way. Interface
	// assertion keeps SecretStore mocks free of the method.
	if claimer, ok := repos.Secrets.(interface {
		ClaimUnowned(context.Context, string) error
	}); ok {
		backfills = append(backfills, claimer.ClaimUnowned)
	}
	return auth.NewService(ctx, auth.Deps{
		Cfg:       cfg,
		Store:     repos.Auth,
		Users:     repos.UserAccounts,
		Backfills: backfills,
		Log:       log,
	})
}

// gatewayAuthPolicy assembles the WS gateway scoping hooks from the auth and
// task services. The workspace-owner resolver caches lookups briefly: it runs
// on every workspace-scoped broadcast.
func gatewayAuthPolicy(authSvc *auth.Service, taskSvc *taskservice.Service, taskRepo *sqliterepo.Repository) gateways.AuthPolicy {
	return gateways.AuthPolicy{
		Enforced:     func() bool { return authSvc.Mode() != auth.ModeDisabled },
		ResolveToken: authSvc.ResolveBearer,
		// A restored subtree-capability identity must pass the same live
		// account gate as cookie/PAT auth: IdentityForUser fails when the
		// account is missing or no longer active.
		ActiveUser: func(ctx context.Context, userID string) bool {
			_, ok := authSvc.IdentityForUser(ctx, userID)
			return ok
		},
		Subscriptions: gateways.SubscriptionAccessPolicy{
			Task:    taskSvc.AuthorizeTaskAccess,
			Session: taskSvc.AuthorizeSessionAccess,
		},
		WorkspaceOwner:   newWorkspaceOwnerResolver(taskRepo),
		WorkspaceReaders: taskSvc.WorkspaceReaderIDs,
		// The user-shell actions name a task environment and treat task_id as
		// optional, so the dispatch backstop needs an environment-keyed check.
		ActionEnvironment: taskSvc.AuthorizeEnvironmentAccess,
	}
}

// ownerCacheTTL bounds staleness of broadcast routing after an ownership
// change (only the setup wizard's claim changes owners in practice).
const ownerCacheTTL = 30 * time.Second

func newWorkspaceOwnerResolver(taskRepo *sqliterepo.Repository) gateways.WorkspaceOwnerResolver {
	type cacheEntry struct {
		owner    string
		cachedAt time.Time
	}
	var mu sync.Mutex
	cache := map[string]cacheEntry{}
	return func(ctx context.Context, workspaceID string) (string, error) {
		mu.Lock()
		if entry, ok := cache[workspaceID]; ok && time.Since(entry.cachedAt) < ownerCacheTTL {
			mu.Unlock()
			return entry.owner, nil
		}
		mu.Unlock()
		workspace, err := taskRepo.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return "", err
		}
		mu.Lock()
		cache[workspaceID] = cacheEntry{owner: workspace.OwnerID, cachedAt: time.Now()}
		mu.Unlock()
		return workspace.OwnerID, nil
	}
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
		"enable the Authentication feature toggle (Settings > System > Feature Toggles) "+
		"or set KANDEV_FEATURES_AUTH=true",
		zap.Strings("binds", binds))
}
