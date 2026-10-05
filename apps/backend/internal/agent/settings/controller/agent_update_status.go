package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const (
	runtimeUpdateStatusSuccessTTL    = 6 * time.Hour
	runtimeUpdateStatusFailureTTL    = 15 * time.Minute
	runtimeUpdateStatusMaxConcurrent = 5
	runtimeUpdateStatusLookupTimeout = 10 * time.Second
)

// RuntimeUpdateStatusResolver is the latest-version lookup seam used by the
// cached status endpoint. Production resolves through the configured runtime
// updater; tests and embedders can provide a deterministic implementation.
type RuntimeUpdateStatusResolver func(context.Context, string) (string, error)

type runtimeUpdateStatusCacheEntry struct {
	latest    string
	metadata  *RuntimeVersionMetadata
	checkedAt time.Time
	expiresAt time.Time
	ok        bool
}

type runtimeUpdateStatusTarget struct {
	agentName        string
	updateMode       dto.AgentUpdateMode
	packageName      string
	defaultVersion   string
	activeVersion    string
	effectiveVersion string
	selectionErr     error
	capability       agents.RuntimeUpdateCapability
	displayName      string
	currentVersion   string
	available        bool
	enabled          bool
}

// SetRuntimeUpdateStatusClock injects the clock used for status cache TTLs.
// Passing nil restores the wall clock.
func (c *Controller) SetRuntimeUpdateStatusClock(now func() time.Time) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	if now == nil {
		c.runtimeUpdateStatusNow = time.Now
		return
	}
	c.runtimeUpdateStatusNow = now
}

// SetRuntimeUpdateStatusResolver injects the read-only latest-version lookup
// used by the status endpoint and clears prior results from another resolver.
func (c *Controller) SetRuntimeUpdateStatusResolver(resolver RuntimeUpdateStatusResolver) {
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	c.runtimeUpdateStatusResolver = resolver
	c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
}

// InvalidateRuntimeUpdateStatus expires only the affected package. Successful
// activation and return-to-default jobs call this after persistence/probing.
func (c *Controller) InvalidateRuntimeUpdateStatus(packageName string) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return
	}
	c.runtimeUpdateStatusMu.Lock()
	defer c.runtimeUpdateStatusMu.Unlock()
	delete(c.runtimeUpdateStatusCache, runtimeUpdateStatusCacheKey(packageName, dto.AgentUpdateModePinned))
	delete(c.runtimeUpdateStatusCache, runtimeUpdateStatusCacheKey(packageName, dto.AgentUpdateModeSelfUpdate))
}

func runtimeUpdateStatusCacheKey(packageName string, mode dto.AgentUpdateMode) string {
	return string(mode) + "\x00" + packageName
}

// ListAgentUpdateStatuses returns one non-mutating status item for each
// registered runtime. Source failures are represented as
// unknown entries rather than failing the whole batch.
func (c *Controller) ListAgentUpdateStatuses(ctx context.Context) (*dto.ListAgentUpdateStatusResponse, error) {
	targets, err := c.runtimeUpdateStatusTargets(ctx)
	if err != nil {
		return nil, err
	}

	now := c.runtimeUpdateStatusTime()
	entries := make(map[string]runtimeUpdateStatusCacheEntry, len(targets))
	uniquePackages := make(map[string]struct {
		packageName string
		mode        dto.AgentUpdateMode
	}, len(targets))
	for _, target := range targets {
		if target.packageName == "" || !target.available || !target.enabled {
			continue
		}
		key := runtimeUpdateStatusCacheKey(target.packageName, target.updateMode)
		uniquePackages[key] = struct {
			packageName string
			mode        dto.AgentUpdateMode
		}{packageName: target.packageName, mode: target.updateMode}
	}
	type result struct {
		key   string
		entry runtimeUpdateStatusCacheEntry
	}
	results := make(chan result, len(uniquePackages))
	for key, target := range uniquePackages {
		go func(key, packageName string, mode dto.AgentUpdateMode) {
			entry := c.runtimeUpdateStatusEntry(ctx, packageName, mode, now)
			results <- result{key: key, entry: entry}
		}(key, target.packageName, target.mode)
	}
	for range uniquePackages {
		item := <-results
		entries[item.key] = item.entry
	}

	statuses := make([]dto.AgentUpdateStatusDTO, 0, len(targets))
	for _, target := range targets {
		entry := entries[runtimeUpdateStatusCacheKey(target.packageName, target.updateMode)]
		status := dto.AgentUpdateStatusDTO{
			UpdateMode:          target.updateMode,
			ManagedFallback:     target.capability.ManagedFallback != nil && c.verifiedManagedActivation(),
			AgentName:           target.agentName,
			DisplayName:         target.displayName,
			RuntimeID:           target.capability.RuntimeID,
			Owner:               target.capability.Owner,
			Mechanism:           target.capability.Mechanism,
			Management:          target.capability.Management,
			Source:              target.packageName,
			GuidanceURL:         target.capability.Source.GuidanceURL,
			CurrentVersion:      target.currentVersion,
			Available:           target.available,
			Enabled:             target.enabled,
			AutoUpdateSupported: c.automaticUpdateSupported(target.capability) && target.available && target.enabled,
			Package:             target.packageName,
			DefaultVersion:      target.defaultVersion,
			ActiveVersion:       target.activeVersion,
			EffectiveVersion:    comparableRuntimeVersion(target, entry),
			CheckState:          dto.AgentUpdateCheckStateUnknown,
		}
		if !entry.checkedAt.IsZero() {
			checkedAt := entry.checkedAt
			status.CheckedAt = &checkedAt
		}
		if entry.ok {
			status.LatestVersion = entry.latest
		}
		if target.selectionErr == nil && entry.ok {
			if target.updateMode == dto.AgentUpdateModeSelfUpdate {
				status.CheckState = compareHarnessUpdateStatus(entry.latest, status.EffectiveVersion)
			} else {
				status.CheckState = compareRuntimeUpdateStatus(entry.latest, status.EffectiveVersion)
			}
		}
		if c.runtimeAutoUpdateStore != nil {
			c.runtimeAutoUpdateMu.Lock()
			policy, err := c.runtimeAutoUpdateStore.Get(ctx, target.agentName, target.capability.RuntimeID)
			c.runtimeAutoUpdateMu.Unlock()
			if err == nil {
				status.AutoUpdate, status.LastOutcome = policy.Enabled, policy.Outcome
			}
		}
		statuses = append(statuses, status)
	}
	return &dto.ListAgentUpdateStatusResponse{Statuses: statuses}, nil
}

func comparableRuntimeVersion(target runtimeUpdateStatusTarget, entry runtimeUpdateStatusCacheEntry) string {
	if target.updateMode == dto.AgentUpdateModeSelfUpdate || target.capability.Managed != nil || target.capability.Source.NPM == "" {
		return target.effectiveVersion
	}
	current, err := managedruntime.ParseStableVersion(target.effectiveVersion)
	if err != nil || entry.metadata == nil {
		return ""
	}
	// An ACP observation may describe the vendor CLI rather than its npm adapter.
	for _, published := range entry.metadata.Versions {
		version, err := managedruntime.ParseStableVersion(published)
		if err == nil && current.Equal(version) {
			return target.effectiveVersion
		}
	}
	return ""
}

func (c *Controller) runtimeUpdateStatusTargets(ctx context.Context) ([]runtimeUpdateStatusTarget, error) {
	if c.agentRegistry == nil {
		return nil, nil
	}
	available := map[string]bool{}
	if c.discovery != nil {
		results, err := c.detectAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("detect available agents: %w", err)
		}
		for _, result := range results {
			available[result.Name] = result.Available
		}
	}

	targets := make([]runtimeUpdateStatusTarget, 0)
	for _, ag := range c.agentRegistry.List() {
		capability := agents.RuntimeUpdateCapabilities(ag)
		target := runtimeUpdateStatusTarget{
			agentName: ag.ID(), displayName: ag.DisplayName(), capability: capability,
			available: c.discovery == nil || available[ag.ID()], enabled: ag.Enabled(),
		}
		target.packageName = capability.Source.NPM
		if capability.Source.GitHubRepository != "" {
			target.packageName = "github:" + capability.Source.GitHubRepository
		}
		if c.runtimeUpdater != nil {
			if caps, found := c.runtimeUpdater.CurrentCapabilities(ag.ID()); found {
				target.currentVersion = caps.AgentVersion
			}
		}
		target.effectiveVersion = target.currentVersion
		if harness, ok := ag.(agents.HarnessUpdateAgent); ok {
			spec := harness.HarnessUpdate()
			if spec.Package == "" || spec.UpdateCommand.IsEmpty() {
				continue
			}
			target.packageName = spec.Package
			target.updateMode = dto.AgentUpdateModeSelfUpdate
			targets = append(targets, target)
			continue
		}
		if capability.Managed != nil {
			target.updateMode = dto.AgentUpdateModePinned
			target.activeVersion, target.effectiveVersion, target.defaultVersion, target.selectionErr = c.runtimeVersions(ctx, ag.ID(), *capability.Managed)
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (c *Controller) runtimeUpdateStatusEntry(
	ctx context.Context,
	packageName string,
	mode dto.AgentUpdateMode,
	now time.Time,
) runtimeUpdateStatusCacheEntry {
	if ctx.Err() != nil {
		return runtimeUpdateStatusCacheEntry{}
	}
	c.runtimeUpdateStatusMu.Lock()
	key := runtimeUpdateStatusCacheKey(packageName, mode)
	if entry, ok := c.runtimeUpdateStatusCache[key]; ok && now.Before(entry.expiresAt) {
		c.runtimeUpdateStatusMu.Unlock()
		return entry
	}
	lookup := c.runtimeUpdateStatusLookup
	if lookup == nil {
		lookup = make(chan struct{}, runtimeUpdateStatusMaxConcurrent)
		c.runtimeUpdateStatusLookup = lookup
	}
	c.runtimeUpdateStatusMu.Unlock()

	result := c.runtimeUpdateStatusFlight.DoChan(key, func() (interface{}, error) {
		lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeUpdateStatusLookupTimeout)
		defer cancel()
		select {
		case lookup <- struct{}{}:
		case <-lookupCtx.Done():
			return runtimeUpdateStatusCacheEntry{}, nil
		}
		defer func() { <-lookup }()
		c.runtimeUpdateStatusMu.Lock()
		cached, found := c.runtimeUpdateStatusCache[key]
		c.runtimeUpdateStatusMu.Unlock()
		if found && now.Before(cached.expiresAt) {
			return cached, nil
		}
		latest, metadata, err := c.resolveRuntimeUpdateLatest(lookupCtx, packageName, mode)
		entry := runtimeUpdateStatusCacheEntry{expiresAt: now.Add(runtimeUpdateStatusFailureTTL)}
		if err == nil {
			entry.latest, entry.ok, entry.checkedAt, entry.expiresAt = latest, true, now, now.Add(runtimeUpdateStatusSuccessTTL)
			entry.metadata = metadata
		}
		c.runtimeUpdateStatusMu.Lock()
		if c.runtimeUpdateStatusCache == nil {
			c.runtimeUpdateStatusCache = make(map[string]runtimeUpdateStatusCacheEntry)
		}
		c.runtimeUpdateStatusCache[key] = entry
		c.runtimeUpdateStatusMu.Unlock()
		return entry, nil
	})
	select {
	case <-ctx.Done():
		return runtimeUpdateStatusCacheEntry{}
	case value := <-result:
		return value.Val.(runtimeUpdateStatusCacheEntry)
	}
}

func (c *Controller) resolveRuntimeUpdateLatest(ctx context.Context, packageName string, mode dto.AgentUpdateMode) (string, *RuntimeVersionMetadata, error) {
	c.runtimeUpdateStatusMu.Lock()
	resolver := c.runtimeUpdateStatusResolver
	c.runtimeUpdateStatusMu.Unlock()
	if resolver != nil {
		latest, err := validateRuntimeUpdateLatest(resolver(ctx, packageName))
		return latest, nil, err
	}
	if mode == dto.AgentUpdateModeSelfUpdate {
		latest, err := c.resolveHarnessLatest(ctx, packageName)
		return latest, nil, err
	}
	if strings.HasPrefix(packageName, "github:") {
		latest, err := c.resolveGitHubRuntimeRelease(ctx, strings.TrimPrefix(packageName, "github:"))
		return latest, nil, err
	}
	if c.runtimeUpdater == nil {
		return "", nil, errors.New("runtime updater unavailable")
	}
	if metadataResolver, ok := c.runtimeUpdater.(RuntimeVersionResolver); ok {
		metadata, err := metadataResolver.ResolveVersions(ctx, packageName)
		if err != nil {
			return "", nil, err
		}
		latest, err := validateRuntimeUpdateLatest(metadata.Latest, nil)
		metadata.Versions = append([]string(nil), metadata.Versions...)
		return latest, &metadata, err
	}
	latest, err := validateRuntimeUpdateLatest(c.runtimeUpdater.ResolveTarget(ctx, packageName))
	return latest, nil, err
}

func (c *Controller) validateAutomaticRuntimeTarget(ctx context.Context, spec agents.ManagedNPMRuntimeSpec, target string) error {
	now := c.runtimeUpdateStatusTime()
	c.runtimeUpdateStatusMu.Lock()
	entry := c.runtimeUpdateStatusCache[runtimeUpdateStatusCacheKey(spec.Package, dto.AgentUpdateModePinned)]
	c.runtimeUpdateStatusMu.Unlock()
	if entry.ok && now.Before(entry.expiresAt) && entry.latest == target && entry.metadata != nil {
		return validateRuntimeCatalogueTarget(*entry.metadata, target)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, runtimeUpdateStatusLookupTimeout)
	defer cancel()
	return c.validateAgentUpdateTarget(lookupCtx, spec, target)
}

func validateRuntimeUpdateLatest(latest string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	latest = strings.TrimSpace(latest)
	if _, parseErr := managedruntime.ParseStableVersion(latest); parseErr != nil {
		return "", parseErr
	}
	return latest, nil
}

func compareRuntimeUpdateStatus(latest, effective string) dto.AgentUpdateCheckState {
	latestVersion, latestErr := managedruntime.ParseStableVersion(latest)
	effectiveVersion, effectiveErr := managedruntime.ParseStableVersion(effective)
	if latestErr != nil || effectiveErr != nil {
		return dto.AgentUpdateCheckStateUnknown
	}
	if latestVersion.GreaterThan(effectiveVersion) {
		return dto.AgentUpdateCheckStateUpdateAvailable
	}
	return dto.AgentUpdateCheckStateUpToDate
}

func compareHarnessUpdateStatus(stableLatest, effective string) dto.AgentUpdateCheckState {
	latest, latestErr := managedruntime.ParseStableVersion(stableLatest)
	installed, installedErr := managedruntime.ParseStableVersion(effective)
	if latestErr != nil || installedErr != nil {
		return dto.AgentUpdateCheckStateUnknown
	}
	if latest.GreaterThan(installed) {
		return dto.AgentUpdateCheckStateUpdateAvailable
	}
	// The stable tag cannot establish that a configured canary channel is current.
	return dto.AgentUpdateCheckStateUnknown
}

func (c *Controller) runtimeUpdateStatusTime() time.Time {
	c.runtimeUpdateStatusMu.Lock()
	now := c.runtimeUpdateStatusNow
	c.runtimeUpdateStatusMu.Unlock()
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}
