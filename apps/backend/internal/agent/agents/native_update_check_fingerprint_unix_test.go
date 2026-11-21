//go:build !windows

package agents

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO where a project configuration directory is expected must not block
// fingerprinting: opening it for reading would wait for a writer forever.
func TestFingerprintDoesNotBlockOnNonDirectoryProjectPath(t *testing.T) {
	for _, viaSymlink := range []bool{false, true} {
		f := newOmpContextFixture(t)
		if err := os.MkdirAll(f.cwd, 0o700); err != nil {
			t.Fatal(err)
		}
		fifo := filepath.Join(f.cwd, ".omp")
		if viaSymlink {
			fifo = filepath.Join(f.root, "pipe")
		}
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		if viaSymlink {
			if err := os.Symlink(fifo, filepath.Join(f.cwd, ".omp")); err != nil {
				t.Fatal(err)
			}
		}
		done := make(chan string, 1)
		go func() {
			resolver := NativeUpdateCheckResolver{WorkingDir: f.cwd, HomeDir: f.home}
			done <- NewOmpACP().NativeUpdateCheck().Context.Fingerprint(resolver)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Unblock the stuck open so the goroutine can exit.
			if writer, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
				_ = writer.Close()
			}
			t.Fatalf("fingerprint blocked on a FIFO at the project .omp path (via symlink: %v)", viaSymlink)
		}
	}
}
