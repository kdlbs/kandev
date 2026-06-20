package launcher

import (
	"slices"
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
	if !strings.Contains(plist, "<key>RunAtLoad</key>\n  <true/>") {
		t.Fatalf("plist should start at load by default:\n%s", plist)
	}
}

func TestRenderLaunchdPlistCanDisableBootStart(t *testing.T) {
	plist := renderLaunchdPlist(nativeServiceUnitInput{
		Executable:  "/opt/kandev/bin/kandev",
		HomeDir:     "/Users/alice/.kandev",
		LogDir:      "/Users/alice/.kandev/logs",
		NoBootStart: true,
	})

	if !strings.Contains(plist, "<key>RunAtLoad</key>\n  <false/>") {
		t.Fatalf("plist should not start at load with no boot start:\n%s", plist)
	}
}

func TestBuildJournalArgs(t *testing.T) {
	tests := []struct {
		name string
		args serviceArgs
		want []string
	}{
		{
			name: "user logs",
			args: serviceArgs{Action: actionLogs},
			want: []string{"--user-unit", "kandev.service", "-n", "200", "--no-pager"},
		},
		{
			name: "user logs follow keeps line count",
			args: serviceArgs{Action: actionLogs, Follow: true},
			want: []string{"--user-unit", "kandev.service", "-n", "200", "-f"},
		},
		{
			name: "system logs",
			args: serviceArgs{Action: actionLogs, System: true},
			want: []string{"-u", "kandev.service", "-n", "200", "--no-pager"},
		},
		{
			name: "system logs follow keeps line count",
			args: serviceArgs{Action: actionLogs, System: true, Follow: true},
			want: []string{"-u", "kandev.service", "-n", "200", "-f"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildJournalArgs(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("buildJournalArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
