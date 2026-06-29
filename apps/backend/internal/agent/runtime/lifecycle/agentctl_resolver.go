package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// AgentctlResolver finds the path to platform-specific agentctl binaries for
// remote executors.
// Resolution order:
//  1. KANDEV_AGENTCTL_<OS>_<ARCH>_BINARY env var
//  2. KANDEV_AGENTCTL_LINUX_BINARY legacy env var for linux/amd64
//  3. platform-suffixed helper relative to the running binary
//  4. native agentctl when the remote platform matches the control plane
type AgentctlResolver struct {
	logger *logger.Logger
}

// NewAgentctlResolver creates a new resolver.
func NewAgentctlResolver(log *logger.Logger) *AgentctlResolver {
	return &AgentctlResolver{
		logger: log.WithFields(zap.String("component", "agentctl_resolver")),
	}
}

// ResolveLinuxBinary returns the path to a linux/amd64 agentctl binary.
func (r *AgentctlResolver) ResolveLinuxBinary() (string, error) {
	return r.ResolveRemoteBinary(SSHRemotePlatform{GOOS: sshRemoteGOOSLinux, GOARCH: sshRemoteGOARCHAMD64})
}

// ResolveRemoteBinary returns the path to an agentctl binary for the given
// remote platform.
func (r *AgentctlResolver) ResolveRemoteBinary(platform SSHRemotePlatform) (string, error) {
	if err := requireSupportedRemotePlatform(platform); err != nil {
		return "", err
	}
	for _, envName := range agentctlBinaryEnvNames(platform) {
		envPath := os.Getenv(envName)
		if envPath == "" {
			continue
		}
		if _, err := os.Stat(envPath); err == nil {
			r.logger.Debug("using agentctl from env var",
				zap.String("env", envName),
				zap.String("path", envPath),
				zap.String("remote_platform", platform.String()))
			return envPath, nil
		}
		return "", fmt.Errorf("%s=%q does not exist", envName, envPath)
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := agentctlBinaryCandidates(exeDir, platform)
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				abs, _ := filepath.Abs(candidate)
				r.logger.Debug("found agentctl binary",
					zap.String("path", abs),
					zap.String("remote_platform", platform.String()))
				return abs, nil
			}
		}
	}

	return "", fmt.Errorf(
		"agentctl helper for %s not found; build it with 'make build-agentctl-remote' "+
			"or set %s (control plane os=%s arch=%s)",
		platform.String(), agentctlBinaryEnvNames(platform)[0], runtime.GOOS, runtime.GOARCH,
	)
}

func agentctlBinaryName(platform SSHRemotePlatform) string {
	return fmt.Sprintf("agentctl-%s-%s", platform.GOOS, platform.GOARCH)
}

func agentctlBinaryEnvNames(platform SSHRemotePlatform) []string {
	primary := fmt.Sprintf(
		"KANDEV_AGENTCTL_%s_%s_BINARY",
		strings.ToUpper(platform.GOOS),
		strings.ToUpper(platform.GOARCH),
	)
	if platform.GOOS == sshRemoteGOOSLinux && platform.GOARCH == sshRemoteGOARCHAMD64 {
		return []string{primary, "KANDEV_AGENTCTL_LINUX_BINARY"}
	}
	return []string{primary}
}

func agentctlBinaryCandidates(exeDir string, platform SSHRemotePlatform) []string {
	name := agentctlBinaryName(platform)
	candidates := []string{
		filepath.Join(exeDir, name),
		filepath.Join(exeDir, "..", "build", name),
		filepath.Join(exeDir, "..", "bin", name),
	}
	if platform.GOOS == runtime.GOOS && platform.GOARCH == runtime.GOARCH {
		// Same-platform remotes can use the native binary in development even
		// when the suffixed helper has not been built yet. Released bundles are
		// still validated separately and must include every supported helper.
		candidates = append(candidates,
			filepath.Join(exeDir, "agentctl"),
			filepath.Join(exeDir, "..", "build", "agentctl"),
			filepath.Join(exeDir, "..", "bin", "agentctl"),
		)
	}
	return candidates
}
