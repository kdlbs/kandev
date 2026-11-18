package profile

import (
	"encoding/json"
	"errors"
)

const ProviderMCPBindingMetadataKey = "kandev.provider_mcp_binding"
const ProviderMCPEffectsClosedMetadataKey = "kandev.provider_mcp_effects_closed"

// ProviderMCPBinding preserves the accepted generation separately from the
// retained execution's immutable initial binding. Profile is a sealed snapshot.
type ProviderMCPBinding struct {
	Scope   ProviderMCPScope `json:"scope"`
	Profile string           `json:"profile"`
}

func ParseProviderMCPBinding(value any) (ProviderMCPBinding, *Context, error) {
	var binding ProviderMCPBinding
	var raw []byte
	var err error
	if text, ok := value.(string); ok {
		raw = []byte(text)
	} else {
		raw, err = json.Marshal(value)
	}
	if err != nil {
		return binding, nil, err
	}
	if err = json.Unmarshal(raw, &binding); err != nil {
		return binding, nil, err
	}
	if err = binding.Scope.Validate(); err != nil {
		return binding, nil, err
	}
	profile, err := ParseBuiltinBindingMetadata(binding.Profile)
	if err != nil {
		return binding, nil, err
	}
	digest, err := BuiltinBindingDigest(*profile)
	if err != nil {
		return binding, nil, err
	}
	if digest != binding.Scope.ProfileDigest {
		return binding, nil, errors.New("provider MCP binding digest mismatch")
	}
	return binding, profile, nil
}

const ProviderMCPRuntimeProfileMetadataKey = "kandev.provider_mcp_runtime_profile"

// ProviderRuntimeProfile separates mutable title/provider context from the
// immutable package binding. It cannot change the owner, surface, or manifest.
func ProviderRuntimeProfile(binding ProviderMCPBinding, value any, present bool) (*Context, error) {
	_, base, err := ParseProviderMCPBinding(binding)
	if err != nil {
		return nil, err
	}
	if !present {
		return base, nil
	}
	runtime, err := ParseBuiltinBindingMetadata(value)
	if err != nil {
		return nil, err
	}
	fixed := Normalize(*runtime)
	fixed.Capabilities = base.Capabilities
	fixed.Providers = base.Providers
	digest, err := BuiltinBindingDigest(fixed)
	if err != nil {
		return nil, err
	}
	if digest != binding.Scope.ProfileDigest {
		return nil, errors.New("provider runtime context cannot change its accepted binding")
	}
	return runtime, nil
}

const ProviderMCPOperationMetadataKey = "kandev.provider_mcp_operation"

// IsProviderMCPMetadataKey identifies authority owned by the runtime process.
// Launch preferences and executor configuration cannot supply these fields.
func IsProviderMCPMetadataKey(key string) bool {
	switch key {
	case ProviderMCPBindingMetadataKey, ProviderMCPEffectsClosedMetadataKey, ProviderMCPRuntimeProfileMetadataKey, ProviderMCPOperationMetadataKey:
		return true
	default:
		return false
	}
}
