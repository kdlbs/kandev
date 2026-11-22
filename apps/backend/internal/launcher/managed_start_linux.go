//go:build linux

package launcher

import (
	"os/exec"
	"runtime"
	"syscall"
)

const managedBackendLabel = "backend"

func startManagedCommand(cmd *exec.Cmd, label string) (func() error, error) {
	if label != managedBackendLabel || processEnvValue(cmd.Env, "KANDEV_SERVICE_MANAGER") != nativeServiceManagerSystemd {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd.Wait, nil
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
	started := make(chan error)
	waited := make(chan error, 1)
	go func() {
		// Linux binds Pdeathsig to this thread, which must outlive the backend.
		runtime.LockOSThread()
		if err := cmd.Start(); err != nil {
			runtime.UnlockOSThread()
			started <- err
			return
		}
		started <- nil
		err := cmd.Wait()
		runtime.UnlockOSThread()
		waited <- err
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return func() error { return <-waited }, nil
}
