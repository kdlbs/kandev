//go:build windows

package acpdbg

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureProcessTreeStartsSuspended(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	configureProcessTree(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("configureProcessTree did not set process attributes")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_SUSPENDED", cmd.SysProcAttr.CreationFlags)
	}
	if cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", cmd.SysProcAttr.CreationFlags)
	}
}

func TestConfigureProcessTreeHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	configureProcessTree(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("configureProcessTree did not set process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW != 0 {
		t.Fatalf("CreationFlags = %#x, must not include CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
