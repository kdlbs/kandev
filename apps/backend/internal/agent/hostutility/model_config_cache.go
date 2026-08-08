package hostutility

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

const (
	modelConfigCacheTTL        = 5 * time.Minute
	modelConfigCacheMaxEntries = 128
)

type modelConfigCacheEntry struct {
	resolution ModelConfigResolution
	expiresAt  time.Time
}

type modelConfigCache struct {
	mu    sync.RWMutex
	items map[string]modelConfigCacheEntry
}

func newModelConfigCache() *modelConfigCache {
	return &modelConfigCache{items: make(map[string]modelConfigCacheEntry)}
}

func (c *modelConfigCache) get(key string, now time.Time) (ModelConfigResolution, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return ModelConfigResolution{}, false
	}
	if !now.Before(entry.expiresAt) {
		c.mu.Lock()
		if current, exists := c.items[key]; exists && !now.Before(current.expiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return ModelConfigResolution{}, false
	}
	return cloneModelConfigResolution(entry.resolution), true
}

func (c *modelConfigCache) set(key string, resolution ModelConfigResolution, now time.Time) {
	c.mu.Lock()
	for existingKey, entry := range c.items {
		if !now.Before(entry.expiresAt) {
			delete(c.items, existingKey)
		}
	}
	if _, exists := c.items[key]; !exists && len(c.items) >= modelConfigCacheMaxEntries {
		var oldestKey string
		var oldestExpiry time.Time
		for existingKey, entry := range c.items {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = existingKey
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey != "" {
			delete(c.items, oldestKey)
		}
	}
	c.items[key] = modelConfigCacheEntry{
		resolution: cloneModelConfigResolution(resolution),
		expiresAt:  now.Add(modelConfigCacheTTL),
	}
	c.mu.Unlock()
}

func (c *modelConfigCache) invalidateAgent(agentType string) {
	c.mu.Lock()
	for key, entry := range c.items {
		if entry.resolution.AgentType == agentType {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}

func (c *modelConfigCache) clear() {
	c.mu.Lock()
	c.items = make(map[string]modelConfigCacheEntry)
	c.mu.Unlock()
}

type modelConfigCacheOption struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type modelConfigCacheKeyData struct {
	AgentType     string                   `json:"agent_type"`
	Model         string                   `json:"model"`
	Mode          string                   `json:"mode,omitempty"`
	ConfigOptions []modelConfigCacheOption `json:"config_options,omitempty"`
}

func modelConfigCacheKey(agentType string, req ModelConfigResolutionRequest) string {
	keys := make([]string, 0, len(req.ConfigOptions))
	for key := range req.ConfigOptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	options := make([]modelConfigCacheOption, 0, len(keys))
	for _, key := range keys {
		options = append(options, modelConfigCacheOption{ID: key, Value: req.ConfigOptions[key]})
	}
	data, _ := json.Marshal(modelConfigCacheKeyData{
		AgentType:     agentType,
		Model:         req.Model,
		Mode:          req.Mode,
		ConfigOptions: options,
	})
	return string(data)
}

func cloneModelConfigResolution(in ModelConfigResolution) ModelConfigResolution {
	out := in
	out.ConfigOptions = append([]ConfigOption(nil), in.ConfigOptions...)
	for i := range out.ConfigOptions {
		out.ConfigOptions[i].Options = append([]ConfigOptionChoice(nil), in.ConfigOptions[i].Options...)
	}
	return out
}
