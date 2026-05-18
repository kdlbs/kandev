package acp

import "github.com/coder/acp-go-sdk"

// AuthMethodFields collapses upstream's tagged-union acp.AuthMethod
// ({Agent, Terminal, EnvVar}) into the (id, name, description, meta) tuple
// that callers want before storing or transmitting. Adding a new variant
// here is the single place that needs to learn about it.
func AuthMethodFields(m acp.AuthMethod) (id, name string, description *string, meta map[string]any) {
	switch {
	case m.Agent != nil:
		return m.Agent.Id, m.Agent.Name, m.Agent.Description, m.Agent.Meta
	case m.Terminal != nil:
		return m.Terminal.Id, m.Terminal.Name, m.Terminal.Description, m.Terminal.Meta
	case m.EnvVar != nil:
		return m.EnvVar.Id, m.EnvVar.Name, m.EnvVar.Description, m.EnvVar.Meta
	}
	return "", "", nil, nil
}
