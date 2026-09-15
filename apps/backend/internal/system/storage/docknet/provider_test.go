package docknet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/system/storage"
)

// fakeDocker is an in-memory daemon surface for provider tests.
type fakeDocker struct {
	mu            sync.Mutex
	pingErr       error
	networks      map[string]agentdocker.NetworkInfo
	inspectErr    map[string]error
	inspectMutate func(agentdocker.NetworkInfo) agentdocker.NetworkInfo
	// attachAfterInspects makes InspectNetwork add a container endpoint after
	// the Nth call, simulating a container attaching mid-cycle between
	// classification and revalidation.
	inspectCount int
	attachAfter  int
	listErr      error
	removed      []string
	createErr    error
	createdName  string
	createInfo   agentdocker.NetworkInfo
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		networks:   map[string]agentdocker.NetworkInfo{},
		inspectErr: map[string]error{},
		createInfo: agentdocker.NetworkInfo{
			ID: "probe-net", Name: "kd-probe-run", Driver: "bridge",
			IPv4Subnets: []netip.Prefix{netip.MustParsePrefix("172.31.255.0/24")},
		},
	}
}

func (f *fakeDocker) addNetwork(network agentdocker.NetworkInfo) {
	f.networks[network.ID] = network
}

func (f *fakeDocker) Ping(context.Context) error { return f.pingErr }

func (f *fakeDocker) ListNetworks(
	context.Context, agentdocker.NetworkListOptions,
) ([]agentdocker.NetworkInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]agentdocker.NetworkInfo, 0, len(f.networks))
	for _, network := range f.networks {
		out = append(out, summaryOf(network))
	}
	return out, nil
}

func (f *fakeDocker) InspectNetwork(_ context.Context, id string) (agentdocker.NetworkInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectCount++
	if err, ok := f.inspectErr[id]; ok {
		return agentdocker.NetworkInfo{}, err
	}
	network, ok := f.networks[id]
	if !ok {
		return agentdocker.NetworkInfo{}, errors.New("network not found")
	}
	if f.attachAfter > 0 && f.inspectCount > f.attachAfter {
		attached := network
		attached.Containers = map[string]string{"late-container": "web"}
		return attached, nil
	}
	if f.inspectMutate != nil {
		return f.inspectMutate(network), nil
	}
	return network, nil
}

func (f *fakeDocker) RemoveNetwork(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.networks[id]; !ok {
		return errors.New("network not found")
	}
	delete(f.networks, id)
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeDocker) CreateNetwork(
	_ context.Context, options agentdocker.CreateNetworkOptions,
) (agentdocker.NetworkInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return agentdocker.NetworkInfo{}, f.createErr
	}
	f.createdName = options.Name
	f.createInfo.ID = "probe-net"
	f.networks["probe-net"] = f.createInfo
	return f.createInfo, nil
}

// summaryOf simulates the SDK list behavior: no containers in summaries.
func summaryOf(network agentdocker.NetworkInfo) agentdocker.NetworkInfo {
	return agentdocker.NetworkInfo{
		ID: network.ID, Name: network.Name, Driver: network.Driver,
		Scope: network.Scope, ConfigOnly: network.ConfigOnly, Labels: network.Labels,
		IPv4Subnets: network.IPv4Subnets,
	}
}

// fakeSettings serves a fixed settings snapshot; a zero value means enabled
// defaults with hour-scale windows for tests.
type fakeSettings struct {
	settings storage.StorageMaintenanceSettings
	err      error
}

func enabledTestSettings() storage.StorageMaintenanceSettings {
	settings := storage.DefaultSettings()
	settings.Docker.DedicatedDaemonAcknowledged = true
	settings.DockerNetworks = storage.DockerNetworkSettings{
		Enabled: true, StaleHours: 24, QuarantineHours: 24,
		OrphanGraceHours: 1, ProbeEnabled: true,
	}
	return settings
}

func (s *fakeSettings) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	if s.err != nil {
		return storage.StorageMaintenanceSettings{}, s.err
	}
	if s.settings == (storage.StorageMaintenanceSettings{}) {
		return enabledTestSettings(), nil
	}
	return s.settings, nil
}

type staticOracle struct {
	lookups map[string]TaskLookup
	err     error
}

func (o *staticOracle) Ownership(_ context.Context, network agentdocker.NetworkInfo) (string, TaskLookup, error) {
	if o.err != nil {
		return "", 0, o.err
	}
	key := OwnershipKeyFromLabels(network.Labels)
	lookup, ok := o.lookups[key]
	if !ok {
		lookup = TaskLookupUnknown
	}
	return key, lookup, nil
}

// memoryLedger is an in-memory LedgerStore simulating the persisted table.
type memoryLedger struct {
	mu      sync.Mutex
	entries map[string]storage.NetworkLedgerEntry
}

func newMemoryLedger() *memoryLedger {
	return &memoryLedger{entries: map[string]storage.NetworkLedgerEntry{}}
}

func (l *memoryLedger) GetNetworkLedgerEntry(
	_ context.Context, networkID string,
) (storage.NetworkLedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[networkID]
	if !ok {
		return storage.NetworkLedgerEntry{}, storage.ErrNotFound
	}
	return entry, nil
}

func (l *memoryLedger) UpsertNetworkLedgerObserved(
	_ context.Context, networkID, networkName string, at time.Time,
) (storage.NetworkLedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[networkID]
	if !ok {
		entry = storage.NetworkLedgerEntry{
			NetworkID: networkID, NetworkName: networkName,
			FirstSeenAt: at, LastSeenAt: at,
			State: storage.NetworkLedgerStateObserved, Metadata: json.RawMessage(`{}`),
		}
	} else {
		entry.NetworkName = networkName
		entry.LastSeenAt = at
	}
	l.entries[networkID] = entry
	return entry, nil
}

func (l *memoryLedger) TransitionNetworkLedgerEntry(
	_ context.Context, networkID string, next storage.NetworkLedgerState,
	markedAt, deleteAfter time.Time, _ json.RawMessage, lastError string,
) (storage.NetworkLedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[networkID]
	if !ok {
		return storage.NetworkLedgerEntry{}, storage.ErrNotFound
	}
	entry.State = next
	if !markedAt.IsZero() {
		at := markedAt
		entry.MarkedAt = &at
		after := deleteAfter
		entry.DeleteAfter = &after
	}
	entry.LastError = lastError
	l.entries[networkID] = entry
	return entry, nil
}

func (l *memoryLedger) RecordNetworkLedgerRemoved(
	_ context.Context, networkID, networkName string, at time.Time, _ string,
) (storage.NetworkLedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[networkID]
	if !ok {
		entry = storage.NetworkLedgerEntry{NetworkID: networkID, FirstSeenAt: at}
	}
	removedAt := at
	entry.NetworkName = networkName
	entry.State = storage.NetworkLedgerStateRemoved
	entry.RemovedAt = &removedAt
	l.entries[networkID] = entry
	return entry, nil
}

func (l *memoryLedger) ListNetworkLedgerEntries(
	context.Context,
) ([]storage.NetworkLedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]storage.NetworkLedgerEntry, 0, len(l.entries))
	for _, entry := range l.entries {
		if entry.State != storage.NetworkLedgerStateRemoved {
			out = append(out, entry)
		}
	}
	return out, nil
}

type providerHarness struct {
	provider *Provider
	docker   *fakeDocker
	oracle   *staticOracle
	ledger   *memoryLedger
	now      time.Time
}

func newHarness(t *testing.T) *providerHarness {
	t.Helper()
	h := &providerHarness{}
	h.now = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	h.docker = newFakeDocker()
	h.oracle = &staticOracle{lookups: map[string]TaskLookup{}}
	h.ledger = newMemoryLedger()
	h.provider = h.buildProvider()
	return h
}

func (h *providerHarness) buildProvider() *Provider {
	return NewProvider(ProviderConfig{
		Docker: h.docker, Oracle: h.oracle,
		Ledger:   NewLedger(LedgerConfig{Store: h.ledger, Now: func() time.Time { return h.now }}),
		Settings: &fakeSettings{},
		NewID:    func() string { return "run-1" },
		Now:      func() time.Time { return h.now },
	})
}

// rebuildProvider re-creates the provider against the current clock, as a
// restart between cycles would.
func (h *providerHarness) rebuildProvider() {
	h.provider = h.buildProvider()
}

func netWithContainers(name string, labels map[string]string, containers map[string]string) agentdocker.NetworkInfo {
	return agentdocker.NetworkInfo{
		ID: "net-" + name, Name: name, Driver: "bridge", Scope: "local",
		Labels: labels, Containers: containers,
	}
}

func TestCleanupDryRunRecordsCandidatesWithoutRemoval(t *testing.T) {
	// AC10: disabled (default first-run) records what would be removed, removes nothing.
	h := newHarness(t)
	h.docker.addNetwork(netWithContainers("kd_orphan", map[string]string{"kandev.task_id": "gone"}, nil))
	h.oracle.lookups["gone"] = TaskLookupInactive
	// Age the network through prior cycles: first_seen 2h ago, grace 1h.
	h.ledger.entries["net-kd_orphan"] = storage.NetworkLedgerEntry{
		NetworkID: "net-kd_orphan", FirstSeenAt: h.now.Add(-2 * time.Hour),
		State: storage.NetworkLedgerStateObserved,
	}
	settings := storage.DefaultSettings() // enabled=false
	h.provider.settings.(*fakeSettings).settings = settings

	result, err := h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result["available"] != true || result["dry_run"] != true {
		t.Fatalf("result = %#v, want available dry run", result)
	}
	if result["would_remove"] != float64(1) {
		t.Fatalf("would_remove = %v, want 1", result["would_remove"])
	}
	if len(h.docker.removed) != 0 {
		t.Fatal("dry run must not remove any network")
	}
}

func TestCleanupRemovesOrphanedNetworkEndToEnd(t *testing.T) {
	// AC3 + AC8: enabled provider marks the orphaned network in cycle one and
	// removes it once the quarantine window elapses in cycle two.
	h := newHarness(t)
	h.docker.addNetwork(netWithContainers("kd_orphan", map[string]string{"kandev.task_id": "gone"}, nil))
	h.oracle.lookups["gone"] = TaskLookupInactive
	h.ledger.entries["net-kd_orphan"] = storage.NetworkLedgerEntry{
		NetworkID: "net-kd_orphan", FirstSeenAt: h.now.Add(-3 * time.Hour),
		State: storage.NetworkLedgerStateObserved, // grace(1h) elapsed
	}
	probeless := enabledTestSettings()
	probeless.DockerNetworks.ProbeEnabled = false
	h.provider.settings.(*fakeSettings).settings = probeless

	// Cycle one: the network is quarantine-marked; removal waits for the window.
	result, err := h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup mark cycle: %v", err)
	}
	if result["removed"] != float64(0) || result["marked"] != float64(1) {
		t.Fatalf("mark cycle result = %#v, want marked=1 removed=0", result)
	}
	if len(h.docker.removed) != 0 {
		t.Fatal("first cycle must not remove; quarantine window must elapse")
	}
	// Repeating a still-pending cycle preserves the existing mark but must not
	// report it as a newly-created quarantine action.
	result, err = h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup repeated mark cycle: %v", err)
	}
	if result["marked"] != float64(0) {
		t.Fatalf("repeated mark cycle = %#v, want marked=0", result)
	}
	entry, err := h.ledger.GetNetworkLedgerEntry(context.Background(), "net-kd_orphan")
	if err != nil {
		t.Fatalf("ledger entry: %v", err)
	}
	if entry.State != storage.NetworkLedgerStateMarked || entry.DeleteAfter == nil {
		t.Fatalf("ledger entry = %#v, want marked with a deadline", entry)
	}

	// Cycle two after the quarantine window elapsed: the mark is due and the
	// revalidated network is removed.
	h.now = entry.FirstSeenAt.Add(72 * time.Hour) // past grace, quarantine(24h), stale
	h.rebuildProvider()
	h.provider.settings.(*fakeSettings).settings = probeless

	analysis := h.provider.Analyze(context.Background())
	if !analysis.Available || analysis.Classified["orphaned"] != 1 {
		t.Fatalf("classified = %#v, want orphaned=1", analysis.Classified)
	}
	result, err = h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup remove cycle: %v", err)
	}
	if result["removed"] != float64(1) {
		t.Fatalf("remove cycle result = %#v, want removed=1", result)
	}
	if len(h.docker.removed) != 1 || h.docker.removed[0] != "net-kd_orphan" {
		t.Fatalf("removed = %v, want the orphaned network", h.docker.removed)
	}
	// The ledger keeps a terminal removed row: rollback never un-deletes.
	removed, err := h.ledger.GetNetworkLedgerEntry(context.Background(), "net-kd_orphan")
	if err != nil || removed.State != storage.NetworkLedgerStateRemoved {
		t.Fatalf("removed ledger entry = %#v (%v), want terminal removed row", removed, err)
	}
}

func TestCleanupNeverTouchesActiveOrAttached(t *testing.T) {
	h := newHarness(t)
	active := netWithContainers(
		"kd_live", map[string]string{"kandev.task_id": "alive"},
		map[string]string{"c1": "web"},
	)
	attached := netWithContainers("kd_attached", map[string]string{"kandev.task_id": "alive"}, nil)
	h.docker.addNetwork(active)
	h.docker.addNetwork(attached)
	h.oracle.lookups["alive"] = TaskLookupActive

	if _, err := h.provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	// The probe legitimately removes its own throwaway network; no census
	// network may be removed.
	for _, id := range h.docker.removed {
		if id != "probe-net" {
			t.Fatalf("unexpected removal of %s", id)
		}
	}
	if _, ok := h.docker.networks[active.ID]; !ok {
		t.Fatal("active network was removed")
	}
	if _, ok := h.docker.networks[attached.ID]; !ok {
		t.Fatal("attached network was removed")
	}
}

func TestCleanupStopsWhenLeaseLostMidCycle(t *testing.T) {
	// AC6: lease context cancelled between networks stops further deletions.
	h := newHarness(t)
	h.oracle.lookups["gone"] = TaskLookupInactive
	older := h.now.Add(-48 * time.Hour)
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("kd_orphan_%d", i)
		h.docker.addNetwork(netWithContainers(name, map[string]string{"kandev.task_id": "gone"}, nil))
		h.ledger.entries["net-"+name] = storage.NetworkLedgerEntry{
			NetworkID: "net-" + name, FirstSeenAt: older,
			State: storage.NetworkLedgerStateObserved,
		}
	}
	// Pre-mark all three with elapsed quarantine deadlines.
	deadline := h.now.Add(-2 * time.Hour)
	markAllElapsed(t, h, deadline)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before the first removal: the loop must abort before removing any.
	cancel()

	result, err := h.provider.Cleanup(ctx)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	aborted, _ := result["aborted"].(string)
	if aborted == "" {
		t.Fatal("cancelled lease context must record an abort reason")
	}
	if result["removed"] != float64(0) {
		t.Fatalf("removed = %v, want 0 under a cancelled lease", result["removed"])
	}
}

func TestCleanupRemovesOnlyAfterLeaseHoldAndRevalidation(t *testing.T) {
	// AC6 happy path: lease held, marks due, revalidation clean => all removed.
	h := newHarness(t)
	h.oracle.lookups["gone"] = TaskLookupInactive
	older := h.now.Add(-48 * time.Hour)
	names := []string{"kd_orphan_a", "kd_orphan_b"}
	for _, name := range names {
		h.docker.addNetwork(netWithContainers(name, map[string]string{"kandev.task_id": "gone"}, nil))
		h.ledger.entries["net-"+name] = storage.NetworkLedgerEntry{
			NetworkID: "net-" + name, FirstSeenAt: older,
			State: storage.NetworkLedgerStateObserved,
		}
	}
	markAllElapsed(t, h, h.now.Add(-2*time.Hour))

	result, err := h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result["removed"] != float64(2) || result["skipped"] != float64(0) {
		t.Fatalf("result = %#v, want removed=2", result)
	}
	for _, id := range h.docker.removed {
		if id != "probe-net" && id != "net-kd_orphan_a" && id != "net-kd_orphan_b" {
			t.Fatalf("unexpected removal of %s", id)
		}
	}
	if _, ok := h.docker.networks["net-kd_orphan_a"]; ok {
		t.Fatal("orphan a must be removed")
	}
	if _, ok := h.docker.networks["net-kd_orphan_b"]; ok {
		t.Fatal("orphan b must be removed")
	}
}

func TestCleanupRevalidationAbortSkipsNetwork(t *testing.T) {
	// AC7: concurrent attachment detected during revalidation aborts removal.
	h := newHarness(t)
	h.oracle.lookups["gone"] = TaskLookupInactive
	h.docker.addNetwork(netWithContainers("kd_race", map[string]string{"kandev.task_id": "gone"}, nil))
	h.ledger.entries["net-kd_race"] = storage.NetworkLedgerEntry{
		NetworkID: "net-kd_race", FirstSeenAt: h.now.Add(-48 * time.Hour),
		State: storage.NetworkLedgerStateObserved,
	}
	// Elapsed quarantine mark.
	deadline := h.now.Add(-2 * time.Hour)
	markAllElapsed(t, h, deadline)
	// A container attaches between classification and removal: the inspect
	// during revalidation (the second inspect of this network this cycle)
	// sees an attached endpoint.
	h.docker.attachAfter = 1

	result, err := h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result["skipped"] != float64(1) || result["removed"] != float64(0) {
		t.Fatalf("result = %#v, want skipped=1 removed=0", result)
	}
	for _, id := range h.docker.removed {
		if id != "probe-net" {
			t.Fatalf("unexpected removal of %s; the racing network must not be removed", id)
		}
	}
	if _, ok := h.docker.networks["net-kd_race"]; !ok {
		t.Fatal("the racing network must survive")
	}
}

func TestCleanupIgnoresFailureOfOthers(t *testing.T) {
	// Provider errors degrade this provider only: settings failure is a
	// warning, never a panic; a failed removal is skipped not fatal.
	h := newHarness(t)
	h.provider.settings.(*fakeSettings).err = errors.New("settings store down")
	result, err := h.provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup must not fail the run for provider-scoped errors: %v", err)
	}
	if result["available"] != false {
		t.Fatalf("result = %#v, want available=false warning", result)
	}
}

func TestRunProbeCreatesAssertsAndRemoves(t *testing.T) {
	// AC11 + AC12: probe creates, asserts subnet, removes; overlap marks failure.
	docker := newFakeDocker()
	docker.createInfo = agentdocker.NetworkInfo{
		ID: "probe-net", Name: "kd-probe-x", Driver: "bridge",
		IPv4Subnets: []netip.Prefix{netip.MustParsePrefix("172.31.0.0/16")},
	}

	result := RunProbe(context.Background(), docker, "x", nil)
	if !result.Passed || result.Subnet != "172.31.0.0/16" {
		t.Fatalf("probe = %#v, want pass with the allocated subnet", result)
	}
	if len(docker.removed) != 1 || docker.removed[0] != "probe-net" {
		t.Fatalf("probe cleanup = %v, want the probe network removed", docker.removed)
	}

	// Overlap: active network on the same subnet makes the probe fail.
	docker2 := newFakeDocker()
	docker2.createInfo = docker.createInfo
	active := agentdocker.NetworkInfo{
		ID: "n1", Name: "kd_live", Driver: "bridge",
		IPv4Subnets: []netip.Prefix{netip.MustParsePrefix("172.31.0.0/16")},
	}
	result = RunProbe(context.Background(), docker2, "x", []agentdocker.NetworkInfo{active})
	if result.Passed {
		t.Fatal("overlapping subnet must fail the probe")
	}
	if len(result.Overlaps) != 1 || result.Overlaps[0] != "kd_live" {
		t.Fatalf("overlaps = %v, want the active network name", result.Overlaps)
	}
	if len(docker2.removed) != 1 {
		t.Fatal("probe must remove its network even on overlap failure")
	}
}

func TestRunProbeNoSubnetAllocatedFails(t *testing.T) {
	// Pool exhaustion symptom: no subnet => probe fails with a clear error.
	docker := newFakeDocker()
	docker.createInfo = agentdocker.NetworkInfo{ID: "probe-net", Name: "kd-probe-x"}

	result := RunProbe(context.Background(), docker, "x", nil)
	if result.Passed || result.Error == "" {
		t.Fatalf("probe = %#v, want failure with the missing-subnet error", result)
	}
}

func markAllElapsed(t *testing.T, h *providerHarness, deadline time.Time) {
	t.Helper()
	for id, entry := range h.ledger.entries {
		if entry.State != storage.NetworkLedgerStateObserved {
			continue
		}
		marked := deadline.Add(-time.Hour)
		entry.State = storage.NetworkLedgerStateMarked
		entry.MarkedAt = &marked
		entry.DeleteAfter = &deadline
		h.ledger.entries[id] = entry
	}
}
