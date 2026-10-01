package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

func TestManagedGitToolsActivation(t *testing.T) {
	shimDir := filepath.Join(t.TempDir(), "managed github shims")
	startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
	firstParent := filepath.Join(t.TempDir(), "first parent.sh")
	secondParent := filepath.Join(t.TempDir(), "second parent.sh")
	pathKey := pathEnvKey
	pathValue := strings.Join([]string{"/usr/bin", "/bin"}, string(filepath.ListSeparator))
	if runtime.GOOS == windowsOS {
		pathKey = "Path"
	}
	env := map[string]string{
		pathKey:     pathValue,
		"BASH_ENV":  "${HOOK_ROOT}/first parent.sh",
		"HOOK_ROOT": filepath.Dir(firstParent),
	}

	ActivateManagedGitTools(env, shimDir, startupEnv)
	DeactivateManagedGitTools(env, shimDir, startupEnv)
	ActivateManagedGitTools(env, shimDir, startupEnv)
	wantPath := strings.Join([]string{shimDir, "/usr/bin", "/bin"}, string(filepath.ListSeparator))
	if got := env[pathKey]; got != wantPath {
		t.Fatalf("repeated activation PATH = %q, want %q", got, wantPath)
	}
	requireManagedGitBashEnv(t, env, "${HOOK_ROOT}/first parent.sh", startupEnv, firstParent)

	// A caller may replace PATH or the parent hook between configurations.
	env[pathKey] = strings.Join([]string{"/custom/bin", "/bin"}, string(filepath.ListSeparator))
	env["BASH_ENV"] = secondParent
	ActivateManagedGitTools(env, shimDir, startupEnv)
	wantPath = strings.Join([]string{shimDir, "/custom/bin", "/bin"}, string(filepath.ListSeparator))
	if got := env[pathKey]; got != wantPath {
		t.Fatalf("activation after replacement PATH = %q, want %q", got, wantPath)
	}
	requireManagedGitBashEnv(t, env, secondParent, startupEnv, secondParent)

	DeactivateManagedGitTools(env, shimDir, startupEnv)
	if got := env[pathKey]; got != strings.Join([]string{"/custom/bin", "/bin"}, string(filepath.ListSeparator)) {
		t.Fatalf("deactivated PATH = %q, want replacement path without the shim", got)
	}
	if runtime.GOOS != windowsOS && env["BASH_ENV"] != secondParent {
		t.Fatalf("deactivated BASH_ENV = %q, want parent hook %q", env["BASH_ENV"], secondParent)
	}

	// Deactivation only restores a hook that still points at Kandev's wrapper.
	env["BASH_ENV"] = "/user/changed-after-activation.sh"
	ActivateManagedGitTools(env, shimDir, startupEnv)
	env["BASH_ENV"] = "/user/unrelated-hook.sh"
	DeactivateManagedGitTools(env, shimDir, startupEnv)
	if runtime.GOOS != windowsOS && env["BASH_ENV"] != "/user/unrelated-hook.sh" {
		t.Fatalf("deactivation changed an unrelated BASH_ENV: %q", env["BASH_ENV"])
	}

	withoutPath := map[string]string{githubauth.CredentialCLIShimDirEnv: shimDir}
	DeactivateManagedGitTools(withoutPath, shimDir, "")
	if _, exists := withoutPath[pathKey]; exists {
		t.Fatalf("deactivation created an executable path that was absent: %v", withoutPath)
	}
}

func requireManagedGitBashEnv(t *testing.T, env map[string]string, wantWindows, wantUnix, wantParent string) {
	t.Helper()
	wantBashEnv := wantUnix
	wantParentEnv := wantParent
	if runtime.GOOS == windowsOS {
		wantBashEnv = wantWindows
		wantParentEnv = ""
	}
	if got := env[githubauth.CredentialParentBashEnv]; got != wantParentEnv {
		t.Fatalf("parent Bash environment = %q, want %q", got, wantParentEnv)
	}
	if got := env["BASH_ENV"]; got != wantBashEnv {
		t.Fatalf("BASH_ENV = %q, want %q", got, wantBashEnv)
	}
}
