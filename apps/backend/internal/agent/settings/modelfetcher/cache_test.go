package modelfetcher

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestCache_SetAndGet(t *testing.T) {
	cache := NewCache()

	models := []agents.Model{
		{ID: "anthropic/claude-sonnet", Name: "Claude Sonnet", Provider: "anthropic"},
		{ID: "openai/gpt-4", Name: "GPT-4", Provider: "openai"},
	}

	cache.Set("test-agent", models, nil)

	entry, exists := cache.Get("test-agent")
	if !exists {
		t.Fatal("expected cache entry to exist")
	}

	if len(entry.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(entry.Models))
	}

	if entry.Models[0].ID != "anthropic/claude-sonnet" {
		t.Errorf("expected first model ID to be 'anthropic/claude-sonnet', got %q", entry.Models[0].ID)
	}

	if entry.Error != nil {
		t.Errorf("expected no error, got %v", entry.Error)
	}
}

func TestCache_GetNonExistent(t *testing.T) {
	cache := NewCache()

	_, exists := cache.Get("non-existent")
	if exists {
		t.Error("expected cache entry to not exist")
	}
}

func TestCache_SetWithError(t *testing.T) {
	cache := NewCache()

	testErr := &testError{msg: "command failed"}
	cache.Set("test-agent", nil, testErr)

	entry, exists := cache.Get("test-agent")
	if !exists {
		t.Fatal("expected cache entry to exist")
	}

	if entry.Error == nil {
		t.Error("expected error to be stored")
	}

	if entry.Error.Error() != "command failed" {
		t.Errorf("expected error message 'command failed', got %q", entry.Error.Error())
	}
}

func TestCache_Invalidate(t *testing.T) {
	cache := NewCache()

	models := []agents.Model{
		{ID: "test/model", Name: "Test Model", Provider: "test"},
	}

	cache.Set("test-agent", models, nil)

	// Verify it exists
	_, exists := cache.Get("test-agent")
	if !exists {
		t.Fatal("expected cache entry to exist before invalidation")
	}

	// Invalidate
	cache.Invalidate("test-agent")

	// Verify it's gone
	_, exists = cache.Get("test-agent")
	if exists {
		t.Error("expected cache entry to not exist after invalidation")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache()

	models := []agents.Model{
		{ID: "test/model", Name: "Test Model", Provider: "test"},
	}

	cache.Set("agent1", models, nil)
	cache.Set("agent2", models, nil)

	cache.Clear()

	_, exists1 := cache.Get("agent1")
	_, exists2 := cache.Get("agent2")

	if exists1 || exists2 {
		t.Error("expected all cache entries to be cleared")
	}
}

func TestCacheEntry_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		entry    CacheEntry
		expected bool
	}{
		{
			name: "valid entry",
			entry: CacheEntry{
				Models:    []agents.Model{{ID: "test"}},
				CachedAt:  time.Now(),
				ExpiresAt: time.Now().Add(5 * time.Second),
				Error:     nil,
			},
			expected: true,
		},
		{
			name: "expired entry",
			entry: CacheEntry{
				Models:    []agents.Model{{ID: "test"}},
				CachedAt:  time.Now().Add(-1 * time.Minute),
				ExpiresAt: time.Now().Add(-30 * time.Second),
				Error:     nil,
			},
			expected: false,
		},
		{
			name: "entry with error",
			entry: CacheEntry{
				Models:    nil,
				CachedAt:  time.Now(),
				ExpiresAt: time.Now().Add(5 * time.Second),
				Error:     &testError{msg: "some error"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.IsValid()
			if result != tt.expected {
				t.Errorf("IsValid() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCacheEntry_IsStale(t *testing.T) {
	tests := []struct {
		name     string
		entry    CacheEntry
		expected bool
	}{
		{
			name: "fresh entry - not stale",
			entry: CacheEntry{
				CachedAt:  time.Now(),
				ExpiresAt: time.Now().Add(5 * time.Second),
			},
			expected: false,
		},
		{
			name: "expired but within max age - stale",
			entry: CacheEntry{
				CachedAt:  time.Now().Add(-30 * time.Second),
				ExpiresAt: time.Now().Add(-20 * time.Second),
			},
			expected: true,
		},
		{
			name: "beyond max age - not stale (too old)",
			entry: CacheEntry{
				CachedAt:  time.Now().Add(-2 * time.Hour),
				ExpiresAt: time.Now().Add(-2 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.IsStale()
			if result != tt.expected {
				t.Errorf("IsStale() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
