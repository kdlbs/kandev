//go:build windows

package utility

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func configureRuntimeObservationCommand(
	cmd *exec.Cmd,
	executable string,
	args []string,
	_ []string,
) error {
	extension := filepath.Ext(executable)
	if !strings.EqualFold(extension, ".cmd") && !strings.EqualFold(extension, ".bat") {
		return nil
	}
	if !runtimeObservationCmdTextSafe(executable) {
		return errors.New("runtime command shim path is unsupported")
	}
	for _, arg := range args {
		if !runtimeObservationCmdTextSafe(arg) {
			return errors.New("runtime command shim argument is unsupported")
		}
	}
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		return errors.New("Windows system root is unavailable")
	}
	shell := filepath.Join(systemRoot, "System32", "cmd.exe")
	if _, err := os.Stat(shell); err != nil {
		return errors.New("Windows command interpreter is unavailable")
	}
	command := "call \"" + executable + "\""
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			command += " \"" + arg + "\""
		} else {
			command += " " + arg
		}
	}
	cmd.Path = shell
	cmd.Args = []string{shell, "/d", "/s", "/c", command}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = syscall.EscapeArg(shell) + " /d /s /c " + command
	return nil
}

func runtimeObservationCmdTextSafe(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\x00\r\n\"%&|<>()^!")
}
