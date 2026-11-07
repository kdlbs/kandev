package agents

import "testing"

func TestBuiltinManagedNPMRuntimeRegistration(t *testing.T) {
	tests := []struct {
		id          string
		packageName string
	}{
		{"claude-acp", NewClaudeACP().ManagedNPMRuntime().Package},
		{"codex-acp", NewCodexACP().ManagedNPMRuntime().Package},
		{"codex-app-server", NewCodexAppServer(true).ManagedNPMRuntime().Package},
		{CopilotACPAgentID, NewCopilotACP().ManagedNPMRuntime().Package},
		{"gemini", NewGemini().ManagedNPMRuntime().Package},
		{OpenCodeACPAgentID, NewOpenCodeACP().ManagedNPMRuntime().Package},
		{"pi-acp", NewPiACP().ManagedNPMRuntime().Package},
		{"muse-acp", NewMuseACP().ManagedNPMRuntime().Package},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			spec, ok := BuiltinManagedNPMRuntime(test.id)
			if !ok || spec.Package != test.packageName {
				t.Fatalf("BuiltinManagedNPMRuntime(%q) = (%#v, %v), want package %q", test.id, spec, ok, test.packageName)
			}
		})
	}
	for _, id := range []string{"auggie", "mock-agent", "operator-defined"} {
		if _, ok := BuiltinManagedNPMRuntime(id); ok {
			t.Fatalf("BuiltinManagedNPMRuntime(%q) accepted a non-managed runtime", id)
		}
	}
}
