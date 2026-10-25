package backendapp

import (
	"context"
	"testing"

	officeshared "github.com/kandev/kandev/internal/office/shared"
)

type stubModelsDevLookup struct {
	pricing officeshared.ModelPricing
	version string
	ok      bool
}

func (s stubModelsDevLookup) LookupForModelWithVersion(_ context.Context, _ string) (officeshared.ModelPricing, string, bool) {
	return s.pricing, s.version, s.ok
}

// TestUsagePricingAdapter_ConvertsSharedModelPricingFieldForField pins
// docs/specs/task-cost-ledger/spec.md AC-26: the adapter must translate
// *modelsdev.Client's office/shared.ModelPricing into internal/task/usage's
// own commoncosts.ModelPricing verbatim, field for field, so the ledger
// writer's pricing resolution never depends on internal/office/** types.
func TestUsagePricingAdapter_ConvertsSharedModelPricingFieldForField(t *testing.T) {
	stub := stubModelsDevLookup{
		pricing: officeshared.ModelPricing{
			InputPerMillion:       111,
			CachedReadPerMillion:  222,
			CachedWritePerMillion: 333,
			OutputPerMillion:      444,
		},
		version: "models-dev-2026-08-23",
		ok:      true,
	}
	adapter := usagePricingAdapter{lookup: stub}

	got, version, ok := adapter.LookupForModelWithVersion(context.Background(), "claude-sonnet-5")
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if version != "models-dev-2026-08-23" {
		t.Errorf("version = %q, want models-dev-2026-08-23", version)
	}
	if got.InputPerMillion != 111 || got.CachedReadPerMillion != 222 || got.CachedWritePerMillion != 333 || got.OutputPerMillion != 444 {
		t.Errorf("got = %+v, want fields copied verbatim from office/shared.ModelPricing", got)
	}
}

// TestUsagePricingAdapter_PassesThroughNotOK pins the miss path: an unknown
// model must surface as ok=false, not a zero-valued "success".
func TestUsagePricingAdapter_PassesThroughNotOK(t *testing.T) {
	stub := stubModelsDevLookup{ok: false}
	adapter := usagePricingAdapter{lookup: stub}

	_, _, ok := adapter.LookupForModelWithVersion(context.Background(), "unknown-model")
	if ok {
		t.Fatal("ok = true, want false when the underlying lookup misses")
	}
}
