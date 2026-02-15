//go:build windows

package process

import "os"

// defaultShellCommand returns the command and args for starting an interactive shell.
// On Windows, uses %COMSPEC% (typically cmd.exe) or falls back to powershell.exe.
// No -l flag since Windows shells don't use login shell mode.
func defaultShellCommand(preferredShell string) []string {
	shell := preferredShell
	if shell == "" {
		shell = os.Getenv("COMSPEC")
	}
	if shell == "" {
		shell = "powershell.exe"
	}
	return []string{shell}
}

// shellExecArgs returns the program and arguments needed to execute a command
// string through the system shell.
// On Windows: cmd /c "command"
func shellExecArgs(command string) (prog string, args []string) {
	return "cmd", []string{"/c", command}
}
