package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPrepareScriptKubernetesUsesContainerWorkspaceContract(t *testing.T) {
	script := DefaultPrepareScript("k8s")
	if script == "" {
		t.Fatal("DefaultPrepareScript(\"k8s\") returned empty")
	}
	for _, want := range []string{
		"{{repository.clone_url}}",
		"{{repository.branch}}",
		"{{workspace.path}}",
		"{{repository.setup_script}}",
		"{{kandev.agents.install}}",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("Kubernetes default prepare script missing %q", want)
		}
	}
}

func TestDefaultPrepareScriptKubernetesReusesRetainedPVCWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	_, origin := setupPostludeRepo(t, "main")
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "lost+found"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	cloneTmp := filepath.Join(root, "runtime", "workspace-clone")
	if err := os.MkdirAll(filepath.Dir(cloneTmp), 0o700); err != nil {
		t.Fatal(err)
	}

	script := strings.NewReplacer(
		"{{git.identity_setup}}", ":",
		"{{github.auth_setup}}", ":",
		"{{repository.branch}}", shellQuote("main"),
		"{{repository.clone_url}}", shellQuote(origin),
		"{{workspace.path}}", shellQuote(workspace),
		"{{repository.setup_script}}", ":",
		"{{kandev.agents.install}}", ":",
		"/opt/kandev/.workspace-clone", cloneTmp,
	).Replace(DefaultPrepareScript("k8s"))
	run := func() {
		t.Helper()
		cmd := exec.Command("sh", "-eu", "-c", script)
		cmd.Env = append(os.Environ(), "HOME="+home)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Kubernetes prepare failed: %v\n%s", err, output)
		}
	}

	run()
	localFile := filepath.Join(workspace, "local-untracked.txt")
	if err := os.WriteFile(localFile, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}
	run()

	if data, err := os.ReadFile(localFile); err != nil || string(data) != "preserve me" {
		t.Fatalf("retained workspace data = %q, %v", data, err)
	}
	if output := runIn(t, workspace, "git", "branch", "--show-current"); strings.TrimSpace(string(output)) != "main" {
		t.Fatalf("workspace branch = %q", output)
	}
}

func TestDefaultPrepareScriptKubernetesRejectsRetainedWorkspaceFromDifferentRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	workspace, _ := setupPostludeRepo(t, "main")
	_, expectedOrigin := setupPostludeRepo(t, "main")
	root := t.TempDir()
	home := filepath.Join(root, "home")
	requireNoError(t, os.MkdirAll(home, 0o700))
	marker := filepath.Join(workspace, "local-untracked.txt")
	requireNoError(t, os.WriteFile(marker, []byte("preserve me"), 0o600))

	script := strings.NewReplacer(
		"{{git.identity_setup}}", ":",
		"{{github.auth_setup}}", ":",
		"{{repository.branch}}", shellQuote("main"),
		"{{repository.clone_url}}", shellQuote(expectedOrigin),
		"{{workspace.path}}", shellQuote(workspace),
		"{{repository.setup_script}}", ":",
		"{{kandev.agents.install}}", ":",
		"/opt/kandev/.workspace-clone", filepath.Join(root, "runtime", "workspace-clone"),
	).Replace(DefaultPrepareScript("k8s"))
	cmd := exec.Command("sh", "-eu", "-c", script)
	cmd.Env = append(os.Environ(), "HOME="+home)
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("Kubernetes prepare unexpectedly reused a checkout from another repository")
	}
	if !strings.Contains(string(output), "origin does not match") {
		t.Fatalf("prepare output = %q, want repository origin mismatch", output)
	}
	data, readErr := os.ReadFile(marker)
	if readErr != nil || string(data) != "preserve me" {
		t.Fatalf("retained workspace data = %q, %v", data, readErr)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
