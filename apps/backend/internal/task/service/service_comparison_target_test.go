package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func comparisonCandidate(headPath, headID, targetPath, targetID string) models.ComparisonTargetCandidate {
	return models.ComparisonTargetCandidate{
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       1154,
		HeadBranch:   "feature/cursor-cost",
		TargetBranch: "main",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: headPath, ProviderID: headID,
			RemoteURL: "https://github.com/" + headPath + ".git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: targetPath, ProviderID: targetID,
			RemoteURL: "https://github.com/" + targetPath + ".git",
		},
	}
}

func TestReconcileComparisonTargetMatchesExactForkAttachment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Fork comparison",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	result, err := svc.ReconcileComparisonTarget(ctx, taskResult.Task.ID, comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99"))
	if err != nil {
		t.Fatalf("ReconcileComparisonTarget: %v", err)
	}
	if result.Status != models.ComparisonTargetMatched || result.Target == nil {
		t.Fatalf("reconciliation = %#v, want matched target", result)
	}
	rows, err := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskRepositories: %v rows=%d", err, len(rows))
	}
	stored, ok, err := models.LoadComparisonTarget(rows[0].Metadata)
	if err != nil || !ok || !stored.Equal(*result.Target) {
		t.Fatalf("stored target = %#v present=%v err=%v", stored, ok, err)
	}
}

func TestReconcileComparisonTargetLeavesStaleForkAndAmbiguousAttachmentsUnchanged(t *testing.T) {
	t.Run("stale fork", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		seedComparisonServiceWorkspace(t, repo, "old-fork/widget", "old-7")
		taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Stale fork",
			Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		result, err := svc.ReconcileComparisonTarget(ctx, taskResult.Task.ID, comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99"))
		if err != nil {
			t.Fatalf("ReconcileComparisonTarget: %v", err)
		}
		if result.Status != models.ComparisonTargetNoMatch {
			t.Fatalf("stale-fork reconciliation status = %q, want no_match", result.Status)
		}
	})

	t.Run("ambiguous sibling attachments", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
		taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Ambiguous siblings",
			Repositories: []TaskRepositoryInput{
				{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"},
				{RepositoryID: "repo-comparison", BaseBranch: "develop", CheckoutBranch: "feature/cursor-cost"},
			},
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		result, err := svc.ReconcileComparisonTarget(ctx, taskResult.Task.ID, comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99"))
		if err != nil {
			t.Fatalf("ReconcileComparisonTarget: %v", err)
		}
		if result.Status != models.ComparisonTargetAmbiguous {
			t.Fatalf("ambiguous reconciliation status = %q, want ambiguous", result.Status)
		}
		rows, err := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
		if err != nil {
			t.Fatalf("ListTaskRepositories: %v", err)
		}
		for _, row := range rows {
			if _, ok, err := models.LoadComparisonTarget(row.Metadata); err != nil || ok {
				t.Fatalf("ambiguous row %#v has target: present=%v err=%v", row, ok, err)
			}
		}
	})
}

func TestReconcileComparisonTargetClearsSameRepositoryBinding(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Same repository",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	rows, _ := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
	seed := comparisonCandidate("other/widget", "other-1", "contributor/widget", "head-42")
	seedTarget, err := seed.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, rows[0].ID, &seedTarget, nil); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	result, err := svc.ReconcileComparisonTarget(ctx, taskResult.Task.ID, comparisonCandidate("contributor/widget", "head-42", "contributor/widget", "head-42"))
	if err != nil {
		t.Fatalf("ReconcileComparisonTarget: %v", err)
	}
	if result.Status != models.ComparisonTargetSameRepository {
		t.Fatalf("same-repository status = %q, want same_repository", result.Status)
	}
	updated, err := repo.GetTaskRepository(ctx, rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := models.LoadComparisonTarget(updated.Metadata); err != nil || ok {
		t.Fatalf("same-repository target remains: present=%v err=%v", ok, err)
	}
}

func TestReconcileComparisonTargetFromSyncDoesNotReplaceNewerChange(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Historical sync",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	newer := comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99")
	newer.Number = 2002
	if _, err := svc.ReconcileComparisonTarget(ctx, taskResult.Task.ID, newer); err != nil {
		t.Fatalf("seed newer target: %v", err)
	}
	older := comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99")
	older.Number = 2001
	result, err := svc.ReconcileComparisonTargetFromSync(ctx, taskResult.Task.ID, older)
	if err != nil {
		t.Fatalf("historical sync: %v", err)
	}
	if result.Status != models.ComparisonTargetNoMatch {
		t.Fatalf("historical sync status = %q, want no_match", result.Status)
	}
	rows, err := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskRepositories: %v rows=%d", err, len(rows))
	}
	stored, ok, err := models.LoadComparisonTarget(rows[0].Metadata)
	if err != nil || !ok || stored.Number != newer.Number {
		t.Fatalf("stored target = %#v present=%v err=%v, want newer change", stored, ok, err)
	}
}

func TestRemoveComparisonTargetForChangeResolvesRepositoryAttachment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Detach target",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	rows, err := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskRepositories: %v rows=%d", err, len(rows))
	}
	target, err := comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99").Build()
	if err != nil {
		t.Fatalf("Build target: %v", err)
	}
	if _, _, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, rows[0].ID, &target, nil); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := svc.RemoveComparisonTargetForChange(ctx, taskResult.Task.ID, "repo-comparison", models.ComparisonTargetProviderGitHub, models.ComparisonTargetKindPullRequest, target.Number); err != nil {
		t.Fatalf("RemoveComparisonTargetForChange: %v", err)
	}
	updated, err := repo.GetTaskRepository(ctx, rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := models.LoadComparisonTarget(updated.Metadata); err != nil || ok {
		t.Fatalf("target remains after repository-scoped detach: present=%v err=%v", ok, err)
	}
}

func seedComparisonServiceWorkspace(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateRepository(context.Context, *models.Repository) error
}, path, providerID string) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-comparison", Name: "Comparison"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(context.Background(), &models.Workflow{ID: "wf-comparison", WorkspaceID: "ws-comparison", Name: "Comparison"}); err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(path, "/", 2)
	if err := repo.CreateRepository(context.Background(), &models.Repository{
		ID: "repo-comparison", WorkspaceID: "ws-comparison", Name: "widget", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com", ProviderRepoID: providerID,
		ProviderOwner: parts[0], ProviderName: parts[1], RemoteURL: "https://github.com/" + path + ".git", DefaultBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
}
