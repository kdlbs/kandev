package launcher

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var lookupLocalAgentUser = user.LookupId

func withLocalAgentPath(env []string) []string {
	if runtime.GOOS == goosWindows {
		return env
	}
	home := localAgentHome(env)
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

func localAgentHome(env []string) string {
	if processEnvValue(env, "KANDEV_SERVICE_MODE") == nativeServiceModeSystem {
		// A LaunchDaemon's inherited HOME may still name the installing user.
		account, err := lookupLocalAgentUser(strconv.Itoa(os.Geteuid()))
		if err != nil {
			return ""
		}
		return account.HomeDir
	}
	home := processEnvValue(env, "HOME")
	if !filepath.IsAbs(home) {
		home, _ = os.UserHomeDir()
	}
	return home
}
