package process

import (
	"os"
	osExec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultShellCommand_CustomShell(t *testing.T) {
	cmd := defaultShellCommand("/bin/sh")
	if cmd[0] != "/bin/sh" {
		t.Errorf("expected preferred shell as first element, got %q", cmd[0])
	}
}

func TestDefaultShellCommand_EmptyPreferred(t *testing.T) {
	cmd := defaultShellCommand("")
	if len(cmd) == 0 {
		t.Fatal("expected non-empty command")
	}
	// On Unix the shell must be invoked WITHOUT -l so /etc/profile doesn't
	// reset PATH (which would drop /data/.npm-global/bin where agent CLIs
	// installed via the Settings → Agents install button live).
	if runtime.GOOS != "windows" {
		for _, a := range cmd[1:] {
			if a == "-l" {
				t.Errorf("expected no -l flag on Unix shell, got %v", cmd)
			}
		}
	}
}

func TestDefaultShellCommand_InvalidPreferredFallsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fallback behavior")
	}
	originalShell := os.Getenv("SHELL")
	defer func() { _ = os.Setenv("SHELL", originalShell) }()

	_ = os.Setenv("SHELL", "/bin/sh")
	cmd := defaultShellCommand("/this/path/does/not/exist")
	if cmd[0] != "/bin/sh" {
		t.Fatalf("expected fallback to SHELL (/bin/sh), got %q", cmd[0])
	}
}

func TestShellExecArgs(t *testing.T) {
	prog, args := shellExecArgs("echo hello")
	if prog == "" {
		t.Fatal("expected non-empty program")
	}
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	if runtime.GOOS == "windows" {
		if args[len(args)-1] != "echo hello" {
			t.Errorf("expected command string in args, got prog=%q args=%v", prog, args)
		}
		return
	}
	if !strings.Contains(args[len(args)-1], "echo hello") {
		t.Errorf("expected command string in shell wrapper, got prog=%q args=%v", prog, args)
	}
}

func TestShellExecArgsRestoresManagedGitHubPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell wrapper only")
	}
	_, args := shellExecArgs("echo hello")
	command := args[len(args)-1]
	for _, want := range []string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL",
		"KANDEV_GITHUB_CLI_SHIM_DIR",
		"PATH=\"${KANDEV_GITHUB_CLI_SHIM_DIR}:${PATH:-}\"",
		"echo hello",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("shell command missing %q: %s", want, command)
		}
	}
}

func TestShellExecArgsRestoresManagedGitHubPathInProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell wrapper only")
	}
	root := t.TempDir()
	shimDir := filepath.Join(root, "managed github shim")
	homeDir := filepath.Join(root, "home")
	if err := os.MkdirAll(shimDir, 0o700); err != nil {
		t.Fatalf("create shim directory: %v", err)
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("create home directory: %v", err)
	}
	marker := filepath.Join(shimDir, "kandev-marker")
	if err := os.WriteFile(marker, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write marker executable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".profile"), []byte("PATH=/usr/bin:/bin\nexport PATH\n"), 0o700); err != nil {
		t.Fatalf("write login profile: %v", err)
	}

	prog, args := shellExecArgs("command -v kandev-marker")
	command := osExec.Command(prog, args...)
	command.Env = []string{
		"HOME=" + homeDir,
		"PATH=" + shimDir + ":/usr/bin:/bin",
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL=https://kandev.example/resolve",
		"KANDEV_GITHUB_CLI_SHIM_DIR=" + shimDir,
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("managed shell command failed: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != marker {
		t.Fatalf("marker resolution = %q, want %q", output, marker)
	}
}
