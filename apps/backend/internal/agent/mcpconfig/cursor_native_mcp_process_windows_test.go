//go:build windows

package mcpconfig

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/common/winproc"

	"golang.org/x/sys/windows"
)

func TestPrepareNativeMCPProcessHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command(os.Args[0], "mcp", "list")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS}
	if err := prepareNativeMCPProcess(cmd); err != nil {
		t.Fatalf("prepareNativeMCPProcess: %v", err)
	}
	flags := cmd.SysProcAttr.CreationFlags
	for _, want := range []uint32{
		windows.BELOW_NORMAL_PRIORITY_CLASS,
		syscall.CREATE_NEW_PROCESS_GROUP,
		windows.CREATE_SUSPENDED,
	} {
		if flags&want == 0 {
			t.Fatalf("CreationFlags = %#x, want %#x set", flags, want)
		}
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if got, want := flags&windows.CREATE_NO_WINDOW, winproc.NoConsoleWindowFlag(); got != want {
		t.Fatalf("CREATE_NO_WINDOW flags = %#x, want %#x", got, want)
	}

	console := exec.Command(os.Args[0], "mcp", "list")
	console.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := prepareNativeMCPProcess(console); err == nil {
		t.Fatal("prepareNativeMCPProcess accepted CREATE_NEW_CONSOLE")
	}
}
