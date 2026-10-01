//go:build windows

package launcher

import (
	"syscall"
	"testing"

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
	if attr.CreationFlags&windows.CREATE_NO_WINDOW != 0 {
		t.Fatalf("CreationFlags = %#x, must not include CREATE_NO_WINDOW", attr.CreationFlags)
	}
}
