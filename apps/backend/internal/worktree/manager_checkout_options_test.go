package worktree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-TASKS-REMOTE-OPTIONS-003.1
// @covers AC-TASKS-REMOTE-OPTIONS-003.5
func TestRepositoryCheckoutOptionsIndependentWorktrees(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	for _, directory := range []string{"app", "other"} {
		if err := os.Mkdir(filepath.Join(repository, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repository, directory, "file.txt"), []byte(directory), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repository, "add", ".")
	runGit(t, repository, "-c", "commit.gpgsign=false", "commit", "-m", "directories")
	manager, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]string)
	for _, directory := range []string{"app", "other"} {
		request := CreateRequest{TaskID: directory, SessionID: directory, RepositoryID: "repo", RepositoryPath: repository, BaseBranch: "main", TaskDirName: directory, RepoName: "repo"}
		if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"standard","sparse_directories":["`+directory+`"]}}`), &request); err != nil {
			t.Fatal(err)
		}
		wt, err := manager.Create(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		paths[directory] = wt.Path
		if value, err := os.ReadFile(filepath.Join(wt.Path, directory, "file.txt")); err != nil || string(value) != directory {
			t.Fatalf("selected directory missing: %q, %v", value, err)
		}
		if output := runGit(t, wt.Path, "status", "--porcelain"); output != "" {
			t.Fatalf("sparse files appear as changes: %q", output)
		}
	}
	for selected, excluded := range map[string]string{"app": "other", "other": "app"} {
		if _, err := os.Stat(filepath.Join(paths[selected], excluded, "file.txt")); !os.IsNotExist(err) {
			t.Fatalf("task %s populated excluded folder %s", selected, excluded)
		}
	}
	if _, err := os.Stat(filepath.Join(repository, "other", "file.txt")); err != nil {
		t.Fatal("source checkout changed")
	}
}

func TestRepositoryCheckoutOptionsResumePreservesEdits(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	if err := os.Mkdir(filepath.Join(repository, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "app", "file.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".")
	runGit(t, repository, "-c", "commit.gpgsign=false", "commit", "-m", "app")
	manager, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	req := CreateRequest{TaskID: "task", SessionID: "session", RepositoryID: "repo", RepositoryPath: repository, BaseBranch: "main", TaskDirName: "task", RepoName: "repo"}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"standard","sparse_directories":["app"]}}`), &req); err != nil {
		t.Fatal(err)
	}
	wt, err := manager.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(wt.Path, "app", "file.txt")
	if err := os.WriteFile(file, []byte("user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	req.CheckoutOptions.SparseDirectories = []string{"app", "app"}
	if _, err := manager.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "user edit" {
		t.Fatalf("edit changed: %q %v", data, err)
	}
	req.CheckoutOptions.SparseDirectories = []string{"missing"}
	if _, err := manager.Create(context.Background(), req); err == nil {
		t.Fatal("changed scope silently reused a dirty worktree")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "user edit" {
		t.Fatalf("edit changed: %q %v", data, err)
	}
}

func TestRepositoryCheckoutOptionsSkipExcludedSubmodules(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	runGit(t, repository, "update-index", "--add", "--cacheinfo", "160000,"+strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))+",excluded/module")
	runGit(t, repository, "-c", "commit.gpgsign=false", "commit", "-m", "submodule")
	req := CreateRequest{}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"standard","sparse_directories":["app"]}}`), &req); err != nil {
		t.Fatal(err)
	}
	ctx, err := withCheckoutOptions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := manager.scopedSubmoduleUpdateCmd(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != nil {
		t.Fatal("excluded submodule would be initialized")
	}
}

func TestRepositoryCheckoutOptionsIgnoreScriptGitControls(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	req := CreateRequest{ScriptEnv: map[string]string{"GIT_DIR": "/missing/git", "GIT_WORK_TREE": "/missing/worktree", "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.bare", "GIT_CONFIG_VALUE_0": "true"}}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"on_demand"}}`), &req); err != nil {
		t.Fatal(err)
	}
	ctx, err := withCheckoutOptions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newGitCommand(ctx, "-C", repository, "rev-parse", "--is-bare-repository")
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "false" {
		t.Fatalf("profile redirected checkout: %s %v", output, err)
	}
}

func TestRepositoryCheckoutOptionsAcceptExactSubmodule(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	runGit(t, repository, "update-index", "--add", "--cacheinfo", "160000,"+strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))+",module")
	runGit(t, repository, "-c", "commit.gpgsign=false", "commit", "-m", "submodule")
	req := CreateRequest{}
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"standard","sparse_directories":["module"]}}`), &req); err != nil {
		t.Fatal(err)
	}
	ctx, err := withCheckoutOptions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := applySparseCheckout(ctx, repository); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryCheckoutOptionsDetectsUnpopulatedGitCrypt(t *testing.T) {
	repository := initGitRepoForWorktreeTest(t)
	attributes := filepath.Join(repository, ".gitattributes")
	if err := os.WriteFile(attributes, []byte("secret filter=git-crypt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".gitattributes")
	runGit(t, repository, "-c", "commit.gpgsign=false", "commit", "-m", "attributes")
	if err := os.Remove(attributes); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if !manager.usesGitCrypt(repository) {
		t.Fatal("unpopulated cache lost git-crypt detection")
	}
}
