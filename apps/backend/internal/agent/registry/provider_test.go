package registry

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestProvide_MockAgentModes(t *testing.T) {
	tests := []struct {
		name              string
		envValue          string
		wantMockEnabled   bool
		wantOnlyMock      bool // only mock-agent registered (no other defaults)
		wantDefaultsCount int  // minimum expected agent count (0 = don't check)
	}{
		{
			name:              "unset: all defaults loaded, mock disabled",
			envValue:          "",
			wantMockEnabled:   false,
			wantOnlyMock:      false,
			wantDefaultsCount: 2, // at least auggie + mock-agent
		},
		{
			name:            "true: all defaults loaded, mock enabled",
			envValue:        "true",
			wantMockEnabled: true,
			wantOnlyMock:    false,
		},
		{
			name:            "only: only mock-agent registered and enabled",
			envValue:        "only",
			wantMockEnabled: true,
			wantOnlyMock:    true,
		},
		{
			name:              "arbitrary value: treated as unset",
			envValue:          "false",
			wantMockEnabled:   false,
			wantOnlyMock:      false,
			wantDefaultsCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KANDEV_MOCK_AGENT", tt.envValue)

			log := newTestLogger()
			reg, cleanup, err := Provide(log)
			if err != nil {
				t.Fatalf("Provide() error: %v", err)
			}
			defer cleanup() //nolint:errcheck

			// Check mock-agent presence and enabled state
			mock, hasMock := reg.Get("mock-agent")
			if !hasMock {
				t.Fatal("mock-agent should always be registered")
			}
			if mock.Enabled() != tt.wantMockEnabled {
				t.Errorf("mock-agent Enabled() = %v, want %v", mock.Enabled(), tt.wantMockEnabled)
			}

			// Check agent count
			all := reg.List()
			if tt.wantOnlyMock {
				if len(all) != 1 {
					t.Errorf("only mode: expected 1 agent, got %d", len(all))
				}
			}
			if tt.wantDefaultsCount > 0 && len(all) < tt.wantDefaultsCount {
				t.Errorf("expected at least %d agents, got %d", tt.wantDefaultsCount, len(all))
			}

			// In non-only mode, verify other default agents exist
			if !tt.wantOnlyMock {
				if !reg.Exists("auggie") {
					t.Error("expected default agent 'auggie' to be loaded")
				}
			}
		})
	}
}

// TestProvide_MockProviders_RegistersExtraAliases ensures that when
// KANDEV_MOCK_AGENT=only and KANDEV_MOCK_PROVIDERS lists canonical
// routing provider IDs, each accepted ID is registered as an extra
// MockAgent instance so routing config validation accepts them.
func TestProvide_MockProviders_RegistersExtraAliases(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_MOCK_PROVIDERS", "claude-acp,codex-acp")

	log := newTestLogger()
	reg, cleanup, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide() error: %v", err)
	}
	defer cleanup() //nolint:errcheck

	for _, id := range []string{"mock-agent", "claude-acp", "codex-acp"} {
		ag, ok := reg.Get(id)
		if !ok {
			t.Fatalf("expected agent %q to be registered", id)
		}
		if _, isMock := ag.(*agents.MockAgent); !isMock {
			t.Errorf("agent %q should be *agents.MockAgent, got %T", id, ag)
		}
		if !ag.Enabled() {
			t.Errorf("agent %q should be enabled", id)
		}
	}

	// opencode-acp not in the env var, must not be registered.
	if reg.Exists("opencode-acp") {
		t.Error("opencode-acp should NOT be registered when not in KANDEV_MOCK_PROVIDERS")
	}
}

// TestProvide_MockProviders_SkipsUnknownIDs verifies that entries in
// KANDEV_MOCK_PROVIDERS not present in RoutableProviderIDs are skipped
// with a warning (not registered).
func TestProvide_MockProviders_SkipsUnknownIDs(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_MOCK_PROVIDERS", "claude-acp,bogus-provider, ,not-real")

	log := newTestLogger()
	reg, cleanup, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide() error: %v", err)
	}
	defer cleanup() //nolint:errcheck

	if !reg.Exists("claude-acp") {
		t.Error("claude-acp should be registered")
	}
	if reg.Exists("bogus-provider") {
		t.Error("bogus-provider should NOT be registered")
	}
	if reg.Exists("not-real") {
		t.Error("not-real should NOT be registered")
	}
}

// TestProvide_MockProviders_DevModeExplicitOverridesReal pins the
// updated dev-mode behavior: when KANDEV_MOCK_AGENT=true and the user
// explicitly sets KANDEV_MOCK_PROVIDERS, the named real-provider agents
// are replaced with MockAgent aliases so the routing UI runs against
// the mock binary instead of requiring the real CLI to be installed.
func TestProvide_MockProviders_DevModeExplicitOverridesReal(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "true")
	t.Setenv("KANDEV_MOCK_PROVIDERS", "claude-acp,codex-acp")

	log := newTestLogger()
	reg, cleanup, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide() error: %v", err)
	}
	defer cleanup() //nolint:errcheck

	for _, id := range []string{"claude-acp", "codex-acp"} {
		ag, ok := reg.Get(id)
		if !ok {
			t.Fatalf("expected agent %q to be registered in dev mode", id)
		}
		if _, isMock := ag.(*agents.MockAgent); !isMock {
			t.Errorf("agent %q should be the MockAgent alias in dev mode, got %T", id, ag)
		}
	}
	// opencode-acp was not in the explicit env var; the real agent
	// loaded by LoadDefaults must still be present.
	if oc, ok := reg.Get("opencode-acp"); ok {
		if _, isMock := oc.(*agents.MockAgent); isMock {
			t.Error("opencode-acp should remain the real agent when not listed in KANDEV_MOCK_PROVIDERS")
		}
	}
}

// TestProvide_MockProviders_VisibleThroughRoutingCatalogue confirms that
// providers registered via KANDEV_MOCK_PROVIDERS are visible through
// reg.Exists() — the path used by routing.KnownProviders to build the
// routing catalogue. When any requested provider is missing from
// reg.Exists, KnownProviders returns an empty slice and every
// non-empty provider_order is rejected with "max 0 allowed".
func TestProvide_MockProviders_VisibleThroughRoutingCatalogue(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_MOCK_PROVIDERS", "claude-acp,codex-acp,opencode-acp")

	log := newTestLogger()
	reg, cleanup, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide() error: %v", err)
	}
	defer cleanup() //nolint:errcheck

	// Simulate what routing.KnownProviders does: iterate RoutableProviderIDs
	// and check reg.Exists(id). All three requested providers must be visible.
	for _, id := range []string{"claude-acp", "codex-acp", "opencode-acp"} {
		if !reg.Exists(id) {
			t.Errorf("provider %q is registered via KANDEV_MOCK_PROVIDERS but reg.Exists returns false; "+
				"routing.KnownProviders would exclude it and reject any provider_order containing it", id)
		}
	}

	// Providers NOT in KANDEV_MOCK_PROVIDERS must not be in the registry
	// (no silently-materialised extras that could pollute agent counts).
	for _, id := range []string{"copilot-acp", "amp-acp"} {
		if reg.Exists(id) {
			t.Errorf("provider %q was NOT requested in KANDEV_MOCK_PROVIDERS but is in the registry", id)
		}
	}
}

// TestProvide_DevModeDoesNotMockCanonicalsByDefault pins the dev-mode
// default: when KANDEV_MOCK_AGENT=true and KANDEV_MOCK_PROVIDERS is
// unset, the real canonical agents remain — devs see Claude/Codex/
// OpenCode, not Mock aliases. Routing-UI testing is opt-in via an
// explicit KANDEV_MOCK_PROVIDERS value.
func TestProvide_DevModeDoesNotMockCanonicalsByDefault(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "true")
	// Intentionally clear KANDEV_MOCK_PROVIDERS — neither unset nor set
	// should trigger the mock aliasing.
	if _, ok := os.LookupEnv("KANDEV_MOCK_PROVIDERS"); ok {
		if err := os.Unsetenv("KANDEV_MOCK_PROVIDERS"); err != nil {
			t.Fatalf("unset env: %v", err)
		}
	}

	log := newTestLogger()
	reg, cleanup, err := Provide(log)
	if err != nil {
		t.Fatalf("Provide() error: %v", err)
	}
	defer cleanup() //nolint:errcheck

	if mock, ok := reg.Get("mock-agent"); !ok || !mock.Enabled() {
		t.Fatal("mock-agent must remain enabled in dev mode")
	}

	for _, id := range []string{"claude-acp", "codex-acp", "opencode-acp"} {
		ag, ok := reg.Get(id)
		if !ok {
			t.Fatalf("expected canonical %q registered (real agent) in dev mode", id)
		}
		if _, isMock := ag.(*agents.MockAgent); isMock {
			t.Errorf("canonical %q should remain the real agent when KANDEV_MOCK_PROVIDERS is unset, got MockAgent", id)
		}
	}
}
