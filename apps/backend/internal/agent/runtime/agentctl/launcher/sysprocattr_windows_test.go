//go:build windows

package launcher

import (
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/common/winproc"

	"golang.org/x/sys/windows"
)

func TestBuildSysProcAttrHidesConsoleWindow(t *testing.T) {
	attr := buildSysProcAttr(false)
	if attr == nil {
		t.Fatal("buildSysProcAttr returned nil")
	}
	if attr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", attr.CreationFlags)
	}
	if !attr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if got, want := attr.CreationFlags&windows.CREATE_NO_WINDOW, winproc.NoConsoleWindowFlag(); got != want {
		t.Fatalf("CREATE_NO_WINDOW flags = %#x, want %#x", got, want)
	}
}

func TestBuildSysProcAttrStartsSuspendedForJobAssignment(t *testing.T) {
	attr := buildSysProcAttr(false)
	want := uint32(windows.CREATE_NEW_PROCESS_GROUP|windows.CREATE_SUSPENDED) | winproc.NoConsoleWindowFlag()
	if attr.CreationFlags != want {
		t.Fatalf("CreationFlags = %#x, want %#x", attr.CreationFlags, want)
	}
}
