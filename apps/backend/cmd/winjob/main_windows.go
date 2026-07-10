//go:build windows

// winjob spawns a child program inside a Windows Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so the OS terminates the child (and its
// own descendants, which inherit the job) the moment winjob itself exits — by
// Ctrl-C, by parent kill, by crash, by anything.
//
// It exists because the chain that runs `make dev` on Windows from Git Bash
// (bash → make → pnpm → node → make → sh → kandev) drops signals between
// MSYS-aware and native-Win32 processes. Even if Node's tree-kill supervisor
// runs, the chain has multiple links that can leak processes when Ctrl-C only
// reaches the top-level shell. Wrapping the backend spawn in winjob makes
// cleanup a kernel-level guarantee instead of a "please forward" chain.
//
// On Unix this binary is a transparent passthrough (see main_other.go) — POSIX
// process groups already give us reliable cascading termination.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/kandev/kandev/internal/agentctl/server/winproc"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: winjob <program> [args...]")
		os.Exit(2)
	}

	// Ignore Ctrl-C in winjob itself. The child receives CTRL_C_EVENT via the
	// shared console (Go's default handler exits on it). When the child exits,
	// winjob proceeds past cmd.Wait and closes the job handle. If winjob took
	// the Ctrl-C first and exited before the child, KILL_ON_JOB_CLOSE would
	// still fire — but giving the child a chance to clean up is friendlier
	// for processes that handle SIGINT themselves (e.g. flush state, save a
	// session).
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	go func() {
		for range signalCh {
			// Drop the signal; nothing to do — the child got it too.
		}
	}()

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		die("start child: %v", err)
	}

	job, err := winproc.InstallKillOnCloseJobForCommand(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		die("assign job: %v", err)
	}
	// Don't `defer job.Close()` — we want the OS to close it as part of
	// process teardown so KILL_ON_JOB_CLOSE fires correctly on any exit path
	// including os.Exit and panics. Manual close on success is also fine
	// because the child has already exited by then.

	waitErr := cmd.Wait()
	_ = job.Close()
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		die("wait: %v", waitErr)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "winjob: "+format+"\n", args...)
	os.Exit(1)
}
