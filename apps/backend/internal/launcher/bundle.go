package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type runtimeBundle struct {
	Dir      string
	Launcher string
	Source   string
}

func resolveRuntimeBundle() (runtimeBundle, error) {
	dir := os.Getenv("KANDEV_BUNDLE_DIR")
	if dir == "" {
		return runtimeBundle{}, fmt.Errorf("no Kandev runtime found; KANDEV_BUNDLE_DIR is not set")
	}
	return validateRuntimeBundle(dir, "env")
}

func validateRuntimeBundle(dir, source string) (runtimeBundle, error) {
	launcher := filepath.Join(dir, "bin", executableName("kandev"))
	if !exists(launcher) {
		return runtimeBundle{}, fmt.Errorf("launcher binary not found in bundle at %s", launcher)
	}
	agentctl := filepath.Join(dir, "bin", executableName("agentctl"))
	if !exists(agentctl) {
		return runtimeBundle{}, fmt.Errorf("agentctl binary not found in bundle at %s", agentctl)
	}
	agentctlLinuxAMD64 := filepath.Join(dir, "bin", "agentctl-linux-amd64")
	if requiresAgentctlLinuxAMD64(runtime.GOOS, runtime.GOARCH) && !exists(agentctlLinuxAMD64) {
		return runtimeBundle{}, fmt.Errorf("agentctl linux/amd64 helper not found in bundle at %s", agentctlLinuxAMD64)
	}
	return runtimeBundle{Dir: dir, Launcher: launcher, Source: source}, nil
}

func requiresAgentctlLinuxAMD64(goos, goarch string) bool {
	return goos != "linux" || goarch != "amd64"
}

func executableName(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}
