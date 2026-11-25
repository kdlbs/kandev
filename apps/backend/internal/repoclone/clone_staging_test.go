package repoclone

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestImproveKandevClonePublishesOnSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake git process wrapper is Unix-only")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "user.name", "Test User")
	runGit(t, source, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "initial")

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	startedPath := filepath.Join(root, "clone-started")
	fakeGit := "#!/bin/sh\n" +
		"if [ \"$1\" = clone ] && [ \"$BLOCK_WORKSPACE_CLONE\" = 1 ]; then\n" +
		"  for target do :; done\n" +
		"  mkdir -p \"$target/.git\"\n" +
		"  : > \"$CLONE_STARTED_PATH\"\n" +
		"  exec sleep 30\n" +
		"fi\n" +
		"exec \"$REAL_GIT_PATH\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REAL_GIT_PATH", realGit)
	t.Setenv("CLONE_STARTED_PATH", startedPath)
	t.Setenv("BLOCK_WORKSPACE_CLONE", "1")

	cloner := NewCloner(Config{BasePath: filepath.Join(root, "managed")}, ProtocolHTTPS, "", logger.Default())
	target, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "kdlbs", "kandev")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	type cloneResult struct {
		path string
		err  error
	}
	result := make(chan cloneResult, 1)
	go func() {
		path, cloneErr := cloner.EnsureWorkspaceCloned(ctx, "workspace-a", "gitlab", source, "kdlbs", "kandev")
		result <- cloneResult{path: path, err: cloneErr}
	}()
	markerDeadline := time.NewTimer(5 * time.Second)
	markerPoll := time.NewTicker(10 * time.Millisecond)
	defer markerDeadline.Stop()
	defer markerPoll.Stop()
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		select {
		case <-markerPoll.C:
		case clone := <-result:
			t.Fatalf("clone returned before fake git started: path=%q err=%v", clone.path, clone.err)
		case <-markerDeadline.C:
			cancel()
			t.Fatal("fake git clone did not start")
		}
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("clone exposed unpublished destination %q while Git was blocked: lstat error = %v", target, err)
	}
	cancel()
	select {
	case clone := <-result:
		if clone.err == nil {
			t.Fatal("cancelled clone error = nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled clone did not exit")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("cancelled clone published %q: lstat error = %v", target, err)
	}

	t.Setenv("BLOCK_WORKSPACE_CLONE", "0")
	path, err := cloner.EnsureWorkspaceCloned(context.Background(), "workspace-a", "gitlab", source, "kdlbs", "kandev")
	if err != nil {
		t.Fatalf("retry clone: %v", err)
	}
	if path != target {
		t.Fatalf("retry clone path = %q, want %q", path, target)
	}
	if got := strings.TrimSpace(runGit(t, path, "rev-parse", "HEAD")); got == "" {
		t.Fatal("retry clone has no commit")
	}
}

func TestClonePublishesIntoExistingEmptyDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "user.name", "Test User")
	runGit(t, source, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "initial")

	cloner := NewCloner(Config{BasePath: filepath.Join(root, "managed")}, ProtocolHTTPS, "", logger.Default())
	target, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "kdlbs", "kandev")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := cloner.EnsureWorkspaceCloned(context.Background(), "workspace-a", "gitlab", source, "kdlbs", "kandev")
	if err != nil {
		t.Fatalf("clone into empty destination: %v", err)
	}
	if path != target || !gitCheckoutExists(path) {
		t.Fatalf("clone path = %q, target = %q, git checkout exists = %v", path, target, gitCheckoutExists(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat published checkout: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("published checkout mode = %04o, want original destination mode 0700", got)
	}
}

func TestCloneIntoExistingEmptyDestinationWhenParentIsNotWritable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "user.name", "Test User")
	runGit(t, source, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "initial")

	cloner := NewCloner(Config{BasePath: filepath.Join(root, "managed")}, ProtocolHTTPS, "", logger.Default())
	target, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "kdlbs", "kandev")
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(parent, 0o755); err != nil {
			t.Errorf("restore parent permissions: %v", err)
		}
	}()
	probe, err := os.MkdirTemp(parent, ".permission-probe-")
	if err == nil {
		if removeErr := os.Remove(probe); removeErr != nil {
			t.Fatal(removeErr)
		}
		t.Skip("test process can write to a read-only directory")
	}

	path, err := cloner.EnsureWorkspaceCloned(context.Background(), "workspace-a", "gitlab", source, "kdlbs", "kandev")
	if err != nil {
		t.Fatalf("clone into existing destination with a non-writable parent: %v", err)
	}
	if path != target || !gitCheckoutExists(path) {
		t.Fatalf("clone path = %q, target = %q, git checkout exists = %v", path, target, gitCheckoutExists(path))
	}
}

func TestCloneRejectsOccupiedDestinationBeforeClone(t *testing.T) {
	root := t.TempDir()
	cloner := NewCloner(Config{BasePath: filepath.Join(root, "managed")}, ProtocolHTTPS, "", logger.Default())
	target, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "kdlbs", "kandev")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = cloner.EnsureWorkspaceCloned(context.Background(), "workspace-a", "gitlab", filepath.Join(root, "missing-source"), "kdlbs", "kandev")
	if err == nil || !strings.Contains(err.Error(), "managed repository path already exists") {
		t.Fatalf("clone error = %v, want occupied-path rejection before clone", err)
	}
	if got, readErr := os.ReadFile(sentinel); readErr != nil || string(got) != "keep" {
		t.Fatalf("occupied destination changed: contents=%q err=%v", got, readErr)
	}
}

func TestEnsureWorkspaceClonedCancelsWhileWaitingForPathLock(t *testing.T) {
	root := t.TempDir()
	cloner := NewCloner(Config{BasePath: filepath.Join(root, "managed")}, ProtocolHTTPS, "", logger.Default())
	target, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "kdlbs", "kandev")
	if err != nil {
		t.Fatal(err)
	}
	lock := cloner.repoMu(target)
	lock.Lock()
	locked := true
	defer func() {
		if locked {
			lock.Unlock()
		}
	}()
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	ctx := &lockWaitContext{Context: baseCtx, waiting: waiting}
	result := make(chan error, 1)
	go func() {
		_, cloneErr := cloner.EnsureWorkspaceCloned(ctx, "workspace-a", "gitlab", filepath.Join(root, "source"), "kdlbs", "kandev")
		result <- cloneErr
	}()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("clone did not reach the path-lock wait")
	}
	cancel()
	select {
	case cloneErr := <-result:
		if !errors.Is(cloneErr, context.Canceled) {
			t.Fatalf("clone error = %v, want context cancellation", cloneErr)
		}
	case <-time.After(5 * time.Second):
		lock.Unlock()
		locked = false
		cloneErr := <-result
		t.Fatalf("clone stayed blocked behind path lock; after release error = %v", cloneErr)
	}
}

type lockWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *lockWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}
