package profileconfig

import (
	"errors"
	"reflect"
	"testing"
)

func TestApplyDevinProfileDefaults(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		mode     string
		entries  []EnvEntry
		wantMode string
		wantEnv  []EnvEntry
	}{
		{
			name:     "new profile defaults to Smart and an explicit disabled sandbox",
			agentID:  DevinAgentID,
			mode:     "",
			wantMode: DevinSmartMode,
			wantEnv:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}},
		},
		{
			name:     "explicit mode is preserved",
			agentID:  DevinAgentID,
			mode:     DevinBypassMode,
			wantMode: DevinBypassMode,
			wantEnv:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}},
		},
		{
			name:     "enabled sandbox without a mode selects Autonomous",
			agentID:  DevinAgentID,
			mode:     "",
			entries:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}},
			wantMode: DevinAutonomousMode,
			wantEnv:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}},
		},
		{
			name:     "explicit disabled sandbox keeps the Smart default",
			agentID:  DevinAgentID,
			mode:     "",
			entries:  []EnvEntry{{Key: "OTHER", Value: "1"}, {Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}},
			wantMode: DevinSmartMode,
			wantEnv:  []EnvEntry{{Key: "OTHER", Value: "1"}, {Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}},
		},
		{
			name:     "custom sandbox value stays untouched",
			agentID:  DevinAgentID,
			mode:     "",
			entries:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: "inherit"}},
			wantMode: DevinSmartMode,
			wantEnv:  []EnvEntry{{Key: DevinSandboxEnvKey, Value: "inherit"}},
		},
		{
			name:     "secret-backed sandbox stays untouched",
			agentID:  DevinAgentID,
			mode:     DevinSmartMode,
			entries:  []EnvEntry{{Key: DevinSandboxEnvKey, SecretID: "secret-1"}},
			wantMode: DevinSmartMode,
			wantEnv:  []EnvEntry{{Key: DevinSandboxEnvKey, SecretID: "secret-1"}},
		},
		{
			name:     "other agents keep their values",
			agentID:  "codex-acp",
			mode:     "",
			entries:  []EnvEntry{{Key: "OTHER", Value: "1"}},
			wantMode: "",
			wantEnv:  []EnvEntry{{Key: "OTHER", Value: "1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, entries := ApplyDevinProfileDefaults(tt.agentID, tt.mode, tt.entries, true)
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
			if !reflect.DeepEqual(entries, tt.wantEnv) {
				t.Fatalf("env = %#v, want %#v", entries, tt.wantEnv)
			}
		})
	}
}

func TestApplyDevinProfileDefaultsKeepsExplicitInheritance(t *testing.T) {
	mode, entries := ApplyDevinProfileDefaults(DevinAgentID, "", nil, false)
	if mode != DevinSmartMode {
		t.Fatalf("mode = %q, want Smart", mode)
	}
	if len(entries) != 0 {
		t.Fatalf("explicit inheritance gained an override: %#v", entries)
	}
}

func TestApplyDevinProfileDefaultsDoesNotMutateCallerSlice(t *testing.T) {
	entries := []EnvEntry{{Key: "OTHER", Value: "1"}}
	_, updated := ApplyDevinProfileDefaults(DevinAgentID, "", entries, true)
	if len(entries) != 1 {
		t.Fatalf("caller slice was mutated: %#v", entries)
	}
	if len(updated) != 2 {
		t.Fatalf("updated entries = %#v", updated)
	}
}

func TestValidateDevinPermissionIntent(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		mode    string
		entries []EnvEntry
		wantErr bool
	}{
		{name: "Smart with a disabled sandbox", agentID: DevinAgentID, mode: DevinSmartMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}}},
		{name: "Autonomous with an enabled sandbox", agentID: DevinAgentID, mode: DevinAutonomousMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}}},
		{name: "inherited setting is provider-owned", agentID: DevinAgentID, mode: DevinSmartMode},
		{name: "custom value is provider-owned", agentID: DevinAgentID, mode: DevinSmartMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: "inherit"}}},
		{name: "other agents are untouched", agentID: "codex-acp", mode: DevinSmartMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}}},
		{name: "enabled sandbox with Smart", agentID: DevinAgentID, mode: DevinSmartMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}}, wantErr: true},
		{name: "enabled sandbox with Bypass", agentID: DevinAgentID, mode: DevinBypassMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxEnabled}}, wantErr: true},
		{name: "disabled sandbox with Autonomous", agentID: DevinAgentID, mode: DevinAutonomousMode,
			entries: []EnvEntry{{Key: DevinSandboxEnvKey, Value: devinSandboxDisabled}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDevinPermissionIntent(tt.agentID, tt.mode, tt.entries)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidDevinPermissionIntent) {
					t.Fatalf("error = %v, want ErrInvalidDevinPermissionIntent", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
