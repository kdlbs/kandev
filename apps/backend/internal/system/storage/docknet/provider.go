package docknet

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/system/storage"
)

// ProviderName is the cleanup-provider identity used by the runner, run
// results, and manual per-resource selection.
const ProviderName = "docker_networks"

// DockerClient is the provider's typed view of the agent docker client.
type DockerClient interface {
	Ping(context.Context) error
	ListNetworks(context.Context, agentdocker.NetworkListOptions) ([]agentdocker.NetworkInfo, error)
	InspectNetwork(context.Context, string) (agentdocker.NetworkInfo, error)
	RemoveNetwork(context.Context, string) error
	CreateNetwork(context.Context, agentdocker.CreateNetworkOptions) (agentdocker.NetworkInfo, error)
}

// SettingsReader reads the storage-maintenance settings snapshot.
type SettingsReader interface {
	GetSettings(context.Context) (storage.StorageMaintenanceSettings, error)
}

// ProviderConfig wires the provider. NewID and Now are injectable for tests.
type ProviderConfig struct {
	Docker   DockerClient
	Oracle   TaskOracle
	Ledger   *Ledger
	Settings SettingsReader
	NewID    func() string
	Now      func() time.Time
}

// Analysis is the read-only census result (dry-run shape).
type Analysis struct {
	Available  bool           `json:"available"`
	Classified map[string]int `json:"classified"`
	Candidates []Candidate    `json:"candidates"`
	Kept       []KeptNetwork  `json:"kept"`
	Warnings   []string       `json:"warnings,omitempty"`
}

// Candidate is a network eligible for quarantined removal, with full
// classification evidence (AC10).
type Candidate struct {
	NetworkID   string         `json:"network_id"`
	NetworkName string         `json:"network_name"`
	Class       Classification `json:"class"`
	FirstSeenAt string         `json:"first_seen_at"`
	Reason      string         `json:"reason"`
}

// KeptNetwork is a non-excluded network the provider will not touch, with why.
type KeptNetwork struct {
	NetworkID string         `json:"network_id"`
	Name      string         `json:"network_name"`
	Class     Classification `json:"class"`
	Reason    string         `json:"reason"`
}

// CleanupResult is what one reclaim cycle reports inside the storage run.
type CleanupResult struct {
	Available  bool           `json:"available"`
	DryRun     bool           `json:"dry_run"`
	Classified map[string]int `json:"classified"`
	// WouldRemove counts candidates a dry run records but does not act on.
	WouldRemove int          `json:"would_remove"`
	Marked      int          `json:"marked"`
	Removed     int          `json:"removed"`
	Skipped     int          `json:"skipped"`
	Probe       *ProbeResult `json:"probe,omitempty"`
	Warnings    []string     `json:"warnings,omitempty"`
	Aborted     string       `json:"aborted,omitempty"`
}

// Provider implements the storage CleanupProvider. Cleanup is lease-gated by
// the runner: the ctx it receives is the maintenance-lease context, and losing
// the lease cancels that context, which the reclaim loop checks between every
// network (AC6).
type Provider struct {
	docker   DockerClient
	oracle   TaskOracle
	ledger   *Ledger
	settings SettingsReader
	newID    func() string
	now      func() time.Time
}

func NewProvider(config ProviderConfig) *Provider {
	newID := config.NewID
	if newID == nil {
		newID = func() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Provider{
		docker: config.Docker, oracle: config.Oracle, ledger: config.Ledger,
		settings: config.Settings, newID: newID, now: now,
	}
}

// Name identifies the provider to the runner.
func (p *Provider) Name() string { return ProviderName }

// thresholds carries the settings-derived classification windows.
type thresholds struct {
	graceWindow time.Duration
	quarantine  time.Duration
	staleAge    time.Duration
}

func thresholdsFrom(settings storage.DockerNetworkSettings) thresholds {
	return thresholds{
		graceWindow: time.Duration(settings.OrphanGraceHours) * time.Hour,
		quarantine:  time.Duration(settings.QuarantineHours) * time.Hour,
		staleAge:    time.Duration(settings.StaleHours) * time.Hour,
	}
}

// Analyze runs the read-only census: classify every network, record
// candidates with evidence, remove nothing (AC10).
func (p *Provider) Analyze(ctx context.Context) Analysis {
	settings, warn := p.loadSettings(ctx)
	if warn != "" {
		return Analysis{Warnings: []string{warn}}
	}
	if warn := p.availabilityWarning(ctx); warn != "" {
		return Analysis{Warnings: []string{warn}}
	}
	classified, warnings := p.census(ctx, thresholdsFrom(settings.DockerNetworks))
	if len(warnings) > 0 {
		return Analysis{Warnings: warnings}
	}
	counts, candidates, kept := summarize(classified)
	return Analysis{
		Available: true, Classified: counts, Candidates: candidates, Kept: kept,
	}
}

// Cleanup runs one reclaim cycle. Removal requires settings enabled (the
// destructive half ships disabled: the default first run is a dry run), a
// quarantine mark whose window has elapsed, a fresh list+inspect
// revalidation in the same cycle before the SDK call (AC7), per-ID removal
// only (AC9), the two-phase ledger (AC8), and the probe after the
// reclamation phase (AC11).
func (p *Provider) Cleanup(ctx context.Context) (map[string]any, error) {
	settings, warn := p.loadSettings(ctx)
	if warn != "" {
		return unavailableResult(warn), nil
	}
	if warn := p.availabilityWarning(ctx); warn != "" {
		return unavailableResult(warn), nil
	}
	thresholds := thresholdsFrom(settings.DockerNetworks)
	recordRunTimestamp(p.now())

	classified, warnings := p.census(ctx, thresholds)
	result := CleanupResult{
		Available: true, DryRun: !settings.DockerNetworks.Enabled,
		Classified: classifiedCounts(classified), Warnings: warnings,
	}
	if !settings.DockerNetworks.Enabled {
		result.WouldRemove = len(eligibleItems(classified))
		return resultMap(result), nil
	}

	p.reclaim(ctx, classified, thresholds, &result)
	if settings.DockerNetworks.ProbeEnabled && result.Aborted == "" {
		probe := p.runProbeAfterReclaim(ctx)
		result.Probe = &probe
	}
	return resultMap(result), nil
}

// reclaim walks the eligible candidates through the two-phase ledger. Between
// networks it honors ctx: a lost maintenance lease cancels ctx and the loop
// stops before the next removal; an in-flight single removal finishes first
// (AC6).
func (p *Provider) reclaim(
	ctx context.Context,
	classified []ClassifiedNetwork,
	t thresholds,
	result *CleanupResult,
) {
	for _, item := range eligibleItems(classified) {
		if ctx.Err() != nil {
			result.Aborted = "maintenance lease lost; stopped before next removal"
			return
		}
		due, marked, proceed := p.advanceQuarantine(ctx, item, t)
		if !proceed {
			continue
		}
		if marked {
			result.Marked++
		}
		if !due {
			continue // mark persisted; the quarantine window runs to a later cycle
		}
		revalidated, err := p.revalidate(ctx, item.Network.ID)
		if err != nil {
			result.Skipped++
			p.recordSkip(ctx, item.Network.ID, err.Error())
			continue
		}
		if !revalidated {
			result.Skipped++
			p.recordSkip(ctx, item.Network.ID, "concurrent attachment detected on revalidation")
			continue
		}
		if err := p.docker.RemoveNetwork(ctx, item.Network.ID); err != nil {
			result.Skipped++
			p.recordSkip(ctx, item.Network.ID, err.Error())
			continue
		}
		if _, err := p.ledger.RecordRemoved(ctx, item.Network.ID, item.Network.Name, p.runID()); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"record removal of network %s in ledger: %v", item.Network.ID, err,
			))
		}
		result.Removed++
		recordRemoval("removed")
	}
}

// advanceQuarantine moves one eligible candidate through the ledger phases
// and returns (removalDue, markRecorded, proceed). proceed=false is the
// fail-closed shed: the live ledger row or a failed mark keeps the network
// for the next cycle. A mark whose delete-after deadline is still in the
// future stays marked until a later cycle proves it due, so a rollback of
// an in-progress cycle never un-deletes (AC8).
func (p *Provider) advanceQuarantine(
	ctx context.Context,
	item ClassifiedNetwork,
	t thresholds,
) (removalDue, markRecorded, proceed bool) {
	entry, err := p.ledger.ObserveSighting(ctx, item.Network.ID, item.Network.Name)
	if err != nil {
		_, _ = p.ledger.RecordSkipped(ctx, item.Network.ID, fmt.Sprintf("observe ledger: %v", err))
		return false, false, false
	}
	reclassified := Classify(
		ctx, item.Network, p.oracle, entry.FirstSeenAt, t.graceWindow, t.staleAge,
		ClassifyOptions{Now: p.now},
	)
	if !Eligible(reclassified.Class) {
		return false, false, false
	}
	marked, markRecorded, err := p.ledger.Mark(ctx, item.Network.ID, item.Network.Name, t.quarantine, evidenceMetadata(reclassified))
	if err != nil {
		_, _ = p.ledger.RecordSkipped(ctx, item.Network.ID, fmt.Sprintf("mark ledger: %v", err))
		return false, false, false
	}
	if marked.DeleteAfter == nil {
		return false, false, false
	}
	return !p.now().Before(*marked.DeleteAfter), markRecorded, true
}

// revalidation re-lists and re-inspects the network inside the same cycle
// right before removal (AC7): the network must still be listed and a fresh
// inspect must show zero connected containers, else abort this network.
func (p *Provider) revalidate(ctx context.Context, networkID string) (bool, error) {
	current, err := p.docker.ListNetworks(ctx, agentdocker.NetworkListOptions{Driver: "bridge"})
	if err != nil {
		return false, fmt.Errorf("re-list networks: %w", err)
	}
	found := false
	for _, net := range current {
		if net.ID == networkID {
			found = true
			break
		}
	}
	if !found {
		return false, fmt.Errorf("network %s no longer present", networkID)
	}
	detail, err := p.docker.InspectNetwork(ctx, networkID)
	if err != nil {
		return false, fmt.Errorf("re-inspect network %s: %w", networkID, err)
	}
	if len(detail.Containers) > 0 {
		return false, nil
	}
	return true, nil
}

func (p *Provider) recordSkip(ctx context.Context, networkID, reason string) {
	recordRemoval("skipped")
	_, _ = p.ledger.RecordSkipped(ctx, networkID, reason)
}

// census lists and inspects every bridge network on the daemon, feeds every
// sighting through the ledger (recording first-seen evidence), classifies,
// and collects warnings instead of failing the cycle: Docker degradation
// degrades only this provider (AC13).
func (p *Provider) census(ctx context.Context, t thresholds) ([]ClassifiedNetwork, []string) {
	networks, err := p.docker.ListNetworks(ctx, agentdocker.NetworkListOptions{Driver: "bridge"})
	if err != nil {
		return nil, []string{fmt.Sprintf("list Docker networks: %v", err)}
	}
	var warnings []string
	classified := make([]ClassifiedNetwork, 0, len(networks))
	for _, network := range networks {
		classified = append(classified, p.classifyOne(ctx, network, t, &warnings))
	}
	return classified, warnings
}

func (p *Provider) classifyOne(
	ctx context.Context,
	network agentdocker.NetworkInfo,
	t thresholds,
	warnings *[]string,
) ClassifiedNetwork {
	detail, err := p.docker.InspectNetwork(ctx, network.ID)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("inspect network %s: %v", network.ID, err))
		return ClassifiedNetwork{
			Network: network, Class: ClassStaleUncertain,
			Evidence: Evidence{Reason: "inspect failed; kept pending next cycle"},
		}
	}
	entry, err := p.ledger.ObserveSighting(ctx, detail.ID, detail.Name)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("observe network %s in ledger: %v", detail.ID, err))
		return ClassifiedNetwork{
			Network: detail, Class: ClassStaleUncertain,
			Evidence: Evidence{Reason: "ledger write failed; kept pending next cycle"},
		}
	}
	return Classify(ctx, detail, p.oracle, entry.FirstSeenAt, t.graceWindow, t.staleAge, ClassifyOptions{Now: p.now})
}

func (p *Provider) loadSettings(
	ctx context.Context,
) (storage.StorageMaintenanceSettings, string) {
	settings, err := p.settings.GetSettings(ctx)
	if err != nil {
		return storage.StorageMaintenanceSettings{}, fmt.Sprintf("read storage settings: %v", err)
	}
	normalized, err := storage.NormalizeSettings(settings)
	if err != nil {
		return storage.StorageMaintenanceSettings{}, fmt.Sprintf("validate storage settings: %v", err)
	}
	return normalized, ""
}

func (p *Provider) availabilityWarning(ctx context.Context) string {
	if p.docker == nil {
		return "Docker unavailable: client not configured"
	}
	if err := p.docker.Ping(ctx); err != nil {
		return fmt.Sprintf("Docker unavailable: %v", err)
	}
	return ""
}

// runProbeAfterReclaim runs the capacity probe once the reclamation phase is
// complete. The overlap assertion needs the fresh post-reclaim census of
// every container-attached network's subnets (AC12).
func (p *Provider) runProbeAfterReclaim(ctx context.Context) ProbeResult {
	networks, err := p.docker.ListNetworks(ctx, agentdocker.NetworkListOptions{Driver: "bridge"})
	if err != nil {
		probe := ProbeResult{Passed: false, Error: fmt.Sprintf("post-reclaim census: %v", err)}
		recordProbe(false)
		return probe
	}
	var active []agentdocker.NetworkInfo
	for _, network := range networks {
		detail, err := p.docker.InspectNetwork(ctx, network.ID)
		if err != nil {
			continue
		}
		if len(detail.Containers) > 0 {
			active = append(active, detail)
		}
	}
	probe := RunProbe(ctx, p.docker, p.runID(), active)
	recordProbe(probe.Passed)
	return probe
}

func (p *Provider) runID() string { return p.newID() }

func eligibleItems(classified []ClassifiedNetwork) []ClassifiedNetwork {
	items := make([]ClassifiedNetwork, 0)
	for _, item := range classified {
		if Eligible(item.Class) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Network.ID < items[j].Network.ID })
	return items
}

func summarize(classified []ClassifiedNetwork) (map[string]int, []Candidate, []KeptNetwork) {
	counts := make(map[string]int, 6)
	var candidates []Candidate
	var kept []KeptNetwork
	for _, item := range classified {
		counts[string(item.Class)]++
		switch {
		case Eligible(item.Class):
			candidates = append(candidates, Candidate{
				NetworkID: item.Network.ID, NetworkName: item.Network.Name,
				Class: item.Class, FirstSeenAt: item.Evidence.FirstSeenAt,
				Reason: item.Evidence.Reason,
			})
		case item.Class == ClassActive || item.Class == ClassAttached || item.Class == ClassStaleUncertain:
			kept = append(kept, KeptNetwork{
				NetworkID: item.Network.ID, Name: item.Network.Name,
				Class: item.Class, Reason: item.Evidence.Reason,
			})
		}
	}
	return counts, candidates, kept
}

func unavailableResult(warning string) map[string]any {
	return map[string]any{"available": false, "warnings": []string{warning}}
}

func resultMap(result CleanupResult) map[string]any {
	encoded, err := json.Marshal(result)
	if err != nil {
		return map[string]any{"available": false, "error": "encode docknet result"}
	}
	out := make(map[string]any)
	if err := json.Unmarshal(encoded, &out); err != nil {
		return map[string]any{"available": false, "error": "encode docknet result"}
	}
	return out
}

func evidenceMetadata(item ClassifiedNetwork) json.RawMessage {
	encoded, err := json.Marshal(item.Evidence)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}
