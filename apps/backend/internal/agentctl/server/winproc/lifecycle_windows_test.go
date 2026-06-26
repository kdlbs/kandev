//go:build windows

package winproc

import (
	"os/exec"
	"testing"
)

func TestIsTaskkillMissing(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{name: "empty", msg: "", want: false},
		{name: "not found", msg: "ERROR: The process \"123\" not found.", want: true},
		{name: "not be found", msg: "ERROR: The process with PID 123 could not be found.", want: true},
		{name: "no running instance", msg: "ERROR: No running instance of the task.", want: true},
		{name: "other error", msg: "ERROR: Access is denied.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTaskkillMissing(tt.msg); got != tt.want {
				t.Fatalf("IsTaskkillMissing(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestInstallKillOnCloseJobForSuspendedCommandRejectsNilCommand(t *testing.T) {
	if _, err := InstallKillOnCloseJobForSuspendedCommand(nil); err == nil {
		t.Fatal("InstallKillOnCloseJobForSuspendedCommand(nil) error = nil, want non-nil")
	}
	if _, err := InstallKillOnCloseJobForSuspendedCommand(&exec.Cmd{}); err == nil {
		t.Fatal("InstallKillOnCloseJobForSuspendedCommand(cmd without process) error = nil, want non-nil")
	}
}
