//go:build !linux

package launcher

import "os/exec"

func startManagedCommand(cmd *exec.Cmd, _ string) (func() error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}
