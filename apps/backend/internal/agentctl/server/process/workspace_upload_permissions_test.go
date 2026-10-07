package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
func TestWriteFileStreamReplacementPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execute and permission bits; Windows attributes have separate coverage")
	}
	t.Run("direct script execution survives replacement", func(t *testing.T) {
		dir, wt := uploadPermissionTracker(t)
		path := filepath.Join(dir, "deploy.sh")
		initial := "#!/bin/sh\nprintf '%s\\n' 'original deployment'\n"
		incoming := "#!/bin/sh\nprintf '%s\\n' 'uploaded deployment'\n"
		seedUploadPermissionFile(t, path, initial, 0o755)
		assertUploadPermissionScript(t, path, "original deployment\n")
		written, size, err := wt.WriteFileStream("deploy.sh", UploadResolutionReplace, strings.NewReader(incoming))
		assertUploadPermissionResult(t, written, size, err, "deploy.sh", incoming)
		assertUploadPermissionFile(t, path, incoming, 0o755)
		assertUploadPermissionScript(t, path, "uploaded deployment\n")
		assertUploadPermissionNoTemps(t, dir)
	})
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "private", mode: 0o600},
		{name: "read only", mode: 0o400},
		{name: "group readable", mode: 0o640},
		{name: "no access", mode: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "settings.dat")
			seedUploadPermissionFile(t, path, "saved settings", tc.mode)
			incoming := "replacement settings\x00\xff"
			written, size, err := wt.WriteFileStream("settings.dat", UploadResolutionReplace, strings.NewReader(incoming))
			assertUploadPermissionResult(t, written, size, err, "settings.dat", incoming)
			assertUploadPermissionFile(t, path, incoming, tc.mode)
			assertUploadPermissionNoTemps(t, dir)
		})
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.8
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.6
func TestWriteFileStreamReplacementCurrentTarget(t *testing.T) {
	for _, scenario := range []string{"mode changes", "target removed", "target appears", "unresolved target appears"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "mode changes" && runtime.GOOS == "windows" {
				t.Skip("changing POSIX bits during a stream; native Windows attributes covered separately")
			}
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "current.txt")
			resolution := UploadResolutionReplace
			if scenario == "mode changes" || scenario == "target removed" {
				seedUploadPermissionFile(t, path, "earlier destination", 0o600)
			}
			if scenario == "unresolved target appears" {
				resolution = UploadResolutionNone
			}
			incoming := "complete streamed revision"
			finish := beginUploadPermissionStream(t, wt, "current.txt", resolution, incoming)
			// A second write must finish while the first source is still gated.
			controlPath, controlSize, controlErr := wt.WriteFileStream("creation-control.txt", UploadResolutionNone, strings.NewReader("control"))
			assertUploadPermissionResult(t, controlPath, controlSize, controlErr, "creation-control.txt", "control")
			defaultMode := uploadPermissionMode(t, filepath.Join(dir, controlPath))
			switch scenario {
			case "mode changes":
				assertUploadPermissionFile(t, path, "earlier destination", 0o600)
				if err := os.Chmod(path, 0o750); err != nil {
					t.Fatal(err)
				}
			case "target removed":
				assertUploadPermissionFile(t, path, "earlier destination", 0o600)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "target appears", "unresolved target appears":
				seedUploadPermissionFile(t, path, "concurrent destination", 0o600)
			}
			result := finish()
			if scenario == "unresolved target appears" {
				if !errors.Is(result.err, ErrUploadConflict) {
					t.Fatalf("expected conflict, got %+v", result)
				}
				assertUploadPermissionFile(t, path, "concurrent destination", 0o600)
			} else {
				assertUploadPermissionResult(t, result.path, result.size, result.err, "current.txt", incoming)
				mode := os.FileMode(0o600)
				switch scenario {
				case "mode changes":
					mode = 0o750
				case "target removed":
					mode = defaultMode
				}
				assertUploadPermissionFile(t, path, incoming, mode)
			}
			assertUploadPermissionNoTemps(t, dir)
		})
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.9
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.6
func TestWriteFileStreamReplacementFailures(t *testing.T) {
	for _, scenario := range []string{"broken stream", "unresolved conflict", "missing staged pathname"} {
		t.Run(scenario, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			path := filepath.Join(dir, "unchanged.txt")
			seedUploadPermissionFile(t, path, "original bytes", 0o600)
			subscriber := watchUploadPermissionChanges(t, wt)
			resolution := UploadResolutionReplace
			var reader io.Reader = &failingReader{prefix: []byte("partial bytes")}
			switch scenario {
			case "unresolved conflict":
				resolution = UploadResolutionNone
				reader = strings.NewReader("complete bytes")
			case "missing staged pathname":
				if runtime.GOOS == "windows" {
					t.Skip("unlinking an open staging descriptor requires POSIX semantics")
				}
				reader = &uploadPermissionActionReader{Reader: strings.NewReader("incoming"), action: func() {
					removeUploadPermissionStage(t, dir)
				}}
			}
			written, size, err := wt.WriteFileStream("unchanged.txt", resolution, reader)
			if err == nil || written != "" || size != 0 {
				t.Fatalf("failed upload reported success: path=%q size=%d err=%v", written, size, err)
			}
			assertUploadPermissionFile(t, path, "original bytes", 0o600)
			assertUploadPermissionNoTemps(t, dir)
			assertUploadPermissionNoChange(t, subscriber)
			written, size, err = wt.WriteFileStream("notification-control.txt", UploadResolutionNone, strings.NewReader("ok"))
			assertUploadPermissionResult(t, written, size, err, "notification-control.txt", "ok")
			assertUploadPermissionChange(t, subscriber, "notification-control.txt")
		})
	}
	t.Run("directory rejected without publication", func(t *testing.T) {
		dir, wt := uploadPermissionTracker(t)
		if err := os.Mkdir(filepath.Join(dir, "existing-dir"), 0o700); err != nil {
			t.Fatal(err)
		}
		seedUploadPermissionFile(t, filepath.Join(dir, "existing-dir", "sentinel"), "retained", 0o600)
		if _, _, err := wt.WriteFileStream("existing-dir", UploadResolutionReplace, strings.NewReader("bad")); err == nil {
			t.Fatal("replaced a directory")
		}
		assertUploadPermissionFile(t, filepath.Join(dir, "existing-dir", "sentinel"), "retained", 0o600)
		assertUploadPermissionNoTemps(t, dir)
	})
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.9
func TestWriteFileStreamReplacementPublicationFailures(t *testing.T) {
	for _, existing := range []bool{true, false} {
		name := "sync closed staging descriptor"
		if existing {
			name = "chmod closed staging descriptor"
		}
		t.Run(name, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			target := filepath.Join(dir, "retained.txt")
			if existing {
				seedUploadPermissionFile(t, target, "original", 0o600)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Close() })
			stageRel, stage, err := createUploadTemp(root, ".")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stage.Close(); _ = root.Remove(stageRel) })
			if _, err := stage.WriteString("incoming"); err != nil {
				t.Fatal(err)
			}
			if err := stage.Close(); err != nil {
				t.Fatal(err)
			}
			rel, request, err := wt.publishUpload(root, stage, stageRel, "retained.txt", "retained.txt", UploadResolutionReplace)
			operation := "flush"
			if existing {
				operation = "preserve upload permissions"
			}
			if err == nil || !strings.Contains(err.Error(), operation) || rel != "" || request != "" {
				t.Fatalf("publication result rel=%q request=%q err=%v", rel, request, err)
			}
			if existing {
				assertUploadPermissionFile(t, target, "original", 0o600)
			} else if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("failed publication created destination: %v", err)
			}
			if err := root.Remove(stageRel); err != nil {
				t.Fatal(err)
			}
			assertUploadPermissionNoTemps(t, dir)
			// A successful write on the same tracker also proves the lock was released.
			written, size, err := wt.WriteFileStream("control.txt", UploadResolutionNone, strings.NewReader("control"))
			assertUploadPermissionResult(t, written, size, err, "control.txt", "control")
		})
	}
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.6
func TestWriteFileStreamReplacementNotification(t *testing.T) {
	dir, wt := uploadPermissionTracker(t)
	seedUploadPermissionFile(t, filepath.Join(dir, "notified.txt"), "old", 0o600)
	subscriber := watchUploadPermissionChanges(t, wt)
	written, size, err := wt.WriteFileStream("notified.txt", UploadResolutionReplace, strings.NewReader("new"))
	assertUploadPermissionResult(t, written, size, err, "notified.txt", "new")
	assertUploadPermissionChange(t, subscriber, "notified.txt")
	assertUploadPermissionNoChange(t, subscriber)
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.1
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-004.7
func TestWriteFileStreamReplacementContainment(t *testing.T) {
	t.Run("stable workspace file link", func(t *testing.T) {
		dir, wt := uploadPermissionTracker(t)
		target := filepath.Join(dir, "real.txt")
		alias := filepath.Join(dir, "alias.txt")
		seedUploadPermissionFile(t, target, "linked original", 0o600)
		if err := os.Symlink("real.txt", alias); err != nil {
			t.Skipf("native symlink creation unavailable: %v", err)
		}
		written, size, err := wt.WriteFileStream("alias.txt", UploadResolutionReplace, strings.NewReader("linked replacement"))
		assertUploadPermissionResult(t, written, size, err, "alias.txt", "linked replacement")
		assertUploadPermissionFile(t, target, "linked replacement", 0o600)
		info, err := os.Lstat(alias)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("original file link was replaced: info=%v err=%v", info, err)
		}
		assertUploadPermissionNoTemps(t, dir)
	})
	for _, kind := range []string{"file link", "directory link", "native outside absolute", "parent traversal"} {
		t.Run(kind, func(t *testing.T) {
			dir, wt := uploadPermissionTracker(t)
			outside := t.TempDir()
			target := filepath.Join(outside, "protected.txt")
			seedUploadPermissionFile(t, target, "outside original", 0o600)
			reqPath := target
			switch kind {
			case "file link":
				if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
					t.Skipf("native symlink creation unavailable: %v", err)
				}
				reqPath = "link"
			case "directory link":
				if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
					t.Skipf("native symlink creation unavailable: %v", err)
				}
				reqPath = "link/protected.txt"
			case "parent traversal":
				var err error
				reqPath, err = filepath.Rel(dir, target)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := wt.WriteFileStream(reqPath, UploadResolutionReplace, strings.NewReader("escape")); err == nil {
				t.Fatal("upload escaped its authorized root")
			}
			assertUploadPermissionFile(t, target, "outside original", 0o600)
			assertUploadPermissionNoTemps(t, dir)
			assertUploadPermissionNoTemps(t, outside)
		})
	}
	t.Run("directory swapped after resolution", testUploadPermissionDirectorySwap)
	t.Run("registered source retains mode", testUploadPermissionRegisteredSource)
}

func testUploadPermissionDirectorySwap(t *testing.T) {
	dir, wt := uploadPermissionTracker(t)
	outside := t.TempDir()
	switchable := filepath.Join(dir, "switchable")
	if err := os.Mkdir(switchable, 0o700); err != nil {
		t.Fatal(err)
	}
	seedUploadPermissionFile(t, filepath.Join(switchable, "kept.txt"), "workspace original", 0o600)
	seedUploadPermissionFile(t, filepath.Join(outside, "kept.txt"), "outside original", 0o600)
	probe := filepath.Join(dir, "symlink-probe")
	if err := os.Symlink(outside, probe); err != nil {
		t.Skipf("native symlink creation unavailable: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	workspaceMutationBarrier.Store(func() {
		if err := os.Rename(switchable, filepath.Join(dir, "retained")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, switchable); err != nil {
			t.Fatal(err)
		}
	})
	t.Cleanup(func() { workspaceMutationBarrier.Store((func())(nil)) })
	if _, _, err := wt.WriteFileStream("switchable/kept.txt", UploadResolutionReplace, strings.NewReader("escape")); err == nil {
		t.Fatal("upload traversed the swapped directory")
	}
	assertUploadPermissionFile(t, filepath.Join(dir, "retained", "kept.txt"), "workspace original", 0o600)
	assertUploadPermissionFile(t, filepath.Join(outside, "kept.txt"), "outside original", 0o600)
	assertUploadPermissionNoTemps(t, dir)
	assertUploadPermissionNoTemps(t, outside)
}

func testUploadPermissionRegisteredSource(t *testing.T) {
	dir, wt := uploadPermissionTracker(t)
	source := t.TempDir()
	wt.SetAllowedSourceRoots([]string{source})
	if err := os.Symlink(source, filepath.Join(dir, "source")); err != nil {
		t.Skipf("native symlink creation unavailable: %v", err)
	}
	path := filepath.Join(source, "source.txt")
	seedUploadPermissionFile(t, path, "source original", 0o600)
	written, size, err := wt.WriteFileStream("source/source.txt", UploadResolutionReplace, strings.NewReader("source replacement"))
	assertUploadPermissionResult(t, written, size, err, "source/source.txt", "source replacement")
	assertUploadPermissionFile(t, path, "source replacement", 0o600)
	assertUploadPermissionNoTemps(t, source)
}

func uploadPermissionTracker(t *testing.T) (string, *WorkspaceTracker) {
	t.Helper()
	dir, wt := setupTestDir(t)
	t.Cleanup(wt.Stop)
	return dir, wt
}

func seedUploadPermissionFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			t.Errorf("restore owned fixture permissions: %v", err)
		}
	})
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	assertUploadPermissionMode(t, path, mode)
}

func uploadPermissionMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func assertUploadPermissionMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	got := uploadPermissionMode(t, path)
	if runtime.GOOS == "windows" {
		got &= 0o200
		want &= 0o200
	}
	if got != want {
		t.Errorf("destination permissions = %04o, want %04o", got, want)
	}
}

func assertUploadPermissionFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	assertUploadPermissionMode(t, path, mode)
	observed := uploadPermissionMode(t, path)
	// Read zero-mode contents only after inspecting the published permission bits.
	if observed&0o400 == 0 && runtime.GOOS != "windows" {
		if err := os.Chmod(path, observed|0o400); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.Chmod(path, observed); err != nil {
				t.Errorf("restore inspected mode: %v", err)
			}
		}()
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != content {
		t.Fatalf("content=%q, want=%q, err=%v", got, content, err)
	}
}

func assertUploadPermissionResult(t *testing.T, path string, size int64, err error, wantPath, content string) {
	t.Helper()
	if err != nil || path != wantPath || size != int64(len(content)) {
		t.Fatalf("upload path=%q size=%d err=%v; want path=%q size=%d", path, size, err, wantPath, len(content))
	}
}

func assertUploadPermissionScript(t *testing.T, path, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != want {
		t.Fatalf("direct script launch: output=%q want=%q err=%v", output, want, err)
	}
}

func assertUploadPermissionNoTemps(t *testing.T, dir string) {
	t.Helper()
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), uploadTempPrefix) {
			t.Errorf("upload staging artifact remains: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func removeUploadPermissionStage(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), uploadTempPrefix) {
			found++
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	if found != 1 {
		t.Fatalf("expected exactly one owned staging file, got %d", found)
	}
}

func watchUploadPermissionChanges(t *testing.T, wt *WorkspaceTracker) types.WorkspaceStreamSubscriber {
	t.Helper()
	subscriber := make(types.WorkspaceStreamSubscriber, 8)
	wt.workspaceSubMu.Lock()
	if wt.workspaceStreamSubscribers == nil {
		wt.workspaceStreamSubscribers = make(map[types.WorkspaceStreamSubscriber]struct{})
	}
	wt.workspaceStreamSubscribers[subscriber] = struct{}{}
	wt.workspaceSubMu.Unlock()
	t.Cleanup(func() {
		wt.workspaceSubMu.Lock()
		delete(wt.workspaceStreamSubscribers, subscriber)
		wt.workspaceSubMu.Unlock()
	})
	return subscriber
}

func assertUploadPermissionNoChange(t *testing.T, subscriber types.WorkspaceStreamSubscriber) {
	t.Helper()
	select {
	case change := <-subscriber:
		t.Fatalf("unexpected workspace change: %+v", change)
	default:
	}
}

func assertUploadPermissionChange(t *testing.T, subscriber types.WorkspaceStreamSubscriber, path string) {
	t.Helper()
	select {
	case change := <-subscriber:
		if change.FileChange == nil || change.FileChange.Path != path || change.FileChange.Operation != types.FileOpCreate {
			t.Fatalf("workspace change=%+v, want create at %s", change, path)
		}
	default:
		t.Fatal("successful upload emitted no workspace change")
	}
}

type uploadPermissionActionReader struct {
	io.Reader
	action func()
	once   sync.Once
}

func (reader *uploadPermissionActionReader) Read(p []byte) (int, error) {
	reader.once.Do(reader.action)
	return reader.Reader.Read(p)
}

type uploadPermissionResult struct {
	path string
	size int64
	err  error
}

type uploadPermissionGatedReader struct {
	io.Reader
	ctx     context.Context
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (reader *uploadPermissionGatedReader) Read(p []byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	select {
	case <-reader.ctx.Done():
		return 0, reader.ctx.Err()
	case <-reader.release:
		return reader.Reader.Read(p)
	}
}

func beginUploadPermissionStream(t *testing.T, wt *WorkspaceTracker, path string, resolution UploadResolution, content string) func() uploadPermissionResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	reader := &uploadPermissionGatedReader{
		Reader: strings.NewReader(content), ctx: ctx,
		started: make(chan struct{}), release: make(chan struct{}),
	}
	results := make(chan uploadPermissionResult, 1)
	done := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(reader.release) }) }
	t.Cleanup(func() {
		cancel()
		unblock()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("owned upload did not join during cleanup")
		}
	})
	go func() {
		written, size, err := wt.WriteFileStream(path, resolution, reader)
		results <- uploadPermissionResult{path: written, size: size, err: err}
		close(done)
	}()
	select {
	case <-reader.started:
	case <-ctx.Done():
		t.Fatal("upload never reached its source read")
	}
	return func() uploadPermissionResult {
		unblock()
		select {
		case result := <-results:
			<-done
			return result
		case <-ctx.Done():
			t.Fatal("owned upload did not complete")
			return uploadPermissionResult{}
		}
	}
}
