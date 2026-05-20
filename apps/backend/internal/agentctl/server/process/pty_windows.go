//go:build windows

package process

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
// Windows handles and triggers STATUS_HEAP_CORRUPTION (0xC0000374), crashing
// the whole backend. Stop() in interactive_lifecycle.go closes the PTY when
// the user closes a terminal tab, and wait() also closes it once cmd.Wait
// returns — without the gate, both paths hit the same handles.
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

// startPTYWithSize starts the command in a Windows ConPTY with the given dimensions.
// ConPTY manages process creation internally, so this builds a command line from
// the exec.Cmd and starts the process via ConPTY. After this call, cmd.Process
// is set so callers can manage the process lifecycle.
func startPTYWithSize(cmd *exec.Cmd, cols, rows int) (PtyHandle, error) {
	cmdLine := resolveConPtyCmdLine(cmd)

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

// resolveConPtyCmdLine produces the command line conpty.Start should run.
//
// Win32 CreateProcessW (which ConPTY uses internally) has two limitations that
// matter here:
//
//  1. It does NOT apply PATHEXT — a bare "opencode" won't resolve to
//     "opencode.cmd" the way Go's exec.LookPath does. exec.Command on Windows
//     already PATHEXT-resolved cmd.Path for us, but cmd.Args[0] is still the
//     unresolved name. We have to substitute cmd.Path so CreateProcess sees a
//     concrete file.
//
//  2. It cannot execute .cmd / .bat scripts directly — those are interpreted
//     by cmd.exe, not by the Win32 loader. We wrap such commands with
//     "cmd.exe /c <script> <args...>" so the batch interpreter handles them.
//
// Without this, npm-installed CLIs (which install as .cmd shims on Windows)
// fail to launch under ConPTY with "Failed to create console process: The
// system cannot find the file specified" — even though `where.exe` and
// `exec.LookPath` both find them. This is the same family of issue handled in
// apps/cli/src/web.ts for Node's spawn().
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
