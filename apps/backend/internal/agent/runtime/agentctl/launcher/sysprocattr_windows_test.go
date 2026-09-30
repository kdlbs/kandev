//go:build windows

package launcher

import (
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBuildSysProcAttrRequestsNoConsoleWindow(t *testing.T) {
	attr := buildSysProcAttr(false)
	if attr == nil {
		t.Fatal("buildSysProcAttr returned nil")
	}
	if attr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", attr.CreationFlags)
	}
	if attr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", attr.CreationFlags)
	}
}
