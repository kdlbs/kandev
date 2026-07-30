// Package gitinit runs Git initialization inside an inherited directory descriptor.
package gitinit

import (
	"fmt"
	"os"
)

const helperArgument = "__git-init-open-directory"

// RunHelper handles the internal inherited-directory command when requested.
func RunHelper(args []string) (int, bool) {
	if len(args) == 0 || args[0] != helperArgument {
		return 0, false
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "git init helper requires the Git executable path")
		return 2, true
	}
	return runInheritedDirectory(args[1]), true
}
