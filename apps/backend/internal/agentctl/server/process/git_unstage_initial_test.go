package process

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
func TestGitOperatorUnstageBeforeFirstCommit(t *testing.T) {
	for _, selection := range []struct {
		name  string
		paths []string
	}{
		{"all-nil", nil},
		{"all-empty", []string{}},
		{"selected", []string{"new[ab].txt"}},
		{"invalid-empty", []string{""}},
	} {
		t.Run(selection.name, func(t *testing.T) {
			dir := initUnstageOperatorRepo(t)
			files := []string{"new[ab].txt", "newa.txt", "nested/add.txt"}
			writeUnstageOperatorFiles(t, dir, files)
			op := NewGitOperator(dir, newTestLogger(t), nil)
			staged, stageErr := op.Stage(context.Background(), nil)
			requireUnstageOperatorResult(t, staged, stageErr)
			runGitExpectFailure(t, dir, "rev-parse", "--verify", "HEAD")
			writeFile(t, dir, files[0], "later working edit\x00\n")
			before := captureUnstageOperatorRepo(t, dir, files)
			if before.index == "" || before.refs != "" {
				t.Fatalf("fixture must have staged files without refs: %+v", before)
			}
			result, err := op.Unstage(context.Background(), selection.paths)
			if err != nil || result == nil {
				t.Fatalf("Unstage returned result=%+v err=%v", result, err)
			}
			if selection.name == "invalid-empty" {
				if result.Success || result.Error == "" {
					t.Errorf("invalid selection must fail: %+v", result)
				}
			} else if !result.Success {
				t.Errorf("Unstage must succeed: %+v", result)
			}
			after := captureUnstageOperatorRepo(t, dir, files)
			checkUnstageOperatorPreservation(t, before, after)
			checkInitialUnstageIndex(t, dir, selection.name, before.index, after.index)
			runGitExpectFailure(t, dir, "rev-parse", "--verify", "HEAD")
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
func TestGitOperatorUnstageAllCommitted(t *testing.T) {
	dir := initUnstageOperatorRepo(t)
	files := []string{"tracked.txt", "removed.txt"}
	writeUnstageOperatorFiles(t, dir, files)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "committed unstage fixture")
	runGit(t, dir, "tag", "preserved")
	head := runGit(t, dir, "rev-parse", "HEAD")
	baseIndex := runGit(t, dir, "ls-files", "--stage", "-z")
	writeFile(t, dir, files[0], "staged tracked version\n")
	writeFile(t, dir, "added.txt", "staged added version\n")
	if err := os.Remove(filepath.Join(dir, files[1])); err != nil {
		t.Fatal(err)
	}
	op := NewGitOperator(dir, newTestLogger(t), nil)
	staged, stageErr := op.Stage(context.Background(), nil)
	requireUnstageOperatorResult(t, staged, stageErr)
	writeFile(t, dir, files[0], "later tracked work\x00\n")
	writeFile(t, dir, "added.txt", "later added work\n")
	before := captureUnstageOperatorRepo(t, dir, append(files, "added.txt"))
	if before.index == baseIndex {
		t.Fatal("fixture must have staged changes")
	}
	result, err := op.Unstage(context.Background(), nil)
	requireUnstageOperatorResult(t, result, err)
	after := captureUnstageOperatorRepo(t, dir, append(files, "added.txt"))
	checkUnstageOperatorPreservation(t, before, after)
	if after.index != baseIndex {
		t.Errorf("index=%q, want committed index %q", after.index, baseIndex)
	}
	if got := runGit(t, dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD changed: %q, want %q", got, head)
	}
	if got := runGit(t, dir, "rev-parse", "ORIG_HEAD"); got != head {
		t.Errorf("whole reset bookkeeping=%q, want %q", got, head)
	}
}

func initUnstageOperatorRepo(t *testing.T) string {
	t.Helper()
	isolateTestGitEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.name", "Unstage Test")
	runGit(t, dir, "config", "user.email", "unstage@example.test")
	runGit(t, dir, "config", "core.autocrlf", "false")
	runGit(t, dir, "config", "core.hooksPath", filepath.Join(t.TempDir(), "no-hooks"))
	return dir
}

func writeUnstageOperatorFiles(t *testing.T, dir string, files []string) {
	t.Helper()
	for _, path := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, path, "distinct staged bytes: "+path+"\n")
	}
}

type unstageWorkingFile struct {
	bytes  string
	mode   os.FileMode
	exists bool
}

type unstageOperatorSnapshot struct {
	index, head, config, refs string
	files                     map[string]unstageWorkingFile
	environment               []string
}

func captureUnstageOperatorRepo(t *testing.T, dir string, files []string) unstageOperatorSnapshot {
	t.Helper()
	snapshot := unstageOperatorSnapshot{
		index:       runGit(t, dir, "ls-files", "--stage", "-z"),
		refs:        runGit(t, dir, "for-each-ref"),
		files:       make(map[string]unstageWorkingFile, len(files)),
		environment: os.Environ(),
	}
	for _, item := range []struct {
		name  string
		value *string
	}{
		{"HEAD", &snapshot.head}, {"config", &snapshot.config},
	} {
		data, err := os.ReadFile(filepath.Join(dir, ".git", item.name))
		if err != nil {
			t.Fatal(err)
		}
		*item.value = string(data)
	}
	for _, path := range files {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if os.IsNotExist(err) {
			snapshot.files[path] = unstageWorkingFile{}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		snapshot.files[path] = unstageWorkingFile{string(data), info.Mode().Perm(), true}
	}
	return snapshot
}

func checkUnstageOperatorPreservation(t *testing.T, before, after unstageOperatorSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(after.files, before.files) {
		t.Errorf("working bytes/permissions changed: got %+v want %+v", after.files, before.files)
	}
	if after.head != before.head || after.config != before.config || after.refs != before.refs {
		t.Errorf("HEAD/config/refs changed: got %+v want %+v", after, before)
	}
	if !reflect.DeepEqual(after.environment, before.environment) {
		t.Error("operation changed process environment")
	}
}

func checkInitialUnstageIndex(t *testing.T, dir, selection, before, after string) {
	t.Helper()
	switch selection {
	case "invalid-empty":
		if after != before {
			t.Errorf("invalid selection changed index: got %q want %q", after, before)
		}
	case "selected":
		if got := runGit(t, dir, "ls-files", "-z"); got != "nested/add.txt\x00newa.txt\x00" {
			t.Errorf("selected index membership=%q", got)
		}
		for _, sibling := range []string{"nested/add.txt", "newa.txt"} {
			if got := runGit(t, dir, "show", ":"+sibling); got != "distinct staged bytes: "+sibling+"\n" {
				t.Errorf("sibling %q index bytes=%q", sibling, got)
			}
		}
	default:
		if after != "" {
			t.Errorf("initial Unstage all left index entries: %q", after)
		}
	}
}

func requireUnstageOperatorResult(t *testing.T, result *GitOperationResult, err error) {
	t.Helper()
	if err != nil || result == nil || !result.Success {
		t.Fatalf("operation result=%+v err=%v", result, err)
	}
}
