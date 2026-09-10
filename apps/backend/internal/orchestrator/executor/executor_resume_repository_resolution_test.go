package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// These tests cover REQ-TASKS-LAUNCH-REPOSITORY-RESOLUTION-001 and -002: a
// launch must resolve its repositories from the task's attachment set
// (task_repositories) rather than the session's stale repository_id/base_branch
// snapshot, so a repository attached after a session already exists is not
// permanently unlaunchable.

// TestApplyResumeRepoConfig_ResolvesPrimaryFromAttachmentSetWhenSessionHasNoPreference
// covers AC-...-001.1: a session created before its task had any attachments
// carries an empty repository preference. The launch must still resolve a
// primary from the task's current attachment set.
func TestApplyResumeRepoConfig_ResolvesPrimaryFromAttachmentSetWhenSessionHasNoPreference(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:        "repo-1",
		LocalPath: "/tmp/repo",
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:     "sess-1",
		TaskID: "task-1",
		// RepositoryID intentionally empty: the repository was attached after
		// this session was created.
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	gotID, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil)
	if err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if gotID != "repo-1" {
		t.Fatalf("resolved repositoryID = %q, want repo-1", gotID)
	}
}

// TestApplyResumeRepoConfig_KeepsSessionPreferenceWhenPresentInAttachmentSet
// covers AC-...-001.3: a non-empty session preference naming a repository
// still present in the attachment set is used unchanged, not re-derived from
// the set's ordering.
func TestApplyResumeRepoConfig_KeepsSessionPreferenceWhenPresentInAttachmentSet(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-primary"] = &models.Repository{ID: "repo-primary", LocalPath: "/tmp/primary"}
	repo.repositories["repo-secondary"] = &models.Repository{ID: "repo-secondary", LocalPath: "/tmp/secondary"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-primary", Position: 0, BaseBranch: "main",
	}
	repo.taskRepositories["tr-2"] = &models.TaskRepository{
		ID: "tr-2", TaskID: "task-1", RepositoryID: "repo-secondary", Position: 1, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:           "sess-1",
		TaskID:       "task-1",
		RepositoryID: "repo-secondary",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	gotID, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil)
	if err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if gotID != "repo-secondary" {
		t.Fatalf("resolved repositoryID = %q, want repo-secondary (the session's stored preference, not the set's first attachment)", gotID)
	}
}

// TestApplyResumeRepoConfig_StampsRepositoryIdentityOnNonWorktreeExecutor
// covers the mandatory non-worktree regression: the reported failure runs on
// a remote (SSH) executor, and req.RepositoryID was only ever stamped inside
// the worktree-only branch. A single-attachment launch on a non-worktree
// executor must still carry the resolved primary's identity so
// environmentReposForLaunch can produce an inventory row and the guard admits
// the environment.
func TestApplyResumeRepoConfig_StampsRepositoryIdentityOnNonWorktreeExecutor(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:     "sess-1",
		TaskID: "task-1",
		// Empty preference: the repository was attached after session creation.
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "ssh"}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if req.RepositoryID != "repo-1" {
		t.Fatalf("req.RepositoryID = %q, want repo-1: a non-worktree executor must still carry the resolved primary's identity", req.RepositoryID)
	}
	if req.UseWorktree {
		t.Fatalf("expected UseWorktree=false for the ssh executor")
	}
}

// TestApplyResumeRepoConfig_StampsRepositoryIdentityWithEmptyLocalPath covers
// the mandatory empty-local-path regression: a resolved primary can have no
// local clone yet (no cloner configured, no provider identity, ...). The
// identity stamp must not be gated on repositoryPath != "", or exactly that
// population produces no inventory row and is refused by the guard.
func TestApplyResumeRepoConfig_StampsRepositoryIdentityWithEmptyLocalPath(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1",
		// LocalPath intentionally empty and no provider identity set, so
		// ensureRepoLocalPathForSessionAndState resolves with no clone.
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if req.RepositoryID != "repo-1" {
		t.Fatalf("req.RepositoryID = %q, want repo-1 even though the repository has no local clone yet", req.RepositoryID)
	}
	if req.RepositoryPath != "" {
		t.Fatalf("req.RepositoryPath = %q, want empty", req.RepositoryPath)
	}
}

// TestApplyResumeRepoConfig_BaseBranchPrefersRepositoryDefaultOverSessionValue
// pins AC-...-002.4's worked case: a session base branch of "main", exactly
// one attachment whose raw base_branch is empty, and a repository default
// branch of "develop" must resolve to "develop". "That attachment's base
// branch" in AC-...-002.4 denotes repoInfo.BaseBranch after
// resolveTaskRepoInfoForSession has already defaulted the empty raw column to
// the repository's default branch, not the raw column itself.
func TestApplyResumeRepoConfig_BaseBranchPrefersRepositoryDefaultOverSessionValue(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		LocalPath:     "/tmp/repo",
		DefaultBranch: "develop",
	}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
		// BaseBranch intentionally empty: the raw attachment column.
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:           "sess-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if req.BaseBranch != "develop" {
		t.Fatalf("BaseBranch = %q, want develop (the repository default outranks the session's stored value)", req.BaseBranch)
	}
}

// TestResumeSession_MultiRepo_PopulatesRequestRepositoriesWhenSessionHasNoPreference
// covers the multi-repo regression the applyResumeMultiRepoConfig comment
// describes: a session created before the task had any attachments must
// still resolve and configure every attachment, not just recover a primary.
func TestResumeSession_MultiRepo_PopulatesRequestRepositoriesWhenSessionHasNoPreference(t *testing.T) {
	repo := newMockRepository()
	const taskID = "task-multi-resume-no-pref"
	const sessionID = "session-multi-resume-no-pref"
	seedMultiRepoTask(t, repo, taskID)
	seedWorktreeExecutor(repo)

	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: "Multi Resume No Preference"}
	repo.sessions[sessionID] = &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		AgentProfileID: "profile-123",
		ExecutorID:     models.ExecutorIDWorktree,
		// RepositoryID intentionally empty: both attachments were added after
		// this session was created.
		State:        models.TaskSessionStateCancelled,
		ErrorMessage: models.SessionArchiveTreeCancelReason,
		StartedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	var captured *LaunchAgentRequest
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			captured = req
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-resume-multi-no-pref",
				WorktreePath:     "/tasks/y",
				Worktrees: []RepoWorktreeResult{
					{RepositoryID: "repo-front", WorktreeID: "wt-front", WorktreeBranch: "feat/y-1", WorktreePath: "/tasks/y/frontend"},
					{RepositoryID: "repo-back", WorktreeID: "wt-back", WorktreeBranch: "feat/y-2", WorktreePath: "/tasks/y/backend"},
				},
			}, nil
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions[sessionID], false); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}

	if captured == nil {
		t.Fatal("expected launch agent to be called")
	}
	if captured.RepositoryID != "repo-front" {
		t.Fatalf("resolved primary repositoryID = %q, want repo-front (position 0)", captured.RepositoryID)
	}
	if len(captured.Repositories) != 2 {
		t.Fatalf("expected req.Repositories length 2, got %d: %+v", len(captured.Repositories), captured.Repositories)
	}
	if captured.Repositories[0].RepositoryID != "repo-front" || captured.Repositories[1].RepositoryID != "repo-back" {
		t.Errorf("unexpected repo order: %+v", captured.Repositories)
	}
}

// TestApplyResumeRepoConfig_FailsClosedWhenAttachmentSetReadFails covers
// AC-...-001.6: a failed read of the attachment set must fail the launch and
// report the read failure, not proceed as though the task had no
// attachments.
func TestApplyResumeRepoConfig_FailsClosedWhenAttachmentSetReadFails(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	wantErr := errors.New("transient attachment set read failure")
	repo.listTaskRepositoriesFunc = func(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
		return nil, wantErr
	}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	_, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("applyResumeRepoConfig error = %v, want %v", err, wantErr)
	}
	if req.RepositoryID != "" {
		t.Fatalf("req.RepositoryID = %q, want empty: a failed read must not proceed as though the task had no attachments", req.RepositoryID)
	}
}
