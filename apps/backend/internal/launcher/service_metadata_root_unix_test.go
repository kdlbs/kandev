//go:build unix

package launcher

import "testing"

func TestCanonicalSystemMetadataHome(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		homeDir string
		want    string
	}{
		{
			name:    "darwin var root",
			goos:    goosDarwin,
			homeDir: "/var",
			want:    "/private/var",
		},
		{
			name:    "darwin var descendant",
			goos:    goosDarwin,
			homeDir: "/var/lib/kandev",
			want:    "/private/var/lib/kandev",
		},
		{
			name:    "darwin non-prefix",
			goos:    goosDarwin,
			homeDir: "/various/lib/kandev",
			want:    "/various/lib/kandev",
		},
		{
			name:    "darwin cleaned traversal",
			goos:    goosDarwin,
			homeDir: "/tmp/../var/lib/../lib/kandev",
			want:    "/private/var/lib/kandev",
		},
		{
			name:    "linux identity",
			goos:    goosLinux,
			homeDir: "/var/lib/../lib/kandev",
			want:    "/var/lib/kandev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalSystemMetadataHome(tt.goos, tt.homeDir); got != tt.want {
				t.Fatalf("canonicalSystemMetadataHome(%q, %q) = %q, want %q", tt.goos, tt.homeDir, got, tt.want)
			}
		})
	}
}
