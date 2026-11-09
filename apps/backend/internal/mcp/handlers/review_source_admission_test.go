package handlers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func initReviewSourceRepo(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "review@example.test"},
		{"config", "user.name", "Review Test"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	return dir
}

type reviewSourceFixture struct {
	h          *Handlers
	runner     *stubReviewRunner
	agent      context.Context
	session    string // worktree of the task's local session
	registered string // the task's registered repository checkout
	remote     string // workspace path of the task's Docker session
	outside    string // unrelated backend-host repository
	sibling    string // shares the session worktree's string prefix
}

func newReviewSourceFixture(t *testing.T) reviewSourceFixture {
	t.Helper()
	h, _, runner := newReviewHandlers(t)
	runner.run = &models.TaskReviewRun{ID: "run-1", Status: models.ReviewRunPending}
	base := t.TempDir()
	f := reviewSourceFixture{
		h: h, runner: runner,
		session:    initReviewSourceRepo(t, filepath.Join(base, "session")),
		registered: initReviewSourceRepo(t, filepath.Join(base, "registered")),
		remote:     filepath.Join(base, "remote"),
		outside:    initReviewSourceRepo(t, filepath.Join(base, "outside")),
		sibling:    filepath.Join(base, "session-evil"),
	}
	// Rejection is reported as forbidden before Git validation, so these only
	// need to exist.
	require.NoError(t, os.MkdirAll(f.remote, 0o755))
	require.NoError(t, os.MkdirAll(f.sibling, 0o755))
	ctx := context.Background()
	seedReviewHandlerTask(t, h, "task-a")
	repo := h.taskRepo.(interface {
		CreateTaskSession(context.Context, *models.TaskSession) error
		CreateRepository(context.Context, *models.Repository) error
		CreateTaskRepository(context.Context, *models.TaskRepository) error
	})
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-a", TaskID: "task-a", IsPrimary: true, ExecutorID: models.ExecutorIDLocal,
		WorkspacePath: f.session, State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-docker", TaskID: "task-a", ExecutorID: models.ExecutorIDLocalDocker,
		WorkspacePath: f.remote, State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-a", WorkspaceID: "ws-r", Name: "a", SourceType: "local", LocalPath: f.registered,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-a", TaskID: "task-a", RepositoryID: "repo-a"}))
	f.agent = mcpscope.WithPrincipal(ctx, mcpscope.Principal{WorkspaceID: "ws-r", CallerTaskID: "task-a", CallerSessionID: "session-a"})
	return f
}

func (f reviewSourceFixture) setSource(t *testing.T, ctx context.Context, path string) *ws.Message {
	t.Helper()
	resp, err := f.h.handleSetReviewSource(ctx, makeWSMessage(t, ws.ActionTaskReviewSourceSet, map[string]interface{}{
		"task_id": "task-a", "worktree_path": path, "base_ref": "HEAD",
	}))
	require.NoError(t, err)
	return resp
}

func requireWSForbidden(t *testing.T, resp *ws.Message) {
	t.Helper()
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Contains(t, string(resp.Payload), ws.ErrorCodeForbidden)
}

func TestAgentReviewSourceIsLimitedToTheTaskRepositories(t *testing.T) {
	f := newReviewSourceFixture(t)
	for _, path := range []string{f.session, f.registered} {
		requireWSSuccess(t, f.setSource(t, f.agent, path))
	}
	for name, path := range map[string]string{"outside": f.outside, "prefix sibling": f.sibling, "remote session": f.remote} {
		t.Run(name, func(t *testing.T) {
			requireWSForbidden(t, f.setSource(t, f.agent, path))
		})
	}
	task, err := f.h.taskSvc.GetTask(context.Background(), "task-a")
	require.NoError(t, err)
	source, err := review.SourceFromMetadata(task.Metadata)
	require.NoError(t, err)
	require.Equal(t, f.registered, source.WorktreePath, "a rejected path must not replace the saved source")
}

func TestPersonReviewSourceMayNameAnyLocalCheckout(t *testing.T) {
	f := newReviewSourceFixture(t)
	requireWSSuccess(t, f.setSource(t, context.Background(), f.outside))
}

func TestAgentReviewRunIsLimitedToTheTaskRepositories(t *testing.T) {
	f := newReviewSourceFixture(t)
	run := func(path string) *ws.Message {
		resp, err := f.h.handleRunTaskReview(f.agent, makeWSMessage(t, ws.ActionTaskReviewRun, map[string]interface{}{
			"task_id": "task-a", "worktree_path": path, "base_ref": "HEAD",
		}))
		require.NoError(t, err)
		return resp
	}
	requireWSForbidden(t, run(f.outside))
	requireWSForbidden(t, run(f.sibling))
	require.Empty(t, f.runner.launched, "a rejected path must never reach the runner")
	requireWSSuccess(t, run(f.session))
	require.Len(t, f.runner.launched, 1)
	require.Equal(t, f.session, f.runner.launched[0].WorktreePath)
}
