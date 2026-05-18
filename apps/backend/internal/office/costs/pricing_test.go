package costs_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/costs"
)

func TestCalculateCostSubcents(t *testing.T) {
	// Anthropic-ish rates: $3/M input, $0.30/M cached-read, $3.75/M cached-write,
	// $15/M output. In subcents/M that's 30000, 3000, 37500, 150000.
	pricing := costs.ModelPricing{
		InputPerMillion:       30000,
		CachedReadPerMillion:  3000,
		CachedWritePerMillion: 37500,
		OutputPerMillion:      150000,
	}

	tests := []struct {
		name                             string
		in, cachedRead, cachedWrite, out int64
		want                             int64
	}{
		{"zero tokens => zero cost", 0, 0, 0, 0, 0},
		{"input only 1M => 30000 subcents = $3", 1_000_000, 0, 0, 0, 30000},
		{"output only 1M => 150000 subcents = $15", 0, 0, 0, 1_000_000, 150000},
		{"cached read vs cached write priced separately",
			0, 1_000_000, 1_000_000, 0,
			3000 + 37500},
		{"mixed: realistic turn",
			500, 1000, 2000, 100,
			(500*30000 + 1000*3000 + 2000*37500 + 100*150000) / 1_000_000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := costs.CalculateCostSubcents(tc.in, tc.cachedRead, tc.cachedWrite, tc.out, pricing)
			if got != tc.want {
				t.Errorf("CalculateCostSubcents() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCalculateCostSubcents_ZeroPricing(t *testing.T) {
	if got := costs.CalculateCostSubcents(1_000_000, 0, 0, 1_000_000, costs.ModelPricing{}); got != 0 {
		t.Errorf("zero pricing should yield zero cost, got %d", got)
	}
}

func TestProviderForModel(t *testing.T) {
	tests := []struct {
		model, want string
	}{
		{"claude-opus-4-7", "anthropic"},
		{"claude-sonnet-4-5", "anthropic"},
		{"gpt-5-mini", "openai"},
		{"gpt-4.1", "openai"},
		{"o3-mini", "openai"},
		{"o4-mini", "openai"},
		{"gemini-2.5-pro", "google"},
		{"gemini-3-pro-preview", "google"},
		{"butler_a", ""},
		{"kimi-k2p6", ""},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			if got := costs.ProviderForModel(tc.model); got != tc.want {
				t.Errorf("ProviderForModel(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}
