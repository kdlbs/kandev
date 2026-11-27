//go:build windows

package codexdbg

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureProcessTreeHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	configureProcessTree(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("configureProcessTree did not set process attributes")
	}
	flags := cmd.SysProcAttr.CreationFlags
	if flags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", flags)
	}
	if flags&windows.CREATE_SUSPENDED == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_SUSPENDED", flags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if flags&windows.CREATE_NO_WINDOW != 0 {
		t.Fatalf("CreationFlags = %#x, must not include CREATE_NO_WINDOW", flags)
	}
}
