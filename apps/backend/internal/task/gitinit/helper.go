// Package gitinit runs Git initialization inside an inherited directory descriptor.
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
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "git init helper requires the Git executable path")
		return 2, true
	}
	return runInheritedDirectory(args[1]), true
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
