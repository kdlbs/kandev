package copilotplugins

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var errUnsupportedLaunchContext = errors.New("unsupported copilot plugin launch context")

const windowsOS = "windows"

// inventoryQualifiedVersions lists the managed Copilot packages whose inventory
// format, installation metadata, and repeated mount syntax were reproduced.
// An unlisted version returns a typed unsupported result instead of falling
// back to a globally installed CLI.
var inventoryQualifiedVersions = map[string]struct{}{
	"1.0.88":  {},
	"1.0.104": {},
}

// policyVerifiedVersions lists runtimes for which native fixtures established
// that an explicit --plugin-dir mount preserves native MCP disables and managed
// prohibitions. No runtime has that evidence yet, so nonempty starts stay
// fail-closed while discovery remains available.
var policyVerifiedVersions = map[string]struct{}{}

// RuntimeSupported reports whether safe inventory discovery is qualified for
// the selected managed runtime version.
func RuntimeSupported(version string) bool {
	_, ok := inventoryQualifiedVersions[strings.TrimSpace(version)]
	return ok
}

// MountsPermitted reports whether explicit plugin mounts are permitted for the
// selected runtime. Inventory qualification is deliberately separate.
func MountsPermitted(version string) bool {
	_, ok := policyVerifiedVersions[strings.TrimSpace(version)]
	return ok
}

// RequireMountPolicy returns a typed policy_unverified error until native
// evidence proves that explicit mounts preserve the runtime's vetoes.
func RequireMountPolicy(version string) error {
	if MountsPermitted(version) {
		return nil
	}
	return &Error{Reason: ReasonPolicyUnverified}
}

// RuntimeVersionFromCommand extracts the exact @github/copilot package version
// from an already-prepared argv. Binding discovery to this command prevents a
// runtime activation between command construction and plugin resolution.
func RuntimeVersionFromCommand(argv []string) (string, bool) {
	const packagePrefix = "@github/copilot@"
	for _, arg := range argv {
		if strings.HasPrefix(arg, packagePrefix) {
			version := strings.TrimPrefix(arg, packagePrefix)
			if version != "" && !strings.ContainsAny(version, " \t/") {
				return version, true
			}
		}
	}
	return "", false
}

func (c LaunchContext) validate() error {
	if c.AgentType != ProviderID {
		return errUnsupportedLaunchContext
	}
	if !SupportedExecutor(c.ExecutorType) {
		return errUnsupportedLaunchContext
	}
	if len(c.RuntimeCommand) == 0 {
		return errUnsupportedLaunchContext
	}
	if len(c.CommandPrefix) > 0 {
		// The adapter has no proven wrapper-equivalent context for inventory
		// or plugin roots. Fail before native commands or filesystem reads.
		return errUnsupportedLaunchContext
	}
	if _, _, err := configDirForLaunch(c); err != nil {
		return err
	}
	return nil
}

// SupportedExecutor admits only backend-owned local/worktree execution,
// including the legacy local_pc representation.
func SupportedExecutor(executorType string) bool {
	switch strings.TrimSpace(executorType) {
	case "", "local", "local_pc", "worktree":
		return true
	default:
		return false
	}
}

// configDirForLaunch applies the documented precedence: a qualified
// `--config-dir` selector from the profile, then COPILOT_HOME, then the
// effective user home plus `.copilot`.
func configDirForLaunch(launch LaunchContext) (string, bool, error) {
	env := effectiveEnvironment(launch.Env)
	if configDir, ok, err := configDirFromCLIFlags(launch.CLIFlags); err != nil {
		return "", false, err
	} else if ok {
		return absoluteConfigDir(configDir)
	}
	if configDir, ok := environmentValue(env, "COPILOT_HOME", runtime.GOOS == windowsOS); ok && strings.TrimSpace(configDir) != "" {
		return absoluteConfigDir(configDir)
	}
	home, ok := environmentValue(env, "HOME", runtime.GOOS == windowsOS)
	if !ok || strings.TrimSpace(home) == "" {
		home, ok = environmentValue(env, "USERPROFILE", runtime.GOOS == windowsOS)
	}
	if !ok || strings.TrimSpace(home) == "" {
		return "", false, errUnsupportedLaunchContext
	}
	home = strings.TrimSpace(home)
	if !filepath.IsAbs(home) {
		return "", false, errUnsupportedLaunchContext
	}
	return filepath.Join(home, ".copilot"), false, nil
}

// absoluteConfigDir rejects relative native state directories. The inventory
// command and the installation-state reader must resolve one directory; a
// relative value would resolve against different working directories.
func absoluteConfigDir(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) {
		return "", false, errUnsupportedLaunchContext
	}
	return filepath.Clean(value), true, nil
}

func effectiveEnvironment(overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(overlay)+16)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && key != "" {
			merged[key] = value
		}
	}
	for key, value := range overlay {
		if key != "" && !strings.ContainsAny(key, "=\x00") && !strings.ContainsRune(value, '\x00') {
			merged[key] = value
		}
	}
	return merged
}

func configDirFromCLIFlags(flags []string) (string, bool, error) {
	value := ""
	found := false
	for index := 0; index < len(flags); index++ {
		flag := flags[index]
		if flag == "--" {
			break
		}
		switch {
		case flag == "--config-dir":
			if index+1 >= len(flags) || strings.TrimSpace(flags[index+1]) == "" {
				return "", false, errUnsupportedLaunchContext
			}
			index++
			if found && value != strings.TrimSpace(flags[index]) {
				return "", false, errUnsupportedLaunchContext
			}
			value = strings.TrimSpace(flags[index])
			found = true
		case strings.HasPrefix(flag, "--config-dir="):
			next := strings.TrimSpace(strings.TrimPrefix(flag, "--config-dir="))
			if next == "" {
				return "", false, errUnsupportedLaunchContext
			}
			if found && value != next {
				return "", false, errUnsupportedLaunchContext
			}
			value = next
			found = true
		}
	}
	return value, found, nil
}
