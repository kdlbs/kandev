package acpdbg

import "testing"

func TestInitializeParamsOmitsAIRWithoutCapabilities(t *testing.T) {
	params := initializeParams(nil)
	caps, ok := params["clientCapabilities"].(map[string]any)
	if !ok {
		t.Fatalf("clientCapabilities = %#v", params["clientCapabilities"])
	}
	if _, present := caps["_meta"]; present {
		t.Fatalf("clientCapabilities._meta should be absent: %#v", caps)
	}
}

func TestInitializeParamsAdvertisesAIRCapabilities(t *testing.T) {
	params := initializeParams([]string{"asyncTasks"})
	caps, ok := params["clientCapabilities"].(map[string]any)
	if !ok {
		t.Fatalf("clientCapabilities = %#v", params["clientCapabilities"])
	}
	meta, ok := caps["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("clientCapabilities._meta = %#v", caps["_meta"])
	}
	jetbrains, ok := meta["jetbrains"].(map[string]any)
	if !ok {
		t.Fatalf("_meta.jetbrains = %#v", meta["jetbrains"])
	}
	air, ok := jetbrains["air"].(map[string]any)
	if !ok {
		t.Fatalf("_meta.jetbrains.air = %#v", jetbrains["air"])
	}
	if air["version"] != 1 {
		t.Fatalf("air version = %#v, want 1", air["version"])
	}
	capabilities, ok := air["capabilities"].([]string)
	if !ok || len(capabilities) != 1 || capabilities[0] != "asyncTasks" {
		t.Fatalf("air capabilities = %#v, want [asyncTasks]", air["capabilities"])
	}
}
