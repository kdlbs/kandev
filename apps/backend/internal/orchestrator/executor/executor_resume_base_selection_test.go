package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func resumeSelectionTask(t *testing.T, source models.BaseBranchSelectionSource) (*mockRepository, *models.Task, *models.TaskSession) {
	t.Helper()
	repo := newMockRepository()
	task := &models.Task{ID: "task-resume-selection", WorkspaceID: "workspace-1", Title: "Resume selection"}
	repo.tasks[task.ID] = task
	repo.repositories["repo-resume-selection"] = &models.Repository{
		ID: "repo-resume-selection", WorkspaceID: "workspace-1", Name: "resume",
		LocalPath: t.TempDir(), DefaultBranch: "main",
	}
	metadata := map[string]interface{}{}
	if err := models.PutBaseBranchSelection(metadata, &models.BaseBranchSelection{
		Version: models.BaseBranchSelectionVersion,
		Branch:  "feature/missing-base",
		Source:  source,
	}); err != nil {
		t.Fatalf("put selection: %v", err)
	}
	repo.taskRepositories["task-repo-resume-selection"] = &models.TaskRepository{
		ID: "task-repo-resume-selection", TaskID: task.ID, RepositoryID: "repo-resume-selection",
		BaseBranch: "feature/missing-base", Metadata: metadata,
	}
	session := &models.TaskSession{
		ID: "session-resume-selection", TaskID: task.ID, RepositoryID: "repo-resume-selection",
		BaseBranch: "feature/missing-base", AgentProfileID: "mock-profile", ExecutionProfileID: "mock-profile",
		ExecutorID: models.ExecutorIDWorktree, State: models.TaskSessionStateFailed,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	repo.sessions[session.ID] = session
	repo.executors[models.ExecutorIDWorktree] = &models.Executor{
		ID: models.ExecutorIDWorktree, Name: "Worktree", Type: models.ExecutorTypeWorktree,
		Status: models.ExecutorStatusActive,
	}
	return repo, task, session
}

// TestBaseBranchSelection_ResumePreservesPolicy drives the actual resume
// request builder: a single-repository resume must carry the exact
// attachment's derived policy and default candidate, not fall back to strict.
func TestBaseBranchSelection_ResumePreservesPolicy(t *testing.T) {
	cases := []struct {
		name      string
		source    models.BaseBranchSelectionSource
		permitted bool
	}{
		{name: "inherited missing base keeps recovery", source: models.BaseBranchSelectionSourceInherited, permitted: true},
		{name: "explicit base stays strict", source: models.BaseBranchSelectionSourceExplicit, permitted: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, task, session := resumeSelectionTask(t, tc.source)
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			req, _, _, _, _, err := exec.buildResumeRequestAtCredentialBoundaryWithOptions(
				context.Background(), task.ToAPI(), session, false, nil, ResumeOptions{},
			)
			if err != nil {
				t.Fatalf("buildResumeRequest: %v", err)
			}
			if got := req.BaseBranchFallbackPolicy.PermitsRepositoryDefaultSubstitution(); got != tc.permitted {
				t.Fatalf("resume policy = %q, permitted=%v want %v", req.BaseBranchFallbackPolicy, got, tc.permitted)
			}
			if req.BaseBranch != "feature/missing-base" {
				t.Fatalf("resume base = %q, want the attachment base", req.BaseBranch)
			}
			if req.DefaultBranch != "main" {
				t.Fatalf("resume default candidate = %q, want main", req.DefaultBranch)
			}
		})
	}
}

// TestBaseBranchSelection_ResumeAmbiguousSessionBaseStaysStrict pins the
// unbound legacy session fallback: policy comes only from an exact matching
// attachment base.
func TestBaseBranchSelection_ResumeAmbiguousSessionBaseStaysStrict(t *testing.T) {
	permissive := []*repoInfo{
		{RepositoryID: "repo-1", BaseBranch: "feature/a", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault},
		{RepositoryID: "repo-1", BaseBranch: "feature/b", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault},
	}
	req := &LaunchAgentRequest{}
	applyResumeAttachmentFallbackPolicy(req, "repo-1", "feature/a", permissive)
	if req.BaseBranchFallbackPolicy.PermitsRepositoryDefaultSubstitution() {
		t.Fatalf("ambiguous policy = %q, want strict", req.BaseBranchFallbackPolicy)
	}

	req = &LaunchAgentRequest{}
	applyResumeAttachmentFallbackPolicy(req, "repo-1", "legacy-session-base", []*repoInfo{
		{RepositoryID: "repo-1", BaseBranch: "", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault},
	})
	if req.BaseBranchFallbackPolicy.PermitsRepositoryDefaultSubstitution() {
		t.Fatalf("unbound session base policy = %q, want strict", req.BaseBranchFallbackPolicy)
	}
}
