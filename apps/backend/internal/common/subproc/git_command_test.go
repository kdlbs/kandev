package subproc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// placeholderGit creates a file that exec.LookPath accepts as "git" in dir. It
// is resolved by the tests below but never executed. Every test installs its
// own temporary PATH, so a Git path remembered by another test never applies.
func placeholderGit(t *testing.T, dir string) string {
	t.Helper()
	name := "git"
	if runtime.GOOS == "windows" {
		name = "git.exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatalf("write placeholder git: %v", err)
	}
	return path
}

func samePath(t *testing.T, got, want string) {
	t.Helper()
	if !filepath.IsAbs(got) || !os.SameFile(statOrFail(t, got), statOrFail(t, want)) {
		t.Fatalf("resolved Git = %q, want %q", got, want)
	}
}

func statOrFail(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	return info
}

func TestNewGitCommandReusesResolvedGitUnderUnchangedPath(t *testing.T) {
	earlier, later := t.TempDir(), t.TempDir()
	remembered := placeholderGit(t, later)
	t.Setenv("PATH", earlier+string(os.PathListSeparator)+later)

	first := NewGitCommand(context.Background(), "status")
	samePath(t, first.Path, remembered)

	// A new candidate earlier on the same PATH is only found by another PATH
	// search; reusing the remembered executable must not perform one.
	placeholderGit(t, earlier)
	second := NewGitCommand(context.Background(), "status")
	samePath(t, second.Path, remembered)

	resolved, err := GitExecutablePath()
	if err != nil {
		t.Fatalf("GitExecutablePath() error = %v", err)
	}
	samePath(t, resolved, remembered)
}

func TestNewGitCommandResolvesAgainWhenPathChanges(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	firstGit := placeholderGit(t, first)
	secondGit := placeholderGit(t, second)

	t.Setenv("PATH", first)
	samePath(t, NewGitCommand(context.Background(), "status").Path, firstGit)

	t.Setenv("PATH", second)
	samePath(t, NewGitCommand(context.Background(), "status").Path, secondGit)
}

func TestNewGitCommandResolvesAgainWhenRememberedGitIsGone(t *testing.T) {
	earlier, later := t.TempDir(), t.TempDir()
	removed := placeholderGit(t, later)
	t.Setenv("PATH", earlier+string(os.PathListSeparator)+later)
	samePath(t, NewGitCommand(context.Background(), "status").Path, removed)

	replacement := placeholderGit(t, earlier)
	if err := os.Remove(removed); err != nil {
		t.Fatalf("remove remembered git: %v", err)
	}
	samePath(t, NewGitCommand(context.Background(), "status").Path, replacement)
}

func TestNewGitCommandKeepsGitArgvAndNotFoundError(t *testing.T) {
	dir := t.TempDir()
	placeholderGit(t, dir)
	t.Setenv("PATH", dir)

	cmd := NewGitCommand(context.Background(), "rev-parse", "HEAD")
	if got := cmd.Args; len(got) != 3 || got[0] != "git" || got[1] != "rev-parse" || got[2] != "HEAD" {
		t.Fatalf("Args = %q, want [git rev-parse HEAD]", got)
	}

	t.Setenv("PATH", t.TempDir())
	missing := NewGitCommand(context.Background(), "status")
	if missing.Args[0] != "git" {
		t.Fatalf("Args[0] = %q, want git", missing.Args[0])
	}
	if !errors.Is(missing.Err, exec.ErrNotFound) {
		t.Fatalf("Err = %v, want exec.ErrNotFound", missing.Err)
	}
	if _, err := GitExecutablePath(); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("GitExecutablePath() error = %v, want exec.ErrNotFound", err)
	}
}

// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.7
func TestNewGitCommandResolvesAgainWhenPathExtChanges(t *testing.T) {
	earlier, later := t.TempDir(), t.TempDir()
	remembered := placeholderGit(t, later)
	t.Setenv("PATH", earlier+string(os.PathListSeparator)+later)
	t.Setenv("PATHEXT", ".EXE")
	samePath(t, NewGitCommand(context.Background(), "status").Path, remembered)
	replacement := placeholderGit(t, earlier)
	t.Setenv("PATHEXT", ".COM;.EXE")
	samePath(t, NewGitCommand(context.Background(), "status").Path, replacement)
}

// @covers AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.7
func TestNewGitCommandResolvesAgainWhenRememberedGitIsUnusable(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "execute permission removed"
		if directory {
			name = "replaced by directory"
		}
		t.Run(name, func(t *testing.T) {
			if !directory && runtime.GOOS == "windows" {
				t.Skip("Unix execute permission contract")
			}
			earlier, later := t.TempDir(), t.TempDir()
			remembered := placeholderGit(t, earlier)
			t.Setenv("PATH", earlier+string(os.PathListSeparator)+later)
			samePath(t, NewGitCommand(context.Background(), "status").Path, remembered)
			if directory {
				if err := os.Remove(remembered); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(remembered, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Chmod(remembered, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := exec.LookPath(remembered); err == nil {
				t.Fatal("fixture remains executable")
			}
			replacement := placeholderGit(t, later)
			samePath(t, NewGitCommand(context.Background(), "status").Path, replacement)
			if err := os.Remove(replacement); err != nil {
				t.Fatal(err)
			}
			cmd := NewGitCommand(context.Background(), "status")
			if cmd.Args[0] != "git" || !errors.Is(cmd.Err, exec.ErrNotFound) {
				t.Fatalf("unresolvable Git: Args=%q Err=%v", cmd.Args, cmd.Err)
			}
		})
	}
}
