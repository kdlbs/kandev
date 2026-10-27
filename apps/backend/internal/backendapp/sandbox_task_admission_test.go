package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyLaunchCapturesTaskLimit(t *testing.T) {
	h := newBootStateTestHarness(t)
	svc, err := newSandboxPolicyService(&config.Config{HomeDir: t.TempDir(), Features: config.FeaturesConfig{SandboxPolicies: true}}, h.db)
	require.NoError(t, err)
	operator := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	svc.SetTargetResolver(func(context.Context, string, string) (sandbox.TargetScope, error) {
		return sandbox.TargetScope{OrgID: "org"}, nil
	})
	policy := sandbox.Policy{ID: "write", Name: "Write", InstallationID: "fixture", ProviderKey: "fixture", Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: t.TempDir(), Writable: true}}}}
	ceiling, err := svc.Publish(operator, policy, 0)
	require.NoError(t, err)
	policy.ID = "read"
	policy.Grants.Filesystem[0].Writable = false
	read, err := svc.Publish(operator, policy, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(operator, sandbox.Binding{Scope: sandbox.ScopeExecutor, TargetID: "executor", Executor: &sandbox.ExecutorPolicy{Mode: sandbox.ModeRequired, InstallationID: "fixture", ProviderKey: "fixture", CeilingRevisionID: ceiling.ID, EligibleRevisionIDs: []string{ceiling.ID, read.ID}, DefaultRevisionID: ceiling.ID}}, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(operator, sandbox.Binding{Scope: sandbox.ScopeAgent, TargetID: "reader", RevisionID: read.ID}, 0)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Workspace", OrgID: "org"}))
	require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{ID: "parent", Title: "Parent", WorkspaceID: "workspace"}))
	admission := sandboxLaunchAdmission{policies: svc, tasks: h.taskRepo}
	req := &lifecycle.LaunchRequest{TaskID: "parent", AgentProfileID: "reader", Metadata: map[string]any{"executor_id": "executor"}}
	require.ErrorIs(t, admission.admit(t.Context(), req), errSandboxRuntimeUnavailable)
	req.AgentProfileID = "writer"
	require.ErrorIs(t, admission.admit(t.Context(), req), sandbox.ErrIneligible)
	require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{ID: "child", Title: "Child", WorkspaceID: "workspace", ParentID: "parent"}))
	req.TaskID = "child"
	require.ErrorIs(t, admission.admit(t.Context(), req), sandbox.ErrIneligible)
	req.Metadata["executor_id"] = "unrestricted"
	require.ErrorIs(t, admission.admit(t.Context(), req), sandbox.ErrIneligible)
}
