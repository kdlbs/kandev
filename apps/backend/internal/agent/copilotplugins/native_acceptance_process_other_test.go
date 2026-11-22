//go:build copilot_native && !(darwin || linux)

package copilotplugins_test

import (
	"os/exec"
	"time"
)

func configureNativeProcessGroup(*exec.Cmd) error { return nil }

func terminateNativeProcessGroup(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if grace > 0 {
		time.Sleep(grace)
	}
	return cmd.Process.Kill()
}

func killNativeProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
