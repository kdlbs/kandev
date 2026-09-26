//go:build windows

package ptyexec

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/UserExistsError/conpty"
)

// windowsPTY wraps a Windows ConPTY pseudo-console.
//
// Close is guarded by sync.Once because the upstream conpty library has no
// internal synchronization: calling its Close twice double-frees the underlying
// Windows handles and triggers STATUS_HEAP_CORRUPTION (0xC0000374), crashing the
// whole backend (issue #894).
//
// Both consumers reach Close from two directions, so the guard is mandatory for
// each of them:
//
//   - interactive terminals (internal/agentctl/server/process): Stop() closes the
//     PTY when the user closes a terminal tab, and wait() also closes it once
//     cmd.Wait returns.
//   - login PTY sessions (internal/agent/loginpty): Session.stop closes on
//     shutdown or timeout, while the readLoop drops the handle when the child
//     exits on its own.
type windowsPTY struct {
	cpty      *conpty.ConPty
	closeOnce sync.Once
	closeErr  error
}

func (p *windowsPTY) Read(b []byte) (int, error)  { return p.cpty.Read(b) }
func (p *windowsPTY) Write(b []byte) (int, error) { return p.cpty.Write(b) }

func (p *windowsPTY) Close() error {
	p.closeOnce.Do(func() {
		p.closeErr = p.cpty.Close()
	})
	return p.closeErr
}

func (p *windowsPTY) Resize(cols, rows uint16) error {
	return p.cpty.Resize(int(cols), int(rows))
}

// Start starts cmd in a Windows ConPTY with the given dimensions.
// ConPTY manages process creation internally, so we build a command line from
// cmd.Args, hand it to conpty.Start, then resolve the PID back into cmd.Process
// so callers can Wait() / Kill() through the usual exec.Cmd API.
func Start(cmd *exec.Cmd, cols, rows uint16) (PtyHandle, error) {
	cmdLine := resolveConPtyCmdLine(cmd)

	opts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(int(cols), int(rows)),
	}
	if cmd.Dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(cmd.Dir))
	}
	// Pass environment variables directly to the child process via ConPTY.
	if cmd.Env != nil {
		opts = append(opts, conpty.ConPtyEnv(cmd.Env))
	}

	cpty, err := conpty.Start(cmdLine, opts...)
	if err != nil {
		return nil, err
	}

	// Set cmd.Process so callers can use PID, Kill, Wait, etc.
	pid := cpty.Pid()
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = cpty.Close()
		return nil, fmt.Errorf("find conpty process %d: %w", pid, err)
	}
	cmd.Process = proc

	return &windowsPTY{cpty: cpty}, nil
}
