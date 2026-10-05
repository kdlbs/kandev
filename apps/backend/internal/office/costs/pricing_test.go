package costs_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/costs"
)

func TestProviderForModel(t *testing.T) {
	// The shared package owns the full model matrix. Keep Office forwarding
	// coverage for a recognized model and the unknown-model fallback.
	tests := []struct {
		model, want string
	}{
		{"gpt-5-mini", "openai"},
		{"butler_a", ""},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			if got := costs.ProviderForModel(tc.model); got != tc.want {
				t.Errorf("ProviderForModel(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}
