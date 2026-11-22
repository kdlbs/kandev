//go:build linux

package acp

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestClaudeMonitorDirectoryOpenDoesNotBlockOnFIFOReplacement(t *testing.T) {
	rootPath := t.TempDir()
	const target, alternate = "project", "alternate"
	targetPath, alternatePath := filepath.Join(rootPath, target), filepath.Join(rootPath, alternate)
	require.NoError(t, os.Mkdir(targetPath, 0o700))
	require.NoError(t, unix.Mkfifo(alternatePath, 0o600))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	stop, flipperDone, flipperErr := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		defer close(flipperDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := unix.Renameat2(unix.AT_FDCWD, targetPath, unix.AT_FDCWD, alternatePath, unix.RENAME_EXCHANGE); err != nil {
				flipperErr <- err
				return
			}
		}
	}()
	var stopOnce sync.Once
	stopFlipper := func() {
		stopOnce.Do(func() { close(stop) })
		<-flipperDone
	}
	defer func() {
		stopFlipper()
	}()

	const workers, attempts = 16, 1000
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			for range attempts {
				child, _ := openClaudeMonitorDirectory(root, target)
				if child != nil {
					_ = child.Close()
				}
			}
		}()
	}
	finished := make(chan struct{})
	go func() {
		wait.Wait()
		close(finished)
	}()
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-finished:
		timer.Stop()
	case <-timer.C:
		t.Fatal("directory open blocked after the path changed to a FIFO")
	}
	stopFlipper()
	select {
	case err := <-flipperErr:
		require.NoError(t, err)
	default:
	}
}
