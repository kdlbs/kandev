package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
)

// TestBuildModelConfig_CLIModelsFallback verifies that a passthrough-only agent
// with CLI-advertised models (e.g. Antigravity's `agy models`) gets a dynamic
// model config even when the host utility has no probe data for it.
func TestBuildModelConfig_CLIModelsFallback(t *testing.T) {
	c := newTestController(nil) // hostUtility is nil → no ACP probe data

	availability := discovery.Availability{
		Name:      "antigravity",
		Available: true,
		Models: []discovery.Model{
			{ID: "Gemini 3.5 Flash (Medium)", Name: "Gemini 3.5 Flash (Medium)"},
			{ID: "Claude Sonnet 4.6 (Thinking)", Name: "Claude Sonnet 4.6 (Thinking)"},
		},
	}

	cfg := c.buildModelConfig("antigravity", availability)

	if !cfg.SupportsDynamicModels {
		t.Fatal("SupportsDynamicModels = false, want true for CLI-listed models")
	}
	if cfg.Status != string(hostutility.StatusOK) {
		t.Fatalf("Status = %q, want %q", cfg.Status, hostutility.StatusOK)
	}
	if len(cfg.AvailableModels) != 2 {
		t.Fatalf("AvailableModels len = %d, want 2", len(cfg.AvailableModels))
	}
	if cfg.AvailableModels[0].ID != "Gemini 3.5 Flash (Medium)" ||
		cfg.AvailableModels[0].Name != "Gemini 3.5 Flash (Medium)" {
		t.Fatalf("AvailableModels[0] = %+v", cfg.AvailableModels[0])
	}
}

// TestFetchDynamicModels_CLIModelsFallback verifies the live model-fetch
// endpoint returns CLI-advertised models for a passthrough-only agent that the
// host utility never probes. This is the path that overwrites the boot-payload
// models in the profile UI, so it must surface the same CLI models.
func TestFetchDynamicModels_CLIModelsFallback(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true") // synth discovery; avoids nil c.discovery

	ctrl := newTestController(map[string]agents.Agent{
		"antigravity": &testAgent{
			id:      "antigravity",
			name:    "antigravity",
			enabled: true,
			models: []agents.DiscoveredModel{
				{ID: "Gemini 3.5 Flash (Medium)", Name: "Gemini 3.5 Flash (Medium)"},
				{ID: "Claude Opus 4.6 (Thinking)", Name: "Claude Opus 4.6 (Thinking)"},
			},
		},
	})

	resp, err := ctrl.FetchDynamicModels(context.Background(), "antigravity", false)
	if err != nil {
		t.Fatalf("FetchDynamicModels() error = %v", err)
	}
	if resp.Status != string(hostutility.StatusOK) {
		t.Fatalf("Status = %q, want %q", resp.Status, hostutility.StatusOK)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("Models len = %d, want 2", len(resp.Models))
	}
	if resp.Models[0].ID != "Gemini 3.5 Flash (Medium)" {
		t.Fatalf("Models[0].ID = %q", resp.Models[0].ID)
	}
}

// TestBuildModelConfig_NoModels verifies the empty path: no host utility data
// and no CLI models yields a non-dynamic config with an empty (non-nil) list.
func TestBuildModelConfig_NoModels(t *testing.T) {
	c := newTestController(nil)

	cfg := c.buildModelConfig("mock-agent", discovery.Availability{Name: "mock-agent", Available: true})

	if cfg.SupportsDynamicModels {
		t.Fatal("SupportsDynamicModels = true, want false with no models")
	}
	if cfg.AvailableModels == nil {
		t.Fatal("AvailableModels is nil; must be non-nil so JSON marshals as []")
	}
	if len(cfg.AvailableModels) != 0 {
		t.Fatalf("AvailableModels len = %d, want 0", len(cfg.AvailableModels))
	}
}
