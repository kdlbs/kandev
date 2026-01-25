package modelfetcher

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestFetcher(t *testing.T, agents map[string]*registry.AgentTypeConfig) *Fetcher {
	t.Helper()

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	reg := registry.NewRegistry(log)
	for _, agent := range agents {
		if err := reg.Register(agent); err != nil {
			t.Fatalf("failed to register agent: %v", err)
		}
	}

	return NewFetcher(reg, log)
}

func TestFetcher_SupportsDynamicModels(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"with-dynamic": {
			ID:   "with-dynamic",
			Name: "With Dynamic",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			ModelConfig: registry.ModelConfig{
				DefaultModel:     "test/model",
				DynamicModelsCmd: []string{"test", "models"},
			},
		},
		"without-dynamic": {
			ID:   "without-dynamic",
			Name: "Without Dynamic",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			ModelConfig: registry.ModelConfig{
				DefaultModel: "test/model",
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	tests := []struct {
		name      string
		agentName string
		want      bool
	}{
		{
			name:      "agent with dynamic models cmd",
			agentName: "with-dynamic",
			want:      true,
		},
		{
			name:      "agent without dynamic models cmd",
			agentName: "without-dynamic",
			want:      false,
		},
		{
			name:      "non-existent agent",
			agentName: "non-existent",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fetcher.SupportsDynamicModels(tt.agentName)
			if got != tt.want {
				t.Errorf("SupportsDynamicModels(%q) = %v, want %v", tt.agentName, got, tt.want)
			}
		})
	}
}

func TestFetcher_MarkModelsAsStatic(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"test-agent": {
			ID:   "test-agent",
			Name: "Test Agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	models := []registry.ModelEntry{
		{ID: "model1", Name: "Model 1", Provider: "test"},
		{ID: "model2", Name: "Model 2", Provider: "test", Source: "dynamic"},
		{ID: "model3", Name: "Model 3", Provider: "test", Source: ""},
	}

	result := fetcher.markModelsAsStatic(models)

	if len(result) != len(models) {
		t.Fatalf("markModelsAsStatic() returned %d models, want %d", len(result), len(models))
	}

	for i, m := range result {
		if m.Source != "static" {
			t.Errorf("model %d Source = %q, want 'static'", i, m.Source)
		}
	}

	// Verify original slice is not modified
	if models[1].Source != "dynamic" {
		t.Error("original slice should not be modified")
	}
}

func TestFetcher_Fetch_AgentNotFound(t *testing.T) {
	fetcher := newTestFetcher(t, map[string]*registry.AgentTypeConfig{})

	_, err := fetcher.Fetch(context.Background(), "non-existent", false)
	if err == nil {
		t.Error("Fetch() should return error for non-existent agent")
	}
}

func TestFetcher_Fetch_StaticModels(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"static-agent": {
			ID:   "static-agent",
			Name: "Static Agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			ModelConfig: registry.ModelConfig{
				DefaultModel: "test/default",
				AvailableModels: []registry.ModelEntry{
					{ID: "test/model1", Name: "Model 1", Provider: "test"},
					{ID: "test/model2", Name: "Model 2", Provider: "test"},
				},
				// No DynamicModelsCmd - uses static models
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	result, err := fetcher.Fetch(context.Background(), "static-agent", false)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if result.Cached {
		t.Error("Fetch() Cached = true for static models, want false")
	}

	if len(result.Models) != 2 {
		t.Errorf("Fetch() returned %d models, want 2", len(result.Models))
	}

	// Verify all models are marked as static
	for i, m := range result.Models {
		if m.Source != "static" {
			t.Errorf("model %d Source = %q, want 'static'", i, m.Source)
		}
	}
}

func TestFetcher_InvalidateCache(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"test-agent": {
			ID:   "test-agent",
			Name: "Test Agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	// Manually set cache
	fetcher.cache.Set("test-agent", []registry.ModelEntry{{ID: "test"}}, nil)

	// Verify cache exists
	if _, exists := fetcher.cache.Get("test-agent"); !exists {
		t.Fatal("cache entry should exist before invalidation")
	}

	// Invalidate
	fetcher.InvalidateCache("test-agent")

	// Verify cache is cleared
	if _, exists := fetcher.cache.Get("test-agent"); exists {
		t.Error("cache entry should not exist after invalidation")
	}
}

func TestFetcher_Fetch_UsesCache(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"cached-agent": {
			ID:   "cached-agent",
			Name: "Cached Agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			ModelConfig: registry.ModelConfig{
				DefaultModel:     "test/default",
				DynamicModelsCmd: []string{"echo", "test/model"}, // Will fail or return something
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	// Pre-populate cache
	cachedModels := []registry.ModelEntry{
		{ID: "cached/model", Name: "Cached Model", Provider: "cached", Source: "dynamic"},
	}
	fetcher.cache.Set("cached-agent", cachedModels, nil)

	// Fetch without refresh - should use cache
	result, err := fetcher.Fetch(context.Background(), "cached-agent", false)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if !result.Cached {
		t.Error("Fetch() Cached = false, want true (should use cache)")
	}

	if len(result.Models) != 1 || result.Models[0].ID != "cached/model" {
		t.Error("Fetch() should return cached models")
	}
}

func TestFetcher_Fetch_RefreshBypassesCache(t *testing.T) {
	agents := map[string]*registry.AgentTypeConfig{
		"refresh-agent": {
			ID:   "refresh-agent",
			Name: "Refresh Agent",
			Cmd:  []string{"test-cli"},
			ResourceLimits: registry.ResourceLimits{
				MemoryMB:       1024,
				CPUCores:       1.0,
				TimeoutSeconds: 3600,
			},
			ModelConfig: registry.ModelConfig{
				DefaultModel:     "test/default",
				DynamicModelsCmd: []string{"echo", "fresh/model"}, // Returns fresh model
				AvailableModels: []registry.ModelEntry{
					{ID: "static/fallback", Name: "Fallback", Provider: "static"},
				},
			},
		},
	}

	fetcher := newTestFetcher(t, agents)

	// Pre-populate cache with different data
	cachedModels := []registry.ModelEntry{
		{ID: "cached/model", Name: "Cached Model", Provider: "cached", Source: "dynamic"},
	}
	fetcher.cache.Set("refresh-agent", cachedModels, nil)

	// Fetch with refresh - should bypass cache and execute command
	result, err := fetcher.Fetch(context.Background(), "refresh-agent", true)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	// Should have executed the command and got "fresh/model"
	if len(result.Models) == 0 {
		t.Fatal("Fetch() returned no models")
	}

	// The echo command returns "fresh/model", so that's what we should get
	if result.Models[0].ID != "fresh/model" {
		t.Errorf("Fetch() first model ID = %q, want 'fresh/model'", result.Models[0].ID)
	}
}
