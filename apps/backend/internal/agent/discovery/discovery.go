// Package discovery provides agent installation detection and discovery functionality.
// It delegates to the agents.Agent interface for discovery and model information.
package discovery

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

const defaultCacheTTL = 30 * time.Second

// Capabilities describes what the agent supports.
type Capabilities struct {
	SupportsSessionResume bool `json:"supports_session_resume"`
	SupportsShell         bool `json:"supports_shell"`
	SupportsWorkspaceOnly bool `json:"supports_workspace_only"`
}

// Availability represents the result of detecting an agent's installation.
type Availability struct {
	Name              string       `json:"name"`
	SupportsMCP       bool         `json:"supports_mcp"`
	MCPConfigPath     string       `json:"mcp_config_path,omitempty"`
	InstallationPaths []string     `json:"installation_paths,omitempty"`
	Available         bool         `json:"available"`
	MatchedPath       string       `json:"matched_path,omitempty"`
	Capabilities      Capabilities `json:"capabilities"`
}

// Registry manages agent discovery using the agents.Agent interface.
//
// The agent list is read from the agent registry on every sweep rather than
// captured once. Custom agents are registered, replaced, and unregistered
// while the backend runs (agent creation, MCP-strategy changes, deletion), and
// a captured list would keep reporting a deleted agent as installed — and keep
// a replaced one's superseded discovery flags — until the next restart.
type Registry struct {
	registry *registry.Registry
	logger   *logger.Logger

	mu            sync.RWMutex
	cachedResults []Availability
	cachedAt      time.Time
	cacheTTL      time.Duration
	// generation counts invalidations. A sweep reads the registry when it
	// starts and writes its results when it finishes, so an invalidation can
	// land in between; the sweep carries the generation it began with and
	// discards its results when that no longer matches.
	generation uint64
}

// LoadRegistry creates a new discovery registry backed by the agent registry.
// The context is unused: nothing is probed here, because detection resolves the
// agent list when a sweep runs.
func LoadRegistry(_ context.Context, reg *registry.Registry, log *logger.Logger) (*Registry, error) {
	return &Registry{
		registry: reg,
		logger:   log,
		cacheTTL: defaultCacheTTL,
	}, nil
}

// enabledAgents returns the agents a sweep should probe: every enabled entry in
// the agent registry except virtual families, which have no CLI to detect.
func (r *Registry) enabledAgents() []agents.Agent {
	enabled := r.registry.ListEnabled()
	result := make([]agents.Agent, 0, len(enabled))
	for _, ag := range enabled {
		if agents.IsVirtualAgent(ag) {
			continue
		}
		result = append(result, ag)
	}
	return result
}

// Detect checks whether each agent is installed by calling IsInstalled.
// Results are cached with a TTL to avoid redundant detection on repeated calls.
func (r *Registry) Detect(ctx context.Context) ([]Availability, error) {
	if cached := r.getCached(); cached != nil {
		return cached, nil
	}

	r.mu.RLock()
	startedAt := r.generation
	r.mu.RUnlock()

	results := r.detectAll(ctx)

	r.mu.Lock()
	if r.generation == startedAt {
		r.cachedResults = results
		r.cachedAt = time.Now()
	}
	r.mu.Unlock()

	return results, nil
}

// InvalidateCache clears the cached detection results, forcing the next
// Detect call to re-run agent detection.
func (r *Registry) InvalidateCache() {
	r.mu.Lock()
	r.cachedResults = nil
	r.cachedAt = time.Time{}
	r.generation++
	r.mu.Unlock()
}

func (r *Registry) getCached() []Availability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.cachedResults == nil {
		return nil
	}
	if time.Since(r.cachedAt) > r.cacheTTL {
		return nil
	}
	// Return a copy to prevent mutation.
	copied := make([]Availability, len(r.cachedResults))
	copy(copied, r.cachedResults)
	return copied
}

const detectAllTimeout = 15 * time.Second

// detectAll runs IsInstalled for all agents concurrently.
// A timeout bounds the overall detection to prevent hanging when agent
// binaries are missing or unresponsive (e.g. fresh K8s deploy).
func (r *Registry) detectAll(ctx context.Context) []Availability {
	ctx, cancel := context.WithTimeout(ctx, detectAllTimeout)
	defer cancel()
	type indexedResult struct {
		index int
		avail Availability
		err   error
	}

	sweep := r.enabledAgents()
	ch := make(chan indexedResult, len(sweep))
	for i, ag := range sweep {
		go func(idx int, ag agents.Agent) {
			result, err := ag.IsInstalled(ctx)
			if err != nil {
				ch <- indexedResult{index: idx, err: err}
				return
			}

			mcpPath := ""
			if len(result.MCPConfigPaths) > 0 {
				mcpPath = result.MCPConfigPaths[0]
			}

			ch <- indexedResult{
				index: idx,
				avail: Availability{
					Name:              ag.ID(),
					SupportsMCP:       result.SupportsMCP,
					MCPConfigPath:     mcpPath,
					InstallationPaths: result.InstallationPaths,
					Available:         result.Available,
					MatchedPath:       result.MatchedPath,
					Capabilities: Capabilities{
						SupportsSessionResume: result.Capabilities.SupportsSessionResume,
						SupportsShell:         result.Capabilities.SupportsShell,
						SupportsWorkspaceOnly: result.Capabilities.SupportsWorkspaceOnly,
					},
				},
			}
		}(i, ag)
	}

	// Collect results preserving original order.
	// If the context expires before all agents respond, return partial results.
	slots := make([]Availability, len(sweep))
	valid := make([]bool, len(sweep))
	for range sweep {
		select {
		case res := <-ch:
			if res.err != nil {
				r.logger.Warn("discovery: detect failed for agent",
					zap.String("agent", sweep[res.index].ID()),
					zap.Error(res.err),
				)
				continue
			}
			slots[res.index] = res.avail
			valid[res.index] = true
		case <-ctx.Done():
			r.logger.Warn("discovery: detectAll timed out, returning partial results")
			goto collect
		}
	}
collect:

	results := make([]Availability, 0, len(sweep))
	for i, v := range valid {
		if v {
			results = append(results, slots[i])
		}
	}
	return results
}
