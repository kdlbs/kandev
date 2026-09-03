package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestDiagnosticLoggingConfigAppliesTheF71Bounds pins the fixed bound values
// Build picked for AC-EXECUTORS-SURVIVAL-001.3 (MaxSizeMB=16, MaxBackups=10,
// MaxAgeDays=14) and that the level/format/path passed through are used
// verbatim, not silently overridden.
func TestDiagnosticLoggingConfigAppliesTheF71Bounds(t *testing.T) {
	got := diagnosticLoggingConfig("debug", "json", "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log")

	want := logger.LoggingConfig{
		Level:      "debug",
		Format:     "json",
		OutputPath: "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log",
		MaxSizeMB:  16,
		MaxBackups: 10,
		MaxAgeDays: 14,
	}
	if got != want {
		t.Fatalf("diagnosticLoggingConfig() = %+v, want %+v", got, want)
	}
}

// TestDiagnosticLoggingConfigWritesToADurableFile proves the wire contract
// end to end: constructing a real *logger.Logger from diagnosticLoggingConfig
// and writing a line actually lands in the named file, rather than a
// hand-fabricated fixture asserting only on the struct fields.
func TestDiagnosticLoggingConfigWritesToADurableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "agentctl-diagnostic.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	log, err := logger.NewLogger(diagnosticLoggingConfig("info", "json", path))
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	log.Info("agentctl running detached")
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if !strings.Contains(string(contents), "agentctl running detached") {
		t.Fatalf("diagnostic log file contents = %q, want it to contain the logged message", contents)
	}
}

// TestResolveRunLoggingConfigStaysOnStdoutUnlessSurvivalIsEngaged pins the
// Layer 5.9 gate: kill-path #6 ("inherited stdout", see design 01's kill
// paths) is only a problem once the capability is actually active for this
// launch, so every other case -- disabled, or a diagnostic log path that
// somehow failed to resolve -- must keep today's stdout behavior byte-for-
// byte rather than silently switching sinks.
func TestResolveRunLoggingConfigStaysOnStdoutUnlessSurvivalIsEngaged(t *testing.T) {
	cases := []struct {
		name                 string
		agentSurvivalEnabled bool
		diagnosticLogPath    string
		wantStdout           bool
	}{
		{"disabled with a resolved path", false, "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log", true},
		{"enabled but path unresolved", true, "", true},
		{"enabled with a resolved path", true, "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				LogLevel:             "debug",
				LogFormat:            "json",
				AgentSurvivalEnabled: tc.agentSurvivalEnabled,
				DiagnosticLogPath:    tc.diagnosticLogPath,
			}
			got := resolveRunLoggingConfig(cfg)
			if tc.wantStdout {
				want := logger.LoggingConfig{Level: "debug", Format: "json", OutputPath: "stdout"}
				if got != want {
					t.Fatalf("resolveRunLoggingConfig() = %+v, want %+v", got, want)
				}
				return
			}
			want := diagnosticLoggingConfig("debug", "json", tc.diagnosticLogPath)
			if got != want {
				t.Fatalf("resolveRunLoggingConfig() = %+v, want %+v", got, want)
			}
		})
	}
}
