package profileconfig

import (
	"errors"
	"fmt"
	"strings"
)

// Devin permission intent constants. Mode stays the single ACP authority; the
// sandbox override is an ordinary profile environment entry.
const (
	DevinAgentID         = "devin-acp"
	DevinSandboxEnvKey   = "DEVIN_SANDBOX"
	DevinSmartMode       = "smart"
	DevinBypassMode      = "bypass"
	DevinAutonomousMode  = "autonomous"
	devinSandboxEnabled  = "true"
	devinSandboxDisabled = "false"
)

// ErrInvalidDevinPermissionIntent reports a mode/sandbox pair that the provider
// cannot accept. It fails before any provider process starts.
var ErrInvalidDevinPermissionIntent = errors.New("invalid Devin permission configuration")

// EnvEntry is the neutral view of one profile environment entry. Callers adapt
// their own DTO so this helper stays independent of the settings layers.
type EnvEntry struct {
	Key      string
	Value    string
	SecretID string
}

// DevinSandboxEntry returns the profile's sandbox override when it is present.
func DevinSandboxEntry(entries []EnvEntry) (EnvEntry, bool) {
	for _, entry := range entries {
		if entry.Key != DevinSandboxEnvKey {
			continue
		}
		return entry, true
	}
	return EnvEntry{}, false
}

// ApplyDevinProfileDefaults applies new-profile permission intent to a Devin
// profile. Saved profiles and non-Devin agents are returned unchanged, so this
// must run only from creation paths.
func ApplyDevinProfileDefaults(
	agentID, mode string,
	entries []EnvEntry,
	addDisabledDefault bool,
) (string, []EnvEntry) {
	if agentID != DevinAgentID {
		return mode, entries
	}
	sandbox, hasSandbox := DevinSandboxEntry(entries)
	explicitOverride := hasSandbox && sandbox.SecretID == "" && isDevinSandboxValue(sandbox.Value)
	effectiveMode := strings.TrimSpace(mode)
	if effectiveMode == "" {
		effectiveMode = DevinSmartMode
		if explicitOverride && sandbox.Value == devinSandboxEnabled {
			effectiveMode = DevinAutonomousMode
		}
	}
	if hasSandbox || !addDisabledDefault {
		return effectiveMode, entries
	}
	withDefaults := append([]EnvEntry{}, entries...)
	withDefaults = append(withDefaults, EnvEntry{Key: DevinSandboxEnvKey, Value: devinSandboxDisabled})
	return effectiveMode, withDefaults
}

// ValidateDevinPermissionIntent rejects an explicitly contradicting pair:
// enabled sandbox with Smart/Bypass, or disabled sandbox with Autonomous.
// Inherited, secret-backed, and custom sandbox values stay provider-owned.
func ValidateDevinPermissionIntent(agentID, mode string, entries []EnvEntry) error {
	if agentID != DevinAgentID {
		return nil
	}
	sandbox, ok := DevinSandboxEntry(entries)
	if !ok || sandbox.SecretID != "" || !isDevinSandboxValue(sandbox.Value) {
		return nil
	}
	effectiveMode := strings.ToLower(strings.TrimSpace(mode))
	if sandbox.Value == devinSandboxEnabled && (effectiveMode == DevinSmartMode || effectiveMode == DevinBypassMode) {
		return fmt.Errorf("%w: an enabled sandbox requires the autonomous mode", ErrInvalidDevinPermissionIntent)
	}
	if sandbox.Value == devinSandboxDisabled && effectiveMode == DevinAutonomousMode {
		return fmt.Errorf("%w: the autonomous mode requires an enabled sandbox", ErrInvalidDevinPermissionIntent)
	}
	return nil
}

func isDevinSandboxValue(value string) bool {
	return value == devinSandboxEnabled || value == devinSandboxDisabled
}
