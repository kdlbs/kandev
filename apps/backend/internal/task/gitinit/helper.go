// Package gitinit runs Git commands inside an inherited directory descriptor.
package gitinit

import (
	"fmt"
	"os"
	"strings"
)

const (
	helperArgument            = "__git-init-open-directory"
	helperEnvironmentVariable = "KANDEV_INTERNAL_GIT_INIT_HELPER"
)

func runHelper(args []string) (int, bool) {
	if len(args) == 0 || args[0] != helperArgument {
		return 0, false
	}
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "git init helper requires Git arguments")
		return 2, true
	}
	return runInheritedDirectory(args[1], args[2:]), true
}

func withHelperEnvironment(environment []string) []string {
	filtered := withoutHelperEnvironment(environment)
	return append(filtered, helperEnvironmentVariable+"=1")
}

func withoutHelperEnvironment(environment []string) []string {
	prefix := helperEnvironmentVariable + "="
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func withIsolatedCommitEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found && isGitCommitEnvironmentKey(key) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
	)
}

func isGitCommitEnvironmentKey(key string) bool {
	upperKey := strings.ToUpper(key)
	if strings.HasPrefix(upperKey, "GIT_CONFIG_") || upperKey == "GIT_CONFIG" {
		return true
	}
	switch upperKey {
	case "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL":
		return true
	default:
		return false
	}
}
