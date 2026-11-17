package lifecycle

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var errProviderMCPScope = errors.New("provider MCP scope is not current")

// beginProviderMCPEffect counts admitted backend work through its final effect.
// Retirement and acceptance share providerEffectMu without holding it across I/O.
func (e *AgentExecution) beginProviderMCPEffect(msg *ws.Message) (func(), *p.ProviderMCPScope, error) {
	if e == nil {
		return func() {}, nil, nil
	}
	e.providerEffectMu.Lock()
	defer e.providerEffectMu.Unlock()
	if e.providerMCPEffectsClosed() {
		return nil, nil, errProviderMCPScope
	}
	scope, err := e.matchProviderMCPScope(msg)
	if err != nil {
		return nil, nil, err
	}
	e.providerEffects++
	release := sync.OnceFunc(func() { e.providerEffectMu.Lock(); e.providerEffects--; e.providerEffectMu.Unlock() })
	return release, scope, nil
}

func (e *AgentExecution) matchProviderMCPScope(msg *ws.Message) (*p.ProviderMCPScope, error) {
	if msg == nil {
		return nil, errProviderMCPScope
	}
	value, present := e.metadataValue(p.ProviderMCPBindingMetadataKey)
	encoded := msg.Metadata[p.ProviderMCPScopeMetadataKey]
	if !present {
		if encoded != "" {
			return nil, errProviderMCPScope
		}
		return nil, nil
	}
	binding, profile, err := p.ParseProviderMCPBinding(value)
	if err != nil || binding.Scope.ExecutionID != e.ID || e.DeliveryHarnessGeneration == 0 || profile.BuiltinPackages.TaskID != e.TaskID || profile.BuiltinPackages.WorkspaceID != e.WorkspaceID {
		return nil, errProviderMCPScope
	}
	var received p.ProviderMCPScope
	if json.Unmarshal([]byte(encoded), &received) != nil || received != binding.Scope {
		return nil, errProviderMCPScope
	}
	return &received, nil
}

// closeProviderMCPEffects is non-destructive. The coordinator must persist this
// barrier before retiring the helper scope or stopping its provider process.
func (e *AgentExecution) closeProviderMCPEffects() error {
	e.providerEffectMu.Lock()
	defer e.providerEffectMu.Unlock()
	if e.providerEffects != 0 {
		return errors.New("provider MCP effects are still active")
	}
	e.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, true)
	return nil
}

func (e *AgentExecution) providerMCPAuthority(value any, resolve BuiltinActionResolver) streams.BuiltinMCPAuthority {
	authority := streams.BuiltinMCPAuthority{Required: true}
	if resolve == nil {
		return authority
	}
	binding, profile, err := p.ParseProviderMCPBinding(value)
	if err != nil || binding.Scope.ExecutionID != e.ID || profile.BuiltinPackages.TaskID != e.TaskID || profile.BuiltinPackages.WorkspaceID != e.WorkspaceID {
		return authority
	}
	runtimeValue, present := e.metadataValue(p.ProviderMCPRuntimeProfileMetadataKey)
	profile, err = p.ProviderRuntimeProfile(binding, runtimeValue, present)
	if err != nil {
		return authority
	}
	actions, err := resolve(*profile)
	if err != nil {
		return authority
	}
	authority.Manifest = profile.BuiltinPackages.Clone()
	authority.Actions = actions
	return authority
}

func (e *AgentExecution) providerMCPEffectsClosed() bool {
	value, present := e.metadataValue(p.ProviderMCPEffectsClosedMetadataKey)
	if !present {
		return false
	}
	closed, valid := value.(bool)
	return !valid || closed
}
