package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
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

// TestApplyResumeRepoConfig_LogsInfoOnlyWhenFallingBackToAttachmentSet covers
// the design's Observability requirement: a resume that falls back to the
// task attachment set because the session carried no preference must log an
// info line with the task id, session id, and resolved repository id, so the
// repaired-session population is distinguishable in logs from a session that
// always had a preference — which must not log anything.
func TestApplyResumeRepoConfig_LogsInfoOnlyWhenFallingBackToAttachmentSet(t *testing.T) {
	newSetup := func(sessionRepositoryID string) (*mockRepository, *v1.Task, *models.TaskSession) {
		repo := newMockRepository()
		repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
		repo.taskRepositories["tr-1"] = &models.TaskRepository{
			ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
		}
		repo.tasks["task-1"] = &models.Task{ID: "task-1"}
		return repo, &v1.Task{ID: "task-1"}, &models.TaskSession{ID: "sess-1", TaskID: "task-1", RepositoryID: sessionRepositoryID}
	}

	t.Run("empty preference logs the fallback", func(t *testing.T) {
		repo, task, session := newSetup("")
		core, logs := observer.New(zapcore.InfoLevel)
		log, err := logger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("NewFromZap: %v", err)
		}
		exec := NewExecutor(&mockAgentManager{}, repo, log, ExecutorConfig{ShellPrefs: &mockShellPrefs{}})
		exec.SetCapabilities(&mockCapabilities{})
		req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

		if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
			t.Fatalf("applyResumeRepoConfig: %v", err)
		}
		entries := logs.FilterMessage("resolved resume repository from task attachment set").All()
		if len(entries) != 1 {
			t.Fatalf("info entries = %d, want 1; all=%v", len(entries), logs.All())
		}
		fields := entries[0].ContextMap()
		if fields["task_id"] != "task-1" || fields["session_id"] != "sess-1" || fields["repository_id"] != "repo-1" {
			t.Fatalf("log fields = %+v, want task_id=task-1 session_id=sess-1 repository_id=repo-1", fields)
		}
	})

	t.Run("present preference logs nothing", func(t *testing.T) {
		repo, task, session := newSetup("repo-1")
		core, logs := observer.New(zapcore.InfoLevel)
		log, err := logger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("NewFromZap: %v", err)
		}
		exec := NewExecutor(&mockAgentManager{}, repo, log, ExecutorConfig{ShellPrefs: &mockShellPrefs{}})
		exec.SetCapabilities(&mockCapabilities{})
		req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

		if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
			t.Fatalf("applyResumeRepoConfig: %v", err)
		}
		if n := logs.FilterMessage("resolved resume repository from task attachment set").Len(); n != 0 {
			t.Fatalf("info entries = %d, want 0 for a session with a present preference; all=%v", n, logs.All())
		}
	})
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

// TestApplyResumeRepoConfig_StampsRepositoryPathAndNameOnLocalExecutor covers
// a regression found in review round 1: the unconditional req.RepositoryID
// stamp above made LaunchRequest.RepoSpecs() synthesize a non-nil spec for
// the "local"/"local_pc" executor (previously RepoSpecs() returned nil there
// since neither RepositoryID nor RepositoryPath were ever set on resume).
// reconcileWorkspaceRepositories rejects a spec whose RepoName/RepositoryPath
// are empty as "invalid durable workspace repository", so every local-executor
// resume that resolves a primary now failed the launch outright. RepositoryPath
// and RepoName must get the same unconditional treatment RepositoryID did,
// mirroring the initial-launch path (applyRepositoryConfig,
// executor_execute.go:1954), which sets them together with no executor-type
// conjunct.
func TestApplyResumeRepoConfig_StampsRepositoryPathAndNameOnLocalExecutor(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", Name: "repo-1", LocalPath: "/tmp/repo"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:           "sess-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1", // present preference: the previously-working case that also broke.
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "local"}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if req.RepositoryID != "repo-1" {
		t.Fatalf("req.RepositoryID = %q, want repo-1", req.RepositoryID)
	}
	if req.RepositoryPath != "/tmp/repo" {
		t.Fatalf("req.RepositoryPath = %q, want /tmp/repo: reconcileWorkspaceRepositories needs a non-empty path for the local executor", req.RepositoryPath)
	}
	if req.RepoName == "" {
		t.Fatalf("req.RepoName is empty, want a filesystem-safe name: reconcileWorkspaceRepositories rejects an empty RepoName")
	}
}

// TestApplyResumeRepoConfig_StampsRepositoryIdentityOnNonWorktreeExecutorWithPresentPreference
// covers test-supervisor's must-fix: AC-001.4's non-worktree arm was only
// tested with an EMPTY session preference. The unconditional stamp added in
// review round 1 also changes behavior for a session that carries a PRESENT
// preference on a non-worktree executor — previously req.RepositoryID was
// never stamped there either (same worktree-only gate), so no inventory row
// was ever produced for that population. This is newly reachable behavior,
// not just the newly-fixed empty-preference population.
func TestApplyResumeRepoConfig_StampsRepositoryIdentityOnNonWorktreeExecutorWithPresentPreference(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0, BaseBranch: "main",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{
		ID:           "sess-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1", // present preference, not the empty-preference population.
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "ssh"}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if req.RepositoryID != "repo-1" {
		t.Fatalf("req.RepositoryID = %q, want repo-1: a present preference on a non-worktree executor must also carry the identity", req.RepositoryID)
	}
}

// TestApplyResumeRepoConfig_EmptyAttachmentSetAndEmptyPreferenceLaunchesWithNoRepoConfig
// covers AC-...-001.5: a genuinely repository-less task whose session carries
// no preference must launch with no repository configuration and no
// req.Repositories, not an error. Flagged as untested by test-supervisor: this
// is the Empty shadow path of resolveResumeRepoIDAndBranch/applyResumeRepoConfig,
// the exact function this diff rewrote, and every other existing call site sets
// a non-empty session.RepositoryID.
func TestApplyResumeRepoConfig_EmptyAttachmentSetAndEmptyPreferenceLaunchesWithNoRepoConfig(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}

	gotID, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil)
	if err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}
	if gotID != "" {
		t.Fatalf("resolved repositoryID = %q, want empty for a repository-less task", gotID)
	}
	if req.RepositoryID != "" {
		t.Fatalf("req.RepositoryID = %q, want empty", req.RepositoryID)
	}
	if req.Repositories != nil {
		t.Fatalf("req.Repositories = %+v, want nil", req.Repositories)
	}
}

// TestApplyResumeRepoConfig_WorktreeBuiltInDefaultAppliesOnlyOnWorktreeExecutor
// covers AC-...-002.2's boundary, flagged untested by test-supervisor: before
// this diff, an empty-preference resume returned early before
// applyResumeWorktreeConfig could ever run, so its built-in "main" fallback
// never fired for the repaired population. It fires now. The built-in default
// must still apply only to the worktree executor's single-repository fields,
// never to any other executor type.
func TestApplyResumeRepoConfig_WorktreeBuiltInDefaultAppliesOnlyOnWorktreeExecutor(t *testing.T) {
	newSetup := func() (*mockRepository, *v1.Task, *models.TaskSession) {
		repo := newMockRepository()
		repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
		repo.taskRepositories["tr-1"] = &models.TaskRepository{
			ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
			// BaseBranch empty and repository DefaultBranch empty: every rung
			// of the branch-precedence chain is empty except the session,
			// which also carries none.
		}
		repo.tasks["task-1"] = &models.Task{ID: "task-1"}
		return repo, &v1.Task{ID: "task-1"}, &models.TaskSession{ID: "sess-1", TaskID: "task-1"}
	}

	t.Run("worktree gets the built-in main default", func(t *testing.T) {
		repo, task, session := newSetup()
		exec := newTestExecutor(t, &mockAgentManager{}, repo)
		req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}
		if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
			t.Fatalf("applyResumeRepoConfig: %v", err)
		}
		if req.BaseBranch != "main" {
			t.Fatalf("BaseBranch = %q, want main (worktree built-in default)", req.BaseBranch)
		}
	})

	t.Run("non-worktree leaves base branch unset", func(t *testing.T) {
		repo, task, session := newSetup()
		exec := newTestExecutor(t, &mockAgentManager{}, repo)
		req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "ssh"}
		if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
			t.Fatalf("applyResumeRepoConfig: %v", err)
		}
		if req.BaseBranch != "" {
			t.Fatalf("BaseBranch = %q, want empty: the worktree built-in default must not leak to other executor types", req.BaseBranch)
		}
	})
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

// TestApplyResumeRepoConfig_LocalDockerFallsBackToRepositoryPathOnResume is a
// PR-fixup regression (greptile finding): applyResumeCloneURL, unlike the
// initial-launch path (applyRepositoryConfig, executor_execute.go), never fell
// back to dockerLocalCloneSource when the repository has no provider identity
// or remote origin. A workspace-source repository with only a local checkout
// could launch initially but never resume on local_docker, failing every
// resume with ErrNoCloneURL.
func TestApplyResumeRepoConfig_LocalDockerFallsBackToRepositoryPathOnResume(t *testing.T) {
	repo := newMockRepository()
	source := t.TempDir()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", Name: "repo-1", LocalPath: source}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v, want nil: a local_docker resume of a repository with only a local checkout must fall back to the repository path", err)
	}
	if req.RepositoryURL != source {
		t.Fatalf("req.RepositoryURL = %q, want %q (the repository path used as the Docker bind-mount clone source)", req.RepositoryURL, source)
	}
}

// TestApplyResumeRepoConfig_LocalDockerRejectsMissingRepositoryPathOnResume
// pins the failure case: when neither a provider/remote clone URL nor a usable
// local repository path exists, resume must still refuse with ErrNoCloneURL
// rather than silently launching with an empty clone source.
func TestApplyResumeRepoConfig_LocalDockerRejectsMissingRepositoryPathOnResume(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", Name: "repo-1"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0,
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := &v1.Task{ID: "task-1"}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1", RepositoryID: "repo-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	_, err := exec.applyResumeRepoConfig(context.Background(), task, session, req, nil)
	if !errors.Is(err, ErrNoCloneURL) {
		t.Fatalf("applyResumeRepoConfig error = %v, want ErrNoCloneURL", err)
	}
}
