package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/mcpmode"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpproviders "github.com/kandev/kandev/internal/mcp/providers"
)

// updateBuiltinProfile serializes control updates with dispatch admission.
// Persist an unavailable state before remote mutation so an interrupted update
// cannot recover with broader stale authority. The package selection is frozen.
func (m *Manager) updateBuiltinProfile(ctx context.Context, execution *AgentExecution, change func(*p.Context) error, apply func() error) error {
	execution.builtinMCPMu.Lock()
	defer execution.builtinMCPMu.Unlock()
	value, bound := execution.metadataValue(p.BuiltinBindingMetadataKey)
	if !bound {
		return apply()
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return err
	}
	if err := change(profile); err != nil {
		return err
	}
	encoded, err := p.MarshalBuiltinBinding(*profile)
	if err != nil {
		return err
	}
	if encoded == value {
		return apply()
	}
	pending := map[string]any{"update_incomplete": true}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, pending)
	if err := m.persistBuiltinMCPBinding(ctx, execution); err != nil {
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, value)
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	if err := m.persistBuiltinMCPBinding(ctx, execution); err != nil {
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, pending)
		return err
	}
	return nil
}

func builtinModeUpdate(mode string) func(*p.Context) error {
	return func(profile *p.Context) error {
		if p.Legacy(mode, false, nil).Surface != profile.Surface {
			return errors.New("built-in MCP surface change requires a new process")
		}
		if mode == mcpmode.TaskTitlePending {
			*profile = profile.WithCapability(p.CapabilityTaskTitle)
		} else {
			*profile = profile.WithoutCapability(p.CapabilityTaskTitle)
		}
		return nil
	}
}

func builtinProviderUpdate(providers []string) func(*p.Context) error {
	return func(profile *p.Context) error { profile.Providers = mcpproviders.Normalize(providers); return nil }
}
