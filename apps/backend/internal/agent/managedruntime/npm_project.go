package managedruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NPMProjectPrefix keeps npm's project configuration out of the task workspace.
// os/exec does not expand ~; npm does, and the launcher provisions that same
// expanded directory before starting the child process.
const NPMProjectPrefix = "~/.kandev/managed-npm-runtime"

// NPMProjectPrefixArgs returns the trusted npm project root arguments used by
// every managed runtime command.
func NPMProjectPrefixArgs() []string {
	return []string{"--prefix", NPMProjectPrefix}
}

// EnsureNPMProjectPrefixForCommand creates the fixed project root when args
// select it. It uses the environment that will be given to the npm child.
func EnsureNPMProjectPrefixForCommand(args []string, env map[string]string) error {
	if !hasNPMProjectPrefix(args) {
		return nil
	}
	return ensureNPMProjectPrefix(homeFromMap(env))
}

// EnsureNPMProjectPrefixInEnvironment is the slice-environment form used by
// direct subprocess launchers.
func EnsureNPMProjectPrefixInEnvironment(args, env []string) error {
	if !hasNPMProjectPrefix(args) {
		return nil
	}
	return ensureNPMProjectPrefix(homeFromEnvironment(env))
}

func hasNPMProjectPrefix(args []string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--prefix" && args[index+1] == NPMProjectPrefix {
			return true
		}
	}
	return false
}

func homeFromMap(env map[string]string) string {
	if home := homeForNPM(runtime.GOOS, env["HOME"], env["USERPROFILE"]); home != "" {
		return home
	}
	return defaultHome()
}

func homeFromEnvironment(env []string) string {
	home := ""
	userProfile := ""
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		switch key {
		case "HOME":
			home = value
		case "USERPROFILE":
			userProfile = value
		}
	}
	if home := homeForNPM(runtime.GOOS, home, userProfile); home != "" {
		return home
	}
	return defaultHome()
}

func homeForNPM(goos, home, userProfile string) string {
	first, second := home, userProfile
	if goos == "windows" {
		first, second = userProfile, home
	}
	if strings.TrimSpace(first) != "" {
		return first
	}
	if strings.TrimSpace(second) != "" {
		return second
	}
	return ""
}

func defaultHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func ensureNPMProjectPrefix(home string) error {
	if home == "" || !filepath.IsAbs(home) {
		return errors.New("managed npm project prefix could not be prepared")
	}
	path := filepath.Join(home, ".kandev", "managed-npm-runtime")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return errors.New("managed npm project prefix could not be prepared")
	}
	return nil
}
