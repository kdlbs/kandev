package backendapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	scriptTurnMarker         = "issue5151-setup-ok"
	scriptTurnCleanupMarker  = "issue5151-cleanup-ok"
	scriptTurnRepoID         = "repo-script-added"
	scriptTurnRepoName       = "script-added"
	scriptTurnHandlerBudget  = 30 * time.Second
	scriptTurnSubprocessEnv  = "KANDEV_BACKENDAPP_SCRIPT_TURN_SUBPROCESS"
	scriptTurnSubprocessWait = 2 * time.Minute
)

// runScriptTurnSubprocess re-runs one test in a child process so a regressed
// task-gate deadlock cannot leave its attachment goroutine running while the
// parent fixture tears down. A hung child is killed and surfaced as a failure.
func runScriptTurnSubprocess(t *testing.T, testName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), scriptTurnSubprocessWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), scriptTurnSubprocessEnv+"=1")
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s subprocess did not terminate within %s; output:\n%s", testName, scriptTurnSubprocessWait, scriptTurnOutputTail(out))
	}
	if err != nil {
		t.Fatalf("%s subprocess failed: %v\n%s", testName, err, scriptTurnOutputTail(out))
	}
	t.Logf("%s passed in an isolated subprocess", testName)
}

func scriptTurnOutputTail(out []byte) string {
	const limit = 8000
	if len(out) <= limit {
		return string(out)
	}
	return "... (truncated)\n" + string(out[len(out)-limit:])
}

type scriptTurnScenario struct {
	ctx          context.Context
	repo         *sqliterepo.Repository
	svc          *taskservice.Service
	manager      *worktree.Manager
	materializer *workspaceSourceMaterializer
	rescan       *workspaceSourceRescanStub
	repoPath     string
	taskRoot     string
	primaryPath  string
	addedPath    string
}

func newScriptTurnScenario(t *testing.T) *scriptTurnScenario {
	t.Helper()
	ctx := context.Background()
	repoPath, taskRoot, primaryPath := setupMaterializerScenario(t)
	repo := newMaterializerRepo(t)
	seedMaterializerTask(t, ctx, repo, repoPath, taskRoot, primaryPath)

	manager := newPersistingMaterializerWorktreeMgr(t, taskRoot, repo)
	svc := newScriptTurnTaskService(t, repo, nil)
	scriptHandler := worktree.NewDefaultScriptMessageHandler(
		newTestLogger(),
		&taskServiceAdapter{svc: svc},
		scriptTurnHandlerBudget,
	)
	manager.SetScriptMessageHandler(scriptHandler)
	manager.SetRepositoryProvider(worktree.NewRepositoryAdapter(svc))

	materializer := newWorkspaceSourceMaterializer(repo, manager, nil, newTestLogger())
	rescan := &workspaceSourceRescanStub{}
	materializer.rescanner = rescan
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetBranchMaterializer(materializer.branches)

	return &scriptTurnScenario{
		ctx:          ctx,
		repo:         repo,
		svc:          svc,
		manager:      manager,
		materializer: materializer,
		rescan:       rescan,
		repoPath:     repoPath,
		taskRoot:     taskRoot,
		primaryPath:  primaryPath,
	}
}

// addScriptRepository registers a second local repository cloned from the
// materializer scenario origin so worktree creation runs a real setup script.
func (s *scriptTurnScenario) addScriptRepository(t *testing.T, setupScript, cleanupScript string) {
	t.Helper()
	path := filepath.Join(canonicalTempDir(t), scriptTurnRepoName)
	runGit(t, filepath.Dir(path), "clone", s.repoPath, path)
	if err := s.repo.CreateRepository(s.ctx, &models.Repository{
		ID:                   scriptTurnRepoID,
		WorkspaceID:          "ws-1",
		Name:                 scriptTurnRepoName,
		LocalPath:            path,
		DefaultBranch:        "main",
		WorktreeBranchPrefix: "feature/",
		SetupScript:          setupScript,
		CleanupScript:        cleanupScript,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	s.addedPath = path
}

func (s *scriptTurnScenario) attach(t *testing.T) (*taskservice.AttachWorkspaceSourcesResult, error) {
	t.Helper()
	return awaitScriptTurn(t, "workspace source attachment", func() (*taskservice.AttachWorkspaceSourcesResult, error) {
		return s.svc.AttachWorkspaceSources(s.ctx, taskservice.AttachWorkspaceSourcesRequest{
			TaskID:  "task-1",
			Sources: []taskservice.WorkspaceSourceInput{{Kind: taskservice.WorkspaceSourceRepository, RepositoryID: scriptTurnRepoID, BaseBranch: "main"}},
		})
	})
}

func awaitScriptTurn[T any](t *testing.T, operation string, run func() (T, error)) (T, error) {
	t.Helper()
	type outcome struct {
		value T
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		value, err := run()
		done <- outcome{value: value, err: err}
	}()
	select {
	case result := <-done:
		return result.value, result.err
	case <-time.After(scriptTurnHandlerBudget):
		t.Fatalf("%s deadlocked", operation)
		var zero T
		return zero, nil
	}
}

func (s *scriptTurnScenario) messages(t *testing.T) []*models.Message {
	t.Helper()
	messages, err := s.repo.ListMessages(s.ctx, "session-1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	return messages
}

func (s *scriptTurnScenario) messagesOfType(t *testing.T, scriptType string) []*models.Message {
	t.Helper()
	found := make([]*models.Message, 0, 1)
	for _, message := range s.messages(t) {
		if message.Metadata["script_type"] == scriptType {
			found = append(found, message)
		}
	}
	return found
}

func (s *scriptTurnScenario) requireScriptMessage(t *testing.T, scriptType, wantStatus, wantOutput string) *models.Message {
	t.Helper()
	messages := s.messagesOfType(t, scriptType)
	if len(messages) != 1 {
		t.Fatalf("%s script messages = %d, want 1", scriptType, len(messages))
	}
	message := messages[0]
	if got := message.Metadata["status"]; got != wantStatus {
		t.Fatalf("%s status = %v, want %q (content %q)", scriptType, got, wantStatus, message.Content)
	}
	if !strings.Contains(message.Content, wantOutput) {
		t.Fatalf("%s content = %q, want %q", scriptType, message.Content, wantOutput)
	}
	return message
}

func (s *scriptTurnScenario) requireCompletedScriptTurn(t *testing.T, message *models.Message) {
	t.Helper()
	if message.TurnID == "" {
		t.Fatalf("script message %s has no turn binding", message.ID)
	}
	turn, err := s.svc.GetTurn(s.ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatalf("script turn %s is active, want completed", turn.ID)
	}
	if turn.Metadata[models.TurnMetaKeyLifecycleOnly] != true {
		t.Fatalf("script turn metadata = %+v, want lifecycle_only", turn.Metadata)
	}
}

func (s *scriptTurnScenario) requireIdle(t *testing.T) {
	t.Helper()
	active, err := s.svc.GetActiveTurn(s.ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if active != nil {
		t.Fatalf("active turn = %q, want none", active.ID)
	}
}

func (s *scriptTurnScenario) requireNextTurnStarts(t *testing.T) {
	t.Helper()
	turn, err := s.svc.StartTurn(s.ctx, "session-1")
	if err != nil {
		t.Fatalf("next user turn rejected: %v", err)
	}
	if turn == nil {
		t.Fatal("next user turn is nil")
	}
}

func (s *scriptTurnScenario) requireTaskRepository(t *testing.T, repositoryID string) *models.TaskRepository {
	t.Helper()
	repositories, err := s.repo.ListTaskRepositories(s.ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	for _, repository := range repositories {
		if repository.RepositoryID == repositoryID {
			return repository
		}
	}
	t.Fatalf("task repositories = %+v, want %s", repositories, repositoryID)
	return nil
}

func (s *scriptTurnScenario) requireWorktree(t *testing.T, repositoryID string) *worktree.Worktree {
	t.Helper()
	worktrees, err := s.manager.GetAllByTaskID(s.ctx, "task-1")
	if err != nil {
		t.Fatalf("GetAllByTaskID: %v", err)
	}
	for _, candidate := range worktrees {
		if candidate.RepositoryID == repositoryID {
			return candidate
		}
	}
	t.Fatalf("worktrees = %+v, want repository %s", worktrees, repositoryID)
	return nil
}

func (s *scriptTurnScenario) requireMarker(t *testing.T, worktreePath, marker string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(worktreePath, "setup-marker.txt"))
	if err != nil {
		t.Fatalf("read setup marker in %s: %v", worktreePath, err)
	}
	if !strings.Contains(string(content), marker) {
		t.Fatalf("setup marker = %q, want %q", string(content), marker)
	}
}

// TestWorkspaceSourceScripts_AttachIdle covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
// and AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5: an idle Worktree attachment with a
// setup script must complete materialization without opening a conversational turn.
func TestWorkspaceSourceScripts_AttachIdle(t *testing.T) {
	if os.Getenv(scriptTurnSubprocessEnv) != "1" {
		runScriptTurnSubprocess(t, "TestWorkspaceSourceScripts_AttachIdle")
		return
	}
	scenario := newScriptTurnScenario(t)
	scenario.addScriptRepository(t, "echo "+scriptTurnMarker+" > setup-marker.txt && echo "+scriptTurnMarker, "")

	result, err := scenario.attach(t)
	if err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	if result == nil || !result.Changed {
		t.Fatalf("attachment result = %+v, want changed", result)
	}
	scenario.requireTaskRepository(t, scriptTurnRepoID)
	worktree := scenario.requireWorktree(t, scriptTurnRepoID)
	scenario.requireMarker(t, worktree.Path, scriptTurnMarker)

	message := scenario.requireScriptMessage(t, "setup", "exited", scriptTurnMarker)
	scenario.requireCompletedScriptTurn(t, message)
	scenario.requireIdle(t)
	scenario.requireNextTurnStarts(t)
}

// TestWorkspaceSourceScripts_ExactRetry covers the exact-retry contract: a
// repeated identical attachment must not re-run setup or open a turn.
func TestWorkspaceSourceScripts_ExactRetry(t *testing.T) {
	scenario := newScriptTurnScenario(t)
	scenario.addScriptRepository(t, "echo "+scriptTurnMarker+" > setup-marker.txt && echo "+scriptTurnMarker, "")

	if _, err := scenario.attach(t); err != nil {
		t.Fatalf("first AttachWorkspaceSources: %v", err)
	}
	retry, err := scenario.attach(t)
	if err != nil {
		t.Fatalf("retry AttachWorkspaceSources: %v", err)
	}
	if retry == nil || retry.Changed {
		t.Fatalf("retry result = %+v, want unchanged", retry)
	}
	repositories, err := scenario.repo.ListTaskRepositories(scenario.ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(repositories) != 2 {
		t.Fatalf("task repositories after retry = %d, want 2", len(repositories))
	}
	if messages := scenario.messagesOfType(t, "setup"); len(messages) != 1 {
		t.Fatalf("setup messages after retry = %d, want 1", len(messages))
	}
	scenario.requireCompletedScriptTurn(t, scenario.requireScriptMessage(t, "setup", "exited", scriptTurnMarker))
	scenario.requireIdle(t)
	scenario.requireNextTurnStarts(t)
}

// TestWorkspaceSourceScripts_SetupFailure covers the nonfatal setup policy: a
// nonzero setup exit keeps the attachment and worktree while the script message
// reports the failure.
func TestWorkspaceSourceScripts_SetupFailure(t *testing.T) {
	scenario := newScriptTurnScenario(t)
	scenario.addScriptRepository(t, "echo "+scriptTurnMarker+"-failed; exit 3", "")

	if _, err := scenario.attach(t); err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	scenario.requireTaskRepository(t, scriptTurnRepoID)
	worktree := scenario.requireWorktree(t, scriptTurnRepoID)
	if _, err := os.Stat(worktree.Path); err != nil {
		t.Fatalf("failed setup removed worktree %s: %v", worktree.Path, err)
	}
	message := scenario.requireScriptMessage(t, "setup", "failed", scriptTurnMarker+"-failed")
	if !strings.Contains(message.Content, "exit code: 3") {
		t.Fatalf("setup failure content = %q, want exit code", message.Content)
	}
	scenario.requireCompletedScriptTurn(t, message)
	scenario.requireIdle(t)
	scenario.requireNextTurnStarts(t)
}

// TestWorkspaceSourceScripts_AddBranch covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
// for both the idle legacy path and a path that owns an existing conversational turn.
func TestWorkspaceSourceScripts_AddBranch(t *testing.T) {
	if os.Getenv(scriptTurnSubprocessEnv) != "1" {
		runScriptTurnSubprocess(t, "TestWorkspaceSourceScripts_AddBranch")
		return
	}
	addBranch := func(t *testing.T, scenario *scriptTurnScenario) *taskservice.AddBranchToTaskResult {
		t.Helper()
		result, err := awaitScriptTurn(t, "legacy add branch", func() (*taskservice.AddBranchToTaskResult, error) {
			return scenario.svc.AddBranchToTask(scenario.ctx, taskservice.AddBranchToTaskRequest{
				TaskID:       "task-1",
				RepositoryID: scriptTurnRepoID,
				BaseBranch:   "main",
			})
		})
		if err != nil {
			t.Fatalf("AddBranchToTask: %v", err)
		}
		return result
	}

	t.Run("idle", func(t *testing.T) {
		scenario := newScriptTurnScenario(t)
		scenario.addScriptRepository(t, "echo "+scriptTurnMarker+" > setup-marker.txt && echo "+scriptTurnMarker, "")

		result := addBranch(t, scenario)
		if result.AgentCWDChanged {
			t.Fatal("legacy add branch changed the agent CWD")
		}
		scenario.requireTaskRepository(t, scriptTurnRepoID)
		worktree := scenario.requireWorktree(t, scriptTurnRepoID)
		scenario.requireMarker(t, worktree.Path, scriptTurnMarker)
		message := scenario.requireScriptMessage(t, "setup", "exited", scriptTurnMarker)
		scenario.requireCompletedScriptTurn(t, message)
		scenario.requireIdle(t)
		scenario.requireNextTurnStarts(t)
	})

	t.Run("active", func(t *testing.T) {
		scenario := newScriptTurnScenario(t)
		scenario.addScriptRepository(t, "echo "+scriptTurnMarker+" > setup-marker.txt && echo "+scriptTurnMarker, "")
		active, err := scenario.svc.StartTurn(scenario.ctx, "session-1")
		if err != nil {
			t.Fatalf("StartTurn: %v", err)
		}

		result := addBranch(t, scenario)
		if result.AgentCWDChanged {
			t.Fatal("legacy add branch changed the agent CWD")
		}
		worktree := scenario.requireWorktree(t, scriptTurnRepoID)
		scenario.requireMarker(t, worktree.Path, scriptTurnMarker)
		message := scenario.requireScriptMessage(t, "setup", "exited", scriptTurnMarker)
		if message.TurnID != active.ID {
			t.Fatalf("script message turn = %q, want active %q", message.TurnID, active.ID)
		}
		current, err := scenario.svc.GetActiveTurn(scenario.ctx, "session-1")
		if err != nil {
			t.Fatalf("GetActiveTurn: %v", err)
		}
		if current == nil || current.ID != active.ID {
			t.Fatalf("active turn = %+v, want %q", current, active.ID)
		}
	})
}

type failingInventoryListRepo struct {
	workspaceSourceMaterializerRepo
	err error
}

func (r *failingInventoryListRepo) ListTaskEnvironmentRepos(context.Context, string) ([]*models.TaskEnvironmentRepo, error) {
	return nil, r.err
}

// TestWorkspaceSourceScripts_Compensation covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4:
// a post-setup materialization failure rolls back the attachment and any cleanup
// message finishes without another turn-creation attempt.
func TestWorkspaceSourceScripts_Compensation(t *testing.T) {
	scenario := newScriptTurnScenario(t)
	recordPath := filepath.Join(t.TempDir(), "created-worktree.txt")
	scenario.addScriptRepository(t,
		`pwd -P > "`+recordPath+`" && echo `+scriptTurnMarker,
		"echo "+scriptTurnCleanupMarker)
	scenario.materializer.repo = &failingInventoryListRepo{
		workspaceSourceMaterializerRepo: scenario.repo,
		err:                             errors.New("inventory unavailable"),
	}

	_, attachErr := scenario.attach(t)
	if !errors.Is(attachErr, taskservice.ErrWorkspaceSourceMaterialize) {
		t.Fatalf("AttachWorkspaceSources error = %v, want materialization failure", attachErr)
	}
	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("setup script did not record its worktree: %v", err)
	}
	createdWorktree := strings.TrimSpace(string(recorded))
	if createdWorktree == "" {
		t.Fatal("setup script recorded an empty worktree path")
	}
	if _, statErr := os.Stat(createdWorktree); !os.IsNotExist(statErr) {
		t.Fatalf("compensation left worktree directory %s: %v", createdWorktree, statErr)
	}
	registrations := runGit(t, scenario.addedPath, "worktree", "list", "--porcelain")
	if strings.Contains(registrations, createdWorktree) {
		t.Fatalf("compensation left Git worktree registration for %s", createdWorktree)
	}
	repositories, err := scenario.repo.ListTaskRepositories(scenario.ctx, "task-1")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(repositories) != 1 || repositories[0].RepositoryID != "repo-1" {
		t.Fatalf("compensated task repositories = %+v, want primary only", repositories)
	}
	worktrees, err := scenario.manager.GetAllByTaskID(scenario.ctx, "task-1")
	if err != nil {
		t.Fatalf("GetAllByTaskID: %v", err)
	}
	for _, worktree := range worktrees {
		if worktree.RepositoryID == scriptTurnRepoID {
			t.Fatalf("compensation kept worktree %+v", worktree)
		}
	}
	cleanup := scenario.requireScriptMessage(t, "cleanup", "exited", scriptTurnCleanupMarker)
	scenario.requireCompletedScriptTurn(t, cleanup)
	scenario.requireIdle(t)
	scenario.requireNextTurnStarts(t)
}
