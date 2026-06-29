package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

func newResolverTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log
}

func TestAgentctlResolverRemoteBinaryUsesPlatformEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "agentctl-darwin-arm64")
	if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_DARWIN_ARM64_BINARY", helper)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != helper {
		t.Fatalf("ResolveRemoteBinary = %q, want %q", got, helper)
	}
}

func TestAgentctlResolverLinuxAMD64KeepsLegacyEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "agentctl-linux-amd64")
	if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", helper)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != helper {
		t.Fatalf("ResolveRemoteBinary = %q, want %q", got, helper)
	}
}
