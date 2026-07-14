package usage

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// freshMaxAge clamps fresh (hover-triggered) fetches: a provider is queried
// again at most every 15 s even when callers keep requesting fresh data.
const freshMaxAge = 15 * time.Second

// hostUsageFetchError is the sanitized per-agent error surfaced through the
// API. Raw provider errors can contain credential paths and response bodies,
// so those are only logged server-side.
const hostUsageFetchError = "failed to fetch usage from provider"

// HostAgentUsage is one host-installed subscription agent's usage entry.
type HostAgentUsage struct {
	AgentID string         `json:"agent_id"`
	Usage   *ProviderUsage `json:"usage,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// hostUsageClient is a ProviderUsageClient that can also report whether
// subscription credentials are present on disk.
type hostUsageClient interface {
	ProviderUsageClient
	HasSubscriptionCredentials() bool
}

type hostEntry struct {
	agentID  string
	cacheKey string
	client   hostUsageClient
}

// HostService lists subscription utilization for agent CLIs installed on the
// kandev host (Claude Code, Codex). Agents without subscription credentials
// are omitted from List results.
type HostService struct {
	cache   *UsageCache
	entries []hostEntry
	logger  *logger.Logger
}

// NewHostService builds a HostService reading credentials from the current
// user's home directory. The service is empty when the home dir is unavailable.
func NewHostService(log *logger.Logger) *HostService {
	s := &HostService{cache: NewUsageCache(), logger: log}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	claudePath := filepath.Join(home, ".claude", ".credentials.json")
	codexPath := filepath.Join(home, ".codex", "auth.json")
	s.entries = []hostEntry{
		{
			agentID:  "claude-acp",
			cacheKey: CacheKey("anthropic", claudePath),
			client:   NewClaudeUsageClientWithPath(claudePath),
		},
		{
			agentID:  "codex-acp",
			cacheKey: CacheKey("openai", codexPath),
			client:   NewCodexUsageClientWithPath(codexPath),
		},
	}
	return s
}

// List returns usage for every host agent that has subscription credentials.
// With fresh=false cached results up to 5 minutes old are served; fresh=true
// re-queries the providers, bounded by freshMaxAge. Fetch failures are
// reported per-agent via the Error field rather than failing the whole listing.
func (s *HostService) List(ctx context.Context, fresh bool) []HostAgentUsage {
	maxAge := cacheTTL
	if fresh {
		maxAge = freshMaxAge
	}
	out := make([]HostAgentUsage, 0, len(s.entries))
	for _, e := range s.entries {
		if !e.client.HasSubscriptionCredentials() {
			continue
		}
		entry := HostAgentUsage{AgentID: e.agentID}
		u, err := s.cache.GetOrFetchWithin(ctx, e.cacheKey, maxAge, e.client.FetchUsage)
		switch {
		case err != nil:
			if s.logger != nil {
				s.logger.Warn("subscription usage fetch failed",
					zap.String("agent", e.agentID), zap.Error(err))
			}
			entry.Error = hostUsageFetchError
		default:
			entry.Usage = u
		}
		out = append(out, entry)
	}
	return out
}
