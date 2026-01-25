package modelfetcher

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	defaultTimeout   = 30 * time.Second
	maxOutputSize    = 1024 * 1024 // 1MB max output
)

// Fetcher handles dynamic model fetching for agents
type Fetcher struct {
	agentRegistry *registry.Registry
	cache         *Cache
	logger        *logger.Logger
}

// NewFetcher creates a new model fetcher
func NewFetcher(agentRegistry *registry.Registry, log *logger.Logger) *Fetcher {
	return &Fetcher{
		agentRegistry: agentRegistry,
		cache:         NewCache(),
		logger:        log.WithFields(zap.String("component", "model-fetcher")),
	}
}

// FetchResult contains the result of fetching models
type FetchResult struct {
	AgentName string
	Models    []registry.ModelEntry
	Cached    bool
	CachedAt  *time.Time
	Error     error
}

// Fetch fetches models for an agent, using cache if available
func (f *Fetcher) Fetch(ctx context.Context, agentName string, refresh bool) (*FetchResult, error) {
	// Get agent config
	agentConfig, ok := f.agentRegistry.Get(agentName)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentName)
	}

	// Check if agent supports dynamic models
	if len(agentConfig.ModelConfig.DynamicModelsCmd) == 0 {
		// Return static models
		return &FetchResult{
			AgentName: agentName,
			Models:    f.markModelsAsStatic(agentConfig.ModelConfig.AvailableModels),
			Cached:    false,
			Error:     nil,
		}, nil
	}

	// Check cache unless refresh is requested
	if !refresh {
		if entry, exists := f.cache.Get(agentName); exists && entry.IsValid() {
			cachedAt := entry.CachedAt
			return &FetchResult{
				AgentName: agentName,
				Models:    entry.Models,
				Cached:    true,
				CachedAt:  &cachedAt,
				Error:     entry.Error,
			}, nil
		}
	}

	// Fetch dynamic models
	models, err := f.executeDynamicFetch(ctx, agentConfig)
	if err != nil {
		f.logger.Warn("dynamic model fetch failed, using static fallback",
			zap.String("agent", agentName),
			zap.Error(err))

		// Cache the error briefly to avoid repeated failures
		f.cache.Set(agentName, nil, err)

		// Return static models as fallback
		return &FetchResult{
			AgentName: agentName,
			Models:    f.markModelsAsStatic(agentConfig.ModelConfig.AvailableModels),
			Cached:    false,
			Error:     err,
		}, nil
	}

	// Cache the result
	f.cache.Set(agentName, models, nil)
	cachedAt := time.Now()

	return &FetchResult{
		AgentName: agentName,
		Models:    models,
		Cached:    true,
		CachedAt:  &cachedAt,
		Error:     nil,
	}, nil
}

// executeDynamicFetch runs the dynamic models command and parses output
func (f *Fetcher) executeDynamicFetch(ctx context.Context, agentConfig *registry.AgentTypeConfig) ([]registry.ModelEntry, error) {
	if len(agentConfig.ModelConfig.DynamicModelsCmd) == 0 {
		return nil, fmt.Errorf("no dynamic models command configured")
	}

	// Determine timeout
	timeout := defaultTimeout
	if agentConfig.ModelConfig.DynamicModelsTimeoutMs > 0 {
		timeout = time.Duration(agentConfig.ModelConfig.DynamicModelsTimeoutMs) * time.Millisecond
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command - use exec.Command directly (no shell)
	cmd := agentConfig.ModelConfig.DynamicModelsCmd
	execCmd := exec.CommandContext(ctx, cmd[0], cmd[1:]...)

	// Capture output
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	// Run command
	err := execCmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %v", timeout)
		}
		return nil, fmt.Errorf("command failed: %w (stderr: %s)", err, stderr.String())
	}

	// Check output size
	if stdout.Len() > maxOutputSize {
		return nil, fmt.Errorf("output exceeds maximum size of %d bytes", maxOutputSize)
	}

	// Get the appropriate parser for this agent
	parser := GetParser(agentConfig.ID)

	// Parse output using the agent-specific parser
	models, err := parser.Parse(stdout.String(), agentConfig.ModelConfig.DefaultModel)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model output: %w", err)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in output")
	}

	return models, nil
}

// markModelsAsStatic sets the Source field to "static" for all models
func (f *Fetcher) markModelsAsStatic(models []registry.ModelEntry) []registry.ModelEntry {
	result := make([]registry.ModelEntry, len(models))
	for i, m := range models {
		m.Source = "static"
		result[i] = m
	}
	return result
}

// SupportsynamicModels returns true if the agent supports dynamic model fetching
func (f *Fetcher) SupportsDynamicModels(agentName string) bool {
	agentConfig, ok := f.agentRegistry.Get(agentName)
	if !ok {
		return false
	}
	return len(agentConfig.ModelConfig.DynamicModelsCmd) > 0
}

// InvalidateCache invalidates the cache for an agent
func (f *Fetcher) InvalidateCache(agentName string) {
	f.cache.Invalidate(agentName)
}
