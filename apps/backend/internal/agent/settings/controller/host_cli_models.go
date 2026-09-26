package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostcli"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// hostCLIRefreshTimeout bounds one post-install rediscovery.
const hostCLIRefreshTimeout = 30 * time.Second

// Model-discovery statuses reported to the settings UI.
const (
	hostCLIModelStatusOK           = "ok"
	hostCLIModelStatusSkipped      = "skipped"
	hostCLIModelStatusPending      = "pending"
	hostCLIModelStatusNotInstalled = "not_installed"
	hostCLIModelStatusNotLoggedIn  = "not_logged_in"
	hostCLIModelStatusTimeout      = "timeout"
	hostCLIModelStatusFailed       = "failed"
)

// Model-list provenance reported to the settings UI.
const (
	modelDiscoverySourceCLI    = "cli_command"
	modelDiscoverySourceProbe  = "acp_probe"
	modelEntrySourceCLI        = "cli"
	modelEntrySourceACP        = "acp"
	hostCLIWarmupMaxConcurrent = 2
)

// hostCLIModelTTL bounds how long a discovered catalogue is reused by the
// agent-load warmup. Explicit refreshes and installs ignore it.
const hostCLIModelTTL = 10 * time.Minute

// hostCLIModelEntry is the process-local result of one model discovery.
type hostCLIModelEntry struct {
	models    []hostcli.Model
	status    string
	errMsg    string
	checkedAt time.Time
}

// SetHostCLIRunner replaces the process boundary used for vendor CLI model
// discovery and version detection. Passing nil restores the real runner.
func (c *Controller) SetHostCLIRunner(runner hostcli.Runner) {
	c.hostCLIMu.Lock()
	if runner == nil {
		c.hostCLIRunner = hostcli.ExecRunner{}
	} else {
		c.hostCLIRunner = runner
	}
	c.hostCLIModels = make(map[string]hostCLIModelEntry)
	c.hostCLIMu.Unlock()
	c.discovery.SetHostCLIRunner(runner)
}

func (c *Controller) hostCLIProcessRunner() hostcli.Runner {
	c.hostCLIMu.Lock()
	defer c.hostCLIMu.Unlock()
	if c.hostCLIRunner == nil {
		c.hostCLIRunner = hostcli.ExecRunner{}
	}
	return c.hostCLIRunner
}

// hostCLISpec returns the compiled vendor CLI metadata for an agent type.
func (c *Controller) hostCLISpec(agentName string) (hostcli.Spec, bool) {
	if c.agentRegistry == nil {
		return hostcli.Spec{}, false
	}
	ag, ok := c.agentRegistry.Get(agentName)
	if !ok {
		return hostcli.Spec{}, false
	}
	cliAgent, ok := ag.(agents.HostCLIAgent)
	if !ok {
		return hostcli.Spec{}, false
	}
	spec := cliAgent.HostCLI()
	if !spec.Valid() {
		return hostcli.Spec{}, false
	}
	return spec, true
}

// InvalidateHostCLIModels drops one agent type's discovered catalogue so the
// next refresh re-reads the CLI. Called after a successful agent install.
func (c *Controller) InvalidateHostCLIModels(agentName string) {
	c.hostCLIMu.Lock()
	delete(c.hostCLIModels, agentName)
	c.hostCLIMu.Unlock()
}

// WarmHostCLIModels discovers models for every available host-CLI agent type
// whose catalogue is missing or stale, off the request path. Backend startup
// and an Agents settings page load both call it; failures only leave the
// bridge list in place.
func (c *Controller) WarmHostCLIModels(ctx context.Context) {
	if c.agentRegistry == nil {
		return
	}
	slots := make(chan struct{}, hostCLIWarmupMaxConcurrent)
	var wg sync.WaitGroup
	for _, ag := range c.agentRegistry.ListEnabled() {
		spec, ok := c.hostCLISpec(ag.ID())
		if !ok || !spec.HasModelSource() || c.hostCLIModelsFresh(ag.ID()) {
			continue
		}
		wg.Add(1)
		go func(agentName string) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			c.refreshHostCLIModels(ctx, agentName)
		}(ag.ID())
	}
	wg.Wait()
}

// hostCLIModelsFresh reports whether a discovered catalogue is recent enough
// for the warmup to skip it. It keeps a settings page load from re-reading
// every CLI on every visit.
func (c *Controller) hostCLIModelsFresh(agentName string) bool {
	c.hostCLIMu.Lock()
	entry, ok := c.hostCLIModels[agentName]
	c.hostCLIMu.Unlock()
	return ok && time.Since(entry.checkedAt) < hostCLIModelTTL
}

// hostCLIInstallSucceeded re-reads a vendor CLI after the existing agent
// install flow replaced it. The install script runs `npm install -g` for the
// CLI, so the binary on disk can be a different release with a different
// model list; discovery caches must not outlive it.
func (c *Controller) hostCLIInstallSucceeded(agentName string) {
	if _, ok := c.hostCLISpec(agentName); !ok {
		return
	}
	c.InvalidateHostCLIModels(agentName)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), hostCLIRefreshTimeout)
		defer cancel()
		c.refreshHostCLIModels(ctx, agentName)
	}()
}

// refreshHostCLIModels runs one discovery and stores its outcome.
func (c *Controller) refreshHostCLIModels(ctx context.Context, agentName string) hostCLIModelEntry {
	spec, ok := c.hostCLISpec(agentName)
	if !ok {
		return hostCLIModelEntry{}
	}
	if !spec.HasModelSource() {
		entry := hostCLIModelEntry{status: hostCLIModelStatusSkipped, checkedAt: time.Now().UTC()}
		c.storeHostCLIModels(agentName, entry)
		return entry
	}
	models, err := hostcli.ListCodexModels(ctx, c.hostCLIProcessRunner(), c.hostCLIPath(ctx, agentName, spec))
	entry := hostCLIModelEntry{checkedAt: time.Now().UTC()}
	if err != nil {
		entry.status, entry.errMsg = classifyHostCLIModelError(err)
		c.logger.Debug("host cli model discovery failed",
			zap.String("agent", agentName), zap.String("status", entry.status), zap.Error(err))
	} else {
		entry.status = hostCLIModelStatusOK
		entry.models = models
		c.logger.Info("host cli models discovered",
			zap.String("agent", agentName), zap.Int("models", len(models)))
	}
	c.storeHostCLIModels(agentName, entry)
	return entry
}

func (c *Controller) storeHostCLIModels(agentName string, entry hostCLIModelEntry) {
	c.hostCLIMu.Lock()
	if c.hostCLIModels == nil {
		c.hostCLIModels = make(map[string]hostCLIModelEntry)
	}
	c.hostCLIModels[agentName] = entry
	c.hostCLIMu.Unlock()
}

// cachedHostCLIModels returns the stored catalogue without running any
// process. An agent whose CLI publishes no list resolves immediately.
func (c *Controller) cachedHostCLIModels(agentName string, spec hostcli.Spec) hostCLIModelEntry {
	c.hostCLIMu.Lock()
	entry, ok := c.hostCLIModels[agentName]
	c.hostCLIMu.Unlock()
	if ok {
		return entry
	}
	if !spec.HasModelSource() {
		return hostCLIModelEntry{status: hostCLIModelStatusSkipped}
	}
	return hostCLIModelEntry{status: hostCLIModelStatusPending}
}

// hostCLIPath returns the detected executable path for an agent type, falling
// back to the bare executable name resolved through PATH at launch.
func (c *Controller) hostCLIPath(ctx context.Context, agentName string, spec hostcli.Spec) string {
	if c.discovery == nil {
		return spec.Executable
	}
	results, err := c.detectAgents(ctx)
	if err != nil {
		return spec.Executable
	}
	for _, result := range results {
		if result.Name == agentName && strings.TrimSpace(result.MatchedPath) != "" {
			return result.MatchedPath
		}
	}
	return spec.Executable
}

func classifyHostCLIModelError(err error) (status, message string) {
	switch {
	case errors.Is(err, hostcli.ErrNotInstalled):
		return hostCLIModelStatusNotInstalled, "the command-line tool is not available to the Kandev process"
	case errors.Is(err, hostcli.ErrNotLoggedIn):
		return hostCLIModelStatusNotLoggedIn, "the command-line tool is not signed in"
	case errors.Is(err, hostcli.ErrTimeout):
		return hostCLIModelStatusTimeout, "the model list request timed out"
	default:
		return hostCLIModelStatusFailed, err.Error()
	}
}

// hostCLIModelProjection merges a CLI catalogue with the bridge-advertised
// list and describes where the result came from. Bridge entries keep their
// order after the CLI entries, and neither list is dropped.
func (c *Controller) hostCLIModelProjection(
	ctx context.Context,
	agentName string,
	bridge []dto.ModelEntryDTO,
	refresh bool,
) ([]dto.ModelEntryDTO, *dto.ModelDiscoveryDTO) {
	spec, ok := c.hostCLISpec(agentName)
	if !ok {
		return bridge, nil
	}
	entry := c.cachedHostCLIModels(agentName, spec)
	if refresh {
		entry = c.refreshHostCLIModels(ctx, agentName)
	}
	merged := mergeHostCLIModels(entry.models, bridge)
	return merged, c.hostCLIDiscoveryDTO(agentName, spec, entry)
}

func (c *Controller) hostCLIDiscoveryDTO(
	agentName string,
	spec hostcli.Spec,
	entry hostCLIModelEntry,
) *dto.ModelDiscoveryDTO {
	item := &dto.ModelDiscoveryDTO{
		Source:            modelDiscoverySourceProbe,
		Executable:        spec.Executable,
		Status:            entry.status,
		Error:             entry.errMsg,
		AllowsCustomModel: true,
		CLIVersion:        c.hostCLIVersion(agentName),
	}
	if entry.status == hostCLIModelStatusOK && len(entry.models) > 0 {
		item.Source = modelDiscoverySourceCLI
	}
	if !entry.checkedAt.IsZero() {
		checkedAt := entry.checkedAt
		item.CheckedAt = &checkedAt
	}
	return item
}

// hostCLIVersion reads the version discovery already detected. It never
// spawns a process of its own.
func (c *Controller) hostCLIVersion(agentName string) string {
	if c.discovery == nil {
		return ""
	}
	results, err := c.detectAgents(context.Background())
	if err != nil {
		return ""
	}
	for _, result := range results {
		if result.Name == agentName {
			return result.CLIVersion
		}
	}
	return ""
}

// mergeHostCLIModels puts CLI-discovered models first and appends every
// bridge model the CLI did not already name. Entries are labelled with their
// source so the selector can explain the list.
func mergeHostCLIModels(cliModels []hostcli.Model, bridge []dto.ModelEntryDTO) []dto.ModelEntryDTO {
	merged := make([]dto.ModelEntryDTO, 0, len(cliModels)+len(bridge))
	seen := make(map[string]struct{}, len(cliModels))
	for _, model := range cliModels {
		if _, exists := seen[model.ID]; exists {
			continue
		}
		seen[model.ID] = struct{}{}
		merged = append(merged, dto.ModelEntryDTO{
			ID:          model.ID,
			Name:        model.Name,
			Description: model.Description,
			IsDefault:   model.IsDefault,
			Source:      modelEntrySourceCLI,
			Meta:        model.Meta,
		})
	}
	for _, model := range bridge {
		if _, exists := seen[model.ID]; exists {
			continue
		}
		seen[model.ID] = struct{}{}
		model.Source = modelEntrySourceACP
		merged = append(merged, model)
	}
	return merged
}
