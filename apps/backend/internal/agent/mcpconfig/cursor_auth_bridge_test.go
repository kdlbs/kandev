package mcpconfig

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDeriveCursorProjectSlug(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "Cursor task dot-folder path", path: "/Users/cfl12/.kandev/tasks/hello_hj2srhm6/master", want: "Users-cfl12-kandev-tasks-hello-hj2srhm6-master"},
		{name: "windows drive path", path: `C:\Users\Alice\my_repo.v2`, want: "C-Users-Alice-my-repo-v2"},
		{name: "windows network path", path: `\\server\share\project`, want: "server-share-project"},
		{name: "punctuation runs collapse", path: "---/work...__///a!!b---", want: "work-a-b"},
		{name: "trim boundary dashes", path: "./work/", want: "work"},
		{name: "non-ASCII characters", path: "/work/café/项目/naïve", want: "work-caf-na-ve"},
		{name: "empty result", path: "/...___", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveCursorProjectSlug(tt.path); got != tt.want {
				t.Fatalf("DeriveCursorProjectSlug(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCursorMCPAuthWorktree(t *testing.T) {
	requireSymlinkSupport(t)
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	worktree := filepath.Join(root, ".kandev", "tasks", "hello_hj2srhm6", "master")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init")
	runGit(t, repository, "config", "user.name", "Kandev Tests")
	runGit(t, repository, "config", "user.email", "tests@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")
	runGit(t, repository, "commit", "-m", "fixture")
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "worktree", "add", "-b", "feature", worktree)

	cursorHome := t.TempDir()
	source := filepath.Join(cursorHome, "projects", "other-project", cursorMCPAuthFilename)
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceBefore := []byte(`{"figma":{"token":"opaque-test-token"}}`)
	if err := os.WriteFile(source, sourceBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LinkCursorMCPAuth(worktree, cursorHome); err != nil {
		t.Fatalf("LinkCursorMCPAuth(worktree): %v", err)
	}

	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	wantProjectDirectory := filepath.Join(cursorHome, "projects", cursorProjectSlugAsSpecifiedForTest(canonicalRoot)+"-kandev-tasks-hello-hj2srhm6-master")
	destination := filepath.Join(wantProjectDirectory, cursorMCPAuthFilename)
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", destination, err)
	}
	wantTarget := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	if target != wantTarget {
		t.Fatalf("worktree link target = %q, want %q", target, wantTarget)
	}
	masterData, err := os.ReadFile(wantTarget)
	if err != nil {
		t.Fatalf("read unified auth file: %v", err)
	}
	if string(masterData) != "{\n  \"figma\": {\n    \"token\": \"opaque-test-token\"\n  }\n}\n" {
		t.Fatalf("unexpected unified auth JSON: %s", masterData)
	}
	sourceAfter, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source auth file: %v", err)
	}
	if !reflect.DeepEqual(sourceAfter, sourceBefore) {
		t.Fatalf("source auth file changed: got %s, want %s", sourceAfter, sourceBefore)
	}
}

func runGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	command := exec.Command("git", commandArgs...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func requireSymlinkSupport(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
}

func TestAggregateCursorMCPAuth(t *testing.T) {
	requireSymlinkSupport(t)
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeCursorAuth(t, projects, "older", `{"shared":{"account":"old"},"older-only":{"scope":"read"}}`, baseTime)
	writeCursorAuth(t, projects, "newer", `{"shared":{"account":"new","unknown":{"kept":true}},"newer-only":{"opaque":[1,2]}}`, baseTime.Add(time.Minute))
	writeCursorAuth(t, projects, "a-tie", `{"tie":{"winner":"a"}}`, baseTime.Add(2*time.Minute))
	writeCursorAuth(t, projects, "z-tie", `{"tie":{"winner":"z"}}`, baseTime.Add(2*time.Minute))
	writeCursorAuth(t, projects, "kandev-tasks-17", `{"task-secret":{"token":"excluded"}}`, baseTime.Add(3*time.Minute))
	writeCursorAuth(t, projects, "invalid", `{"valid":{"ok":true},"invalid":"not-an-object"}`, baseTime.Add(4*time.Minute))
	writeCursorAuth(t, projects, "malformed", `{"broken":`, baseTime.Add(5*time.Minute))

	externalProjects := t.TempDir()
	if err := os.Mkdir(filepath.Join(externalProjects, "symlink-target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalProjects, "symlink-target", "mcp-auth.json"), []byte(`{"symlinked":{"token":"excluded"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(externalProjects, "symlink-target"), filepath.Join(projects, "symlink-project")); err != nil {
		t.Fatal(err)
	}
	linkedFile := writeCursorAuth(t, projects, "linked-file", `{"linked":{"token":"excluded"}}`, baseTime.Add(6*time.Minute))
	if err := os.Remove(linkedFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(projects, "older", "mcp-auth.json"), linkedFile); err != nil {
		t.Fatal(err)
	}

	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("AggregateCursorMCPAuth: %v", err)
	}
	got := readCursorAuth(t, filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
	want := map[string]map[string]any{
		"shared":     {"account": "new", "unknown": map[string]any{"kept": true}},
		"older-only": {"scope": "read"},
		"newer-only": {"opaque": []any{float64(1), float64(2)}},
		"tie":        {"winner": "a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregated auth = %#v, want %#v", got, want)
	}
	info, err := os.Stat(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("unified file mode = %04o, want 0600", mode)
	}
}

func TestAggregateCursorMCPAuth_NoSourcesAndProtectedMaster(t *testing.T) {
	requireSymlinkSupport(t)
	t.Run("no valid source leaves stale master unchanged", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
		before := []byte(`{"stale":{"token":"do-not-link"}}`)
		if err := os.WriteFile(master, before, 0o600); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "bad", `not-json`, time.Now())
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("AggregateCursorMCPAuth: %v", err)
		}
		after, err := os.ReadFile(master)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("stale master changed: %s", after)
		}
	})

	t.Run("symlinked projects root is rejected", func(t *testing.T) {
		cursorHome := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(cursorHome, "projects")); err != nil {
			t.Fatal(err)
		}
		if err := AggregateCursorMCPAuth(cursorHome); err == nil {
			t.Fatal("expected symlinked projects root to be rejected")
		}
	})

	for _, kind := range []string{"symlink", "directory"} {
		t.Run("protected master "+kind, func(t *testing.T) {
			cursorHome := t.TempDir()
			projects := filepath.Join(cursorHome, "projects")
			if err := os.MkdirAll(projects, 0o755); err != nil {
				t.Fatal(err)
			}
			writeCursorAuth(t, projects, "source", `{"server":{"token":"x"}}`, time.Now())
			master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
			if kind == "symlink" {
				target := filepath.Join(cursorHome, "old-master")
				if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, master); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(master, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := AggregateCursorMCPAuth(cursorHome); err == nil {
				t.Fatal("expected protected master to be rejected")
			}
		})
	}
}

func TestAggregateCursorMCPAuthExcludesConfiguredTaskRoot(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}

	taskRoot := filepath.Join(t.TempDir(), "custom-task-storage")
	taskWorkspace := filepath.Join(taskRoot, "task-123", "repository")
	if err := os.MkdirAll(taskWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	absoluteTaskWorkspace, err := filepath.Abs(taskWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	canonicalTaskWorkspace, err := filepath.EvalSymlinks(absoluteTaskWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	writeCursorAuth(t, projects, cursorProjectSlugAsSpecifiedForTest(canonicalTaskWorkspace), `{"task-secret":{"token":"excluded"}}`, time.Now())
	writeCursorAuth(t, projects, "ordinary-project", `{"ordinary":{"token":"included"}}`, time.Now())

	if err := AggregateCursorMCPAuth(cursorHome, taskRoot); err != nil {
		t.Fatalf("AggregateCursorMCPAuth: %v", err)
	}
	got := readCursorAuth(t, filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
	if _, exists := got["task-secret"]; exists {
		t.Fatalf("configured task auth was aggregated: %#v", got)
	}
	if _, exists := got["ordinary"]; !exists {
		t.Fatalf("ordinary project auth was excluded: %#v", got)
	}
}

func TestAggregateCursorMCPAuthAllowsMissingExcludedRoot(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	writeCursorAuth(t, projects, "ordinary-project", `{"ordinary":{"token":"included"}}`, time.Now())
	missingTaskRoot := filepath.Join(t.TempDir(), "not-created-yet")
	taskWorkspace := filepath.Join(missingTaskRoot, "task-123", "repository")
	absoluteExistingParent, err := filepath.Abs(filepath.Dir(missingTaskRoot))
	if err != nil {
		t.Fatal(err)
	}
	canonicalExistingParent, err := filepath.EvalSymlinks(absoluteExistingParent)
	if err != nil {
		t.Fatal(err)
	}
	relativeTaskWorkspace, err := filepath.Rel(filepath.Dir(missingTaskRoot), taskWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	canonicalTaskWorkspace := filepath.Join(canonicalExistingParent, relativeTaskWorkspace)
	writeCursorAuth(t, projects, cursorProjectSlugAsSpecifiedForTest(canonicalTaskWorkspace), `{"stale-task":{"token":"excluded"}}`, time.Now())

	if err := AggregateCursorMCPAuth(cursorHome, missingTaskRoot); err != nil {
		t.Fatalf("AggregateCursorMCPAuth with an absent task root: %v", err)
	}
	got := readCursorAuth(t, filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
	if _, exists := got["stale-task"]; exists {
		t.Fatalf("stale task auth under the absent root was aggregated: %#v", got)
	}
	if _, exists := got["ordinary"]; !exists {
		t.Fatalf("ordinary auth was not published: %#v", got)
	}
}

func TestLinkCursorMCPAuthSharesCredentialByNameWithoutOriginBinding(t *testing.T) {
	requireSymlinkSupport(t)
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	writeCursorAuth(t, projects, "trusted-source", `{"calendar":{"access_token":"synthetic-token"}}`, time.Now())

	workspace := t.TempDir()
	projectConfig := filepath.Join(workspace, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(projectConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectConfig, []byte(`{"mcpServers":{"calendar":{"url":"https://untrusted.invalid"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
		t.Fatalf("LinkCursorMCPAuth: %v", err)
	}
	got := readCursorAuth(t, cursorAuthDestinationForTest(t, cursorHome, workspace))
	if got["calendar"]["access_token"] != "synthetic-token" {
		t.Fatalf("same-name auth was not copied across project origins: %#v", got)
	}
}

func TestLinkCursorMCPAuth(t *testing.T) {
	requireSymlinkSupport(t)
	t.Run("workspace under symlinked parent uses canonical project slug", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		writeCursorAuth(t, projects, "source", `{"server":{"token":"secret"}}`, time.Now())

		realParent := t.TempDir()
		aliasParent := filepath.Join(t.TempDir(), "parent-alias")
		if err := os.Symlink(realParent, aliasParent); err != nil {
			t.Fatal(err)
		}
		workspace := filepath.Join(aliasParent, "worktree")
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatal(err)
		}
		workspaceAbs, err := filepath.Abs(workspace)
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := filepath.EvalSymlinks(workspaceAbs)
		if err != nil {
			t.Fatal(err)
		}
		if DeriveCursorProjectSlug(workspaceAbs) == DeriveCursorProjectSlug(canonical) {
			t.Fatalf("test paths produced identical slugs: %q", canonical)
		}

		if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if _, err := os.Readlink(destination); err != nil {
			t.Fatalf("canonical destination is not linked: %v", err)
		}
		rawDestination := filepath.Join(projects, DeriveCursorProjectSlug(workspaceAbs), cursorMCPAuthFilename)
		if _, err := os.Lstat(rawDestination); !os.IsNotExist(err) {
			t.Fatalf("non-canonical destination unexpectedly exists: %v", err)
		}
	})

	t.Run("canonical workspace gets an absolute symlink", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "source", `{"server":{"opaque":{"token":"secret"}}}`, time.Now())
		workspace := t.TempDir()
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(workspace, alias); err != nil {
			t.Fatal(err)
		}
		projectPath := filepath.Dir(cursorAuthDestinationForTest(t, cursorHome, workspace))
		if err := os.Mkdir(projectPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := LinkCursorMCPAuth(alias, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		canonical, err := filepath.EvalSymlinks(workspace)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(projects, DeriveCursorProjectSlug(canonical), "mcp-auth.json")
		got, err := os.Readlink(destination)
		if err != nil {
			t.Fatal(err)
		}
		master, err := filepath.Abs(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename))
		if err != nil {
			t.Fatal(err)
		}
		if got != master {
			t.Fatalf("link target = %q, want %q", got, master)
		}
		info, err := os.Stat(filepath.Dir(destination))
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o700 {
			t.Fatalf("existing project directory mode = %04o, want unchanged 0700", mode)
		}
	})

	t.Run("protected regular destination is unchanged", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "source", `{"server":{"token":"new"}}`, time.Now())
		workspace := t.TempDir()
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		before := []byte("user-owned-auth-bytes\n")
		if err := os.WriteFile(destination, before, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		after, err := os.ReadFile(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("regular destination changed: %q", after)
		}
	})

	t.Run("protected directory destination is unchanged", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "source", `{"server":{"token":"new"}}`, time.Now())
		workspace := t.TempDir()
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if err := os.MkdirAll(destination, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		info, err := os.Lstat(destination)
		if err != nil || !info.IsDir() {
			t.Fatalf("protected destination changed: info=%v err=%v", info, err)
		}
	})

	t.Run("no valid source does not link stale master", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename), []byte(`{"stale":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "invalid", `{"server":false}`, time.Now())
		workspace := t.TempDir()
		if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if _, err := os.Lstat(destination); !os.IsNotExist(err) {
			t.Fatalf("destination should remain absent, stat error = %v", err)
		}
	})

	t.Run("existing bridge link remains when refresh has no valid source", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
		if err := os.WriteFile(master, []byte(`{"stale":{"token":"synthetic"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		writeCursorAuth(t, projects, "invalid", `not-json`, time.Now())
		workspace := t.TempDir()
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(master, destination); err != nil {
			t.Fatal(err)
		}

		if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatalf("LinkCursorMCPAuth: %v", err)
		}
		got, err := os.Readlink(destination)
		if err != nil {
			t.Fatalf("existing bridge link was removed: %v", err)
		}
		if got != master {
			t.Fatalf("existing link target = %q, want %q", got, master)
		}
	})
}

func cursorAuthDestinationForTest(t *testing.T, cursorHome, workspacePath string) string {
	t.Helper()
	absolutePath, err := filepath.Abs(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cursorHome, "projects", DeriveCursorProjectSlug(canonicalPath), cursorMCPAuthFilename)
}

func cursorProjectSlugAsSpecifiedForTest(path string) string {
	var slug strings.Builder
	previousWasSeparator := true
	for _, char := range path {
		if !isASCIIAlphaNumeric(char) {
			previousWasSeparator = true
			continue
		}
		if slug.Len() > 0 && previousWasSeparator {
			slug.WriteByte('-')
		}
		slug.WriteRune(char)
		previousWasSeparator = false
	}
	return slug.String()
}

func isASCIIAlphaNumeric(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func TestDisableCursorMCPAuth(t *testing.T) {
	requireSymlinkSupport(t)
	for _, targetKind := range []string{"bridge absolute", "bridge relative", "unrelated"} {
		t.Run(targetKind, func(t *testing.T) {
			cursorHome := t.TempDir()
			workspace := t.TempDir()
			destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				t.Fatal(err)
			}
			master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
			if err := os.WriteFile(master, []byte(`{"kept":{}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			target := master
			remove := true
			switch targetKind {
			case "bridge relative":
				target, _ = filepath.Rel(filepath.Dir(destination), master)
			case "unrelated":
				target = filepath.Join(cursorHome, "other.json")
				if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
					t.Fatal(err)
				}
				remove = false
			}
			if err := os.Symlink(target, destination); err != nil {
				t.Fatal(err)
			}
			if err := disableCursorMCPAuth(workspace, cursorHome); err != nil {
				t.Fatalf("disableCursorMCPAuth: %v", err)
			}
			_, err := os.Lstat(destination)
			if remove && !os.IsNotExist(err) {
				t.Fatalf("matching link remains or stat failed: %v", err)
			}
			if !remove && err != nil {
				t.Fatalf("unrelated link was removed: %v", err)
			}
			if _, err := os.Stat(master); err != nil {
				t.Fatalf("master was removed: %v", err)
			}
		})
	}

	t.Run("regular file is preserved", func(t *testing.T) {
		cursorHome := t.TempDir()
		workspace := t.TempDir()
		destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		before := []byte("keep this regular file")
		if err := os.WriteFile(destination, before, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := disableCursorMCPAuth(workspace, cursorHome); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("regular file changed: %q", after)
		}
	})
}

func TestCursorMCPAuthConcurrentPreparation(t *testing.T) {
	requireSymlinkSupport(t)
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCursorAuth(t, projects, "source", `{"server":{"opaque":{"token":"value"}}}`, time.Now())
	workspace := t.TempDir()
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- LinkCursorMCPAuth(workspace, cursorHome)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent link: %v", err)
		}
	}
	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	if got := readCursorAuth(t, master); !reflect.DeepEqual(got, map[string]map[string]any{"server": {"opaque": map[string]any{"token": "value"}}}) {
		t.Fatalf("concurrent snapshot = %#v", got)
	}
	destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
	if target, err := os.Readlink(destination); err != nil || !strings.HasSuffix(target, cursorMCPAuthUnifiedFilename) {
		t.Fatalf("concurrent destination target = %q, err = %v", target, err)
	}
}

func writeCursorAuth(t *testing.T, projects, name, content string, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(projects, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mcp-auth.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCursorAuth(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid auth JSON: %v", err)
	}
	return got
}
