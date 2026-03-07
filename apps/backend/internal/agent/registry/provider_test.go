package registry

import (
	"testing"
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
