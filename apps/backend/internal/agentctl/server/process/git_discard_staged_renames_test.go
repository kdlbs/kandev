package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type renameDiscardState struct {
	index, blob, worktree, kind string
}

type renameDiscardFixture struct {
	dir, source, destination string
	paths                    []string
	want                     map[string]renameDiscardState
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestGitOperatorDiscardStagedRenames(t *testing.T) {
	for _, edit := range []string{"pure", "staged", "mixed"} {
		t.Run(edit, func(t *testing.T) {
			fixture := newRenameDiscardFixture(t, "old.txt", "new.txt", edit)
			before := renameDiscardRepositoryEvidence(t, fixture.dir)
			op := NewGitOperator(fixture.dir, newTestLogger(t), nil)
			result, err := op.Discard(context.Background(), fixture.paths)
			require.NoError(t, err)
			assert.True(t, result.Success, "result=%+v", result)
			assert.Empty(t, result.Error)
			assertRenameDiscardState(t, fixture.dir, fixture.want)
			assert.Equal(t, before, renameDiscardRepositoryEvidence(t, fixture.dir))
			assert.NotContains(t, strings.Split(runGit(t, fixture.dir, "diff", "--cached", "--name-only", "--diff-filter=D", "-z"), "\x00"), fixture.source)
		})
	}
	t.Run("multiple-and-duplicate", testDiscardMultipleRenames)
	t.Run("source-only", testDiscardRenameSourceOnly)
	for _, pair := range [][2]string{{"old日本語.txt", "new日本語.txt"}, {"old[ab].txt", "new[ab].txt"},
		{" old.txt ", " new.txt "}, {"old\tline\n.txt", "new\tline\n.txt"}, {"old -> text.txt", "new -> text.txt"},
		{"old\"quote.txt", "new\"quote.txt"}, {":(glob)old*.txt", ":(exclude)new*.txt"}, {"-old.txt", "-new.txt"}} {
		t.Run("native/"+pair[0], func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.ContainsAny(pair[0]+pair[1], ":*?\"<>\t\n") {
				t.Skip("filename cannot be represented by the native Windows filesystem")
			}
			if runtime.GOOS == "windows" && strings.HasSuffix(pair[0], " ") {
				t.Skip("trailing spaces cannot be represented by the native Windows filesystem")
			}
			fixture := newRenameDiscardFixture(t, pair[0], pair[1], "mixed")
			captured := append(filterTestGitEnv(os.Environ()), "GIT_LITERAL_PATHSPECS=1", "GIT_ICASE_PATHSPECS=1")
			op := NewGitOperator(fixture.dir, newTestLogger(t), nil)
			op.setEnvironmentProvider(func() []string { return captured })
			before := append([]string(nil), captured...)
			ambient := os.Environ()
			result, err := op.Discard(context.Background(), fixture.paths)
			require.NoError(t, err)
			assert.True(t, result.Success, "result=%+v", result)
			assertRenameDiscardState(t, fixture.dir, fixture.want)
			assert.Equal(t, before, captured)
			assert.Equal(t, ambient, os.Environ())
		})
	}
}

func testDiscardMultipleRenames(t *testing.T) {
	f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
	runGit(t, f.dir, "rm", "--force", "--", "copied.txt")
	runGit(t, f.dir, "mv", "--", "copy-source.txt", "second.txt")
	require.Contains(t, runGit(t, f.dir, "status", "--porcelain", "-z", "--untracked-files=no"), "R  second.txt\x00copy-source.txt\x00")
	f.want["second.txt"] = renameDiscardState{}
	f.paths = append(f.paths, "second.txt", "new.txt", "old.txt")
	result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), f.paths)
	require.NoError(t, err)
	assert.True(t, result.Success, "result=%+v", result)
	assertRenameDiscardState(t, f.dir, f.want)
}

func testDiscardRenameSourceOnly(t *testing.T) {
	f := newRenameDiscardFixture(t, "old.txt", "new.txt", "pure")
	for path := range f.want {
		if path != f.source {
			f.want[path] = readRenameDiscardState(t, f.dir, path)
		}
	}
	result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), []string{f.source})
	require.NoError(t, err)
	assert.True(t, result.Success, "result=%+v", result)
	assertRenameDiscardState(t, f.dir, f.want)
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestGitOperatorDiscardStagedRenameRelativeSelections(t *testing.T) {
	selections := []string{"./new.txt", "././new.txt", ".//new.txt", "unused/../new.txt"}
	if runtime.GOOS == "windows" {
		selections = append(selections, ".\\new.txt")
	}
	for _, selection := range selections {
		t.Run(selection, func(t *testing.T) {
			f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
			before := renameDiscardRepositoryEvidence(t, f.dir)
			f.paths[0] = selection
			result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), f.paths)
			require.NoError(t, err)
			assert.True(t, result.Success, "result=%+v", result)
			assertRenameDiscardState(t, f.dir, f.want)
			assert.Equal(t, before, renameDiscardRepositoryEvidence(t, f.dir))
		})
	}
	t.Run("duplicate-endpoints", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		f.paths = append([]string{"./new.txt", "new.txt", "././new.txt", "./old.txt", "old.txt"}, f.paths[1:]...)
		result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), f.paths)
		require.NoError(t, err)
		assert.True(t, result.Success, "result=%+v", result)
		assertRenameDiscardState(t, f.dir, f.want)
	})
	t.Run("source-only", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		for path := range f.want {
			if path != f.source {
				f.want[path] = readRenameDiscardState(t, f.dir, path)
			}
		}
		result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), []string{"./old.txt"})
		require.NoError(t, err)
		assert.True(t, result.Success, "result=%+v", result)
		assertRenameDiscardState(t, f.dir, f.want)
	})
	for _, path := range []string{"new[ab].txt", " new.txt ", "new\\literal.txt"} {
		t.Run("native/"+path, func(t *testing.T) {
			if runtime.GOOS == "windows" && (strings.Contains(path, "\\") || strings.HasSuffix(path, " ")) {
				t.Skip("literal backslash and trailing-space filenames are not native Windows filenames")
			}
			f := newRenameDiscardFixture(t, "old.txt", path, "mixed")
			f.paths[0] = "./" + path
			result, err := NewGitOperator(f.dir, newTestLogger(t), nil).Discard(context.Background(), f.paths)
			require.NoError(t, err)
			assert.True(t, result.Success, "result=%+v", result)
			assertRenameDiscardState(t, f.dir, f.want)
		})
	}
	t.Run("occupied-source", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		writeFile(t, f.dir, f.source, "preserve recreated source\n")
		f.paths[0] = "./new.txt"
		assertRenameDiscardRefusal(t, f, NewGitOperator(f.dir, newTestLogger(t), nil))
	})
	t.Run("raw-rejected-before-cleaning", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		f.paths = []string{"invalid\x00/../new.txt"}
		assertRenameDiscardRefusal(t, f, NewGitOperator(f.dir, newTestLogger(t), nil))
	})
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestGitOperatorDiscardStagedRenameRefusals(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "empty-entry"} {
		t.Run(kind, func(t *testing.T) {
			f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
			switch kind {
			case "file":
				writeFile(t, f.dir, f.source, "unrelated recreated source\n")
			case "directory":
				require.NoError(t, os.Mkdir(filepath.Join(f.dir, f.source), 0o755))
				writeFile(t, f.dir, f.source+"/child.txt", "preserve directory content\n")
				f.want[f.source+"/child.txt"] = readRenameDiscardState(t, f.dir, f.source+"/child.txt")
			case "symlink":
				if err := os.Symlink("missing-target", filepath.Join(f.dir, f.source)); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("native symlink unavailable: %v", err)
					}
					t.Fatal(err)
				}
			case "empty-entry":
				f.paths = append(f.paths, "")
			}
			assertRenameDiscardRefusal(t, f, NewGitOperator(f.dir, newTestLogger(t), nil))
		})
	}
	for _, shape := range []string{"failed", "truncated", "diagnostic", "overlap", "worktree-only", "committed-destination"} {
		t.Run("external-evidence/"+shape, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("the fault-injection shell executable requires POSIX")
			}
			f := newRenameDiscardFixture(t, "old.txt", "new.txt", "pure")
			op := newFaultedRenameDiscardOperator(t, f, shape)
			if shape == "committed-destination" {
				f.paths = append(f.paths, "neighbor.txt")
			}
			assertRenameDiscardRefusal(t, f, op)
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
func TestGitOperatorDiscardCopyEndpointEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the status-boundary fault executable requires POSIX")
	}
	for _, shape := range []string{"copy-shared-source", "copy-shared-destination"} {
		t.Run(shape, func(t *testing.T) {
			f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
			op := newFaultedRenameDiscardOperator(t, f, shape)
			ambient := os.Environ()
			assertRenameDiscardRefusal(t, f, op)
			assert.Equal(t, ambient, os.Environ())
		})
	}
	t.Run("independent-copy", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		op := newFaultedRenameDiscardOperator(t, f, "copy-independent")
		before := renameDiscardRepositoryEvidence(t, f.dir)
		result, err := op.Discard(context.Background(), f.paths)
		require.NoError(t, err)
		assert.True(t, result.Success, "result=%+v", result)
		assert.Empty(t, result.Error)
		assertRenameDiscardState(t, f.dir, f.want)
		assert.Equal(t, before, renameDiscardRepositoryEvidence(t, f.dir))
	})
	t.Run("standalone-copy-literal-aliases", func(t *testing.T) {
		f := newRenameDiscardFixture(t, "old.txt", "new.txt", "mixed")
		committedSource := f.want["copy-source.txt"]
		for path := range f.want {
			f.want[path] = readRenameDiscardState(t, f.dir, path)
		}
		writeFile(t, f.dir, "copy-source.txt", "ordinary staged source edit\n")
		runGit(t, f.dir, "add", "--", "copy-source.txt")
		writeFile(t, f.dir, "copy-source.txt", "ordinary working source edit\n")
		f.want["copy-source.txt"], f.want["copied.txt"] = committedSource, renameDiscardState{}
		f.paths = []string{"./copied.txt", "./copy-source.txt"}
		op := newFaultedRenameDiscardOperator(t, f, "copy-standalone")
		before := renameDiscardRepositoryEvidence(t, f.dir)
		ambient := os.Environ()
		result, err := op.Discard(context.Background(), f.paths)
		require.NoError(t, err)
		assert.True(t, result.Success, "result=%+v", result)
		assert.Empty(t, result.Error)
		assertRenameDiscardState(t, f.dir, f.want)
		assert.Equal(t, before, renameDiscardRepositoryEvidence(t, f.dir))
		assert.Equal(t, ambient, os.Environ())
	})
}

func assertRenameDiscardRefusal(t *testing.T, f renameDiscardFixture, op *GitOperator) {
	t.Helper()
	for path := range f.want {
		f.want[path] = readRenameDiscardState(t, f.dir, path)
	}
	before := renameDiscardRepositoryEvidence(t, f.dir)
	result, err := op.Discard(context.Background(), f.paths)
	require.NoError(t, err)
	assert.False(t, result.Success, "result=%+v", result)
	assert.NotEmpty(t, result.Error)
	assertRenameDiscardState(t, f.dir, f.want)
	assert.Equal(t, before, renameDiscardRepositoryEvidence(t, f.dir))
}

func newFaultedRenameDiscardOperator(t *testing.T, f renameDiscardFixture, shape string) *GitOperator {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	dir := t.TempDir()
	output := "R  new.txt\x00old.txt\x00"
	exit := 0
	switch shape {
	case "failed":
		exit = 7
	case "truncated":
		output = "R  new.txt\x00old.txt"
	case "diagnostic":
		output += "unexpected Git diagnostic\n"
	case "overlap":
		output += "R  extra.txt\x00new.txt\x00"
	case "copy-shared-source":
		output += "C  copied.txt\x00old.txt\x00"
	case "copy-shared-destination":
		output += "C  copied.txt\x00new.txt\x00"
	case "copy-independent":
		output += "C  copied.txt\x00copy-source.txt\x00"
	case "copy-standalone":
		output = "C  copied.txt\x00copy-source.txt\x00"
	case "worktree-only":
		output = " R new.txt\x00old.txt\x00"
	case "committed-destination":
		output = "R  neighbor.txt\x00old.txt\x00"
	}
	outputPath := filepath.Join(dir, "output")
	require.NoError(t, os.WriteFile(outputPath, []byte(output), 0o600))
	git := filepath.Join(dir, "git")
	writeExecutable(t, git, fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = status ] && [ \"$3\" = -z ]; then cat %q; exit %d; fi\nexec %q \"$@\"\n", outputPath, exit, realGit))
	probe, probeErr := exec.Command(git, "status", "--porcelain", "-z", "--untracked-files=no").CombinedOutput()
	require.Equal(t, output, string(probe), "faulted external boundary must be reachable")
	assert.Equal(t, exit == 0, probeErr == nil)
	op := NewGitOperator(f.dir, newTestLogger(t), nil)
	env := append(filterTestGitEnv(os.Environ()), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	op.setEnvironmentProvider(func() []string { return env })
	// exec.LookPath resolves Git against the parent PATH, independently of cmd.Env.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return op
}

func newRenameDiscardFixture(t *testing.T, source, destination, edit string) renameDiscardFixture {
	t.Helper()
	isolatePlainPatchEnvironment(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Rename Test")
	runGit(t, dir, "config", "core.autocrlf", "false")
	content := strings.Repeat("committed source line\n", 40)
	for _, path := range []string{source, "neighbor.txt", "modified.txt", "copy-source.txt", "deleted-neighbor.txt"} {
		data := strings.Repeat(path+" HEAD line\n", 40)
		if path == source {
			data = content + source + "\n"
		}
		writeFile(t, dir, path, data)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "rename discard fixture")
	want := map[string]renameDiscardState{source: readRenameDiscardState(t, dir, source), "modified.txt": readRenameDiscardState(t, dir, "modified.txt")}
	runGit(t, dir, "mv", "--", source, destination)
	if edit != "pure" {
		writeFile(t, dir, destination, content+source+"\nstaged destination edit\n")
		runGit(t, dir, "add", "--", ":(literal)"+destination)
	}
	if edit == "mixed" {
		writeFile(t, dir, destination, content+source+"\nstaged destination edit\nworking destination edit\n")
	}
	xy := "R "
	if edit == "mixed" {
		xy = "RM"
	}
	require.Contains(t, runGit(t, dir, "status", "--porcelain", "-z", "--untracked-files=no"), xy+" "+destination+"\x00"+source+"\x00", "fixture must be a real recognized staged rename")
	for _, path := range []string{"neighbor.txt", "modified.txt", "added.txt"} {
		writeFile(t, dir, path, path+" staged\n")
		runGit(t, dir, "add", "--", path)
		writeFile(t, dir, path, path+" worktree\n")
	}
	writeFile(t, dir, "untracked.txt", "untracked worktree\n")
	copyBytes, err := os.ReadFile(filepath.Join(dir, "copy-source.txt"))
	require.NoError(t, err)
	writeFile(t, dir, "copied.txt", string(copyBytes))
	runGit(t, dir, "add", "--", "copied.txt")
	require.NoError(t, os.Remove(filepath.Join(dir, "deleted-neighbor.txt")))
	runGit(t, dir, "add", "--", "deleted-neighbor.txt")
	for _, path := range []string{"neighbor.txt", "copy-source.txt", "deleted-neighbor.txt"} {
		want[path] = readRenameDiscardState(t, dir, path)
	}
	for _, path := range []string{destination, "added.txt", "untracked.txt", "copied.txt"} {
		want[path] = renameDiscardState{}
	}
	return renameDiscardFixture{dir: dir, source: source, destination: destination,
		paths: []string{destination, "modified.txt", "added.txt", "untracked.txt", "copied.txt"}, want: want}
}

func readRenameDiscardState(t *testing.T, dir, path string) renameDiscardState {
	t.Helper()
	state := renameDiscardState{index: runGit(t, dir, "ls-files", "--stage", "-z", "--", ":(literal)"+path)}
	if state.index != "" {
		state.blob = runGit(t, dir, "show", ":"+path)
	}
	info, err := os.Lstat(filepath.Join(dir, path))
	if os.IsNotExist(err) {
		return state
	}
	require.NoError(t, err)
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		state.kind = "symlink"
		state.worktree, err = os.Readlink(filepath.Join(dir, path))
	case info.IsDir():
		state.kind = "directory"
	default:
		state.kind = "file"
		var data []byte
		data, err = os.ReadFile(filepath.Join(dir, path))
		state.worktree = string(data)
	}
	require.NoError(t, err)
	return state
}

func assertRenameDiscardState(t *testing.T, dir string, want map[string]renameDiscardState) {
	t.Helper()
	for path, expected := range want {
		assert.Equal(t, expected, readRenameDiscardState(t, dir, path), "path %q", path)
	}
}

func renameDiscardRepositoryEvidence(t *testing.T, dir string) []string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	require.NoError(t, err)
	return []string{runGit(t, dir, "rev-parse", "HEAD"), runGit(t, dir, "symbolic-ref", "HEAD"),
		runGit(t, dir, "for-each-ref", "--format=%(refname) %(objectname)"), string(config)}
}
