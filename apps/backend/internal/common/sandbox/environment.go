package sandbox

import (
	"slices"
	"strings"
)

const (
	environmentHome       = "HOME"
	environmentBashEnv    = "BASH_ENV"
	environmentShellEnv   = "ENV"
	environmentTempDir    = "TMPDIR"
	environmentPath       = "PATH"
	environmentConfigHome = "XDG_CONFIG_HOME"
	environmentStateHome  = "XDG_STATE_HOME"
)

func canonicalEnvironmentKeys(keys []string) ([]string, error) {
	if len(keys) > 128 {
		return nil, ErrInvalidPolicy
	}
	out := slices.Clone(keys)
	for _, key := range out {
		if !validEnvironmentKey(key) || privateEnvironmentKey(key) {
			return nil, ErrInvalidPolicy
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func validEnvironmentKey(key string) bool {
	if key == "" || len(key) > 128 {
		return false
	}
	for i, char := range key {
		allowed := char == '_' || (char >= 'A' && char <= 'Z') || (i > 0 && char >= '0' && char <= '9')
		if !allowed {
			return false
		}
	}
	return true
}

func privateEnvironmentKey(key string) bool {
	for _, prefix := range []string{"KANDEV_", "LD_", "DYLD_", "XDG_", "GIT_CONFIG_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return slices.Contains([]string{environmentHome, environmentTempDir, "TMP", "TEMP", environmentPath, environmentBashEnv, environmentShellEnv, "NODE_OPTIONS", "PYTHONPATH", "PYTHONHOME", "RUBYOPT", "PERL5OPT", "SHELLOPTS", "BASHOPTS"}, key)
}
