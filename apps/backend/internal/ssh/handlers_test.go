package ssh

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestReadinessShellForRequest(t *testing.T) {
	cases := []struct {
		name     string
		shell    string
		platform lifecycle.SSHRemotePlatform
		want     string
	}{
		{
			name:     "explicit shell wins",
			shell:    " fish ",
			platform: lifecycle.SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"},
			want:     "fish",
		},
		{
			name:     "darwin defaults to zsh",
			platform: lifecycle.SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"},
			want:     "zsh",
		},
		{
			name:     "linux defaults to bash",
			platform: lifecycle.SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"},
			want:     "bash",
		},
		{
			name:     "windows defaults to powershell",
			platform: lifecycle.SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"},
			want:     "powershell",
		},
		{
			name: "unknown defaults to bash",
			want: "bash",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readinessShellForRequest(tc.shell, tc.platform); got != tc.want {
				t.Fatalf("readinessShellForRequest() = %q, want %q", got, tc.want)
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.3
func TestSSHWindowsShellProbeOffersOnlyPowerShell(t *testing.T) {
	winShells, winProbe := candidateShellsForPlatform(lifecycle.SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"})
	if winProbe != "powershell" {
		t.Fatalf("windows probe shell = %q, want powershell", winProbe)
	}
	if want := []string{"powershell", "pwsh"}; !reflect.DeepEqual(winShells, want) {
		t.Fatalf("windows candidate shells = %v, want %v", winShells, want)
	}

	posixShells, posixProbe := candidateShellsForPlatform(lifecycle.SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if posixProbe != "sh" {
		t.Fatalf("linux probe shell = %q, want sh", posixProbe)
	}
	if len(posixShells) == 0 || posixShells[0] != "bash" {
		t.Fatalf("linux candidate shells = %v, want bash first", posixShells)
	}
}
