package discovery

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
)

type discoveryTestAgent struct {
	id             string
	discovery      *agents.DiscoveryResult
	installedCalls int
	mu             sync.Mutex
}

func (a *discoveryTestAgent) ID() string                             { return a.id }
func (a *discoveryTestAgent) Name() string                           { return a.id }
func (a *discoveryTestAgent) DisplayName() string                    { return a.id }
func (a *discoveryTestAgent) Description() string                    { return "" }
func (a *discoveryTestAgent) Enabled() bool                          { return true }
func (a *discoveryTestAgent) DisplayOrder() int                      { return 0 }
func (a *discoveryTestAgent) Logo(variant agents.LogoVariant) []byte { return nil }
func (a *discoveryTestAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *discoveryTestAgent) Runtime() *agents.RuntimeConfig { return nil }
func (a *discoveryTestAgent) BillingType() usage.BillingType { return usage.BillingTypeAPIKey }
func (a *discoveryTestAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *discoveryTestAgent) InstallScript() string          { return "" }
func (a *discoveryTestAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	return agents.NewCommand()
}

func (a *discoveryTestAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.mu.Lock()
	a.installedCalls++
	res := *a.discovery
	a.mu.Unlock()
	return &res, nil
}

func (a *discoveryTestAgent) setMatchedPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.discovery.MatchedPath = path
}

func (a *discoveryTestAgent) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.installedCalls
}

// newSingleAgentRegistry builds a discovery registry over an agent registry
// holding exactly one agent, with an explicit cache TTL.
func newSingleAgentRegistry(t *testing.T, ag agents.Agent, ttl time.Duration) *Registry {
	t.Helper()
	log := newDiscoveryTestLogger(t)
	agentRegistry := registry.NewRegistry(log)
	if err := agentRegistry.Register(ag); err != nil {
		t.Fatalf("register %s: %v", ag.ID(), err)
	}
	discoveryRegistry := loadDiscoveryRegistry(t, agentRegistry, log)
	discoveryRegistry.cacheTTL = ttl
	return discoveryRegistry
}

func newDiscoveryTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func TestRegistryDetectCachesResults(t *testing.T) {
	a := &discoveryTestAgent{
		id: "agent-a",
		discovery: &agents.DiscoveryResult{
			Available:   true,
			MatchedPath: "/usr/bin/agent-a",
		},
	}

	reg := newSingleAgentRegistry(t, a, time.Minute)

	if _, err := reg.Detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if _, err := reg.Detect(context.Background()); err != nil {
		t.Fatalf("second detect: %v", err)
	}

	if got := a.calls(); got != 1 {
		t.Fatalf("IsInstalled calls = %d, want 1", got)
	}
}

func TestRegistryInvalidateCacheForcesRefresh(t *testing.T) {
	a := &discoveryTestAgent{
		id: "agent-a",
		discovery: &agents.DiscoveryResult{
			Available:   true,
			MatchedPath: "/usr/bin/agent-a-v1",
		},
	}

	reg := newSingleAgentRegistry(t, a, time.Minute)

	first, err := reg.Detect(context.Background())
	if err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if first[0].MatchedPath != "/usr/bin/agent-a-v1" {
		t.Fatalf("first matched path = %q", first[0].MatchedPath)
	}

	a.setMatchedPath("/usr/bin/agent-a-v2")
	reg.InvalidateCache()

	second, err := reg.Detect(context.Background())
	if err != nil {
		t.Fatalf("second detect: %v", err)
	}
	if second[0].MatchedPath != "/usr/bin/agent-a-v2" {
		t.Fatalf("second matched path = %q, want v2", second[0].MatchedPath)
	}
	if got := a.calls(); got != 2 {
		t.Fatalf("IsInstalled calls = %d, want 2", got)
	}
}

func TestRegistryDetectCacheTTLExpiry(t *testing.T) {
	a := &discoveryTestAgent{
		id:        "agent-a",
		discovery: &agents.DiscoveryResult{Available: true},
	}

	reg := newSingleAgentRegistry(t, a, 20*time.Millisecond)

	if _, err := reg.Detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := reg.Detect(context.Background()); err != nil {
		t.Fatalf("second detect: %v", err)
	}

	if got := a.calls(); got != 2 {
		t.Fatalf("IsInstalled calls = %d, want 2", got)
	}
}

type virtualTestAgent struct {
	*discoveryTestAgent
}

func (a *virtualTestAgent) IsVirtual() bool { return true }

func detectNames(t *testing.T, r *Registry) []string {
	t.Helper()
	results, err := r.Detect(context.Background())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.Name)
	}
	return names
}

func loadDiscoveryRegistry(t *testing.T, reg *registry.Registry, log *logger.Logger) *Registry {
	t.Helper()
	discoveryRegistry, err := LoadRegistry(context.Background(), reg, log)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	return discoveryRegistry
}

// A custom agent deleted through the settings API is unregistered from the
// agent registry, but the Installed Agents list is rendered from discovery. If
// discovery keeps its own copy of the agent list, the deleted agent stays
// listed (and a newly created one stays missing) until the backend restarts.
func TestRegistryDetectFollowsRegistryMembership(t *testing.T) {
	log := newDiscoveryTestLogger(t)
	reg := registry.NewRegistry(log)
	if err := reg.Register(&discoveryTestAgent{
		id:        "agent-a",
		discovery: &agents.DiscoveryResult{Available: true},
	}); err != nil {
		t.Fatalf("register agent-a: %v", err)
	}

	discoveryRegistry := loadDiscoveryRegistry(t, reg, log)
	if !slices.Contains(detectNames(t, discoveryRegistry), "agent-a") {
		t.Fatal("agent-a missing from the first detect")
	}

	if err := reg.Unregister("agent-a"); err != nil {
		t.Fatalf("unregister agent-a: %v", err)
	}
	discoveryRegistry.InvalidateCache()
	if slices.Contains(detectNames(t, discoveryRegistry), "agent-a") {
		t.Error("agent-a is still detected after being unregistered")
	}

	if err := reg.Register(&discoveryTestAgent{
		id:        "agent-b",
		discovery: &agents.DiscoveryResult{Available: true},
	}); err != nil {
		t.Fatalf("register agent-b: %v", err)
	}
	discoveryRegistry.InvalidateCache()
	if !slices.Contains(detectNames(t, discoveryRegistry), "agent-b") {
		t.Error("agent-b is not detected after being registered")
	}
}

// Changing a custom agent's MCP strategy rebuilds its registry entry through
// Replace. Discovery must report the replacement, otherwise the next sweep
// writes the superseded SupportsMCP value back over the agent row.
func TestRegistryDetectReflectsReplacedAgent(t *testing.T) {
	log := newDiscoveryTestLogger(t)
	reg := registry.NewRegistry(log)
	if err := reg.Register(&discoveryTestAgent{
		id:        "agent-a",
		discovery: &agents.DiscoveryResult{Available: true},
	}); err != nil {
		t.Fatalf("register agent-a: %v", err)
	}

	discoveryRegistry := loadDiscoveryRegistry(t, reg, log)
	detectNames(t, discoveryRegistry)

	if err := reg.Replace(&discoveryTestAgent{
		id:        "agent-a",
		discovery: &agents.DiscoveryResult{Available: true, SupportsMCP: true},
	}); err != nil {
		t.Fatalf("replace agent-a: %v", err)
	}
	discoveryRegistry.InvalidateCache()

	results, err := discoveryRegistry.Detect(context.Background())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, result := range results {
		if result.Name != "agent-a" {
			continue
		}
		if !result.SupportsMCP {
			t.Error("SupportsMCP = false after replacing agent-a with an MCP-capable entry")
		}
		return
	}
	t.Fatal("agent-a missing after replacement")
}

func TestRegistryDetectSkipsVirtualAgents(t *testing.T) {
	log := newDiscoveryTestLogger(t)
	reg := registry.NewRegistry(log)
	if err := reg.Register(&virtualTestAgent{
		discoveryTestAgent: &discoveryTestAgent{
			id:        "agent-virtual",
			discovery: &agents.DiscoveryResult{Available: true},
		},
	}); err != nil {
		t.Fatalf("register agent-virtual: %v", err)
	}

	discoveryRegistry := loadDiscoveryRegistry(t, reg, log)
	if slices.Contains(detectNames(t, discoveryRegistry), "agent-virtual") {
		t.Error("virtual agent reported as a discovery result")
	}
}

// blockingDiscoveryAgent holds IsInstalled open until released, so a test can
// invalidate the cache while a sweep is still in flight.
type blockingDiscoveryAgent struct {
	*discoveryTestAgent
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *blockingDiscoveryAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.once.Do(func() { close(a.entered) })
	<-a.release
	return a.discoveryTestAgent.IsInstalled(ctx)
}

// A sweep reads the registry when it starts and writes its results when it
// finishes, so an unregister landing in between would otherwise be undone: the
// finishing sweep caches the agent it saw, and the deleted card comes back for
// a whole TTL.
func TestRegistryDetectDropsResultsInvalidatedMidSweep(t *testing.T) {
	log := newDiscoveryTestLogger(t)
	reg := registry.NewRegistry(log)
	blocking := &blockingDiscoveryAgent{
		discoveryTestAgent: &discoveryTestAgent{
			id:        "agent-a",
			discovery: &agents.DiscoveryResult{Available: true},
		},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	if err := reg.Register(blocking); err != nil {
		t.Fatalf("register agent-a: %v", err)
	}
	discoveryRegistry := loadDiscoveryRegistry(t, reg, log)

	swept := make(chan []Availability, 1)
	go func() {
		results, err := discoveryRegistry.Detect(context.Background())
		if err != nil {
			t.Errorf("in-flight detect: %v", err)
		}
		swept <- results
	}()

	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sweep never reached IsInstalled")
	}

	if err := reg.Unregister("agent-a"); err != nil {
		t.Fatalf("unregister agent-a: %v", err)
	}
	discoveryRegistry.InvalidateCache()
	close(blocking.release)

	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight sweep never finished")
	}

	if slices.Contains(detectNames(t, discoveryRegistry), "agent-a") {
		t.Error("a sweep that started before the invalidation published its stale results")
	}
}
