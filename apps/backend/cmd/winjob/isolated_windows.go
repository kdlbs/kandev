//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/common/winproc"
	"golang.org/x/sys/windows"
)

// The supervisor owns the job for the child's entire lifetime. It inherits no
// caller pipes, and assigns the suspended child before that child can spawn.
func runIsolated(args []string) (result error) {
	if len(args) < 4 {
		return fmt.Errorf("usage: winjob --isolated <identity> <stdout> <stderr> <program> [args...]")
	}
	stdout, err := os.OpenFile(args[1], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open stdout: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open stderr: %w", err)
	}
	defer stderr.Close()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("open stdin: %w", err)
	}
	defer stdin.Close()
	cmd := exec.Command(args[3], args[4:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start isolated child: %w", err)
	}
	identity, err := readIsolatedIdentity(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	job, err := winproc.InstallKillOnCloseJobForSuspendedCommand(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("assign isolated job: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, job.TerminateAndWait(ctx))
		if cmd.ProcessState == nil {
			result = errors.Join(result, cmd.Wait())
		}
	}()
	if err := writeIsolatedIdentity(args[0], identity); err != nil {
		return err
	}
	return cmd.Wait()
}

func readIsolatedIdentity(pid int) (isolatedIdentity, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return isolatedIdentity{}, fmt.Errorf("open child identity: %w", err)
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return isolatedIdentity{}, fmt.Errorf("read child identity: %w", err)
	}
	// FILETIME counts 100 ns since 1601; .NET ticks count from year 1.
	ticks := (uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)) + 504911232000000000
	// Win32_Process.CreationDate exposes microseconds, rather than 100 ns.
	ticks -= ticks % 10
	return isolatedIdentity{PID: pid, StartedTicks: strconv.FormatUint(ticks, 10)}, nil
}
