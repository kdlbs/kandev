//go:build windows

package loginpty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"
)

// windowsPTY wraps a Windows ConPTY pseudo-console.
//
// Close is guarded by sync.Once because the upstream conpty library has no
// internal synchronization: calling its Close twice double-frees the underlying
// Windows handles and triggers STATUS_HEAP_CORRUPTION (0xC0000374). Session.stop
// and the readLoop both end up closing the PTY (stop on shutdown / timeout, the
// child exit naturally drops the handle), so a guard here is mandatory.
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

// startPTYWithSize starts cmd in a Windows ConPTY with the given dimensions.
// ConPTY manages process creation internally, so we build a command line from
// cmd.Args, hand it to conpty.Start, then resolve the PID back into cmd.Process
// so the supervise goroutine can Wait() / Kill() through the usual exec.Cmd API.
func startPTYWithSize(cmd *exec.Cmd, cols, rows uint16) (ptyHandle, error) {
	cmdLine := resolveConPtyCmdLine(cmd)

	opts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(int(cols), int(rows)),
	}
	if cmd.Dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(cmd.Dir))
	}
	if cmd.Env != nil {
		opts = append(opts, conpty.ConPtyEnv(cmd.Env))
	}

	cpty, err := conpty.Start(cmdLine, opts...)
	if err != nil {
		return nil, err
	}

	pid := cpty.Pid()
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		_ = cpty.Close()
		return nil, fmt.Errorf("find conpty process %d: %w", pid, err)
	}
	cmd.Process = proc

	return &windowsPTY{cpty: cpty}, nil
}

// resolveConPtyCmdLine produces the command line conpty.Start should run.
//
// Win32 CreateProcessW (which ConPTY uses internally) doesn't apply PATHEXT
// and can't execute .cmd / .bat scripts directly. exec.Command already
// PATHEXT-resolved cmd.Path for us, so we substitute it for the bare name in
// cmd.Args[0], and we wrap batch scripts with "cmd.exe /c" so the interpreter
// handles them. Without this, npm-installed login CLIs (claude.cmd,
// gemini.cmd, etc.) fail to launch under ConPTY with "Failed to create
// console process: The system cannot find the file specified". See also the
// agentctl process package's identical fix for passthrough sessions.
func resolveConPtyCmdLine(cmd *exec.Cmd) string {
	args := cmd.Args
	switch {
	case len(args) == 0 && cmd.Path == "":
		return ""
	case len(args) == 0:
		args = []string{cmd.Path}
	case cmd.Path != "":
		args = append([]string{cmd.Path}, args[1:]...)
	}
	if isBatchScript(args[0]) {
		args = append([]string{"cmd.exe", "/c"}, args...)
	}
	return buildCmdLine(args)
}

// isBatchScript reports whether path ends in .cmd or .bat — the two
// CreateProcessW-incompatible Windows script extensions npm shims may use.
func isBatchScript(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}
