package lifecycle

import (
	"os/exec"
	"syscall"
)

// setRawCommandLine passes line to CreateProcess unchanged, as Win32 OpenSSH
// does when it runs `cmd.exe /c <request>`.
func setRawCommandLine(cmd *exec.Cmd, line string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
}
