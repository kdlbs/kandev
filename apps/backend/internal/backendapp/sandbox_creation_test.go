package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func TestSandboxAuthorityCreationKeepsCallerLimit(t *testing.T) {
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
	require.NoError(t, h.taskRepo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Workspace", OrgID: "org", OwnerID: "owner"}))
	require.NoError(t, h.taskRepo.CreateTask(t.Context(), &models.Task{ID: "source", Title: "Source", WorkspaceID: "workspace"}))
	_, err = svc.AdmitTask(t.Context(), sandbox.SelectionContext{OrgID: "org", WorkspaceID: "workspace", TaskID: "source", ExecutorID: "executor", AgentProfileID: "reader"}, "")
	require.NoError(t, err)
	agent := scope.WithPrincipal(authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", OrgID: "org", Role: authn.RoleAdmin}), scope.Principal{WorkspaceID: "workspace", CallerTaskID: "source", CallerSessionID: "session"})
	admission := sandboxCreationAdmission{policies: svc, targets: sandboxTargetResolver{tasks: h.taskSvc, workflows: h.workflowSvc}}
	task := &models.Task{ID: "copy", Title: "Copy", WorkspaceID: "workspace"}
	req := &taskservice.CreateTaskRequest{WorkspaceID: "workspace", ExecutorID: "executor"}
	require.ErrorIs(t, admission.admit(agent, task, req), sandbox.ErrIneligible)
	// A separate permitted copy retains the limit without needing a parent link.
	task.ID = "permitted"
	req.DeferredLaunch = map[string]any{"agent_profile_id": "reader"}
	require.NoError(t, admission.admit(agent, task, req))
	_, err = svc.AdmitTask(agent, sandbox.SelectionContext{OrgID: "org", WorkspaceID: "workspace", TaskID: "permitted", ExecutorID: "executor"}, "")
	require.ErrorIs(t, err, sandbox.ErrIneligible)
}
