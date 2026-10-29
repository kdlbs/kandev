package backendapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/sandbox"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyStartupProtectsControlFiles(t *testing.T) {
	h := newBootStateTestHarness(t)
	root := t.TempDir()
	cfg := &config.Config{HomeDir: root, Features: config.FeaturesConfig{SandboxPolicies: true}}
	for _, path := range []string{cfg.ResolvedDataDir(), filepath.Join(root, "tasks"), filepath.Join(root, "plugins")} {
		require.NoError(t, os.MkdirAll(path, 0700))
	}
	svc, err := newSandboxPolicyService(cfg, h.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser"})
	for _, path := range []string{root, cfg.ResolvedDataDir(), filepath.Join(root, "plugins")} {
		_, err = svc.Publish(ctx, sandbox.Policy{ID: "policy", Name: "Policy", InstallationID: "provider", ProviderKey: "provider", Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: path}}}}, 0)
		require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	}
	_, err = svc.Publish(ctx, sandbox.Policy{ID: "work", Name: "Work", InstallationID: "provider", ProviderKey: "provider", Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: filepath.Join(root, "tasks")}}}}, 0)
	require.NoError(t, err)
}

func TestSandboxPolicyLaunchRequiresRuntime(t *testing.T) {
	h := newBootStateTestHarness(t)
	cfg := &config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}
	svc, err := newSandboxPolicyService(cfg, h.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser"})
	admission := sandboxLaunchAdmission{policies: svc, tasks: h.taskRepo}
	req := &lifecycle.LaunchRequest{AgentProfileID: "agent", Metadata: map[string]any{"executor_id": "sandbox-local"}}
	require.NoError(t, admission.admit(ctx, req))
	require.NoError(t, h.taskRepo.CreateExecutor(t.Context(), &models.Executor{ID: "sandbox-local", Name: "Local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}))
	svc.SetTargetResolver(sandboxTargetResolver{tasks: h.taskSvc}.resolve)
	revision, err := svc.Publish(ctx, sandbox.Policy{ID: "policy", Name: "Policy", InstallationID: "fixture", ProviderKey: "fixture"}, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(ctx, sandbox.Binding{Scope: sandbox.ScopeExecutor, TargetID: "sandbox-local", Executor: &sandbox.ExecutorPolicy{Mode: sandbox.ModeRequired, InstallationID: "fixture", ProviderKey: "fixture", CeilingRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}, DefaultRevisionID: revision.ID}}, 0)
	require.NoError(t, err)
	require.ErrorIs(t, admission.admit(ctx, req), errSandboxRuntimeUnavailable)
	require.ErrorIs(t, admission.admit(ctx, &lifecycle.LaunchRequest{AgentProfileID: "agent"}), sandbox.ErrIneligible)
	disabled, err := newSandboxPolicyService(&config.Config{HomeDir: cfg.HomeDir}, h.db)
	require.NoError(t, err)
	require.ErrorIs(t, (sandboxLaunchAdmission{policies: disabled, tasks: h.taskRepo}).admit(ctx, req), sandbox.ErrDisabled)
}

func TestSandboxAuthorityTargets(t *testing.T) {
	h := newBootStateTestHarness(t)
	profiles, _, err := settingsstore.Provide(h.db, h.db, nil)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.CreateWorkspace(t.Context(), &models.Workspace{ID: "private", Name: "Private", OwnerID: "owner", OrgID: "org"}))
	require.NoError(t, profiles.CreateAgent(t.Context(), &settingsmodels.Agent{ID: "agent", Name: "agent"}))
	require.NoError(t, profiles.CreateAgentProfile(t.Context(), &settingsmodels.AgentProfile{ID: "profile", AgentID: "agent", Name: "Profile", WorkspaceID: "private"}))
	resolver := sandboxTargetResolver{tasks: h.taskSvc, profiles: profiles, workflows: h.workflowSvc}
	owner := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	got, err := resolver.resolve(owner, sandbox.ScopeAgent, "profile")
	require.NoError(t, err)
	require.Equal(t, sandbox.TargetScope{OrgID: "org", WorkspaceID: "private"}, got)
	outsider := authn.WithIdentity(t.Context(), authn.Identity{UserID: "outsider", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	_, err = resolver.resolve(outsider, sandbox.ScopeAgent, "profile")
	require.Error(t, err)
	_, err = resolver.resolve(owner, sandbox.ScopeAgent, "missing")
	require.Error(t, err)
	_, err = resolver.resolve(owner, "unknown", "profile")
	require.Error(t, err)
}

func TestSandboxPolicyLaunchWithoutTenantCannotBypassRequirements(t *testing.T) {
	h := newBootStateTestHarness(t)
	svc, err := newSandboxPolicyService(&config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}, h.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "tenant", Role: authn.RoleAdmin, SessionID: "browser"})
	svc.SetTargetResolver(sandboxTargetResolver{tasks: h.taskSvc}.resolve)
	require.NoError(t, h.taskRepo.CreateExecutor(ctx, &models.Executor{ID: "restricted", Name: "Restricted", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}))
	revision, err := svc.Publish(ctx, sandbox.Policy{ID: "policy", Name: "Policy", InstallationID: "fixture", ProviderKey: "fixture"}, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(ctx, sandbox.Binding{Scope: sandbox.ScopeExecutor, TargetID: "restricted", Executor: &sandbox.ExecutorPolicy{Mode: sandbox.ModeRequired, InstallationID: "fixture", ProviderKey: "fixture", CeilingRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}, DefaultRevisionID: revision.ID}}, 0)
	require.NoError(t, err)
	admission := sandboxLaunchAdmission{policies: svc, tasks: h.taskRepo}
	for _, req := range []*lifecycle.LaunchRequest{{}, {Metadata: map[string]any{"executor_id": "restricted"}}} {
		require.ErrorIs(t, admission.admit(t.Context(), req), sandbox.ErrIneligible)
	}
}

func TestSandboxPolicyProtectsExternalDatabaseSidecars(t *testing.T) {
	h := newBootStateTestHarness(t)
	database := filepath.Join(t.TempDir(), "external.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		require.NoError(t, os.WriteFile(database+suffix, []byte("fixture"), 0600))
	}
	cfg := &config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}
	cfg.Database.Path = database
	svc, err := newSandboxPolicyService(cfg, h.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser"})
	for _, suffix := range []string{"-wal", "-shm"} {
		_, err = svc.Publish(ctx, sandbox.Policy{ID: "policy", Name: "Policy", InstallationID: "provider", ProviderKey: "provider", Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: database + suffix}}}}, 0)
		require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	}
}
