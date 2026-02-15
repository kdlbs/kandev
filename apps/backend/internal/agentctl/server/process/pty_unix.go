//go:build !windows

package process

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

// startPTYWithSize starts the command in a Unix PTY with the given dimensions.
// The command is started via pty.StartWithSize which calls cmd.Start() internally.
func startPTYWithSize(cmd *exec.Cmd, cols, rows int) (PtyHandle, error) {
	f, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, err
	}
	return &unixPTY{f: f}, nil
}
