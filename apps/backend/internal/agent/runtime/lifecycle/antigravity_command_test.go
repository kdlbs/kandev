package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestAntigravityLocalLaunchPolicy(t *testing.T) {
	m := &Manager{}
	base := []string{"agy_acp_server.par", "--uid="}
	policy, _, err := m.applyLocalRuntimeCommand(context.Background(), agents.NewAntigravityACP(),
		agentruntime.RuntimeDocker, base, nil)
	if err != nil {
		t.Fatalf("non-local resolution error: %v", err)
	}
	if policy != nil {
		t.Fatal("docker launch received a personal auth policy")
	}
	if base[0] != "agy_acp_server.par" {
		t.Fatalf("non-local command changed to %q", base[0])
	}
}

func TestAntigravityCommandLocality(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	dir := t.TempDir()
	serverName, harnessName := "agy_acp_server.par", "localharness_external"
	if runtime.GOOS == "windows" {
		serverName, harnessName = "agy_acp_server.exe", "localharness_external.exe"
	}
	server := filepath.Join(dir, serverName)
	if err := os.WriteFile(server, []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, harnessName), []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	args := []string{serverName, "--uid="}
	m := &Manager{}
	policy, _, err := m.applyLocalRuntimeCommand(context.Background(), agents.NewAntigravityACP(),
		agentruntime.RuntimeStandalone, args, nil)
	if err != nil {
		t.Fatalf("local resolution error: %v", err)
	}
	if policy == nil || policy.MethodID != "oauth-personal" || policy.AllowInteractive {
		t.Fatalf("local policy = %+v", policy)
	}
	if args[0] != server {
		t.Fatalf("local command = %q, want %q", args[0], server)
	}
}
