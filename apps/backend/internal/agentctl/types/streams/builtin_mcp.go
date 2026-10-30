package streams

import (
	"slices"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

// BuiltinMCPAuthority is resolved from authenticated execution state. An agent
// request cannot construct or replace this dispatch context.
type BuiltinMCPAuthority struct {
	Required bool
	Manifest *p.BuiltinManifest
	Actions  []string
}

func (a BuiltinMCPAuthority) Allows(action string) bool {
	return !a.Required || (a.Manifest != nil && slices.Contains(a.Actions, action))
}

func (a BuiltinMCPAuthority) Clone() BuiltinMCPAuthority {
	a.Manifest = a.Manifest.Clone()
	a.Actions = slices.Clone(a.Actions)
	return a
}
