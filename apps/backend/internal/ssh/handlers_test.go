package ssh

import (
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
