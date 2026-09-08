package delivery_test

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func seedGitHubStore(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := github.NewStore(db, db); err != nil {
		t.Fatalf("init github store: %v", err)
	}
}

func seedGitHubPR(
	t *testing.T, db *sqlx.DB, id, taskID, repositoryID, owner, repo string,
	prNumber int, prURL, baseBranch string, mergedAt, detachedAt *time.Time,
) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO github_task_prs (
			id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, created_at, merged_at, detached_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, '', ?, ?, ?, ?)
	`), id, taskID, repositoryID, owner, repo, prNumber, prURL, baseBranch, now, mergedAt, detachedAt, now); err != nil {
		t.Fatalf("seed github_task_prs %s: %v", id, err)
	}
}

func seedGitLabStore(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := gitlab.NewStore(db, db); err != nil {
		t.Fatalf("init gitlab store: %v", err)
	}
}

func seedGitLabMR(
	t *testing.T, db *sqlx.DB, id, taskID, repositoryID, projectPath string,
	mrIID int, mrURL, baseBranch string, mergedAt *time.Time,
) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO gitlab_task_mrs (
			id, task_id, repository_id, project_path, mr_iid, mr_url, mr_title,
			head_branch, base_branch, created_at, merged_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?)
	`), id, taskID, repositoryID, projectPath, mrIID, mrURL, baseBranch, now, mergedAt, now); err != nil {
		t.Fatalf("seed gitlab_task_mrs %s: %v", id, err)
	}
}

func seedAzureDevOpsStore(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := azuredevops.NewStore(db, db); err != nil {
		t.Fatalf("init azuredevops store: %v", err)
	}
}

func seedAzurePR(
	t *testing.T, db *sqlx.DB, id, taskID, repositoryID, azureRepositoryID string,
	pullRequestID int, pullRequestURL, targetBranch, status string,
) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO azure_devops_task_prs (
			id, task_id, repository_id, organization_url, project_id, azure_repository_id,
			pull_request_id, pull_request_url, title, source_branch, target_branch,
			author_id, author_name, status, created_at, updated_at
		) VALUES (?, ?, ?, '', '', ?, ?, ?, '', '', ?, '', '', ?, ?, ?)
	`), id, taskID, repositoryID, azureRepositoryID, pullRequestID, pullRequestURL, targetBranch, status, now, now); err != nil {
		t.Fatalf("seed azure_devops_task_prs %s: %v", id, err)
	}
}
