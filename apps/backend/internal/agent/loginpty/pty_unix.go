//go:build !windows

package loginpty

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// unixPTY wraps a Unix PTY master file descriptor.
type unixPTY struct {
	f *os.File
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *unixPTY) Close() error                { return p.f.Close() }

func (p *unixPTY) Resize(cols, rows uint16) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: cols, Rows: rows})
}

// startPTYWithSize starts cmd under a Unix PTY with the given dimensions.
// pty.StartWithSize calls cmd.Start() internally, so cmd.Process is set on return.
func startPTYWithSize(cmd *exec.Cmd, cols, rows uint16) (ptyHandle, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &unixPTY{f: f}, nil
}
