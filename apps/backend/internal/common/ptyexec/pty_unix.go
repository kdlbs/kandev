//go:build !windows

package ptyexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

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
	return startUnixPTY(cmd, cols, rows, func(f *os.File) error {
		// Nonblocking mode lets the os.File poller interrupt pending I/O on Close.
		return unix.SetNonblock(int(f.Fd()), true)
	})
}

func startUnixPTY(cmd *exec.Cmd, cols, rows uint16, initialize func(*os.File) error) (PtyHandle, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	if err := initialize(f); err != nil {
		closeErr := f.Close()
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
	return &unixPTY{f: f}, nil
}
