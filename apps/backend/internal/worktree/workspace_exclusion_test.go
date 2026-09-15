package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNestedWorkspaceExclusionIsPrivateAndIdempotent(t *testing.T) {
	repositoryPath := t.TempDir()
	command := exec.Command("git", "init", repositoryPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	nestedPath := filepath.Join(repositoryPath, "kandev", "payments")
	changed, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath)
	if err != nil {
		t.Fatalf("addNestedWorkspaceExclusion: %v", err)
	}
	if !changed {
		t.Fatal("addNestedWorkspaceExclusion changed = false, want true")
	}
	changed, err = mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath)
	if err != nil {
		t.Fatalf("repeat addNestedWorkspaceExclusion: %v", err)
	}
	if changed {
		t.Fatal("repeat addNestedWorkspaceExclusion changed = true, want false")
	}

	target, err := mgr.nestedWorkspaceExcludeTarget(nestedPath)
	if err != nil {
		t.Fatalf("nestedWorkspaceExcludeTarget: %v", err)
	}
	content, err := os.ReadFile(target.excludePath)
	if err != nil {
		t.Fatalf("read managed exclude: %v", err)
	}
	pattern := "/kandev/payments/"
	marker := managedWorkspaceExclusionPrefix + pattern
	if count := strings.Count(string(content), marker); count != 1 {
		t.Fatalf("managed exclusion marker count = %d, want 1", count)
	}
	if !strings.Contains(string(content), marker+"\n"+pattern+"\n") {
		t.Fatalf("exclude file does not contain managed pattern:\n%s", content)
	}
	if _, err := os.Stat(target.includePath); err != nil {
		t.Fatalf("managed include file: %v", err)
	}
	commonExclude, err := os.ReadFile(filepath.Join(repositoryPath, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read common info/exclude: %v", err)
	}
	if strings.Contains(string(commonExclude), marker) {
		t.Fatal("nested exclusion leaked into the shared info/exclude file")
	}
	check := exec.Command("git", "-C", repositoryPath, "check-ignore", "--no-index", "kandev/payments/file.txt")
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("git check-ignore did not use the worktree-scoped exclusion: %v\n%s", err, output)
	}
	if err := os.MkdirAll(nestedPath, 0o755); err != nil {
		t.Fatalf("mkdir nested repository: %v", err)
	}
	runGit(t, nestedPath, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(nestedPath, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write nested repository file: %v", err)
	}
	runGit(t, repositoryPath, "add", "-A")
	if staged := runGit(t, repositoryPath, "ls-files", "--stage", "--", "kandev/payments"); strings.TrimSpace(staged) != "" {
		t.Fatalf("ordinary outer staging captured attached repository: %q", staged)
	}

	if err := mgr.removeNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); err != nil {
		t.Fatalf("removeNestedWorkspaceExclusion: %v", err)
	}
	if _, err := os.Stat(target.includePath); !os.IsNotExist(err) {
		t.Fatalf("managed include file remains after removal: %v", err)
	}
	if _, err := os.Stat(target.excludePath); !os.IsNotExist(err) {
		t.Fatalf("managed exclude file remains after removal: %v", err)
	}
}

func TestNestedWorkspaceExclusionIsScopedToContainingLinkedWorktree(t *testing.T) {
	repositoryPath := t.TempDir()
	runGit(t, repositoryPath, "init", "-b", "main")
	runGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runGit(t, repositoryPath, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repositoryPath, "add", "README.md")
	runGit(t, repositoryPath, "commit", "-m", "initial")
	configuredExcludes := filepath.Join(repositoryPath, ".git", "existing.exclude")
	if err := os.WriteFile(configuredExcludes, []byte("existing/\n"), 0o644); err != nil {
		t.Fatalf("write configured excludes: %v", err)
	}
	runGit(t, repositoryPath, "config", "core.excludesFile", configuredExcludes)

	linkedPath := filepath.Join(t.TempDir(), "linked")
	runGit(t, repositoryPath, "worktree", "add", "-b", "linked", linkedPath, "HEAD")

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	nestedPath := filepath.Join(linkedPath, "kandev", "payments")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); err != nil {
		t.Fatalf("addNestedWorkspaceExclusion: %v", err)
	}

	target, err := mgr.nestedWorkspaceExcludeTarget(nestedPath)
	if err != nil {
		t.Fatalf("nestedWorkspaceExcludeTarget: %v", err)
	}
	content, err := os.ReadFile(target.excludePath)
	if err != nil {
		t.Fatalf("read managed exclude: %v", err)
	}
	if !strings.Contains(string(content), "existing/\n") {
		t.Fatalf("configured exclusion was not preserved:\n%s", content)
	}
	checkLinked := exec.Command("git", "-C", linkedPath, "check-ignore", "--no-index", "kandev/payments/file.txt")
	if output, err := checkLinked.CombinedOutput(); err != nil {
		t.Fatalf("linked worktree did not use its private exclusion: %v\n%s", err, output)
	}
	checkSource := exec.Command("git", "-C", repositoryPath, "check-ignore", "--no-index", "kandev/payments/file.txt")
	if output, err := checkSource.CombinedOutput(); err == nil {
		t.Fatalf("source worktree unexpectedly used linked exclusion:\n%s", output)
	}

	if err := mgr.removeNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); err != nil {
		t.Fatalf("removeNestedWorkspaceExclusion: %v", err)
	}
	if _, err := os.Stat(target.includePath); !os.IsNotExist(err) {
		t.Fatalf("managed include file remains after removal: %v", err)
	}
	if _, err := os.Stat(target.excludePath); !os.IsNotExist(err) {
		t.Fatalf("managed exclude file remains after removal: %v", err)
	}
}

func TestNestedWorkspaceExclusionPreservesImplicitGlobalIgnore(t *testing.T) {
	configHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configHome, "git"), 0o755); err != nil {
		t.Fatalf("mkdir XDG git config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "git", "ignore"), []byte("global-only/\n"), 0o644); err != nil {
		t.Fatalf("write implicit global ignore: %v", err)
	}
	globalConfig := filepath.Join(t.TempDir(), "global-config")
	if err := os.WriteFile(globalConfig, nil, 0o644); err != nil {
		t.Fatalf("write isolated global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "missing-system-config"))

	repositoryPath := t.TempDir()
	runGit(t, repositoryPath, "init", "-b", "main")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	nestedPath := filepath.Join(repositoryPath, "kandev", "payments")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); err != nil {
		t.Fatalf("addNestedWorkspaceExclusion: %v", err)
	}
	check := exec.Command("git", "-C", repositoryPath, "check-ignore", "--no-index", "global-only/file.txt")
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("implicit global ignore was not preserved: %v\n%s", err, output)
	}
}

func TestNestedWorkspaceExclusionRefreshesImplicitGlobalIgnoreAndRecreatesMissingFile(t *testing.T) {
	configHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configHome, "git"), 0o755); err != nil {
		t.Fatalf("mkdir XDG git config: %v", err)
	}
	globalIgnore := filepath.Join(configHome, "git", "ignore")
	if err := os.WriteFile(globalIgnore, []byte("global-old/\n"), 0o644); err != nil {
		t.Fatalf("write initial implicit global ignore: %v", err)
	}
	globalConfig := filepath.Join(t.TempDir(), "global-config")
	if err := os.WriteFile(globalConfig, nil, 0o644); err != nil {
		t.Fatalf("write isolated global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "missing-system-config"))

	repositoryPath := t.TempDir()
	runGit(t, repositoryPath, "init", "-b", "main")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	firstNestedPath := filepath.Join(repositoryPath, "kandev", "payments")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, firstNestedPath); err != nil {
		t.Fatalf("initial addNestedWorkspaceExclusion: %v", err)
	}
	target, err := mgr.nestedWorkspaceExcludeTarget(firstNestedPath)
	if err != nil {
		t.Fatalf("nestedWorkspaceExcludeTarget: %v", err)
	}
	if err := os.Remove(target.excludePath); err != nil {
		t.Fatalf("remove managed exclude to simulate stale include: %v", err)
	}
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, firstNestedPath); err != nil {
		t.Fatalf("recreate managed exclusion: %v", err)
	}
	if _, err := os.Stat(target.excludePath); err != nil {
		t.Fatalf("managed exclude was not recreated: %v", err)
	}

	if err := os.WriteFile(globalIgnore, []byte("global-old/\nglobal-new/\n"), 0o644); err != nil {
		t.Fatalf("write updated implicit global ignore: %v", err)
	}
	secondNestedPath := filepath.Join(repositoryPath, "kandev", "billing")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, secondNestedPath); err != nil {
		t.Fatalf("refreshing addNestedWorkspaceExclusion: %v", err)
	}
	content, err := os.ReadFile(target.excludePath)
	if err != nil {
		t.Fatalf("read refreshed managed exclude: %v", err)
	}
	if !strings.Contains(string(content), "global-new/\n") {
		t.Fatalf("updated implicit global ignore was not copied into managed excludes:\n%s", content)
	}
	for _, pattern := range []string{"/kandev/payments/", "/kandev/billing/"} {
		marker := managedWorkspaceExclusionPrefix + pattern
		if count := strings.Count(string(content), marker); count != 1 {
			t.Fatalf("managed exclusion marker %q count = %d, want 1", pattern, count)
		}
	}
	check := exec.Command("git", "-C", repositoryPath, "check-ignore", "--no-index", "global-new/file.txt")
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("refreshed implicit global ignore was not effective: %v\n%s", err, output)
	}
}

func TestNestedWorkspaceExclusionRejectsTrackedNegation(t *testing.T) {
	repositoryPath := t.TempDir()
	runGit(t, repositoryPath, "init", "-b", "main")
	runGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runGit(t, repositoryPath, "config", "user.name", "Test User")
	if err := os.MkdirAll(filepath.Join(repositoryPath, "kandev", "payments"), 0o755); err != nil {
		t.Fatalf("mkdir tracked destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repositoryPath, ".gitignore"), []byte("!/kandev/payments/\n"), 0o644); err != nil {
		t.Fatalf("write tracked negation: %v", err)
	}
	runGit(t, repositoryPath, "add", ".gitignore")
	runGit(t, repositoryPath, "commit", "-m", "ignore rules")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	nestedPath := filepath.Join(repositoryPath, "kandev", "payments")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); !errors.Is(err, ErrNestedWorkspaceExclusionNotEffective) {
		t.Fatalf("tracked negation error = %v, want ErrNestedWorkspaceExclusionNotEffective", err)
	}
	target, err := mgr.nestedWorkspaceExcludeTarget(nestedPath)
	if err != nil {
		t.Fatalf("nestedWorkspaceExcludeTarget: %v", err)
	}
	if _, err := os.Stat(target.includePath); !os.IsNotExist(err) {
		t.Fatalf("managed include remains after rejected exclusion: %v", err)
	}
	if _, err := os.Stat(target.excludePath); !os.IsNotExist(err) {
		t.Fatalf("managed exclude remains after rejected exclusion: %v", err)
	}
}

func TestNestedWorkspaceExclusionRejectsAlreadyTrackedDestination(t *testing.T) {
	repositoryPath := t.TempDir()
	runGit(t, repositoryPath, "init", "-b", "main")
	runGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runGit(t, repositoryPath, "config", "user.name", "Test User")
	trackedPath := filepath.Join(repositoryPath, "kandev", "payments", "tracked.txt")
	if err := os.MkdirAll(filepath.Dir(trackedPath), 0o755); err != nil {
		t.Fatalf("mkdir tracked destination: %v", err)
	}
	if err := os.WriteFile(trackedPath, []byte("tracked\n"), 0o644); err != nil {
		t.Fatalf("write tracked destination: %v", err)
	}
	runGit(t, repositoryPath, "add", ".")
	runGit(t, repositoryPath, "commit", "-m", "tracked destination")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	nestedPath := filepath.Join(repositoryPath, "kandev", "payments")
	if _, err := mgr.addNestedWorkspaceExclusion(context.Background(), repositoryPath, nestedPath); !errors.Is(err, ErrNestedWorkspaceExclusionNotEffective) {
		t.Fatalf("tracked destination error = %v, want ErrNestedWorkspaceExclusionNotEffective", err)
	}
}

func TestPrepareTaskRelativeWorktreePathRejectsOccupiedTarget(t *testing.T) {
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	root, err := mgr.TaskRoot("task-occupied")
	if err != nil {
		t.Fatalf("TaskRoot: %v", err)
	}
	target := filepath.Join(root, "repo", "payments")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	_, err = mgr.prepareTaskRelativeWorktreePath(CreateRequest{TaskID: "task-occupied", TaskDirName: "task-occupied", WorkspaceRelativePath: "repo/payments"})
	if !errors.Is(err, ErrWorkspacePathOccupied) {
		t.Fatalf("prepareTaskRelativeWorktreePath error = %v, want ErrWorkspacePathOccupied", err)
	}
}
