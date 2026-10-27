package config

import "testing"

func TestApplyOverrides_GitHooksIsASnapshot(t *testing.T) {
	requested := map[string]bool{"": true, "front": true}
	cfg := &InstanceConfig{}
	applyOverrides(cfg, &InstanceOverrides{GitHooks: requested})
	requested["back"] = true
	if !cfg.GitHooks[""] || !cfg.GitHooks["front"] || cfg.GitHooks["back"] {
		t.Fatalf("GitHooks = %v, want an independent copy of the request", cfg.GitHooks)
	}

	applyOverrides(cfg, &InstanceOverrides{})
	if !cfg.GitHooks["front"] {
		t.Fatalf("an override without a map must keep the launch snapshot, got %v", cfg.GitHooks)
	}
}
