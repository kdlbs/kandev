package hostutility

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestAntigravityHostLaunchPolicy(t *testing.T) {
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
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewAntigravityACP()); err != nil {
		t.Fatal(err)
	}
	m := NewManager(reg, "", 0, nil, log)
	command, _, err := m.resolveInferenceCommand(context.Background(), agents.AntigravityACPAgentID,
		agents.NewAntigravityACP(), agents.Command{})
	if err != nil {
		t.Fatalf("resolveInferenceCommand: %v", err)
	}
	if command.Args()[0] != server {
		t.Fatalf("command = %v, want %s", command.Args(), server)
	}
	policy := hostUtilityAuthPolicy(agents.NewAntigravityACP())
	if policy == nil || policy.MethodID != "oauth-personal" || policy.AllowInteractive {
		t.Fatalf("host utility policy = %+v", policy)
	}
}
