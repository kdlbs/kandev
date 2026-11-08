package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// Synthetic accounts: owner-a and collaborator-b share org-1, outsider-c is
// in org-2. Only owner-a owns the shared global secret.
const (
	sharedSecretID      = "synthetic-shared-global"
	sharedSecretValue   = "synthetic-shared-value"
	ownerWorkspaceID    = "synthetic-owner-workspace"
	collabSecretID      = "synthetic-collab-global"
	sharedTestEnvKey    = "SHARED_TOKEN"
	sharedTestOwner     = "owner-a"
	sharedTestCollab    = "collaborator-b"
	sharedTestOutsider  = "outsider-c"
	sharedTestOrg       = "org-1"
	sharedTestOtherOrg  = "org-2"
	sharedValuePrefix   = "synthetic-"
	sharedTestProfileID = "Shared profile"
)

func sharedSecretTestStore(t *testing.T) secrets.SecretStore {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.OpenSQLite(filepath.Join(dir, "secrets.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	crypto, err := secrets.NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store, cleanup, err := secrets.Provide(sqlxDB, sqlxDB, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	seeds := []struct {
		owner  string
		secret secrets.SecretWithValue
	}{
		{sharedTestOwner, secrets.SecretWithValue{
			Secret: secrets.Secret{ID: sharedSecretID, Name: "shared", Scope: secrets.ScopeGlobal},
			Value:  sharedSecretValue,
		}},
		{sharedTestOwner, secrets.SecretWithValue{
			Secret: secrets.Secret{ID: ownerWorkspaceID, Name: "ws", Scope: secrets.ScopeWorkspace, WorkspaceID: "ws-1"},
			Value:  "synthetic-workspace-value",
		}},
		{sharedTestCollab, secrets.SecretWithValue{
			Secret: secrets.Secret{ID: collabSecretID, Name: "collab", Scope: secrets.ScopeGlobal},
			Value:  "synthetic-collab-value",
		}},
	}
	for _, seed := range seeds {
		value := seed.secret
		if err := store.Create(sharedSecretCtx(seed.owner, sharedTestOrg), &value); err != nil {
			t.Fatalf("seed %s: %v", seed.secret.ID, err)
		}
	}
	return store
}

func sharedSecretCtx(userID, orgID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{
		UserID: userID, OrgID: orgID, Role: authn.RoleMember,
	})
}

// activeGrantors resolves only the listed users, mirroring the auth
// service's active-account lookup.
func activeGrantors(identities ...authn.Identity) SharedProfileSecretGrantorResolver {
	byID := make(map[string]authn.Identity, len(identities))
	for _, identity := range identities {
		byID[identity.UserID] = identity
	}
	return func(_ context.Context, userID string) (authn.Identity, bool) {
		identity, ok := byID[userID]
		return identity, ok
	}
}

var activeOwner = authn.Identity{UserID: sharedTestOwner, OrgID: sharedTestOrg, Role: authn.RoleMember}

func newSharedSecretManager(t *testing.T, resolver SharedProfileSecretGrantorResolver) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	m := &Manager{logger: log, secretStore: sharedSecretTestStore(t)}
	m.SetSharedProfileSecretGrantorResolver(resolver)
	return m
}

func sharedBinding(secretID, grantor string) []settingsmodels.ProfileEnvVar {
	return []settingsmodels.ProfileEnvVar{{Key: sharedTestEnvKey, SecretID: secretID, SharedByUserID: grantor}}
}

func assertNoSecretLeak(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), sharedValuePrefix) {
		t.Fatalf("error leaks a secret identifier or value: %v", err)
	}
}

func TestSharedProfileSecretResolvesForAuthorizedStarters(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"owner", sharedSecretCtx(sharedTestOwner, sharedTestOrg)},
		{"collaborator in the owner's org", sharedSecretCtx(sharedTestCollab, sharedTestOrg)},
		{"agent starter scoped to the collaborator", authn.WithIdentity(context.Background(), authn.Identity{
			UserID: sharedTestCollab, OrgID: sharedTestOrg, Role: authn.RoleMember, TokenID: "synthetic-pat",
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newSharedSecretManager(t, activeGrantors(activeOwner))
			got, err := m.resolveAgentProfileEnvVars(tc.ctx, sharedBinding(sharedSecretID, sharedTestOwner))
			if err != nil || got[sharedTestEnvKey] != sharedSecretValue {
				t.Fatalf("resolve = %v, %v", got, err)
			}
		})
	}
}

func TestSharedProfileSecretDeniesUnauthorizedUse(t *testing.T) {
	collab := sharedSecretCtx(sharedTestCollab, sharedTestOrg)
	cases := []struct {
		name     string
		ctx      context.Context
		resolver SharedProfileSecretGrantorResolver
		envVars  []settingsmodels.ProfileEnvVar
		reason   string
	}{
		{"no grant", collab, activeGrantors(activeOwner), sharedBinding(sharedSecretID, ""), "owned by another user"},
		{"grantor inactive", collab, activeGrantors(), sharedBinding(sharedSecretID, sharedTestOwner), sharedSecretReasonGrantorInactive},
		{"grantor identity mismatch", collab, func(context.Context, string) (authn.Identity, bool) {
			return authn.Identity{UserID: "someone-else", OrgID: sharedTestOrg}, true
		}, sharedBinding(sharedSecretID, sharedTestOwner), sharedSecretReasonGrantorInactive},
		{"synthetic grantor", collab, func(context.Context, string) (authn.Identity, bool) {
			return authn.Identity{UserID: sharedTestOwner, Synthetic: true}, true
		}, sharedBinding(sharedSecretID, sharedTestOwner), sharedSecretReasonGrantorInactive},
		{"cross-org starter", sharedSecretCtx(sharedTestOutsider, sharedTestOtherOrg), activeGrantors(activeOwner),
			sharedBinding(sharedSecretID, sharedTestOwner), sharedSecretReasonCrossOrg},
		{"resolver not wired", collab, nil, sharedBinding(sharedSecretID, sharedTestOwner), sharedSecretReasonUnsupported},
		{"forged grantor who does not own the secret", sharedSecretCtx(sharedTestOutsider, sharedTestOrg),
			activeGrantors(authn.Identity{UserID: sharedTestCollab, OrgID: sharedTestOrg}),
			sharedBinding(sharedSecretID, sharedTestCollab), sharedSecretReasonOwnerMissed},
		{"workspace-scoped secret", collab, activeGrantors(activeOwner),
			sharedBinding(ownerWorkspaceID, sharedTestOwner), sharedSecretReasonOwnerMissed},
		{"missing secret", collab, activeGrantors(activeOwner),
			sharedBinding("synthetic-missing", sharedTestOwner), sharedSecretReasonOwnerMissed},
		{"unauthenticated starter", context.Background(), activeGrantors(activeOwner),
			sharedBinding("synthetic-missing", sharedTestOwner), sharedSecretReasonUnauthenticated},
		{"owner cannot borrow a collaborator's private secret", sharedSecretCtx(sharedTestOwner, sharedTestOrg),
			activeGrantors(activeOwner), sharedBinding(collabSecretID, sharedTestOwner), sharedSecretReasonOwnerMissed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newSharedSecretManager(t, tc.resolver)
			got, err := m.resolveAgentProfileEnvVars(tc.ctx, tc.envVars)
			if !errors.Is(err, ErrProfileSecretUnavailable) || got != nil {
				t.Fatalf("resolve = %v, %v; want unavailable", got, err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("error %q lacks reason %q", err, tc.reason)
			}
			assertNoSecretLeak(t, err)
		})
	}
}

func TestSharedProfileSecretPartialDropsOnlyDeniedEntry(t *testing.T) {
	m := newSharedSecretManager(t, activeGrantors())
	got, err := m.resolveAgentProfileEnvVarsWithPartial(sharedSecretCtx(sharedTestCollab, sharedTestOrg), []settingsmodels.ProfileEnvVar{
		{Key: sharedTestEnvKey, SecretID: sharedSecretID, SharedByUserID: sharedTestOwner},
		{Key: "PLAIN", Value: "plain"},
	}, true)
	if err != nil {
		t.Fatalf("partial resolve: %v", err)
	}
	if _, ok := got[sharedTestEnvKey]; ok || got["PLAIN"] != "plain" {
		t.Fatalf("partial resolve = %v; want only PLAIN", got)
	}
}

func TestSharedProfileSecretPropagatesCancellation(t *testing.T) {
	m := newSharedSecretManager(t, activeGrantors(activeOwner))
	ctx, cancel := context.WithCancel(sharedSecretCtx(sharedTestCollab, sharedTestOrg))
	cancel()
	_, err := m.resolveAgentProfileEnvVars(ctx, sharedBinding(sharedSecretID, sharedTestOwner))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve after cancel = %v; want context.Canceled", err)
	}
}

func TestSharedProfileSecretAuthDisabledStaysUnscoped(t *testing.T) {
	// Auth-disabled installs run as one synthetic admin, whose secret reads
	// are unscoped; a grant neither widens nor narrows that.
	m := newSharedSecretManager(t, activeGrantors())
	ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true, Role: authn.RoleAdmin})
	got, err := m.resolveAgentProfileEnvVars(ctx, sharedBinding(sharedSecretID, ""))
	if err != nil || got[sharedTestEnvKey] != sharedSecretValue {
		t.Fatalf("synthetic resolve = %v, %v", got, err)
	}
}

// An executor profile row carrying a grant (a legacy or direct write) must not
// let the session environment reveal another user's secret, even with an
// active same-org grantor that would satisfy an agent-profile share.
func TestExecutorProfileBindingsNeverHonorAgentProfileShares(t *testing.T) {
	const profileID = "synthetic-executor-profile"
	reader := &fakeExecutorProfileReader{
		session: &models.TaskSession{ID: "session-1", ExecutorProfileID: profileID},
		profiles: map[string]*models.ExecutorProfile{
			profileID: {ID: profileID, EnvVars: sharedBinding(sharedSecretID, sharedTestOwner)},
		},
	}
	m := newSharedSecretManager(t, activeGrantors(activeOwner))
	m.SetExecutorProfileReader(reader)

	ownerEnv, err := m.ExecutorProfileEnvForSession(sharedSecretCtx(sharedTestOwner, sharedTestOrg), "session-1", "")
	if err != nil || ownerEnv[sharedTestEnvKey] != sharedSecretValue {
		t.Fatalf("owner executor profile env = %v", err)
	}

	collabEnv, err := m.ExecutorProfileEnvForSession(sharedSecretCtx(sharedTestCollab, sharedTestOrg), "session-1", "")
	if !errors.Is(err, ErrProfileSecretUnavailable) {
		t.Fatalf("collaborator resolved an executor profile share: %v", err)
	}
	if len(collabEnv) != 0 {
		t.Fatalf("collaborator received executor profile env: %d entries", len(collabEnv))
	}
	assertNoSecretLeak(t, err)
}

func sharedStrictManager(t *testing.T) *Manager {
	t.Helper()
	m := newTestManager(t)
	m.secretStore = sharedSecretTestStore(t)
	m.SetSharedProfileSecretGrantorResolver(activeGrantors(activeOwner))
	return m
}

func sharedProfileInfo() *AgentProfileInfo {
	return &AgentProfileInfo{ProfileName: sharedTestProfileID, EnvVars: sharedBinding(sharedSecretID, sharedTestOwner)}
}

func TestStrictEnvironmentHonorsSharedAgentProfileBinding(t *testing.T) {
	m := sharedStrictManager(t)
	req := &LaunchRequest{TaskID: "task-1", SessionID: "session-1", EnvironmentResolutionRequired: true}
	env, err := m.buildEnvForExecution(sharedSecretCtx(sharedTestCollab, sharedTestOrg), "exec-1", req, nil, sharedProfileInfo())
	if err != nil || env[sharedTestEnvKey] != sharedSecretValue {
		t.Fatalf("strict resolve = %v", err)
	}
}

func TestStrictEnvironmentDeniesSharedSecretOutsideItsBinding(t *testing.T) {
	m := sharedStrictManager(t)
	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1", EnvironmentResolutionRequired: true,
		EnvironmentDefinitions: []runtimeenv.Definition{
			{Key: "OTHER_TOKEN", SecretID: sharedSecretID, Origin: runtimeenv.OriginExecutorProfile},
		},
	}
	_, err := m.buildEnvForExecution(sharedSecretCtx(sharedTestCollab, sharedTestOrg), "exec-1", req, nil, sharedProfileInfo())
	var secretErr *runtimeenv.SecretError
	if !errors.As(err, &secretErr) || secretErr.Key != "OTHER_TOKEN" {
		t.Fatalf("executor binding of a shared secret = %v; want secret error on OTHER_TOKEN", err)
	}
	assertNoSecretLeak(t, err)
}

func TestStrictEnvironmentNamesProfileAndReasonOnDenial(t *testing.T) {
	m := sharedStrictManager(t)
	req := &LaunchRequest{TaskID: "task-1", SessionID: "session-1", EnvironmentResolutionRequired: true}
	_, err := m.buildEnvForExecution(sharedSecretCtx(sharedTestOutsider, sharedTestOtherOrg), "exec-1", req, nil, sharedProfileInfo())
	if err == nil || !strings.Contains(err.Error(), sharedTestProfileID) || !strings.Contains(err.Error(), sharedSecretReasonCrossOrg) {
		t.Fatalf("cross-org strict resolve = %v; want profile name and reason", err)
	}
	assertNoSecretLeak(t, err)
}
