package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSystemdServicePathExecutesNPMGlobalCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("systemd service PATH is POSIX-only")
	}
	home := t.TempDir()
	binDir := filepath.Join(home, ".npm-global", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const cli = "kandev-test-npm-global-agent"
	wantPath := filepath.Join(binDir, cli)
	if err := os.WriteFile(wantPath, []byte("#!/bin/sh\nprintf 'npm-global-agent-ok'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	unit := renderSystemdUnit(nativeServiceUnitInput{
		Executable: "/opt/kandev/bin/kandev",
		HomeDir:    filepath.Join(home, ".kandev"),
	})
	var servicePath string
	for _, line := range strings.Split(unit, "\n") {
		value, ok := strings.CutPrefix(line, "Environment=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		value, ok = strings.CutPrefix(value, "PATH=")
		if !ok {
			continue
		}
		// %h is the systemd home-dir specifier; it differs from input.HomeDir (kandev data dir).
		servicePath = strings.ReplaceAll(value, "%h", home)
		break
	}
	if servicePath == "" {
		t.Fatal("generated unit has no PATH environment entry")
	}
	t.Setenv("PATH", servicePath)
	resolved, err := exec.LookPath(cli)
	if err != nil {
		t.Fatalf("global npm CLI is not discoverable with generated service PATH: %v", err)
	}
	if resolved != wantPath {
		t.Fatalf("resolved CLI = %q, want %q", resolved, wantPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, cli).CombinedOutput()
	if err != nil {
		t.Fatalf("execute global npm CLI: %v: %s", err, output)
	}
	if string(output) != "npm-global-agent-ok" {
		t.Fatalf("CLI output = %q", output)
	}
}
