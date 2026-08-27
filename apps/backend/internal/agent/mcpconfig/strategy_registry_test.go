package mcpconfig

import "testing"

func TestStrategyByKeyResolvesEveryOfferedOption(t *testing.T) {
	// Every key the UI can offer must resolve, or a user could pick an option
	// that is rejected on save.
	for _, opt := range StrategyOptions() {
		strategy, ok := StrategyByKey(opt.Key)
		if !ok {
			t.Errorf("StrategyByKey(%q) not ok, but it is an offered option", opt.Key)
			continue
		}
		if strategy == nil {
			t.Errorf("StrategyByKey(%q) = nil, want an implementation", opt.Key)
		}
		if opt.Description == "" {
			t.Errorf("option %q has an empty description", opt.Key)
		}
	}
}

func TestStrategyByKeyEmptyIsValidAndNil(t *testing.T) {
	// "No MCP injection" is a legitimate stored value — the default for custom
	// TUI agents and for every row written before the field existed.
	strategy, ok := StrategyByKey(StrategyKeyNone)
	if !ok {
		t.Fatal("StrategyByKey(\"\") not ok, want ok (no-injection is valid)")
	}
	if strategy != nil {
		t.Fatalf("StrategyByKey(\"\") = %T, want nil", strategy)
	}
}

func TestStrategyByKeyRejectsUnknown(t *testing.T) {
	// A typo must fail loudly at the API boundary. Resolving it to nil would
	// persist a value that silently disables MCP with no error anywhere.
	if _, ok := StrategyByKey("cluade"); ok {
		t.Fatal("StrategyByKey(\"cluade\") ok = true, want false")
	}
}

func TestStrategyOptionsAreStablyOrdered(t *testing.T) {
	// Map iteration is randomised; a dropdown that reshuffles per request is
	// unusable. Compare several passes rather than pinning exact contents.
	first := StrategyOptions()
	for range 20 {
		next := StrategyOptions()
		if len(next) != len(first) {
			t.Fatalf("option count changed: %d then %d", len(first), len(next))
		}
		for i := range first {
			if next[i].Key != first[i].Key {
				t.Fatalf("option order changed at %d: %q then %q", i, first[i].Key, next[i].Key)
			}
		}
	}
}

func TestStrategyOptionsExcludeNone(t *testing.T) {
	// The empty key is represented by the UI's own "off" choice, not as a
	// selectable strategy with a mechanism description.
	for _, opt := range StrategyOptions() {
		if opt.Key == StrategyKeyNone {
			t.Fatal("StrategyOptions includes the empty key")
		}
	}
}
