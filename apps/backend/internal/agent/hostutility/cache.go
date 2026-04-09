package hostutility

import (
	"sort"
	"sync"
)

// cache is a thread-safe in-memory map of agent capabilities keyed by agent type.
type cache struct {
	mu     sync.RWMutex
	byType map[string]AgentCapabilities
}

func newCache() *cache {
	return &cache{byType: make(map[string]AgentCapabilities)}
}

func (c *cache) set(caps AgentCapabilities) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byType[caps.AgentType] = caps
}

func (c *cache) get(agentType string) (AgentCapabilities, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	caps, ok := c.byType[agentType]
	return caps, ok
}

func (c *cache) all() []AgentCapabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]AgentCapabilities, 0, len(c.byType))
	for _, v := range c.byType {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentType < out[j].AgentType })
	return out
}
