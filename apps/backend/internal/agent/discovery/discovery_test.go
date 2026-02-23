package discovery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
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
func (a *discoveryTestAgent) DefaultModel() string                   { return "" }
func (a *discoveryTestAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *discoveryTestAgent) Runtime() *agents.RuntimeConfig { return nil }
func (a *discoveryTestAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *discoveryTestAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	return agents.NewCommand()
}
func (a *discoveryTestAgent) ListModels(ctx context.Context) (*agents.ModelList, error) {
	return &agents.ModelList{}, nil
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

	reg := &Registry{
		agents:   []agents.Agent{a},
		logger:   newDiscoveryTestLogger(t),
		cacheTTL: time.Minute,
	}

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

	reg := &Registry{
		agents:   []agents.Agent{a},
		logger:   newDiscoveryTestLogger(t),
		cacheTTL: time.Minute,
	}

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

	reg := &Registry{
		agents:   []agents.Agent{a},
		logger:   newDiscoveryTestLogger(t),
		cacheTTL: 20 * time.Millisecond,
	}

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
