package hostutility

import (
	"container/list"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

const (
	profileCapabilityCacheTTL        = 5 * time.Minute
	profileCapabilityCacheMaxEntries = 256
)

type profileCapabilityEntry struct {
	agentType  string
	expiresAt  time.Time
	capability *ProfileCapabilityResult
	resolution *ModelConfigResolution
}

type profileCapabilityCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
}

func newProfileCapabilityCache() *profileCapabilityCache {
	return &profileCapabilityCache{items: make(map[string]*list.Element), lru: list.New()}
}

func (c *profileCapabilityCache) getCapability(key string, now time.Time) (ProfileCapabilityResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.items[key]
	if element == nil {
		return ProfileCapabilityResult{}, false
	}
	entry := c.entry(element)
	if !now.Before(entry.expiresAt) || entry.capability == nil {
		c.remove(key, element)
		return ProfileCapabilityResult{}, false
	}
	c.lru.MoveToFront(element)
	return cloneProfileCapabilityResult(*entry.capability), true
}

func (c *profileCapabilityCache) setCapability(key, agentType string, result ProfileCapabilityResult, now time.Time) {
	if result.Capabilities.Status != StatusOK && result.Capabilities.Status != StatusUnsupported {
		return
	}
	c.set(key, &profileCapabilityEntry{
		agentType: agentType, expiresAt: now.Add(profileCapabilityCacheTTL),
		capability: ptrProfileCapabilityResult(cloneProfileCapabilityResult(result)),
	})
}

func (c *profileCapabilityCache) getResolution(key string, now time.Time) (ModelConfigResolution, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.items[key]
	if element == nil {
		return ModelConfigResolution{}, false
	}
	entry := c.entry(element)
	if !now.Before(entry.expiresAt) || entry.resolution == nil {
		c.remove(key, element)
		return ModelConfigResolution{}, false
	}
	c.lru.MoveToFront(element)
	return cloneModelConfigResolution(*entry.resolution), true
}

func (c *profileCapabilityCache) setResolution(key, agentType string, resolution ModelConfigResolution, now time.Time) {
	if resolution.Status != StatusOK && resolution.Status != StatusUnsupported {
		return
	}
	cloned := cloneModelConfigResolution(resolution)
	c.set(key, &profileCapabilityEntry{agentType: agentType, expiresAt: now.Add(profileCapabilityCacheTTL), resolution: &cloned})
}

func (c *profileCapabilityCache) set(key string, entry *profileCapabilityEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.items[key]; element != nil {
		element.Value = profileCapabilityCacheItem{key: key, entry: entry}
		c.lru.MoveToFront(element)
		return
	}
	for len(c.items) >= profileCapabilityCacheMaxEntries {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		delete(c.items, oldest.Value.(profileCapabilityCacheItem).key)
		c.lru.Remove(oldest)
	}
	element := c.lru.PushFront(profileCapabilityCacheItem{key: key, entry: entry})
	c.items[key] = element
}

type profileCapabilityCacheItem struct {
	key   string
	entry *profileCapabilityEntry
}

func (c *profileCapabilityCache) remove(key string, element *list.Element) {
	delete(c.items, key)
	c.lru.Remove(element)
}

func (c *profileCapabilityCache) invalidateAgent(agentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, element := range c.items {
		if element.Value.(profileCapabilityCacheItem).entry.agentType == agentType {
			c.remove(key, element)
		}
	}
}

func (c *profileCapabilityCache) clear() {
	c.mu.Lock()
	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.mu.Unlock()
}

func (c *profileCapabilityCache) entry(element *list.Element) *profileCapabilityEntry {
	return element.Value.(profileCapabilityCacheItem).entry
}

type profileContextIdentity struct {
	Scope             string            `json:"scope"`
	AgentType         string            `json:"agent_type"`
	Protocol          string            `json:"protocol"`
	Command           []string          `json:"command"`
	OperatorDefined   bool              `json:"operator_defined"`
	CLIFlags          []string          `json:"cli_flags"`
	CommandPrefix     []string          `json:"command_prefix"`
	Env               []profileEnvValue `json:"env"`
	StripEnv          []string          `json:"strip_env"`
	RuntimeGeneration uint64            `json:"runtime_generation"`
	ContextGeneration uint64            `json:"context_generation"`
}

type profileEnvValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func profileContextRevision(key []byte, context ProfileProbeContext, agentType, protocol string, command []string, operatorDefined bool, stripEnv []string, runtimeGeneration, contextGeneration uint64) string {
	envKeys := make([]string, 0, len(context.Env))
	for name := range context.Env {
		envKeys = append(envKeys, name)
	}
	sort.Strings(envKeys)
	env := make([]profileEnvValue, 0, len(envKeys))
	for _, name := range envKeys {
		env = append(env, profileEnvValue{Key: name, Value: context.Env[name]})
	}
	identity := profileContextIdentity{
		Scope: context.Scope, AgentType: agentType, Protocol: protocol,
		Command: append([]string(nil), command...), OperatorDefined: operatorDefined,
		CLIFlags:      append([]string(nil), context.CLIFlags...),
		CommandPrefix: append([]string(nil), context.CommandPrefix...), Env: env,
		StripEnv:          append([]string(nil), stripEnv...),
		RuntimeGeneration: runtimeGeneration, ContextGeneration: contextGeneration,
	}
	data, _ := json.Marshal(identity)
	return profileHMAC(key, append([]byte("profile-context-v1:"), data...))
}

func profileCapabilityKey(key []byte, contextRevision string) string {
	return profileHMAC(key, []byte("profile-capabilities-v1:"+contextRevision))
}

func profileModelConfigKey(key []byte, contextRevision string, req ModelConfigResolutionRequest) string {
	optionKeys := make([]string, 0, len(req.ConfigOptions))
	for option := range req.ConfigOptions {
		optionKeys = append(optionKeys, option)
	}
	sort.Strings(optionKeys)
	options := make([]modelConfigCacheOption, 0, len(optionKeys))
	for _, option := range optionKeys {
		options = append(options, modelConfigCacheOption{ID: option, Value: req.ConfigOptions[option]})
	}
	data, _ := json.Marshal(struct {
		ContextRevision string                   `json:"context_revision"`
		Model           string                   `json:"model"`
		Mode            string                   `json:"mode,omitempty"`
		Options         []modelConfigCacheOption `json:"options,omitempty"`
	}{contextRevision, req.Model, req.Mode, options})
	return profileHMAC(key, append([]byte("profile-model-options-v1:"), data...))
}

func profileHMAC(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func ptrProfileCapabilityResult(in ProfileCapabilityResult) *ProfileCapabilityResult { return &in }

func cloneProfileCapabilityResult(in ProfileCapabilityResult) ProfileCapabilityResult {
	in.Capabilities = cloneAgentCapabilities(in.Capabilities)
	return in
}

func cloneAgentCapabilities(in AgentCapabilities) AgentCapabilities {
	data, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out AgentCapabilities
	if err := json.Unmarshal(data, &out); err != nil {
		return in
	}
	return out
}
