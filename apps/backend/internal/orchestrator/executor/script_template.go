package executor

import "github.com/kandev/kandev/internal/scriptengine"

// ResolveScript replaces {{key}} placeholders with values from the vars map.
//
// Deprecated: Use scriptengine.ResolveScript directly.
func ResolveScript(script string, vars map[string]string) string {
	return scriptengine.ResolveScript(script, vars)
}
