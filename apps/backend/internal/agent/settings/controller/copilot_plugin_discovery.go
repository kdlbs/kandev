package controller

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

const (
	copilotPluginDiscoveryProvider = "copilot-acp"
	copilotPluginCacheTTL          = 30 * time.Second
	copilotPluginCacheMaxEntries   = 256
)

// ErrCopilotPluginDiscoverySuperseded reports that a newer refresh started
// before this result could be published. The caller must not use the result.
var ErrCopilotPluginDiscoverySuperseded = errors.New("copilot plugin discovery superseded")

// DiscoverCopilotPlugins returns a safe, profile-aware inventory of installed
// Copilot plugins for the concrete Copilot provider.
func (c *Controller) DiscoverCopilotPlugins(
	ctx context.Context,
	agentID string,
	req dto.CopilotPluginDiscoveryRequest,
) (*dto.CopilotPluginDiscoveryResponse, error) {
	agent, err := c.copilotPluginAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	unsupported := &dto.CopilotPluginDiscoveryResponse{
		AgentID: agentID, ProfileID: req.ProfileID, ProviderID: copilotPluginDiscoveryProvider,
		Status: "unsupported", Reason: "unsupported_agent",
		Plugins: []dto.CopilotPluginDiscoveryPlugin{},
	}
	registered, ok := c.agentRegistry.Get(agent.Name)
	if !ok || agent.Name != agents.CopilotACPAgentID {
		return unsupported, nil
	}
	managed, ok := registered.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return unsupported, nil
	}
	profileContext, err := c.resolveProfileProbeContext(
		ctx, agent.Name, req.AuthorizationScope, req.ProfileID, req.LaunchSettings,
	)
	if err != nil {
		return nil, err
	}
	spec := managed.ManagedNPMRuntime()
	version, err := c.copilotPluginRuntimeVersion(ctx, agent.Name, spec)
	if err != nil {
		return nil, err
	}
	revision, err := c.copilotPluginContextRevision(req.AuthorizationScope, agentID, req.ProfileID, version, profileContext)
	if err != nil {
		return nil, err
	}
	generation := c.beginCopilotPluginDiscovery(revision, req.Refresh)
	if !req.Refresh {
		if cached, ok := c.copilotPluginCacheInstance().get(revision, time.Now()); ok {
			if !c.copilotPluginDiscoveryCurrent(revision, generation) {
				return nil, ErrCopilotPluginDiscoverySuperseded
			}
			return cached, nil
		}
	}
	launch := copilotplugins.LaunchContext{
		AgentType:      agent.Name,
		ExecutorType:   "",
		Env:            profileContext.Env,
		CLIFlags:       profileContext.CLIFlags,
		CommandPrefix:  profileContext.CommandPrefix,
		RuntimeVersion: version,
		RuntimeCommand: spec.InteractiveCommand(version).Args(),
	}
	snapshot := copilotplugins.Discover(ctx, c.copilotPluginRunner(), launch)
	// A runtime activation may land while the native inventory runs. Never
	// publish or return an inventory qualified for a version that is no
	// longer active.
	activeVersion, err := c.copilotPluginRuntimeVersion(ctx, agent.Name, spec)
	if err != nil {
		return nil, err
	}
	if activeVersion != version {
		return nil, ErrCopilotPluginDiscoverySuperseded
	}
	response := &dto.CopilotPluginDiscoveryResponse{
		AgentID:         agentID,
		ProfileID:       req.ProfileID,
		ProviderID:      copilotPluginDiscoveryProvider,
		Status:          string(snapshot.Status),
		Reason:          string(snapshot.Reason),
		ContextRevision: revision,
		RuntimeVersion:  snapshot.RuntimeVersion,
		MountPermitted:  copilotplugins.MountsPermitted(version),
		Plugins:         copilotPluginRows(snapshot.Rows),
	}
	if !c.publishCopilotPluginDiscovery(revision, generation, response) {
		return nil, ErrCopilotPluginDiscoverySuperseded
	}
	return response, nil
}

func (c *Controller) copilotPluginAgent(ctx context.Context, agentID string) (*models.Agent, error) {
	if c.repo == nil || strings.TrimSpace(agentID) == "" {
		return nil, ErrAgentNotFound
	}
	agent, err := c.repo.GetAgent(ctx, agentID)
	if errors.Is(err, sql.ErrNoRows) || agent == nil {
		return nil, ErrAgentNotFound
	}
	if err != nil {
		return nil, err
	}
	return agent, nil
}

func (c *Controller) copilotPluginRuntimeVersion(ctx context.Context, agentName string, spec agents.ManagedNPMRuntimeSpec) (string, error) {
	defaultVersion := spec.DefaultVersionOrPinned()
	if c.managedRuntimeSelections == nil {
		return defaultVersion, nil
	}
	selection, found, err := c.managedRuntimeSelections.Get(ctx, agentName, spec.Package)
	if err != nil {
		return "", fmt.Errorf("resolve active copilot runtime: %w", err)
	}
	if !found || selection.Package != spec.Package || selection.Version == "" {
		return defaultVersion, nil
	}
	return selection.Version, nil
}

func (c *Controller) copilotPluginRunner() copilotplugins.Runner {
	if c.copilotPluginRunnerImpl != nil {
		return c.copilotPluginRunnerImpl
	}
	return copilotplugins.ExecRunner{}
}

func copilotPluginRows(rows []copilotplugins.InventoryRow) []dto.CopilotPluginDiscoveryPlugin {
	plugins := make([]dto.CopilotPluginDiscoveryPlugin, 0, len(rows))
	for _, row := range rows {
		plugins = append(plugins, dto.CopilotPluginDiscoveryPlugin{
			ID:            row.ID.String(),
			Name:          row.Name,
			Marketplace:   row.Marketplace,
			Version:       row.Version,
			NativeEnabled: row.NativeEnabled,
			Availability:  string(row.Availability),
			ReasonCode:    string(row.Reason),
		})
	}
	return plugins
}

func (c *Controller) copilotPluginContextRevision(scope, agentID, profileID, version string, profileContext hostutility.ProfileProbeContext) (string, error) {
	key, err := c.copilotPluginHMACKey()
	if err != nil {
		return "", err
	}
	hash := hmac.New(sha256.New, key)
	writeRevisionPart(hash, scope)
	writeRevisionPart(hash, agentID)
	writeRevisionPart(hash, profileID)
	writeRevisionPart(hash, version)
	writeRevisionPart(hash, strings.Join(profileContext.CLIFlags, "\x00"))
	writeRevisionPart(hash, strings.Join(profileContext.CommandPrefix, "\x00"))
	envKeys := make([]string, 0, len(profileContext.Env))
	for key := range profileContext.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		writeRevisionPart(hash, key)
		writeRevisionPart(hash, profileContext.Env[key])
	}
	return hex.EncodeToString(hash.Sum(nil))[:32], nil
}

func (c *Controller) copilotPluginHMACKey() ([]byte, error) {
	c.copilotPluginMu.Lock()
	defer c.copilotPluginMu.Unlock()
	if len(c.copilotPluginKey) == 0 {
		key := make([]byte, 32)
		reader := c.copilotPluginRandom
		if reader == nil {
			reader = rand.Reader
		}
		if _, err := io.ReadFull(reader, key); err != nil {
			return nil, fmt.Errorf("generate copilot plugin discovery key: %w", err)
		}
		c.copilotPluginKey = key
	}
	return c.copilotPluginKey, nil
}

func writeRevisionPart(hash hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(value))
}

func (c *Controller) beginCopilotPluginDiscovery(revision string, refresh bool) uint64 {
	c.copilotPluginMu.Lock()
	defer c.copilotPluginMu.Unlock()
	if c.copilotPluginGenerations == nil {
		c.copilotPluginGenerations = make(map[string]uint64)
	}
	if len(c.copilotPluginGenerations) > 2048 {
		// Drop per-revision bookkeeping, never the monotonic counter. A
		// surviving in-flight result holds an older counter value and cannot
		// match a later generation.
		c.copilotPluginGenerations = make(map[string]uint64)
	}
	if c.copilotPluginCache == nil {
		c.copilotPluginCache = newCopilotPluginDiscoveryCache()
	}
	if refresh {
		c.copilotPluginGeneration++
		c.copilotPluginGenerations[revision] = c.copilotPluginGeneration
		c.copilotPluginCache.delete(revision)
	}
	return c.copilotPluginGenerations[revision]
}

func (c *Controller) copilotPluginCacheInstance() *copilotPluginDiscoveryCache {
	c.copilotPluginMu.Lock()
	defer c.copilotPluginMu.Unlock()
	if c.copilotPluginCache == nil {
		c.copilotPluginCache = newCopilotPluginDiscoveryCache()
	}
	return c.copilotPluginCache
}

func (c *Controller) copilotPluginDiscoveryCurrent(revision string, generation uint64) bool {
	c.copilotPluginMu.Lock()
	defer c.copilotPluginMu.Unlock()
	return c.copilotPluginGenerations[revision] == generation
}

func (c *Controller) publishCopilotPluginDiscovery(
	revision string,
	generation uint64,
	response *dto.CopilotPluginDiscoveryResponse,
) bool {
	c.copilotPluginMu.Lock()
	defer c.copilotPluginMu.Unlock()
	if c.copilotPluginGenerations[revision] != generation {
		return false
	}
	if c.copilotPluginCache == nil {
		c.copilotPluginCache = newCopilotPluginDiscoveryCache()
	}
	c.copilotPluginCache.set(revision, response, time.Now())
	return true
}

type copilotPluginCacheEntry struct {
	value     *dto.CopilotPluginDiscoveryResponse
	expiresAt time.Time
}

type copilotPluginDiscoveryCache struct {
	mu      sync.Mutex
	entries map[string]copilotPluginCacheEntry
	order   []string
}

func newCopilotPluginDiscoveryCache() *copilotPluginDiscoveryCache {
	return &copilotPluginDiscoveryCache{entries: make(map[string]copilotPluginCacheEntry)}
}

func (c *copilotPluginDiscoveryCache) get(key string, now time.Time) (*dto.CopilotPluginDiscoveryResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !now.Before(entry.expiresAt) {
		c.deleteLocked(key)
		return nil, false
	}
	return cloneCopilotPluginResponse(entry.value), true
}

func (c *copilotPluginDiscoveryCache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteLocked(key)
}

func (c *copilotPluginDiscoveryCache) deleteLocked(key string) {
	delete(c.entries, key)
	for index, candidate := range c.order {
		if candidate == key {
			c.order = append(c.order[:index], c.order[index+1:]...)
			return
		}
	}
}

func (c *copilotPluginDiscoveryCache) set(key string, value *dto.CopilotPluginDiscoveryResponse, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = copilotPluginCacheEntry{value: cloneCopilotPluginResponse(value), expiresAt: now.Add(copilotPluginCacheTTL)}
	for len(c.order) > copilotPluginCacheMaxEntries {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func cloneCopilotPluginResponse(value *dto.CopilotPluginDiscoveryResponse) *dto.CopilotPluginDiscoveryResponse {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Plugins = append([]dto.CopilotPluginDiscoveryPlugin{}, value.Plugins...)
	return &clone
}
