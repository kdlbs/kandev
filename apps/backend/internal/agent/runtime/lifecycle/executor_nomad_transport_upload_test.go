package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestNomadSSHTransportUploadReturnsOnContextDeadlineWhenLinkIsWedged(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	client := server.dial(t)
	server.setSilent(true)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- nomadSSHTransport{client: client}.Upload(ctx, filepath.Join(t.TempDir(), "f"), []byte("x"), 0o600)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Upload error = %v, want context deadline exceeded", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Upload did not return after the context deadline")
	}
}

func TestNomadSSHTransportUploadReturnsOnCancelBeforeStart(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	client := server.dial(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := nomadSSHTransport{client: client}.Upload(ctx, filepath.Join(t.TempDir(), "f"), []byte("x"), 0o600)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload error = %v, want context canceled", err)
	}
}
