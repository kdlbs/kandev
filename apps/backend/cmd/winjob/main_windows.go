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
	"unsafe"

	"golang.org/x/sys/windows"
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

	job, err := createKillOnCloseJob()
	if err != nil {
		die("create job: %v", err)
	}
	// Don't `defer CloseHandle(job)` — we want the OS to close it as part of
	// process teardown so KILL_ON_JOB_CLOSE fires correctly on any exit path
	// including os.Exit and panics. Manual close on success is also fine
	// because the child has already exited by then.

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		die("start child: %v", err)
	}

	if err := assignProcessToJob(job, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = windows.CloseHandle(job)
		die("assign job: %v", err)
	}

	waitErr := cmd.Wait()
	_ = windows.CloseHandle(job)
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		die("wait: %v", waitErr)
	}
}

func createKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return job, nil
}

func assignProcessToJob(job windows.Handle, pid int) error {
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "winjob: "+format+"\n", args...)
	os.Exit(1)
}
