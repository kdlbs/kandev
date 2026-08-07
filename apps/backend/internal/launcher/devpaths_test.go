package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeRepoTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"apps/backend", "apps/web"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindRepoRootFromNestedWorkingDirectory(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(filepath.Join(root, "apps", "backend", "internal", "launcher"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootFromRepoRootItself(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootFromAppsDirectory(t *testing.T) {
	root := makeRepoTree(t)

	got, err := findRepoRoot(filepath.Join(root, "apps"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("findRepoRoot = %q, want %q", got, root)
	}
}

func TestFindRepoRootRequiresBothApps(t *testing.T) {
	onlyBackend := t.TempDir()
	if err := os.MkdirAll(filepath.Join(onlyBackend, "apps", "backend"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := findRepoRoot(onlyBackend); err == nil {
		t.Fatal("findRepoRoot accepted a tree with only apps/backend")
	}
}

func TestFindRepoRootFailsOutsideRepository(t *testing.T) {
	if _, err := findRepoRoot(t.TempDir()); err == nil {
		t.Fatal("findRepoRoot accepted an empty tree")
	}
}

func TestIsInsideKandevTaskSignals(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_TASK_ID", "")

	repoInTasks := filepath.Join(home, ".kandev", "tasks", "ws", "repo")
	if !isInsideKandevTask(repoInTasks) {
		t.Fatalf("isInsideKandevTask(%q) = false, want true for a repo under ~/.kandev/tasks", repoInTasks)
	}

	if isInsideKandevTask(filepath.Join(home, "projects", "repo")) {
		t.Fatal("isInsideKandevTask = true for a repo outside the tasks dir")
	}

	tasksDir := filepath.Join(home, ".kandev", "tasks")
	if isInsideKandevTask(tasksDir) {
		t.Fatal("isInsideKandevTask = true for the tasks dir itself (no child segment)")
	}

	t.Setenv("KANDEV_TASK_ID", "task-123")
	if !isInsideKandevTask(filepath.Join(home, "projects", "repo")) {
		t.Fatal("isInsideKandevTask = false with KANDEV_TASK_ID set")
	}
}

func devEnvToMap(extra []string) map[string]string {
	out := map[string]string{}
	for _, item := range extra {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func TestResolveDevBackendEnvDefaultUsesRepoLocalHome(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "")

	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)
	wantHome := filepath.Join(repo, ".kandev-dev")
	wantDB := filepath.Join(wantHome, "data", "kandev.db")

	if dbPath != wantDB {
		t.Fatalf("dbPath = %q, want %q", dbPath, wantDB)
	}
	if env["KANDEV_HOME_DIR"] != wantHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want %q", env["KANDEV_HOME_DIR"], wantHome)
	}
	if env["KANDEV_DATABASE_PATH"] != "" {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want empty (derive from home)", env["KANDEV_DATABASE_PATH"])
	}
	if env["KANDEV_DEBUG_DEV_MODE"] != "true" {
		t.Fatalf("KANDEV_DEBUG_DEV_MODE = %q, want true", env["KANDEV_DEBUG_DEV_MODE"])
	}
}

func TestResolveDevBackendEnvHonorsExplicitDatabasePath(t *testing.T) {
	repo := makeRepoTree(t)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "/custom/kandev.db")

	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)

	if dbPath != "/custom/kandev.db" {
		t.Fatalf("dbPath = %q, want the explicit override", dbPath)
	}
	if env["KANDEV_DATABASE_PATH"] != "/custom/kandev.db" {
		t.Fatalf("KANDEV_DATABASE_PATH = %q", env["KANDEV_DATABASE_PATH"])
	}
	if _, ok := env["KANDEV_HOME_DIR"]; ok {
		t.Fatal("explicit override must not set KANDEV_HOME_DIR")
	}
	if env["KANDEV_DEBUG_DEV_MODE"] != "true" {
		t.Fatalf("KANDEV_DEBUG_DEV_MODE = %q, want true", env["KANDEV_DEBUG_DEV_MODE"])
	}
}

func TestResolveDevBackendEnvClearsLeakedPathInTaskWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_TASK_ID", "")
	t.Setenv("KANDEV_DATABASE_PATH", "/leaked/from/parent/kandev.db")

	repo := filepath.Join(home, ".kandev", "tasks", "ws", "repo")
	dbPath, extra := resolveDevBackendEnv(repo)
	env := devEnvToMap(extra)
	wantHome := filepath.Join(repo, ".kandev-dev")

	if dbPath != filepath.Join(wantHome, "data", "kandev.db") {
		t.Fatalf("dbPath = %q, want the repo-local dev db", dbPath)
	}
	if env["KANDEV_HOME_DIR"] != wantHome {
		t.Fatalf("KANDEV_HOME_DIR = %q, want %q", env["KANDEV_HOME_DIR"], wantHome)
	}
	if env["KANDEV_DATABASE_PATH"] != "" {
		t.Fatalf("KANDEV_DATABASE_PATH = %q, want empty (parent-leaked path cleared)", env["KANDEV_DATABASE_PATH"])
	}
}
