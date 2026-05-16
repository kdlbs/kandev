package configloader

import (
	"fmt"
	"strings"
)

// validateWorkspaceName rejects names that could escape the base workspaces
// directory or otherwise produce a path outside the intended root. Callers
// supply this name (via HTTP / config / API), so every public method that
// joins it into a filesystem path must validate first — both for correctness
// and to satisfy CodeQL's "uncontrolled data in path expression" findings.
func validateWorkspaceName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid workspace name: %q", name)
	}
	if strings.ContainsAny(name, "/\\\x00") || strings.Contains(name, "..") {
		return fmt.Errorf("invalid workspace name: %q", name)
	}
	return nil
}
