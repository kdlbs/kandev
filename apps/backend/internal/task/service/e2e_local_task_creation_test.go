package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A task created from an e2e-local file:// or loopback http remote (the rig's
// API path) persists an e2e-local repository that keeps its locator, passes the
// plugin executor cloneability gate, and is cloneable as a workspace source.
func TestCreateTaskWithE2ELocalRemotes(t *testing.T) {
	t.Setenv(e2eMockEnv, "true")
	for name, remote := range map[string]string{
		"file":          "file:///tmp/local-rig/local-rig-file-main.git",
		"http loopback": "http://127.0.0.1:41234/local-rig-http-main.git",
	} {
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
			_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})

			input := TaskRepositoryInput{RemoteURL: remote, BaseBranch: "main"}
			if err := svc.validatePluginExecutorRepositoryCompatibility(ctx, input); err != nil {
				t.Fatalf("plugin executor compatibility: %v", err)
			}
			result, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
				Title: "rig", Repositories: []TaskRepositoryInput{input},
			})
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			links, err := repo.ListTaskRepositories(ctx, result.Task.ID)
			if err != nil || len(links) != 1 {
				t.Fatalf("task repositories = %v, %v", links, err)
			}
			stored, err := repo.GetRepository(ctx, links[0].RepositoryID)
			if err != nil {
				t.Fatalf("GetRepository: %v", err)
			}
			if stored.Provider != providerE2ELocal || stored.RemoteURL != remote {
				t.Fatalf("stored repository = provider %q remote %q, want %q %q", stored.Provider, stored.RemoteURL, providerE2ELocal, remote)
			}
			if !hasCloneableWorkspaceRepositoryLocator(stored) {
				t.Fatal("stored e2e-local repository is not a cloneable workspace source")
			}
		})
	}
}
