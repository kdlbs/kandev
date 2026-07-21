package process

import (
	"fmt"
	"os/exec"
	"time"
)

const failedStartReapTimeout = 2 * time.Second

func killAndWaitStartedCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(failedStartReapTimeout):
		return fmt.Errorf("process %d was not reaped after failed startup", cmd.Process.Pid)
	}
}
