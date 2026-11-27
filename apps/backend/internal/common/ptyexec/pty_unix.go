//go:build !windows

package ptyexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// unixPTY wraps a Unix PTY master file descriptor.
type unixPTY struct {
	f *os.File
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *unixPTY) Close() error                { return p.f.Close() }

func (p *unixPTY) Resize(cols, rows uint16) error {
	conn, err := p.f.SyscallConn()
	if err != nil {
		return err
	}
	var resizeErr error
	// Control keeps the descriptor valid through ioctl, even during concurrent Read/Close.
	if err := conn.Control(func(fd uintptr) {
		resizeErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
	}); err != nil {
		return err
	}
	return resizeErr
}

// Start starts cmd under a Unix PTY with the given dimensions.
// pty.StartWithSize calls cmd.Start() internally, so cmd.Process is set on return.
func Start(cmd *exec.Cmd, cols, rows uint16) (PtyHandle, error) {
	return startUnixPTY(cmd, cols, rows, initializeUnixPTY)
}

func initializeUnixPTY(f *os.File) (*os.File, error) {
	// NewFile only registers an already nonblocking descriptor with the poller.
	// Duplicate ownership so closing the original cannot close the returned file.
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, errors.Join(err, unix.Close(fd))
	}
	initialized := os.NewFile(uintptr(fd), f.Name())
	if err := initialized.SetDeadline(time.Time{}); err != nil {
		return nil, errors.Join(err, initialized.Close())
	}
	if err := f.Close(); err != nil {
		return nil, errors.Join(err, initialized.Close())
	}
	return initialized, nil
}

func startUnixPTY(cmd *exec.Cmd, cols, rows uint16, initialize func(*os.File) (*os.File, error)) (PtyHandle, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	initialized, err := initialize(f)
	if err != nil {
		closeErr := f.Close()
		if errors.Is(closeErr, os.ErrClosed) {
			closeErr = nil
		}
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		waitErr := cmd.Wait()
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			waitErr = nil
		}
		return nil, errors.Join(fmt.Errorf("initialize pty: %w", err), closeErr, killErr, waitErr)
	}
	return &unixPTY{f: initialized}, nil
}
