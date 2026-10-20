package sqlite

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceConfigurationCopier interface {
	CreateWorkspaceCloneTx(context.Context, *sqlx.Tx, *models.Workspace) error
	CopyWorkspaceConfigurationTx(context.Context, *sqlx.Tx, string, string) (map[string]string, error)
}

func TestWorkspaceCloneGraph(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := t.Context()
	source := &models.Workspace{ID: "clone-source", Name: "Source"}
	require.NoError(t, repo.CreateWorkspace(ctx, source))
	local := &models.Repository{ID: "clone-local", WorkspaceID: source.ID, Name: "Local", SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "develop", WorktreeBranchPrefix: "feature/", WorktreeBranchTemplate: "{task_id}", PullBeforeWorktree: true, SetupScript: "setup", CleanupScript: "clean", DevScript: "dev", CopyFiles: ".env"}
	remote := &models.Repository{ID: "clone-remote", WorkspaceID: source.ID, Name: "Remote", SourceType: "provider", Provider: "github", ProviderHost: "github.com", ProviderRepoID: "42", ProviderOwner: "acme", ProviderName: "tools", RemoteURL: "https://github.com/acme/tools.git", LocalPath: "/managed/source"}
	require.NoError(t, repo.CreateRepository(ctx, local))
	require.NoError(t, repo.CreateRepository(ctx, remote))
	policy := &models.RepositoryBranchPolicy{ID: "clone-policy", RepositoryID: local.ID, Name: "Develop", Description: "policy", BaseBranch: "develop", BranchTemplate: "feat/{task_id}", PullRequestTarget: "main"}
	require.NoError(t, repo.CreateRepositoryBranchPolicy(ctx, policy))
	script := &models.RepositoryScript{ID: "clone-script", RepositoryID: local.ID, Name: "Check", Command: "pnpm check", Position: 2}
	require.NoError(t, repo.CreateRepositoryScript(ctx, script))
	set := &models.RepositorySet{ID: "clone-set", WorkspaceID: source.ID, Name: "Pair", Description: "both", Items: []models.RepositorySetItem{{RepositoryID: remote.ID, BaseBranch: "main"}, {RepositoryID: local.ID, BaseBranch: "develop"}}}
	require.NoError(t, repo.CreateRepositorySet(ctx, set))
	workflow := &models.Workflow{ID: "clone-flow", WorkspaceID: source.ID, Name: "Custom", Description: "flow", Prompt: "instructions", Style: models.WorkflowStyleCustom, Source: models.WorkflowSourceGitHub, SourcePath: "flow.yaml"}
	require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "clone-hidden", WorkspaceID: source.ID, Name: "Hidden", Hidden: true}))
	require.NoError(t, repo.ReplaceRepositorySecretBindings(ctx, local.ID, []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "excluded"}}))

	copier, ok := any(repo).(workspaceConfigurationCopier)
	require.True(t, ok, "workspace setup must support copying into a caller-owned transaction")
	target := &models.Workspace{ID: "clone-target", Name: "Copy", OwnerID: "new-owner"}
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, copier.CreateWorkspaceCloneTx(ctx, tx, target))
	workflowIDs, err := copier.CopyWorkspaceConfigurationTx(ctx, tx, source.ID, target.ID)
	require.NoError(t, err)
	require.Len(t, workflowIDs, 1)
	require.NotEqual(t, workflow.ID, workflowIDs[workflow.ID])
	require.NoError(t, tx.Commit())

	repositories, err := repo.ListRepositories(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, repositories, 2)
	byName := make(map[string]*models.Repository)
	for _, repository := range repositories {
		byName[repository.Name] = repository
	}
	copyLocal := byName[local.Name]
	copyRemote := byName[remote.Name]
	require.NotEqual(t, local.ID, copyLocal.ID)
	require.Equal(t, local.LocalPath, copyLocal.LocalPath)
	require.Equal(t, local.SetupScript, copyLocal.SetupScript)
	require.Equal(t, local.CleanupScript, copyLocal.CleanupScript)
	require.Equal(t, local.DevScript, copyLocal.DevScript)
	require.Equal(t, local.CopyFiles, copyLocal.CopyFiles)
	require.Equal(t, local.WorktreeBranchTemplate, copyLocal.WorktreeBranchTemplate)
	require.True(t, copyLocal.PullBeforeWorktree)
	require.Empty(t, copyRemote.LocalPath)
	require.Equal(t, remote.ProviderRepoID, copyRemote.ProviderRepoID)
	require.Equal(t, remote.RemoteURL, copyRemote.RemoteURL)
	require.Empty(t, copyLocal.SecretBindings)
	policies, err := repo.ListRepositoryBranchPolicies(ctx, copyLocal.ID)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	require.NotEqual(t, policy.ID, policies[0].ID)
	require.Equal(t, policy.BranchTemplate, policies[0].BranchTemplate)
	scripts, err := repo.ListRepositoryScripts(ctx, copyLocal.ID)
	require.NoError(t, err)
	require.Len(t, scripts, 1)
	require.NotEqual(t, script.ID, scripts[0].ID)
	require.Equal(t, script.Command, scripts[0].Command)
	sets, err := repo.ListRepositorySets(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, sets, 1)
	require.Equal(t, []string{copyRemote.ID, copyLocal.ID}, sets[0].RepositoryIDs())
	require.Equal(t, "develop", sets[0].Items[1].BaseBranch)
	clonedWorkflow, err := repo.GetWorkflow(ctx, workflowIDs[workflow.ID])
	require.NoError(t, err)
	require.Equal(t, workflow.Prompt, clonedWorkflow.Prompt)
	require.Equal(t, workflow.Style, clonedWorkflow.Style)
	require.Equal(t, models.WorkflowSourceManual, clonedWorkflow.Source)
	require.Empty(t, clonedWorkflow.SourcePath)
	members, err := repo.ListWorkspaceMembers(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, target.OwnerID, members[0].UserID)
	copyLocal.Name = "Edited copy"
	require.NoError(t, repo.UpdateRepository(ctx, copyLocal))
	unchanged, err := repo.GetRepository(ctx, local.ID)
	require.NoError(t, err)
	require.Equal(t, "Local", unchanged.Name)
}

func TestWorkspaceCloneGraphRollback(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "rollback-source", Name: "Source"}))
	copier, ok := any(repo).(workspaceConfigurationCopier)
	require.True(t, ok, "clone participants must share a transaction")
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, copier.CreateWorkspaceCloneTx(ctx, tx, &models.Workspace{ID: "rollback-target", Name: "Copy"}))
	ids, err := copier.CopyWorkspaceConfigurationTx(ctx, tx, "rollback-source", "rollback-target")
	require.NoError(t, err)
	require.Len(t, ids, 0)
	require.NoError(t, tx.Rollback())
	_, err = repo.GetWorkspace(ctx, "rollback-target")
	require.Error(t, err)
}
