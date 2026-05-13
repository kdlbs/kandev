package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// fakeRepoProvider returns canned Repository values per ID.
type fakeRepoProvider struct {
	repos map[string]*worktree.Repository
}

func (p *fakeRepoProvider) GetRepository(_ context.Context, id string) (*worktree.Repository, error) {
	if r, ok := p.repos[id]; ok {
		return r, nil
	}
	return &worktree.Repository{ID: id}, nil
}

// recordingScriptHandler records every ExecuteSetupScript invocation and
// runs the script on disk (matching production DefaultScriptMessageHandler
// behaviour). The recorded calls let tests assert how many times the script
// handler is invoked per repo.
type recordingScriptHandler struct {
	mu          sync.Mutex
	setupCalls  []worktree.ScriptExecutionRequest
	scriptError error
}

func (h *recordingScriptHandler) ExecuteSetupScript(ctx context.Context, req worktree.ScriptExecutionRequest) error {
	h.mu.Lock()
	h.setupCalls = append(h.setupCalls, req)
	h.mu.Unlock()
	if h.scriptError != nil {
		return h.scriptError
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", req.Script)
	cmd.Dir = req.WorkingDir
	return cmd.Run()
}

func (h *recordingScriptHandler) ExecuteCleanupScript(_ context.Context, _ worktree.ScriptExecutionRequest) error {
	return nil
}

func (h *recordingScriptHandler) callsForRepo(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.setupCalls {
		if c.RepositoryID == id {
			n++
		}
	}
	return n
}

func newPreparerWithScriptHandler(t *testing.T, repos map[string]*worktree.Repository) (*WorktreePreparer, *worktree.Manager, *recordingScriptHandler) {
	t.Helper()
	tmp := t.TempDir()
	cfg := worktree.Config{
		Enabled:       true,
		TasksBasePath: filepath.Join(tmp, "tasks"),
		BranchPrefix:  "feat/",
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemoryWorktreeStore()
	mgr, err := worktree.NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	handler := &recordingScriptHandler{}
	mgr.SetScriptMessageHandler(handler)
	mgr.SetRepositoryProvider(&fakeRepoProvider{repos: repos})
	return NewWorktreePreparer(mgr, log), mgr, handler
}

// TestWorktreePreparer_MultiRepo_RunsSetupScriptOncePerRepo ensures that when
// the worktree manager is wired with a script handler (production layout),
// each repo's setup script runs exactly once during a multi-repo prepare.
//
// Regression: the worktree manager runs the setup script via the script
// handler from inside Create(), and the env preparer used to also run the
// same per-repo setup script as a separate prepare step, causing the script
// to run twice per repo. For non-idempotent scripts (e.g. anything that
// echoes to a file with > or appends rows to a database) the second run
// produced unexpected state.
func TestWorktreePreparer_MultiRepo_RunsSetupScriptOncePerRepo(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	repos := map[string]*worktree.Repository{
		"repo-front": {ID: "repo-front", SetupScript: "echo front >> setup-marker.txt"},
		"repo-back":  {ID: "repo-back", SetupScript: "echo back >> setup-marker.txt"},
	}

	preparer, _, handler := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-once",
		SessionID:    "sess-multi-once",
		TaskTitle:    "Once Task",
		ExecutorType: executor.Name("worktree"),
		TaskDirName:  "once-task_xxx",
		Repositories: []RepoPrepareSpec{
			{
				RepositoryID:    "repo-front",
				RepositoryPath:  repoA,
				RepoName:        "frontend",
				BaseBranch:      "main",
				RepoSetupScript: "echo front >> setup-marker.txt",
			},
			{
				RepositoryID:    "repo-back",
				RepositoryPath:  repoB,
				RepoName:        "backend",
				BaseBranch:      "main",
				RepoSetupScript: "echo back >> setup-marker.txt",
			},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success; err: %s; steps: %+v", res.ErrorMessage, res.Steps)
	}

	if got := handler.callsForRepo("repo-front"); got != 1 {
		t.Errorf("repo-front: setup script invoked %d time(s) via handler, want 1", got)
	}
	if got := handler.callsForRepo("repo-back"); got != 1 {
		t.Errorf("repo-back: setup script invoked %d time(s) via handler, want 1", got)
	}

	// Cross-check on disk: with proper single-execution semantics the marker
	// file should have exactly one line per repo. A second execution from the
	// env preparer would append a second line.
	for _, w := range res.Worktrees {
		marker := filepath.Join(w.WorktreePath, "setup-marker.txt")
		data, statErr := os.ReadFile(marker)
		if statErr != nil {
			t.Errorf("repo %s: missing setup marker: %v", w.RepositoryID, statErr)
			continue
		}
		// Each script writes one line of the form "front\n" or "back\n".
		// Two executions would produce two lines.
		if lines := splitNonEmpty(string(data)); len(lines) != 1 {
			t.Errorf("repo %s: expected 1 marker line, got %d (%q)", w.RepositoryID, len(lines), string(data))
		}
	}
}

// TestWorktreePreparer_MultiRepo_NonIdempotentSetupScriptSucceeds reproduces
// the user-facing failure: when both repos have non-idempotent setup scripts
// (e.g. mkdir without -p), the duplicate execution makes the second run
// fail and the whole multi-repo prepare reports failure.
func TestWorktreePreparer_MultiRepo_NonIdempotentSetupScriptSucceeds(t *testing.T) {
	repoA := initBareGitRepo(t, "frontend")
	repoB := initBareGitRepo(t, "backend")

	// `mkdir build` fails the second time it runs (directory already exists).
	// `set -e` makes any failure abort the script with a non-zero exit code.
	script := "set -e; mkdir build"
	repos := map[string]*worktree.Repository{
		"repo-front": {ID: "repo-front", SetupScript: script},
		"repo-back":  {ID: "repo-back", SetupScript: script},
	}

	preparer, _, _ := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:       "task-multi-nonidempotent",
		SessionID:    "sess-multi-nonidempotent",
		TaskTitle:    "Non-Idempotent Task",
		ExecutorType: executor.Name("worktree"),
		TaskDirName:  "nonidempotent_yyy",
		Repositories: []RepoPrepareSpec{
			{
				RepositoryID:    "repo-front",
				RepositoryPath:  repoA,
				RepoName:        "frontend",
				BaseBranch:      "main",
				RepoSetupScript: script,
			},
			{
				RepositoryID:    "repo-back",
				RepositoryPath:  repoB,
				RepoName:        "backend",
				BaseBranch:      "main",
				RepoSetupScript: script,
			},
		},
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected prepare to succeed for both repos; got failure: %s", res.ErrorMessage)
	}
	if len(res.Worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(res.Worktrees))
	}
}

// TestWorktreePreparer_SingleRepo_RunsSetupScriptOnce ensures the single-repo
// worktree path runs the per-repo setup script exactly once. The worktree
// manager runs it via its script handler from inside Create(); the env
// preparer must not re-run the same script (directly or via the default
// worktree prepare template that resolves {{repository.setup_script}}).
func TestWorktreePreparer_SingleRepo_RunsSetupScriptOnce(t *testing.T) {
	repo := initBareGitRepo(t, "single")

	repos := map[string]*worktree.Repository{
		"repo-single": {ID: "repo-single", SetupScript: "echo single >> setup-marker.txt"},
	}
	preparer, _, handler := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:          "task-single-once",
		SessionID:       "sess-single-once",
		TaskTitle:       "Single Once",
		ExecutorType:    executor.Name("worktree"),
		TaskDirName:     "single-once_zzz",
		UseWorktree:     true,
		RepositoryID:    "repo-single",
		RepositoryPath:  repo,
		RepoName:        "single",
		BaseBranch:      "main",
		RepoSetupScript: "echo single >> setup-marker.txt",
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success; err: %s; steps: %+v", res.ErrorMessage, res.Steps)
	}

	if got := handler.callsForRepo("repo-single"); got != 1 {
		t.Errorf("repo-single: setup script invoked %d time(s) via handler, want 1", got)
	}

	// Cross-check on disk: with single-execution semantics the marker file
	// should have exactly one line. A second execution from the env preparer
	// (directly or via the default template) would append a second line.
	marker := filepath.Join(res.WorkspacePath, "setup-marker.txt")
	data, statErr := os.ReadFile(marker)
	if statErr != nil {
		t.Fatalf("missing setup marker at %s: %v", marker, statErr)
	}
	if lines := splitNonEmpty(string(data)); len(lines) != 1 {
		t.Errorf("expected 1 marker line, got %d (%q)", len(lines), string(data))
	}
}

// TestWorktreePreparer_SingleRepo_NonIdempotentSetupScriptSucceeds reproduces
// the user-facing failure for the single-repo worktree path: a non-idempotent
// setup script (e.g. "mkdir build") fails on a second run. The env preparer
// records setup failures as a non-fatal step rather than aborting the whole
// prepare, so duplicate execution surfaces as a "failed" step in the chat
// even though the agent eventually launches.
func TestWorktreePreparer_SingleRepo_NonIdempotentSetupScriptSucceeds(t *testing.T) {
	repo := initBareGitRepo(t, "single")

	script := "set -e; mkdir build"
	repos := map[string]*worktree.Repository{
		"repo-single": {ID: "repo-single", SetupScript: script},
	}
	preparer, _, _ := newPreparerWithScriptHandler(t, repos)

	req := &EnvPrepareRequest{
		TaskID:          "task-single-nonidempotent",
		SessionID:       "sess-single-nonidempotent",
		TaskTitle:       "Single Non-Idempotent",
		ExecutorType:    executor.Name("worktree"),
		TaskDirName:     "single-nonidempotent_qqq",
		UseWorktree:     true,
		RepositoryID:    "repo-single",
		RepositoryPath:  repo,
		RepoName:        "single",
		BaseBranch:      "main",
		RepoSetupScript: script,
	}

	res, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("prepare returned hard error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected single-repo prepare to succeed; got failure: %s", res.ErrorMessage)
	}
	for _, s := range res.Steps {
		if s.Status == PrepareStepFailed {
			t.Errorf("unexpected failed step %q: %s (duplicate execution of non-idempotent script)", s.Name, s.Error)
		}
	}
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
