package launcher

import (
	"fmt"
	"os"
	"path/filepath"
)

type runtimeBundle struct {
	Dir       string
	Launcher  string
	WebServer string
	Source    string
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
	webServer, err := resolveBundleWebServerPath(dir)
	if err != nil {
		return runtimeBundle{}, err
	}
	return runtimeBundle{Dir: dir, Launcher: launcher, WebServer: webServer, Source: source}, nil
}

func resolveBundleWebServerPath(bundleDir string) (string, error) {
	candidates := []string{
		filepath.Join(bundleDir, "web", "server.js"),
		filepath.Join(bundleDir, "web", "web", "server.js"),
		filepath.Join(bundleDir, "web", "apps", "web", "server.js"),
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("web server (server.js) not found in bundle at %s", bundleDir)
}

func executableName(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}
