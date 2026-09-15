package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func withLocalAgentPath(env []string) []string {
	if runtime.GOOS == goosWindows {
		return env
	}
	home := processEnvValue(env, "HOME")
	if !filepath.IsAbs(home) {
		home, _ = os.UserHomeDir()
	}
	if !filepath.IsAbs(home) || strings.ContainsRune(home, os.PathListSeparator) {
		return env
	}
	localBin := filepath.Join(home, ".local", "bin")
	path := processEnvValue(env, "PATH")
	for _, dir := range filepath.SplitList(path) {
		if filepath.Clean(dir) == localBin {
			return env
		}
	}
	// Installers may create this directory after the backend and agentctl start.
	// Append it so explicitly configured executables keep their precedence.
	if path != "" {
		path += string(os.PathListSeparator)
	}
	return upsertEnv(env, "PATH", path+localBin)
}
