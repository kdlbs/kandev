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
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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

// SessionCookieName returns the name of Kandev's own session cookie, used by
// the plugin webhook relay to strip it from headers forwarded to a plugin
// subprocess.
func (b pluginSSOBridge) SessionCookieName() string {
	return b.auth.CookieName()
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
	// Notification providers deliberately have no entry here. Unlike
	// workspaces and secrets, every provider row has always been written with
	// a concrete owner (userstore.DefaultUserID) rather than an empty one, and
	// Setup promotes that very row into the admin account, so pre-auth
	// providers already belong to the admin and there is nothing to claim.
	//
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

// notificationAuthEnforced tells the notification service whether more than
// one account can exist. It decides what an unresolvable notification owner
// means: with authentication enforced the notification is dropped, because
// falling back to the default user would deliver another user's task title and
// session state to the administrator's webhook. A nil auth service is a build
// with authentication unavailable, which is the single-user case.
func notificationAuthEnforced(authSvc *auth.Service) notificationservice.AuthEnforced {
	return func() bool { return authSvc != nil && authSvc.Mode() != auth.ModeDisabled }
}

// gatewayAuthPolicy assembles the WS gateway scoping hooks from the auth and
// task services. The workspace-owner resolver caches lookups briefly: it runs
// on every workspace-scoped broadcast.
func gatewayAuthPolicy(
	authSvc *auth.Service, taskSvc *taskservice.Service, taskRepo *sqliterepo.Repository, officeRepo *officesqlite.Repository,
) gateways.AuthPolicy {
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
			Run:     runSubscriptionCheck(taskSvc, officeRepo),
		},
		WorkspaceOwner:   newWorkspaceOwnerResolver(taskRepo),
		WorkspaceReaders: taskSvc.WorkspaceReaderIDs,
		// The user-shell actions name a task environment and treat task_id as
		// optional, so the dispatch backstop needs an environment-keyed check.
		ActionEnvironment: taskSvc.AuthorizeEnvironmentAccess,
	}
}

// runSubscriptionCheck resolves a run's owning workspace (via its agent
// profile) and defers to the task service's workspace reach rule.
// Unlike WorkspaceOwner (which runs on every workspace broadcast), this
// runs once per run.subscribe — a rare control message — so it is not
// cached.
//
// An ordinary (non-Office) Kanban agent profile has workspace_id=""
// (schema default, never backfilled) — the dominant case, since the run
// processor and workflow-engine queue_run dispatch run for every backend,
// not just Office. AuthorizeWorkspaceAccess's workspaceID=="" branch means
// "no workspace scoping applies" (used for dangling references elsewhere
// in the task service), which would silently allow subscribing to these
// runs; that meaning does not apply here, so an empty resolution is denied
// outright before it ever reaches that helper. officeRepo==nil fails
// closed for the same reason: this is a security check, not a visibility
// fallback.
func runSubscriptionCheck(taskSvc *taskservice.Service, officeRepo *officesqlite.Repository) func(context.Context, string) error {
	return func(ctx context.Context, runID string) error {
		if officeRepo == nil {
			return repoerrors.ErrWorkspaceNotFound
		}
		workspaceID, err := officeRepo.GetRunWorkspaceID(ctx, runID)
		if err != nil {
			return err
		}
		if workspaceID == "" {
			return repoerrors.ErrWorkspaceNotFound
		}
		return taskSvc.AuthorizeWorkspaceAccess(ctx, workspaceID)
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
