package launcher

import (
	"strings"
	"testing"
)

func TestRenderSystemdUnitExecsNativeKandev(t *testing.T) {
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/home/alice/.kandev",
		LogDir:     "/home/alice/.kandev/logs",
		Port:       1234,
	})
	if !strings.Contains(unit, "ExecStart=/opt/kandev/bin/kandev --headless") {
		t.Fatalf("unit does not exec native kandev:\n%s", unit)
	}
	if strings.Contains(unit, "cli.js") || strings.Contains(unit, "node ") {
		t.Fatalf("unit contains Node CLI launcher:\n%s", unit)
	}
	if !strings.Contains(unit, "Environment=KANDEV_SERVER_PORT=1234") {
		t.Fatalf("unit missing port env:\n%s", unit)
	}
}

func TestRenderLaunchdPlistExecsNativeKandev(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    "/Users/alice/.kandev",
		LogDir:     "/Users/alice/.kandev/logs",
	})
	if !strings.Contains(plist, "<string>/opt/kandev/bin/kandev</string>") {
		t.Fatalf("plist does not exec native kandev:\n%s", plist)
	}
	if strings.Contains(plist, "cli.js") {
		t.Fatalf("plist contains Node CLI launcher:\n%s", plist)
	}
}
