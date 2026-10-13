//go:build !windows

package ptyexec

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-SHARED-PTY-001.1
func TestUnixPTYCloseInterruptsRead(t *testing.T) {
	assertUnixPTYCloseInterruptsRead(t, false)
}

// @covers AC-PLATFORM-SHARED-PTY-001.2
func TestUnixPTYResizePreservesInterruptibleRead(t *testing.T) {
	assertUnixPTYCloseInterruptsRead(t, true)
}

// @covers AC-PLATFORM-SHARED-PTY-001.3
func TestUnixPTYInitializationFailureReapsChild(t *testing.T) {
	cmd, ready := heldPTYCommand(t)
	initializationErr := errors.New("PTY initialization failed")
	var master *os.File
	handle, err := startUnixPTY(cmd, 80, 24, func(f *os.File) error {
		master = f
		waitHeldPTYReady(t, ready)
		return initializationErr
	})
	require.ErrorIs(t, err, initializationErr)
	require.Nil(t, handle)
	require.NotNil(t, master)
	require.ErrorIs(t, master.Close(), os.ErrClosed)
	require.NotNil(t, cmd.ProcessState, "the started child must be reaped before Start returns")
	require.ErrorIs(t, cmd.Process.Signal(syscall.Signal(0)), os.ErrProcessDone)
}

func assertUnixPTYCloseInterruptsRead(t *testing.T, resize bool) {
	t.Helper()
	cmd, ready := heldPTYCommand(t)
	handle, err := Start(cmd, 80, 24)
	require.NoError(t, err)
	t.Cleanup(func() { _ = handle.Close() })
	waitHeldPTYReady(t, ready)

	readStarted := make(chan struct{})
	readDone := make(chan error, 1)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = handle.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Error("PTY reader did not stop after child cleanup")
		}
	})
	go func() {
		close(readStarted)
		_, readErr := handle.Read(make([]byte, 1))
		readDone <- readErr
		close(readDone)
	}()
	<-readStarted
	// The child holds the slave open without writing to it. Observe that the
	// read remains pending before exercising close, rather than closing a
	// terminal whose read already completed during child startup.
	select {
	case readErr := <-readDone:
		t.Fatalf("read returned while the silent slave was still open: %v", readErr)
	case <-time.After(25 * time.Millisecond):
	}
	if resize {
		require.NoError(t, handle.Resize(120, 40))
	}
	require.NoError(t, handle.Close())
	select {
	case readErr := <-readDone:
		require.ErrorIs(t, readErr, os.ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt Read while the child held the slave open")
	}
	// The child ignores terminal hangup and waits on an independent pipe.
	// Child exit therefore cannot be what released the PTY read.
	require.NoError(t, cmd.Process.Signal(syscall.Signal(0)))
}

func heldPTYCommand(t *testing.T) (*exec.Cmd, *os.File) {
	t.Helper()
	readyRead, readyWrite, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = readyRead.Close(); _ = readyWrite.Close() })
	releaseRead, releaseWrite, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = releaseRead.Close(); _ = releaseWrite.Close() })
	cmd := exec.Command("/bin/sh", "-c", "trap '' HUP; printf ready >&3; IFS= read -r release <&4")
	cmd.ExtraFiles = []*os.File{readyWrite, releaseRead}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd, readyRead
}

func waitHeldPTYReady(t *testing.T, ready *os.File) {
	t.Helper()
	require.NoError(t, ready.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, len("ready"))
	_, err := io.ReadFull(ready, buf)
	require.NoError(t, err)
	require.Equal(t, "ready", string(buf))
}
