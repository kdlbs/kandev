package lifecycle

import (
	"errors"
	"fmt"
)

// errSSHWindowsLaunchUnsupported marks a launch configuration that has no
// Windows execution path.
var errSSHWindowsLaunchUnsupported = errors.New("ssh: unsupported Windows launch")

// errSSHPowerShellRequiresWindows marks a Linux or macOS profile saved with a
// PowerShell shell. Every PowerShell script Kandev generates targets Windows.
var errSSHPowerShellRequiresWindows = errors.New("ssh: PowerShell shells require a Windows host")

// validateSSHWindowsLaunch rejects, before any remote preparation, the launch
// modes a host cannot run: passthrough or a non-PowerShell shell on Windows,
// and a PowerShell shell on Linux or macOS.
func validateSSHWindowsLaunch(req *ExecutorCreateRequest, platform SSHRemotePlatform) error {
	shell := sshShellFromMetadata(req.Metadata)
	if !isWindowsRemotePlatform(platform) {
		if shell != "" && isPowerShellShell(shell) {
			return fmt.Errorf("%w: shell %q is not supported on %s hosts; use a POSIX login shell such as bash or zsh",
				errSSHPowerShellRequiresWindows, shell, platform.GOOS)
		}
		return nil
	}
	if req.IsPassthrough {
		return fmt.Errorf("%w: passthrough mode is unavailable on Windows hosts", errSSHWindowsLaunchUnsupported)
	}
	if shell != "" && !isPowerShellShell(shell) {
		return fmt.Errorf("%w: shell %q is not supported on Windows hosts; use powershell or pwsh",
			errSSHWindowsLaunchUnsupported, shell)
	}
	return nil
}
