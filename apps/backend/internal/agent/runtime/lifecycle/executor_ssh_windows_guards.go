package lifecycle

import (
	"errors"
	"fmt"
)

// errSSHWindowsLaunchUnsupported marks a launch configuration that has no
// Windows execution path.
var errSSHWindowsLaunchUnsupported = errors.New("ssh: unsupported Windows launch")

// validateSSHWindowsLaunch rejects, before any remote preparation, the launch
// modes a Windows host cannot run: passthrough and a non-PowerShell shell.
func validateSSHWindowsLaunch(req *ExecutorCreateRequest, platform SSHRemotePlatform) error {
	if !isWindowsRemotePlatform(platform) {
		return nil
	}
	if req.IsPassthrough {
		return fmt.Errorf("%w: passthrough mode is unavailable on Windows hosts", errSSHWindowsLaunchUnsupported)
	}
	if shell := sshShellFromMetadata(req.Metadata); shell != "" && !isPowerShellShell(shell) {
		return fmt.Errorf("%w: shell %q is not supported on Windows hosts; use powershell or pwsh",
			errSSHWindowsLaunchUnsupported, shell)
	}
	return nil
}
