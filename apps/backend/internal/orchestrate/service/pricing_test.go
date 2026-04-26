package service

import "testing"

func TestGetModelPricing_KnownModel(t *testing.T) {
	p, found := GetModelPricing("claude-sonnet-4")
	if !found {
		t.Fatal("expected claude-sonnet-4 to be found")
	}
	if p.InputPerMillion != 300 {
		t.Errorf("input = %d, want 300", p.InputPerMillion)
	}
	if p.OutputPerMillion != 1500 {
		t.Errorf("output = %d, want 1500", p.OutputPerMillion)
	}
}

func TestGetModelPricing_UnknownModel(t *testing.T) {
	_, found := GetModelPricing("unknown-model-xyz")
	if found {
		t.Fatal("expected unknown model to not be found")
	}
}

func TestCalculateCostCents(t *testing.T) {
	pricing := ModelPricing{
		InputPerMillion:  300,
		CachedPerMillion: 30,
		OutputPerMillion: 1500,
	}

	tests := []struct {
		name       string
		in, cached int
		out        int
		want       int
	}{
		{"zero tokens", 0, 0, 0, 0},
		{"1M input only", 1_000_000, 0, 0, 300},
		{"1M output only", 0, 0, 1_000_000, 1500},
		{"mixed", 100_000, 50_000, 10_000, 46},
		// 100k*300/1M=30 + 50k*30/1M=1 + 10k*1500/1M=15 = 46
		{"large", 5_000_000, 2_000_000, 1_000_000, 3060},
		// 5M*300/1M=1500 + 2M*30/1M=60 + 1M*1500/1M=1500 = 3060
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateCostCents(tc.in, tc.cached, tc.out, pricing)
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
