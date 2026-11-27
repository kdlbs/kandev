package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
func TestGitOperatorLiteralSelections(t *testing.T) {
	for _, pair := range literalSelectionNames {
		for _, operation := range []string{"stage", "unstage"} {
			t.Run(operation+"/"+pair[0], func(t *testing.T) {
				skipNativeInvalidLiteralPath(t, pair[0])
				dir := setupLiteralSelectionRepo(t, pair)
				checkLiteralMutation(t, dir, operation, []string{pair[0]}, []string{pair[0]}, []string{pair[1]})
			})
		}
	}
	for _, operation := range []string{"stage", "unstage"} {
		t.Run(operation+"/invalid-empty", func(t *testing.T) {
			checkLiteralEmptyEntry(t, operation)
		})
		for _, selection := range []string{"directory", "multiple", "empty", "deleted", "added", "renamed"} {
			t.Run(operation+"/"+selection, func(t *testing.T) {
				dir, paths, selected, unselected := setupLiteralMutationCase(t, selection)
				checkLiteralMutation(t, dir, operation, paths, selected, unselected)
			})
		}
	}
}

func checkLiteralEmptyEntry(t *testing.T, operation string) {
	t.Helper()
	pair := [2]string{"new[ab].txt", "newa.txt"}
	dir := setupLiteralSelectionRepo(t, pair)
	if operation == "unstage" {
		runGit(t, dir, "add", "-A")
	}
	before := [2]literalFileSnapshot{snapshotLiteralFile(t, dir, pair[0]), snapshotLiteralFile(t, dir, pair[1])}
	op := NewGitOperator(dir, newTestLogger(t), nil)
	var result *GitOperationResult
	var err error
	if operation == "stage" {
		result, err = op.Stage(context.Background(), []string{""})
	} else {
		result, err = op.Unstage(context.Background(), []string{""})
	}
	if err != nil || result.Success || result.Error == "" {
		t.Errorf("empty entry must retain Git rejection: result=%+v err=%v", result, err)
	}
	for i, path := range pair {
		if got := snapshotLiteralFile(t, dir, path); got != before[i] {
			t.Errorf("invalid selection changed %q: %+v, want %+v", path, got, before[i])
		}
	}
}

var literalSelectionNames = [][2]string{
	{"new[ab].txt", "newa.txt"}, {"café[ab].txt", "caféa.txt"},
	{"-new[ab].txt", "-newa.txt"}, {"a*.txt", "alpha.txt"}, {"a?.txt", "ab.txt"},
	{":(glob)a*.txt", "alpha.txt"}, {" raw[ab].txt ", " rawa.txt "},
	{"tab\t[ab].txt", "tab\ta.txt"}, {"line\n[ab].txt", "line\na.txt"},
}

func skipNativeInvalidLiteralPath(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" && (strings.ContainsAny(path, ":*?\"<>|\t\n") || strings.HasSuffix(path, " ")) {
		t.Skip("filename cannot be represented by the native Windows filesystem")
	}
}

func setupLiteralSelectionRepo(t *testing.T, names [2]string) string {
	t.Helper()
	dir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	for i, name := range names {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, name, "base-"+string(rune('0'+i))+"\n")
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "literal selection fixture")
	writeFile(t, dir, names[0], "base-0\nselected-marker\n")
	writeFile(t, dir, names[1], "base-1\nunselected-marker\n")
	return dir
}

func setupLiteralMutationCase(t *testing.T, selection string) (dir string, paths, selected, unselected []string) {
	t.Helper()
	pair := [2]string{"new[ab].txt", "newa.txt"}
	if selection == "directory" {
		pair = [2]string{"dir[ab]/child.txt", "dira/child.txt"}
	}
	dir = setupLiteralSelectionRepo(t, pair)
	paths, selected, unselected = []string{pair[0]}, []string{pair[0]}, []string{pair[1]}
	switch selection {
	case "directory":
		paths = []string{"dir[ab]"}
	case "multiple":
		writeFile(t, dir, "second.txt", "second-selected\n")
		paths = append(paths, "second.txt")
		selected = append(selected, "second.txt")
	case "empty":
		paths, selected, unselected = nil, append(selected, unselected...), nil
		if err := os.Remove(filepath.Join(dir, pair[0])); err != nil {
			t.Fatal(err)
		}
	case "deleted":
		if err := os.Remove(filepath.Join(dir, pair[0])); err != nil {
			t.Fatal(err)
		}
	case "added":
		runGit(t, dir, "rm", "--cached", "--", ":(literal)"+pair[0])
		runGit(t, dir, "commit", "-m", "untrack selected fixture")
	case "renamed":
		if err := os.Rename(filepath.Join(dir, pair[0]), filepath.Join(dir, "renamed[ab].txt")); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, "renamed[ab].txt")
		selected = append(selected, "renamed[ab].txt")
	}
	return dir, paths, selected, unselected
}

type literalFileSnapshot struct {
	index, worktree string
	indexed, exists bool
}

func snapshotLiteralFile(t *testing.T, dir, path string) literalFileSnapshot {
	t.Helper()
	var got literalFileSnapshot
	for _, indexedPath := range strings.Split(runGit(t, dir, "--no-literal-pathspecs", "ls-files", "-z", "--", ":(literal)"+path), "\x00") {
		if indexedPath == path {
			got.indexed = true
		}
	}
	if got.indexed {
		got.index = runGit(t, dir, "show", ":"+path)
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	got.exists, got.worktree = err == nil, string(data)
	return got
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
func TestGitOperatorLiteralPathspecEnvironment(t *testing.T) {
	for _, operation := range []string{"stage", "unstage"} {
		t.Run(operation, func(t *testing.T) {
			pair := [2]string{"new[ab].txt", "newa.txt"}
			dir := setupLiteralSelectionRepo(t, pair)
			t.Setenv("GIT_LITERAL_PATHSPECS", "1")
			checkLiteralMutation(t, dir, operation, []string{pair[0]}, []string{pair[0]}, []string{pair[1]})
			if os.Getenv("GIT_LITERAL_PATHSPECS") != "1" {
				t.Fatal("selected command changed the process environment")
			}
		})
	}
}

func checkLiteralMutation(t *testing.T, dir, operation string, paths, selected, unselected []string) {
	t.Helper()
	all := append(append([]string{}, selected...), unselected...)
	baseline := make(map[string]literalFileSnapshot, len(all))
	for _, path := range all {
		baseline[path] = snapshotLiteralFile(t, dir, path)
	}
	if operation == "unstage" {
		runGit(t, dir, "add", "-A")
	}
	before := make(map[string]literalFileSnapshot, len(all))
	for _, path := range all {
		before[path] = snapshotLiteralFile(t, dir, path)
	}
	op := NewGitOperator(dir, newTestLogger(t), nil)
	var result *GitOperationResult
	var err error
	if operation == "stage" {
		result, err = op.Stage(context.Background(), paths)
	} else {
		result, err = op.Unstage(context.Background(), paths)
	}
	if err != nil || !result.Success {
		t.Fatalf("%s result=%+v err=%v", operation, result, err)
	}
	for _, path := range selected {
		want := before[path]
		if operation == "stage" {
			want.indexed, want.index = want.exists, want.worktree
		} else {
			want.indexed, want.index = baseline[path].indexed, baseline[path].index
		}
		if got := snapshotLiteralFile(t, dir, path); got != want {
			t.Errorf("selected %q = %+v, want %+v", path, got, want)
		}
	}
	for _, path := range unselected {
		if got := snapshotLiteralFile(t, dir, path); got != before[path] {
			t.Errorf("unselected %q = %+v, want preserved %+v", path, got, before[path])
		}
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
func TestGitOperatorLiteralCaseSelection(t *testing.T) {
	for _, operation := range []string{"stage", "unstage"} {
		t.Run(operation, func(t *testing.T) {
			pair := [2]string{"Foo.txt", "foo.txt"}
			dir := setupLiteralSelectionRepo(t, pair)
			skipCaseInsensitiveLiteralFilesystem(t, dir, pair)
			t.Setenv("GIT_ICASE_PATHSPECS", "1")
			checkLiteralMutation(t, dir, operation, []string{pair[0]}, []string{pair[0]}, []string{pair[1]})
			if os.Getenv("GIT_ICASE_PATHSPECS") != "1" {
				t.Fatal("selected command changed the process environment")
			}
		})
	}
}

func skipCaseInsensitiveLiteralFilesystem(t *testing.T, dir string, pair [2]string) {
	t.Helper()
	first, err := os.Stat(filepath.Join(dir, pair[0]))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(filepath.Join(dir, pair[1]))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(first, second) {
		t.Skip("filesystem cannot represent case-distinct sibling files")
	}
}
