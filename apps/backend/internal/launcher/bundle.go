package launcher

import (
	"fmt"
	"os"
	"path/filepath"
)

type runtimeBundle struct {
	Dir      string
	Launcher string
	Source   string
}

type agentctlRemoteHelper struct {
	Name  string
	Label string
}

var requiredAgentctlRemoteHelpers = []agentctlRemoteHelper{
	{Name: "agentctl-linux-amd64", Label: "agentctl linux/amd64 helper"},
	{Name: "agentctl-darwin-arm64", Label: "agentctl darwin/arm64 helper"},
	{Name: "agentctl-darwin-amd64", Label: "agentctl darwin/amd64 helper"},
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
	for _, helper := range requiredAgentctlRemoteHelpers {
		path := filepath.Join(dir, "bin", helper.Name)
		if !exists(path) {
			return runtimeBundle{}, fmt.Errorf("%s not found in bundle at %s", helper.Label, path)
		}
	}
	return runtimeBundle{Dir: dir, Launcher: launcher, Source: source}, nil
}

func executableName(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}
