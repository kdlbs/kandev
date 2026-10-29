package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyUtilityRequiresExplicitExecutor(t *testing.T) {
	h := newBootStateTestHarness(t)
	svc, err := newSandboxPolicyService(&config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}, h.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	targets := sandboxTargetResolver{tasks: h.taskSvc}
	svc.SetTargetResolver(targets.resolve)
	admission := sandboxUtilityAdmission{launch: sandboxLaunchAdmission{policies: svc, tasks: h.taskRepo}, targets: targets}
	require.NoError(t, admission.admit(t.Context(), &hostutility.ProfileProbeContext{}))
	require.NoError(t, h.taskRepo.CreateExecutor(t.Context(), &models.Executor{ID: "sandbox-local", Name: "Local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}))
	revision, err := svc.Publish(ctx, sandbox.Policy{ID: "policy", Name: "Policy", InstallationID: "fixture", ProviderKey: "fixture"}, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(ctx, sandbox.Binding{Scope: sandbox.ScopeExecutor, TargetID: "sandbox-local", Executor: &sandbox.ExecutorPolicy{Mode: sandbox.ModeRequired, InstallationID: "fixture", ProviderKey: "fixture", CeilingRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}, DefaultRevisionID: revision.ID}}, 0)
	require.NoError(t, err)
	require.ErrorIs(t, admission.admit(ctx, &hostutility.ProfileProbeContext{}), sandbox.ErrIneligible)
	require.ErrorIs(t, admission.admit(t.Context(), &hostutility.ProfileProbeContext{}), sandbox.ErrIneligible)
	require.ErrorIs(t, admission.admit(ctx, &hostutility.ProfileProbeContext{ExecutorID: "sandbox-local"}), errSandboxRuntimeUnavailable)
	require.Error(t, admission.admit(ctx, &hostutility.ProfileProbeContext{ExecutorID: "missing"}))
}
