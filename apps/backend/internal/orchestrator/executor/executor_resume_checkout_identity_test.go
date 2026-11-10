package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A task created with a working branch that differs from its base records the
// checkout slug in its environment inventory. Resume must look up that same
// slug, or validateReuseEnvironmentInventory rejects every resume.
func TestResumeCheckoutBranchIdentity_MatchesInventoryWrittenAtLaunch(t *testing.T) {
	repos := []*repoInfo{{RepositoryID: "repo-1", BaseBranch: "release/x", CheckoutBranch: "feature/y"}}
	slug := resumeCheckoutBranchIdentity("repo-1", repos)
	if slug != "feature-y" {
		t.Fatalf("slug = %q, want %q", slug, "feature-y")
	}

	// The row the initial launch wrote, keyed on the checkout branch.
	rows := []*models.TaskEnvironmentRepo{{RepositoryID: "repo-1", BranchSlug: "feature-y", Status: taskEnvironmentRepoStatusActive}}

	resumeReq := &LaunchAgentRequest{RepositoryID: "repo-1", BaseBranch: "release/x", BranchIdentitySlug: slug}
	spec, ok := topLevelLaunchRepoSpec(resumeReq)
	if !ok {
		t.Fatal("topLevelLaunchRepoSpec: no spec")
	}
	if got := canonicalInventoryMatches(spec, rows, false); got != 1 {
		t.Fatalf("resume with checkout identity matched %d rows, want 1", got)
	}

	// Before the fix: resume carried only BaseBranch and missed the row.
	before, _ := topLevelLaunchRepoSpec(&LaunchAgentRequest{RepositoryID: "repo-1", BaseBranch: "release/x"})
	if got := canonicalInventoryMatches(before, rows, false); got != 0 {
		t.Fatalf("base-only resume matched %d rows, want 0 (the bug this guards)", got)
	}
}

func TestResumeCheckoutBranchIdentity_EmptyWhenNoCheckoutOrAmbiguous(t *testing.T) {
	if got := resumeCheckoutBranchIdentity("repo-1", []*repoInfo{{RepositoryID: "repo-1", BaseBranch: "main"}}); got != "" {
		t.Fatalf("no checkout branch: got %q, want empty", got)
	}
	twice := []*repoInfo{
		{RepositoryID: "repo-1", CheckoutBranch: "feature/a"},
		{RepositoryID: "repo-1", CheckoutBranch: "feature/b"},
	}
	if got := resumeCheckoutBranchIdentity("repo-1", twice); got != "" {
		t.Fatalf("ambiguous rows: got %q, want empty", got)
	}
	if got := resumeCheckoutBranchIdentity("repo-2", []*repoInfo{{RepositoryID: "repo-1", CheckoutBranch: "feature/a"}}); got != "" {
		t.Fatalf("other repository: got %q, want empty", got)
	}
}

// TestApplyResumeRepoConfig_CarriesCheckoutBranchIdentity drives the real
// resume path: clone-URL executors must carry the checkout-branch identity
// (without re-checking anything out), and non-clone executors stay untouched.
func TestApplyResumeRepoConfig_CarriesCheckoutBranchIdentity(t *testing.T) {
	cases := []struct {
		executorType string
		wantSlug     string
	}{
		{executorType: "local_docker", wantSlug: "feature-y"},
		{executorType: "remote_docker", wantSlug: "feature-y"},
		{executorType: "sprites", wantSlug: "feature-y"},
		{executorType: "local", wantSlug: ""},
	}

	for _, tc := range cases {
		t.Run(tc.executorType, func(t *testing.T) {
			repo := newMockRepository()
			repo.repositories["repo-1"] = &models.Repository{
				ID:            "repo-1",
				LocalPath:     "/tmp/repo",
				Provider:      "github",
				ProviderOwner: "kdlbs",
				ProviderName:  "kandev",
			}
			repo.tasks["task-1"] = &models.Task{ID: "task-1"}
			repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
				ID:             "task-repo-1",
				TaskID:         "task-1",
				RepositoryID:   "repo-1",
				BaseBranch:     "release/x",
				CheckoutBranch: "feature/y",
			}
			session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "release/x"}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: tc.executorType, WorkspaceReuseRequired: true}

			if _, err := exec.applyResumeRepoConfig(context.Background(), &v1.Task{ID: "task-1"}, session, req, nil); err != nil {
				t.Fatalf("applyResumeRepoConfig: %v", err)
			}
			if req.BranchIdentitySlug != tc.wantSlug {
				t.Fatalf("BranchIdentitySlug = %q, want %q", req.BranchIdentitySlug, tc.wantSlug)
			}
			if req.CheckoutBranch != "" {
				t.Fatalf("CheckoutBranch = %q, want empty (identity only, no re-checkout)", req.CheckoutBranch)
			}
		})
	}
}
