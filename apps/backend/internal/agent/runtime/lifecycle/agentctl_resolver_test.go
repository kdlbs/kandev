package lifecycle

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

func TestAgentctlResolverLinuxAMD64PrefersPrimaryEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	primary := filepath.Join(tmp, "agentctl-linux-amd64-primary")
	legacy := filepath.Join(tmp, "agentctl-linux-amd64-legacy")
	for _, helper := range []string{primary, legacy} {
		if err := os.WriteFile(helper, []byte("stub"), 0o755); err != nil {
			t.Fatalf("write helper %s: %v", helper, err)
		}
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary)
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", legacy)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ResolveRemoteBinary: %v", err)
	}
	if got != primary {
		t.Fatalf("ResolveRemoteBinary = %q, want primary %q", got, primary)
	}
}

func TestAgentctlResolverLinuxAMD64BadPrimaryEnvDoesNotUseLegacyFallback(t *testing.T) {
	tmp := t.TempDir()
	primary := filepath.Join(tmp, "missing-agentctl")
	legacy := filepath.Join(tmp, "agentctl-linux-amd64-legacy")
	if err := os.WriteFile(legacy, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write legacy helper: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary)
	t.Setenv("KANDEV_AGENTCTL_LINUX_BINARY", legacy)

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	got, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("ResolveRemoteBinary = %q, want error", got)
	}
	for _, want := range []string{"KANDEV_AGENTCTL_LINUX_AMD64_BINARY", primary} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestAgentctlResolverRemoteBinaryNotFoundErrorNamesPlatformAndEnv(t *testing.T) {
	t.Setenv("KANDEV_AGENTCTL_DARWIN_AMD64_BINARY", "")

	resolver := NewAgentctlResolver(newResolverTestLogger(t))
	_, err := resolver.ResolveRemoteBinary(SSHRemotePlatform{GOOS: "darwin", GOARCH: "amd64"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"darwin/amd64", "KANDEV_AGENTCTL_DARWIN_AMD64_BINARY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestAgentctlBinaryCandidatesIncludesRemoteHelpersAndHostFallback(t *testing.T) {
	exeDir := filepath.Join("tmp", "kandev", "bin")
	platform := SSHRemotePlatform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}

	got := agentctlBinaryCandidates(exeDir, platform)
	name := agentctlBinaryName(platform)
	want := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "build", name),
		filepath.Join(exeDir, "..", "bin", name),
		filepath.Join(exeDir, "agentctl"),
		filepath.Join(exeDir, "..", "build", "agentctl"),
		filepath.Join(exeDir, "..", "bin", "agentctl"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentctlBinaryCandidates = %#v, want %#v", got, want)
	}
}

func TestAgentctlBinaryCandidatesOmitsHostFallbackForDifferentPlatform(t *testing.T) {
	exeDir := filepath.Join("tmp", "kandev", "bin")
	platform := SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		platform = SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	}

	got := agentctlBinaryCandidates(exeDir, platform)
	name := agentctlBinaryName(platform)
	want := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "build", name),
		filepath.Join(exeDir, "..", "bin", name),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentctlBinaryCandidates = %#v, want %#v", got, want)
	}
}
