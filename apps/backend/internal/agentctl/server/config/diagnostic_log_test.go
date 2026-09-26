package config

import (
	"path/filepath"
	"testing"
)

// TestResolveDiagnosticLogPath pins AC-EXECUTORS-SURVIVAL-001.3: the
// diagnostic log lives under HomeDir/logs with a fixed filename, and is
// empty only when HomeDir itself could not be resolved -- there is no
// separate configuration key or environment variable for this path.
func TestResolveDiagnosticLogPath(t *testing.T) {
	tests := []struct {
		name    string
		homeDir string
		want    string
	}{
		{
			name:    "derives from home dir",
			homeDir: "/home/kandev-test/.kandev",
			want:    filepath.Join("/home/kandev-test/.kandev", "logs", "agentctl-diagnostic.log"),
		},
		{
			name:    "empty home dir yields empty path",
			homeDir: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDiagnosticLogPath(tt.homeDir); got != tt.want {
				t.Fatalf("resolveDiagnosticLogPath(%q) = %q, want %q", tt.homeDir, got, tt.want)
			}
		})
	}
}

// TestResolveDiagnosticLogPathDiffersFromBackendLogFile pins the "different
// file from the one the backend writes its own logs to" half of
// AC-EXECUTORS-SURVIVAL-001.3 -- two processes rotating one file corrupts
// both, so the filename must never collide with the backend's own
// "backend-logs.log".
func TestResolveDiagnosticLogPathDiffersFromBackendLogFile(t *testing.T) {
	got := resolveDiagnosticLogPath("/home/kandev-test/.kandev")
	if filepath.Base(got) == "backend-logs.log" {
		t.Fatalf("resolveDiagnosticLogPath() = %q, filename collides with the backend's own log file", got)
	}
}

// TestLoadDerivesDiagnosticLogPathFromTheSameHomeDir pins that Config.HomeDir
// and Config.DiagnosticLogPath are derived from a single resolution, so they
// can never disagree about which installation they belong to.
func TestLoadDerivesDiagnosticLogPathFromTheSameHomeDir(t *testing.T) {
	cfg := Load()
	if cfg.HomeDir == "" {
		t.Skip("HomeDir could not be resolved in this environment")
	}
	want := filepath.Join(cfg.HomeDir, "logs", "agentctl-diagnostic.log")
	if cfg.DiagnosticLogPath != want {
		t.Fatalf("DiagnosticLogPath = %q, want %q (derived from HomeDir %q)", cfg.DiagnosticLogPath, want, cfg.HomeDir)
	}
}
