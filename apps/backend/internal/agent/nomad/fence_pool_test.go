package nomad

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func poolFenceState(t *testing.T, root, envID string) EnvironmentState {
	t.Helper()
	s := placePool(poolState("allocated"))
	s.EnvironmentID = envID
	s.StateRoot = root
	s.WorkspaceRoot = filepath.Join(root, "workspaces")
	s.AllocationRoot = filepath.Join(root, "pool", "allocations")
	path, err := EnvironmentWorkspacePath(s.WorkspaceRoot, envID, s.Generation)
	if err != nil {
		t.Fatal(err)
	}
	s.WorkspacePath = path
	s.AllocationID, s.AllocationCreateIndex = "alloc-"+envID, 12
	if err := s.Validate(); err != nil {
		t.Fatalf("state: %v", err)
	}
	return s
}

func TestFenceDirIsPerEnvironmentForPoolAndWorkerWideForLegacy(t *testing.T) {
	root := t.TempDir()
	a, b := poolFenceState(t, root, "env-a"), poolFenceState(t, root, "env-b")
	if got, want := FenceDir(a, a.AllocationRoot), filepath.Join(root, "environments", "env-a", "active"); got != want {
		t.Fatalf("pool fence = %q, want %q", got, want)
	}
	if FenceDir(a, a.AllocationRoot) == FenceDir(b, b.AllocationRoot) {
		t.Fatal("two pool environments share a fence")
	}
	legacy := destroyState()
	if got := FenceDir(legacy, "/x/root/alloc"); got != "/x/worker-state/"+legacy.NodeID+"/active-environment" {
		t.Fatalf("legacy fence = %q", got)
	}
}

// runBootstrap executes the generated wrapper against a stub agentctl and returns its exit code.
func runBootstrap(t *testing.T, s EnvironmentState, allocDir string) int {
	t.Helper()
	script, err := BuildBootstrapScript(s, s.AllocationRoot)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	agentctl := filepath.Join(dir, "agentctl")
	stub := "#!/bin/sh\nprintf '%s' \"$TMPDIR\" > " + filepath.Join(dir, "tmpdir") + "\n"
	if err := os.WriteFile(agentctl, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(dir, "env")
	if err := os.WriteFile(envFile, []byte("KANDEV_WORKSPACE_ROOT="+s.WorkspacePath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{
		"PATH=/usr/bin:/bin", "KANDEV_AGENTCTL_PATH=" + agentctl, "KANDEV_BOOTSTRAP_ENV_FILE=" + envFile,
		"KANDEV_WORKSPACE_ROOT=" + s.WorkspacePath, "NOMAD_PORT_agentctl_control=1", "NOMAD_PORT_agentctl_instance=2",
		"NOMAD_JOB_ID=" + s.JobID, "NOMAD_ALLOC_ID=" + s.AllocationID, "KANDEV_NOMAD_NODE_ID=" + s.NodeID, "NOMAD_ALLOC_DIR=" + allocDir,
	}
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "tmpdir")); string(got) != filepath.Join(allocDir, "tmp") {
		t.Fatalf("agentctl TMPDIR = %q, want it inside the allocation directory", got)
	}
	return 0
}

func TestPoolBootstrapFencesEachEnvironmentAlone(t *testing.T) {
	root := t.TempDir()
	a, b := poolFenceState(t, root, "env-a"), poolFenceState(t, root, "env-b")
	a.JobID, b.JobID = "job-a", "job-b"
	if code := runBootstrap(t, a, t.TempDir()); code != 0 {
		t.Fatalf("environment a exit %d", code)
	}
	if code := runBootstrap(t, b, t.TempDir()); code != 0 {
		t.Fatalf("environment b blocked by environment a: exit %d", code)
	}
	if code := runBootstrap(t, a, t.TempDir()); code != 73 {
		t.Fatalf("the same environment started twice: exit %d, want 73", code)
	}
}

func TestPoolFenceReleaseTouchesOnlyItsOwnEnvironment(t *testing.T) {
	root := t.TempDir()
	a, b := poolFenceState(t, root, "env-a"), poolFenceState(t, root, "env-b")
	a.JobID, b.JobID = "job-a", "job-b"
	for _, s := range []EnvironmentState{a, b} {
		if code := runBootstrap(t, s, t.TempDir()); code != 0 {
			t.Fatalf("%s exit %d", s.EnvironmentID, code)
		}
		// The stub bootstrap owner record is the real one; align it with the cleanup token.
		fence := FenceDir(s, s.AllocationRoot)
		if err := os.WriteFile(filepath.Join(fence, "owner"), []byte(fenceOwnerToken(s)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command, err := FenceReleaseCommand(PoolProfileFromState(a).EffectiveProfile(a), a)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("release a: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "environments", "env-a")); !os.IsNotExist(err) {
		t.Fatalf("environment a fence directory remains: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(FenceDir(b, b.AllocationRoot), "owner")); err != nil || !strings.Contains(string(got), "alloc-env-b") {
		t.Fatalf("environment b fence changed: %v %q", err, got)
	}
}

func TestPoolWorkspaceRemoveDeletesOnlyItsOwnWorkspace(t *testing.T) {
	root := t.TempDir()
	a, b := poolFenceState(t, root, "env-a"), poolFenceState(t, root, "env-b")
	for _, s := range []EnvironmentState{a, b} {
		if err := os.MkdirAll(filepath.Join(s.WorkspacePath, "repo"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	command, err := WorkspaceRemoveCommand(a)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("remove %d: %v %s", i, err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(a.WorkspaceRoot, "environments", "env-a")); !os.IsNotExist(err) {
		t.Fatalf("environment a workspace remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.WorkspacePath, "repo")); err != nil {
		t.Fatalf("environment b workspace changed: %v", err)
	}
}

func TestPoolWorkspaceRemoveRefusesSymlinksForeignPathsAndLegacy(t *testing.T) {
	root := t.TempDir()
	s := poolFenceState(t, root, "env-a")
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(s.WorkspacePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, s.WorkspacePath); err != nil {
		t.Fatal(err)
	}
	command, err := WorkspaceRemoveCommand(s)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Command("sh", "-c", command).Run() == nil {
		t.Fatal("a symlinked workspace was removed")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
	foreign := s
	foreign.WorkspacePath = filepath.Join(root, "elsewhere", "g1")
	if _, err := WorkspaceRemoveCommand(foreign); err == nil {
		t.Fatal("a workspace outside the environments directory was accepted")
	}
	if command, err := WorkspaceRemoveCommand(destroyState()); err != nil || command != "" {
		t.Fatalf("legacy workspace removal = %q, %v; want none", command, err)
	}
}
