package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutorOwnershipAssignmentAndClear(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	executor := createTestExecutor(t, svc, "Shared", models.ExecutorTypeLocal)
	var request CreateExecutorProfileRequest
	require.NoError(t, json.Unmarshal([]byte("{\"name\":\"Personal\",\"owner_user_id\":\"alice\"}"), &request))
	request.ExecutorID = executor.ID
	profile, err := svc.CreateExecutorProfile(ctx, &request)
	require.NoError(t, err)
	require.Equal(t, "alice", profile.OwnerUserID)
	var patch UpdateExecutorProfileRequest
	require.NoError(t, json.Unmarshal([]byte("{\"owner_user_id\":null}"), &patch))
	profile, err = svc.UpdateExecutorProfile(ctx, profile.ID, &patch)
	require.NoError(t, err)
	require.Empty(t, profile.OwnerUserID)
	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Empty(t, stored.OwnerUserID)
}

// @covers AC-EXECUTORS-OWNERSHIP-001.2, AC-EXECUTORS-OWNERSHIP-003.2
func TestExecutorOwnershipPolicy(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	executor := &models.Executor{ID: "private", Name: "Private", Type: models.ExecutorTypeSSH,
		Status: models.ExecutorStatusActive, OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutor(ctx, executor))
	profile := &models.ExecutorProfile{ID: "private-profile", ExecutorID: executor.ID, Name: "Private"}
	require.NoError(t, repo.CreateExecutorProfile(ctx, profile))
	for _, tc := range []struct {
		name     string
		identity *authn.Identity
		allowed  bool
	}{
		{"owner", &authn.Identity{UserID: "alice", Role: authn.RoleMember}, true},
		{"foreign", &authn.Identity{UserID: "bob", Role: authn.RoleMember}, false},
		{"manager", &authn.Identity{UserID: "admin", Role: authn.RoleAdmin}, true},
		{"internal", nil, false},
		{"synthetic", &authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := context.Background()
			if tc.identity != nil {
				caller = authn.WithIdentity(caller, *tc.identity)
			}
			_, err := svc.GetExecutorProfile(caller, profile.ID)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			_, err = svc.GetExecutor(caller, executor.ID)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestExecutorOwnershipForeignMutationHasNoSideEffects(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := t.Context()
	executor := &models.Executor{ID: "shared", Name: "Shared", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	require.NoError(t, repo.CreateExecutor(ctx, executor))
	profile := &models.ExecutorProfile{ID: "alice-profile", ExecutorID: executor.ID, Name: "Private", OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutorProfile(ctx, profile))
	bus.ClearEvents()
	caller := authn.WithIdentity(ctx, authn.Identity{UserID: "bob", Role: authn.RoleMember})
	name := "stolen"
	_, err := svc.UpdateExecutorProfile(caller, profile.ID, &UpdateExecutorProfileRequest{Name: &name})
	require.Error(t, err)
	require.Error(t, svc.DeleteExecutorProfile(caller, profile.ID))
	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "Private", stored.Name)
	require.Empty(t, bus.GetPublishedEvents())
}

func TestExecutorOwnershipCatalogueAndAccountRevocation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	executor := &models.Executor{ID: "catalogue-shared", Name: "Shared", Type: models.ExecutorTypeLocal}
	require.NoError(t, repo.CreateExecutor(ctx, executor))
	private := &models.ExecutorProfile{ID: "catalogue-private", ExecutorID: executor.ID, Name: "Private", OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutorProfile(ctx, private))
	bob := authn.WithIdentity(ctx, authn.Identity{UserID: "bob", Role: authn.RoleMember})
	profiles, err := svc.ListAllExecutorProfiles(bob)
	require.NoError(t, err)
	for _, profile := range profiles {
		require.NotEqual(t, private.ID, profile.ID)
	}
	alice := authn.WithIdentity(ctx, authn.Identity{UserID: "alice", Role: authn.RoleMember})
	_, err = svc.GetExecutorProfile(alice, private.ID)
	require.NoError(t, err)
	svc.SetUserDirectory(tenancyReviewDirectory{users: map[string]string{"alice": "disabled", "bob": "active"}})
	_, err = svc.GetExecutorProfile(alice, private.ID)
	require.Error(t, err, "a previously authenticated identity must not outlive account revocation")
}

func TestExecutorOwnershipSharedTaskDoesNotGrantExecution(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	executor := &models.Executor{ID: "session-private", Name: "Personal", Type: models.ExecutorTypeLocal, OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutor(ctx, executor))
	seedExecutorSession(t, repo, "private-session", executor.ID)
	bob := authn.WithIdentity(ctx, authn.Identity{UserID: "bob", Role: authn.RoleMember})
	require.NoError(t, svc.AuthorizeSessionAccess(bob, "private-session"), "shared board reads remain allowed")
	_, historyErr := svc.HasUserPromptHistory(bob, "private-session")
	require.NoError(t, historyErr)
	require.Error(t, svc.AuthorizeSessionScope(bob, "private-session", authz.ScopeSessionExec))
	require.Error(t, svc.AuthorizeSessionScope(bob, "private-session", authz.ScopeSessionPrompt))
	require.Error(t, svc.AuthorizeSessionScope(ctx, "private-session", authz.ScopeSessionExec), "missing actor must not borrow session owner")
	alice := authn.WithIdentity(ctx, authn.Identity{UserID: "alice", Role: authn.RoleMember})
	require.NoError(t, svc.AuthorizeSessionScope(alice, "private-session", authz.ScopeSessionExec))
}

func TestExecutorOwnershipSessionRetainsAuthenticatedActor(t *testing.T) {
	_, _, repo := createTestService(t)
	seedExecutorSession(t, repo, "actor-seed", "")
	alice := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	session := &models.TaskSession{ID: "attributed-session", TaskID: "task-actor-seed"}
	require.NoError(t, repo.CreateTaskSession(alice, session))
	var actor string
	require.NoError(t, repo.DB().QueryRowContext(t.Context(), "SELECT executor_actor_user_id FROM task_sessions WHERE id = 'attributed-session'").Scan(&actor))
	require.Equal(t, "alice", actor)
}

func TestExecutorOwnershipMCPDoesNotBorrowWorkspaceOwner(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedExecutorSession(t, repo, "mcp-actor", "")
	bob := authn.WithIdentity(t.Context(), authn.Identity{UserID: "bob", Role: authn.RoleMember})
	require.NoError(t, repo.CreateTaskSession(bob, &models.TaskSession{ID: "bob-stream", TaskID: "task-mcp-actor"}))
	parent := &models.Executor{ID: "mcp-parent", Name: "Shared", Type: models.ExecutorTypeLocal}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	profile := &models.ExecutorProfile{ID: "mcp-private", ExecutorID: parent.ID, Name: "Private", OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	resolver := mcpscope.NewResolver(repo, nil, func() bool { return true }, svc.logger)
	workspaceOwner := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleAdmin})
	scoped, err := resolver.ScopePrincipal(workspaceOwner, "task-mcp-actor", "bob-stream")
	require.NoError(t, err)
	_, err = svc.GetExecutorProfile(scoped, profile.ID)
	require.Error(t, err, "stream actor cannot inherit the workspace administrator's private compute")
}

func TestExecutorOwnershipAutonomousCreatorAndInitiator(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	parent := &models.Executor{ID: "source-parent", Name: "Shared", Type: models.ExecutorTypeLocal}
	require.NoError(t, repo.CreateExecutor(ctx, parent))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "source-private", ExecutorID: parent.ID, Name: "Personal", OwnerUserID: "alice"}))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "source-shared", ExecutorID: parent.ID, Name: "Shared"}))
	svc.SetExecutorIdentityResolver(func(_ context.Context, userID string) (authn.Identity, bool) {
		if userID == "alice" || userID == "bob" {
			return authn.Identity{UserID: userID, Role: authn.RoleMember}, true
		}
		return authn.Identity{}, false
	})
	bob := authn.WithIdentity(ctx, authn.Identity{UserID: "bob", Role: authn.RoleMember})
	_, err := svc.PrepareAutonomousExecutor(bob, "alice", "source-private")
	require.Error(t, err, "an interactive trigger cannot borrow creator authority")
	restored, err := svc.PrepareAutonomousExecutor(ctx, "alice", "source-private")
	require.NoError(t, err)
	identity, found := authn.IdentityFromContext(restored)
	require.True(t, found)
	require.Equal(t, "alice", identity.UserID)
	_, err = svc.PrepareAutonomousExecutor(ctx, "", "source-private")
	require.Error(t, err)
	_, err = svc.PrepareAutonomousExecutor(ctx, "", "source-shared")
	require.NoError(t, err)
	_, err = svc.PrepareAutonomousExecutor(ctx, "disabled", "source-private")
	require.Error(t, err)
}

func TestExecutorOwnershipWorkflowCreatorIsAuthenticated(t *testing.T) {
	_, _, repo := createTestService(t)
	seedExecutorSession(t, repo, "workflow-creator", "")
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	workflow := &models.Workflow{ID: "attributed-workflow", WorkspaceID: "ws-workflow-creator", Name: "Personal"}
	require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	var creator string
	require.NoError(t, repo.DB().QueryRowContext(ctx, "SELECT created_by_user_id FROM workflows WHERE id = 'attributed-workflow'").Scan(&creator))
	require.Equal(t, "alice", creator)
}

func TestExecutorOwnershipTaskAttributionSurvivesQueueAndEdits(t *testing.T) {
	_, _, repo := createTestService(t)
	seedExecutorSession(t, repo, "queued-actor", "")
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	task := &models.Task{ID: "queued-owner-task", WorkspaceID: "ws-queued-actor", Title: "Queued"}
	require.NoError(t, repo.CreateTask(ctx, task))
	actor, err := repo.GetTaskExecutorActor(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", actor)
	task.Title = "Edited"
	require.NoError(t, repo.UpdateTask(authn.WithIdentity(ctx, authn.Identity{UserID: "bob"}), task))
	actor, err = repo.GetTaskExecutorActor(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", actor)
}

func TestExecutorOwnershipWorkflowAndTaskEditsCannotBorrowCompute(t *testing.T) {
	svc, _, repo := createTestService(t)
	parent := &models.Executor{ID: "edit-private", Name: "Private", Type: models.ExecutorTypeLocal, OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	seedExecutorSession(t, repo, "source-edit", parent.ID)
	bob := authn.WithIdentity(t.Context(), authn.Identity{UserID: "bob", Role: authn.RoleMember})
	require.NoError(t, svc.AuthorizeWorkflowAccess(bob, "wf-source-edit"))
	require.Error(t, svc.AuthorizeWorkflowExecutorMutation(bob, "wf-source-edit"))
	prompt := "Run my commands"
	_, err := svc.UpdateTask(bob, "task-source-edit", &UpdateTaskRequest{Description: &prompt})
	require.Error(t, err)
	task, err := repo.GetTask(t.Context(), "task-source-edit")
	require.NoError(t, err)
	require.NotEqual(t, prompt, task.Description)
}

func TestExecutorOwnershipOwnerCanClearParentWithoutChangingProviderConfiguration(t *testing.T) {
	svc, _, repo := createTestService(t)
	parent := &models.Executor{ID: "parent-clear", Name: "Remote", Type: models.ExecutorTypeRemoteDocker, OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	alice := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	_, err := svc.UpdateExecutor(alice, parent.ID, &UpdateExecutorRequest{OwnerUserID: models.OptionalOwner{Set: true}})
	require.NoError(t, err)
	stored, err := repo.GetExecutor(t.Context(), parent.ID)
	require.NoError(t, err)
	require.Empty(t, stored.OwnerUserID)
	require.Equal(t, models.ExecutorTypeRemoteDocker, stored.Type)
}

func TestExecutorOwnershipCanRecoverProfileWhenProviderUnavailable(t *testing.T) {
	svc, _, repo := createTestService(t)
	parent := &models.Executor{ID: "unavailable-provider", Name: "Provider", Type: models.ExecutorTypePluginRemote}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	profile := &models.ExecutorProfile{ID: "recover-profile", ExecutorID: parent.ID, Name: "Private", OwnerUserID: "alice", Config: map[string]string{"credential": "retained-secret"}}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	alice := authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleMember})
	updated, err := svc.UpdateExecutorProfile(alice, profile.ID, &UpdateExecutorProfileRequest{OwnerUserID: models.OptionalOwner{Set: true}})
	require.NoError(t, err)
	require.Empty(t, updated.OwnerUserID)
	require.Equal(t, profile.Config, updated.Config)
}

func TestExecutorOwnershipEnvironmentBindingGuardsLegacySessionWithoutRunner(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedExecutorSession(t, repo, "environment-only", "")
	parent := &models.Executor{ID: "env-only-private", Name: "Private", Type: models.ExecutorTypeLocal, OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	environment := &models.TaskEnvironment{ID: "bound-private", TaskID: "task-environment-only", ExecutorID: parent.ID}
	require.NoError(t, repo.CreateTaskEnvironment(t.Context(), environment))
	session, err := repo.GetTaskSession(t.Context(), "environment-only")
	require.NoError(t, err)
	session.TaskEnvironmentID = environment.ID
	require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
	bob := authn.WithIdentity(t.Context(), authn.Identity{UserID: "bob", Role: authn.RoleMember})
	_, err = svc.PrepareExecutorAdmission(bob, session.ID, "", "")
	require.Error(t, err)
}

func TestExecutorOwnershipMCPActorRetainsProviderAdminRestriction(t *testing.T) {
	svc, _, repo := createTestService(t)
	parent := &models.Executor{ID: "mcp-provider-admin", Name: "Remote Docker", Type: models.ExecutorTypeRemoteDocker}
	require.NoError(t, repo.CreateExecutor(t.Context(), parent))
	profile := &models.ExecutorProfile{ID: "bob-remote", ExecutorID: parent.ID, Name: "Bob", OwnerUserID: "bob"}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	svc.SetExecutorIdentityResolver(func(context.Context, string) (authn.Identity, bool) {
		return authn.Identity{UserID: "bob", Role: authn.RoleMember}, true
	})
	ctx := models.WithExecutorActor(authn.WithIdentity(t.Context(), authn.Identity{UserID: "alice", Role: authn.RoleAdmin}), "bob")
	name := "Changed"
	_, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{Name: &name})
	require.ErrorIs(t, err, ErrRemoteDockerAdminRequired)
}
