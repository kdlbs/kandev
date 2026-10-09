//go:build !windows

package ptyexec

import (
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
)

func TestUnixPTYResizeUpdatesWindowSize(t *testing.T) {
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = master.Close() })
	t.Cleanup(func() { _ = slave.Close() })
	handle := &unixPTY{f: master}

	require.NoError(t, handle.Resize(120, 40))
	size, err := pty.GetsizeFull(master)
	require.NoError(t, err)
	require.EqualValues(t, 120, size.Cols)
	require.EqualValues(t, 40, size.Rows)
}

func TestUnixPTYResizeConcurrentWithReadAndClose(t *testing.T) {
	for range 64 {
		master, slave, err := pty.Open()
		require.NoError(t, err)
		t.Cleanup(func() { _ = master.Close() })
		t.Cleanup(func() { _ = slave.Close() })
		handle := &unixPTY{f: master}
		readStarted, readDone := make(chan struct{}), make(chan struct{})
		resizeStarted, resizeDone := make(chan struct{}), make(chan struct{})
		go func() {
			close(readStarted)
			_, _ = handle.Read(make([]byte, 1))
			close(readDone)
		}()
		<-readStarted
		go func() {
			close(resizeStarted)
			for range 64 {
				_ = handle.Resize(80, 24)
			}
			close(resizeDone)
		}()
		<-resizeStarted
		require.NoError(t, handle.Close())
		require.NoError(t, slave.Close())
		<-readDone
		<-resizeDone
		require.Error(t, handle.Resize(80, 24))
	}
}
