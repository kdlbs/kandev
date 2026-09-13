package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFullWorkerPreparationContract(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../k8s/worker-images/full/prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, part := range []string{"{{repository.clone_url}}", "{{repository.setup_script}}", "{{kandev.agents.install}}", "retained workspace repository origin", "non-empty but is not a valid checkout"} {
		if !strings.Contains(script, part) {
			t.Fatalf("missing preparation contract %s", part)
		}
	}
	if strings.Index(script, "docker info") > strings.Index(script, "git clone") {
		t.Fatal("daemon wait must precede clone")
	}
	if strings.Index(script, "create_workspace_caches\n") > strings.Index(script, "{{repository.setup_script}}") {
		t.Fatal("caches must be writable before source setup")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "DOCKER_READY_TIMEOUT_SECONDS=1")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Docker daemon did not become ready") {
		t.Fatalf("readiness failure: %v %s", err, output)
	}
	if ctx.Err() != nil {
		t.Fatal("preparation exceeded its own readiness deadline")
	}
}

func TestFullWorkerPreparationClonesBeforeCachesAndRetainsWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	_, origin := setupPostludeRepo(t, "main")
	root := t.TempDir()
	raw, err := os.ReadFile("../../../../../../k8s/worker-images/full/prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
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
		"{{repository.setup_script}}", `test -d "$workspace/.cache/go-build" && test -d "$workspace/.npm-global"`,
		"{{kandev.agents.install}}", ":",
		"/opt/kandev/.workspace-clone", cloneTmp,
	).Replace(string(raw))
	run := func() {
		t.Helper()
		cmd := exec.Command("sh", "-eu", "-c", script)
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
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
