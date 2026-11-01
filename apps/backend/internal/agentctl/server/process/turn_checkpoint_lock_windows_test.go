//go:build windows

package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows"
)

func TestTurnCheckpointLockWaitsForPendingWindowsDeletion(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		name := "release"
		if cancelWait {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gitDir := t.TempDir()
				path := filepath.Join(gitDir, "kandev-turn-changes.capture.lock")
				closeHandle := pendingDeletionCheckpointLock(t, path)
				defer closeHandle()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					unlock, err := acquireTurnCheckpointLock(ctx, gitDir)
					if unlock != nil {
						unlock()
					}
					done <- err
				}()
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("pending deletion ended lock acquisition: %v", err)
				default:
				}
				if cancelWait {
					cancel()
				} else {
					closeHandle()
				}
				time.Sleep(25 * time.Millisecond)
				synctest.Wait()
				select {
				case err := <-done:
					if cancelWait && !errors.Is(err, context.Canceled) || !cancelWait && err != nil {
						t.Fatalf("lock acquisition after %s: %v", name, err)
					}
				default:
					t.Fatalf("lock acquisition did not finish after %s", name)
				}
			})
		})
	}
}

func pendingDeletionCheckpointLock(t *testing.T, path string) func() {
	t.Helper()
	nativePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(nativePath, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closeHandle := func() {
		if handle != windows.InvalidHandle {
			_ = windows.CloseHandle(handle)
			handle = windows.InvalidHandle
		}
	}
	t.Cleanup(closeHandle)
	// Keep deletion pending on this handle until the test explicitly closes it.
	deletePending := byte(1)
	if err := windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &deletePending, 1); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if file != nil {
		_ = file.Close()
	}
	if !os.IsPermission(err) {
		t.Fatalf("fixture must retain a delete-pending file: %v", err)
	}
	return closeHandle
}
