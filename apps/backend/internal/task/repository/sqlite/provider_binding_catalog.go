package sqlite

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	"strings"
)

const builtinCoreRuntimeName = "kandev"

// SQL acceptance also requires selected-package coverage. The helper verifies
// the exact registered names against the live native catalog before this step.
func builtinProviderCatalogComplete(manifest *p.BuiltinManifest, catalog map[string][]string) bool {
	selected := make(map[string]bool, len(manifest.Packages))
	for _, id := range manifest.Packages {
		name := "kandev-" + string(id)
		if id == p.PackageCore {
			name = builtinCoreRuntimeName
		}
		tools := catalog[name]
		if !builtinProviderToolsComplete(tools) {
			return false
		}
		selected[name] = true
	}
	for name := range catalog {
		if p.ReservedBuiltinRuntimeName(name) && !selected[name] {
			return false
		}
	}
	return true
}
func builtinProviderToolsComplete(tools []string) bool {
	if len(tools) == 0 {
		return false
	}
	seen := make(map[string]bool, len(tools))
	for _, name := range tools {
		if strings.TrimSpace(name) == "" || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}
