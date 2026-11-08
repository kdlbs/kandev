package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func seedBaseBranchSelectionWorkspace(t *testing.T) (*Service, *sqliterepo.Repository, string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-base-selection", Name: "Selection"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-base-selection", WorkspaceID: "ws-base-selection", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	for _, id := range []string{"repo-selection-a", "repo-selection-b"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-base-selection", Name: id, DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	return svc, repo, "wf-base-selection"
}

func createSelectionTask(t *testing.T, svc *Service, workflowID string, inputs ...TaskRepositoryInput) *models.Task {
	t.Helper()
	result, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
		WorkspaceID:  "ws-base-selection",
		WorkflowID:   workflowID,
		Title:        "Selection",
		Repositories: inputs,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return result.Task
}

func loadOnlySelectionRow(t *testing.T, svc *Service, taskID string) *models.TaskRepository {
	t.Helper()
	rows, err := svc.taskRepos.ListTaskRepositories(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("task repositories = %d, want one", len(rows))
	}
	return rows[0]
}

func requireSelection(t *testing.T, metadata map[string]interface{}, branch string, source models.BaseBranchSelectionSource) {
	t.Helper()
	selection, ok, err := models.LoadBaseBranchSelection(metadata)
	if err != nil {
		t.Fatalf("load base branch selection: %v", err)
	}
	if !ok {
		t.Fatalf("metadata is missing a base branch selection: %#v", metadata)
	}
	if selection.Branch != branch || selection.Source != source {
		t.Fatalf("selection = %#v, want branch %q source %q", selection, branch, source)
	}
}

func TestBaseBranchSelection_Producers(t *testing.T) {
	svc, repo, workflowID := seedBaseBranchSelectionWorkspace(t)
	ctx := context.Background()

	t.Run("explicit branch", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "feature/explicit",
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "feature/explicit", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("explicit repository default name", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "main",
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "main", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("actual default omission", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a",
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "", models.BaseBranchSelectionSourceRepositoryDefault)
	})

	t.Run("automatic default with server selected branch", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "main",
			BaseBranchSelectionSource: models.BaseBranchSelectionSourceRepositoryDefault,
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "main", models.BaseBranchSelectionSourceRepositoryDefault)
	})

	t.Run("unknown source stays strict", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "feature/unknown",
			BaseBranchSelectionSource: "guessed",
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "feature/unknown", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("service inheritance", func(t *testing.T) {
		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "Parent",
			Repositories: []TaskRepositoryInput{{RepositoryID: "repo-selection-a", BaseBranch: "feature/parent"}},
		})
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}
		child, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "Child",
			ParentID: result.Task.ID,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		requireSelection(t, loadOnlySelectionRow(t, svc, child.Task.ID).Metadata, "feature/parent", models.BaseBranchSelectionSourceInherited)
	})

	t.Run("PR-linked omitted base stays strict", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", PRNumber: 42, CheckoutBranch: "feature/head",
		})
		row := loadOnlySelectionRow(t, svc, task.ID)
		requireSelection(t, row.Metadata, "", models.BaseBranchSelectionSourceExplicit)
		if policy := models.ResolveBaseBranchFallbackPolicy(row.Metadata, "main"); policy.PermitsRepositoryDefaultSubstitution() {
			t.Fatalf("PR-linked deferred base policy = %q, want strict", policy)
		}
		if policy := models.ResolveAttachmentBaseBranchFallbackPolicy(row.Metadata, row.BaseBranch); policy.PermitsRepositoryDefaultSubstitution() {
			t.Fatalf("PR-linked empty attachment base policy = %q, want strict", policy)
		}
	})

	t.Run("branch policy snapshot", func(t *testing.T) {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: "repo-selection-policy", WorkspaceID: "ws-base-selection", Name: "policy", DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("create policy repository: %v", err)
		}
		policy, err := svc.CreateRepositoryBranchPolicy(ctx, &CreateRepositoryBranchPolicyRequest{
			RepositoryID: "repo-selection-policy", Name: "Feature", BaseBranch: "develop",
			BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "develop",
		})
		if err != nil {
			t.Fatalf("create policy: %v", err)
		}
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-policy", BranchPolicyID: policy.ID, BaseBranch: "main",
		})
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "develop", models.BaseBranchSelectionSourceExplicit)
	})
}

func TestBaseBranchSelection_UpdateAndAddBranch(t *testing.T) {
	svc, _, workflowID := seedBaseBranchSelectionWorkspace(t)
	ctx := context.Background()

	t.Run("same-name manual edit revokes inherited intent", func(t *testing.T) {
		parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "Parent",
			Repositories: []TaskRepositoryInput{{RepositoryID: "repo-selection-a", BaseBranch: "main"}},
		})
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}
		child, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "Child", ParentID: parent.Task.ID,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		row := loadOnlySelectionRow(t, svc, child.Task.ID)
		updated, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID: child.Task.ID, TaskRepositoryID: row.ID, BaseBranch: "main",
		})
		if err != nil {
			t.Fatalf("manual selection: %v", err)
		}
		requireSelection(t, updated.Metadata, "main", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("omitted repository update preserves the record", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "feature/keep",
		})
		title := "Updated title"
		if _, err := svc.UpdateTask(ctx, task.ID, &UpdateTaskRequest{Title: &title}); err != nil {
			t.Fatalf("update task: %v", err)
		}
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "feature/keep", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("repository replacement derives fresh intent", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "feature/replaced",
			BaseBranchSelectionSource: models.BaseBranchSelectionSourceRepositoryDefault,
		})
		if err := svc.ReplaceTaskRepositories(ctx, task.ID, "ws-base-selection", []TaskRepositoryInput{{
			RepositoryID: "repo-selection-b", BaseBranch: "release",
		}}); err != nil {
			t.Fatalf("replace repositories: %v", err)
		}
		requireSelection(t, loadOnlySelectionRow(t, svc, task.ID).Metadata, "release", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("system fallback rebinds permissive intent", func(t *testing.T) {
		result, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "System",
			Repositories: []TaskRepositoryInput{{RepositoryID: "repo-selection-a", BaseBranch: "main"}},
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		child, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-base-selection", WorkflowID: workflowID, Title: "Child", ParentID: result.Task.ID,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		row := loadOnlySelectionRow(t, svc, child.Task.ID)
		updated, err := svc.UpdateRepositoryBaseBranchFromSystem(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID: child.Task.ID, TaskRepositoryID: row.ID, BaseBranch: "trunk",
		})
		if err != nil {
			t.Fatalf("system update: %v", err)
		}
		requireSelection(t, updated.Metadata, "trunk", models.BaseBranchSelectionSourceInherited)
	})

	t.Run("system update keeps explicit intent strict", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "feature/strict",
		})
		row := loadOnlySelectionRow(t, svc, task.ID)
		updated, err := svc.UpdateRepositoryBaseBranchFromSystem(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID: task.ID, TaskRepositoryID: row.ID, BaseBranch: "trunk",
		})
		if err != nil {
			t.Fatalf("system update: %v", err)
		}
		requireSelection(t, updated.Metadata, "trunk", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("branch add records supplied and defaulted bases", func(t *testing.T) {
		task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
			RepositoryID: "repo-selection-a", BaseBranch: "main", CheckoutBranch: "feature/a",
		})
		explicit, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
			TaskID: task.ID, RepositoryID: "repo-selection-a", BaseBranch: "main", CheckoutBranch: "feature/b",
		})
		if err != nil {
			t.Fatalf("add explicit branch: %v", err)
		}
		requireSelection(t, explicit.Metadata, "main", models.BaseBranchSelectionSourceExplicit)

		defaulted, err := svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
			TaskID: task.ID, RepositoryID: "repo-selection-a", CheckoutBranch: "feature/c",
		})
		if err != nil {
			t.Fatalf("add defaulted branch: %v", err)
		}
		requireSelection(t, defaulted.Metadata, "main", models.BaseBranchSelectionSourceRepositoryDefault)
	})
}

// TestBaseBranchSelection_AddBranchInputForms pins omission capture before
// repository resolution mutates req.BaseBranch for LocalPath and GitHubURL
// inputs, not only the RepositoryID fast path.
func TestBaseBranchSelection_AddBranchInputForms(t *testing.T) {
	svc, repo, workflowID := seedBaseBranchSelectionWorkspace(t)
	ctx := context.Background()
	localPath := t.TempDir()
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-selection-local", WorkspaceID: "ws-base-selection", Name: "local",
		SourceType: "local", LocalPath: localPath, DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("create local repository: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-selection-provider", WorkspaceID: "ws-base-selection", Name: "acme/widgets",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets",
		SourceType: "provider", DefaultBranch: "trunk",
	}); err != nil {
		t.Fatalf("create provider repository: %v", err)
	}
	task := createSelectionTask(t, svc, workflowID, TaskRepositoryInput{
		RepositoryID: "repo-selection-a", BaseBranch: "main", CheckoutBranch: "feature/existing",
	})

	cases := []struct {
		name       string
		request    AddBranchToTaskRequest
		wantBranch string
		wantSource models.BaseBranchSelectionSource
	}{
		{
			name:       "local path omitted base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, LocalPath: localPath},
			wantBranch: "main",
			wantSource: models.BaseBranchSelectionSourceRepositoryDefault,
		},
		{
			name:       "local path explicit base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, LocalPath: localPath, BaseBranch: "main"},
			wantBranch: "main",
			wantSource: models.BaseBranchSelectionSourceExplicit,
		},
		{
			name:       "github url omitted base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, GitHubURL: "https://github.com/acme/widgets"},
			wantBranch: "trunk",
			wantSource: models.BaseBranchSelectionSourceRepositoryDefault,
		},
		{
			name:       "github url explicit base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, GitHubURL: "https://github.com/acme/widgets", BaseBranch: "trunk"},
			wantBranch: "trunk",
			wantSource: models.BaseBranchSelectionSourceExplicit,
		},
		{
			name:       "repository id omitted base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, RepositoryID: "repo-selection-b"},
			wantBranch: "main",
			wantSource: models.BaseBranchSelectionSourceRepositoryDefault,
		},
		{
			name:       "repository id explicit base",
			request:    AddBranchToTaskRequest{TaskID: task.ID, RepositoryID: "repo-selection-b", BaseBranch: "main"},
			wantBranch: "main",
			wantSource: models.BaseBranchSelectionSourceExplicit,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			added, err := svc.AddBranchToTask(ctx, tc.request)
			if err != nil {
				t.Fatalf("AddBranchToTask: %v", err)
			}
			if added.BaseBranch != tc.wantBranch {
				t.Fatalf("base branch = %q, want %q", added.BaseBranch, tc.wantBranch)
			}
			requireSelection(t, added.Metadata, tc.wantBranch, tc.wantSource)
		})
	}
}
