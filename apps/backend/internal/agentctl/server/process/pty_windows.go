//go:build windows

package process

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/UserExistsError/conpty"
)

// windowsPTY wraps a Windows ConPTY pseudo-console.
type windowsPTY struct {
	cpty *conpty.ConPty
}

func (p *windowsPTY) Read(b []byte) (int, error)  { return p.cpty.Read(b) }
func (p *windowsPTY) Write(b []byte) (int, error) { return p.cpty.Write(b) }
func (p *windowsPTY) Close() error                { return p.cpty.Close() }

func (p *windowsPTY) Resize(cols, rows uint16) error {
	return p.cpty.Resize(int(cols), int(rows))
}

// startPTYWithSize starts the command in a Windows ConPTY with the given dimensions.
// ConPTY manages process creation internally, so this builds a command line from
// the exec.Cmd and starts the process via ConPTY. After this call, cmd.Process
// is set so callers can manage the process lifecycle.
func startPTYWithSize(cmd *exec.Cmd, cols, rows int) (PtyHandle, error) {
	cmdLine := buildCmdLine(cmd.Args)
	if len(cmd.Args) == 0 {
		cmdLine = escapeArg(cmd.Path)
	}

	opts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(cols, rows),
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
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		_ = cpty.Close()
		return nil, fmt.Errorf("failed to find ConPTY process %d: %w", pid, err)
	}
	cmd.Process = proc

	return &windowsPTY{cpty: cpty}, nil
}

