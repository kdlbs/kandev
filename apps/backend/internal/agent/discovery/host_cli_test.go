package discovery

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostcli"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

// hostCLITestAgent is a discovery agent that also declares a vendor CLI.
type hostCLITestAgent struct {
	discoveryTestAgent
	spec hostcli.Spec
}

func (a *hostCLITestAgent) HostCLI() hostcli.Spec { return a.spec }

type versionRunner struct {
	mu      sync.Mutex
	output  string
	err     error
	calls   int
	lastCmd []string
}

func (r *versionRunner) Output(_ context.Context, argv []string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCmd = argv
	if len(argv) > 0 && argv[0] == "npm" {
		return "/usr/lib/node_modules\n", nil
	}
	r.calls++
	return r.output, r.err
}

func (r *versionRunner) Start(context.Context, []string) (hostcli.Process, error) {
	return nil, hostcli.ErrNotInstalled
}

func (r *versionRunner) versionCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newHostCLIRegistry(t *testing.T, agentList []agents.Agent, runner hostcli.Runner) *Registry {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	reg := registry.NewRegistry(log)
	for _, ag := range agentList {
		if err := reg.Register(ag); err != nil {
			t.Fatalf("register %s: %v", ag.ID(), err)
		}
	}
	registryInstance, err := LoadRegistry(context.Background(), reg, log)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	registryInstance.SetHostCLIRunner(runner)
	return registryInstance
}

func cliTestAgent(id, path string) *hostCLITestAgent {
	return &hostCLITestAgent{
		discoveryTestAgent: discoveryTestAgent{
			id:        id,
			discovery: &agents.DiscoveryResult{Available: true, MatchedPath: path},
		},
		spec: hostcli.Spec{
			DisplayName: "Test CLI",
			Executable:  "testcli",
			VersionArgs: []string{"--version"},
		},
	}
}

func findAvailability(results []Availability, name string) (Availability, bool) {
	for _, result := range results {
		if result.Name == name {
			return result, true
		}
	}
	return Availability{}, false
}

// @covers AC-AGENTS-HOST-CLI-001.1
func TestDetectCarriesHostCLIVersion(t *testing.T) {
	runner := &versionRunner{output: "2.1.220 (Test CLI)\n"}
	plain := &discoveryTestAgent{id: "plain", discovery: &agents.DiscoveryResult{Available: true}}
	registryInstance := newHostCLIRegistry(t, []agents.Agent{cliTestAgent("with-cli", "/usr/bin/testcli"), plain}, runner)

	results, err := registryInstance.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	withCLI, ok := findAvailability(results, "with-cli")
	if !ok {
		t.Fatalf("results = %+v", results)
	}
	if withCLI.CLIVersion != "2.1.220" || withCLI.CLIVersionError != "" {
		t.Fatalf("with-cli = %+v", withCLI)
	}
	if strings.Join(runner.lastCmd, " ") == "" {
		t.Fatal("version command was never run")
	}

	other, ok := findAvailability(results, "plain")
	if !ok {
		t.Fatalf("results = %+v", results)
	}
	if other.CLIVersion != "" || other.CLIVersionError != "" {
		t.Fatalf("plain agent gained host CLI fields: %+v", other)
	}
}

// @covers AC-AGENTS-HOST-CLI-001.2
func TestDetectReportsHostCLIVersionFailure(t *testing.T) {
	runner := &versionRunner{err: hostcli.ErrNotInstalled}
	registryInstance := newHostCLIRegistry(t, []agents.Agent{cliTestAgent("with-cli", "/usr/bin/testcli")}, runner)

	results, err := registryInstance.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	withCLI, _ := findAvailability(results, "with-cli")
	if withCLI.CLIVersion != "" || withCLI.CLIVersionError == "" {
		t.Fatalf("with-cli = %+v", withCLI)
	}
	if !withCLI.Available {
		t.Fatal("a failed version check must not mark the agent unavailable")
	}
}

// @covers AC-AGENTS-HOST-CLI-001.3
func TestHostCLIVersionCacheAndInvalidation(t *testing.T) {
	runner := &versionRunner{output: "1.0.0"}
	registryInstance := newHostCLIRegistry(t, []agents.Agent{cliTestAgent("with-cli", "/usr/bin/testcli")}, runner)

	if _, err := registryInstance.Detect(context.Background()); err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if runner.versionCalls() != 1 {
		t.Fatalf("version calls = %d after the first sweep", runner.versionCalls())
	}

	// The detection cache expires; the version cache must still answer.
	registryInstance.mu.Lock()
	registryInstance.cachedResults = nil
	registryInstance.mu.Unlock()
	if _, err := registryInstance.Detect(context.Background()); err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if runner.versionCalls() != 1 {
		t.Fatalf("version calls = %d, want the cached version reused", runner.versionCalls())
	}

	runner.mu.Lock()
	runner.output = "1.1.0"
	runner.mu.Unlock()
	registryInstance.InvalidateCache()
	results, err := registryInstance.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if runner.versionCalls() != 2 {
		t.Fatalf("version calls = %d after invalidation", runner.versionCalls())
	}
	withCLI, _ := findAvailability(results, "with-cli")
	if withCLI.CLIVersion != "1.1.0" {
		t.Fatalf("with-cli = %+v, want the re-detected version", withCLI)
	}
}

func TestApplyHostCLISkipsUnavailableAgents(t *testing.T) {
	runner := &versionRunner{output: "1.0.0"}
	agent := cliTestAgent("with-cli", "/usr/bin/testcli")
	agent.discovery.Available = false
	registryInstance := newHostCLIRegistry(t, []agents.Agent{agent}, runner)

	results, err := registryInstance.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	withCLI, _ := findAvailability(results, "with-cli")
	if withCLI.CLIVersion != "" || runner.versionCalls() != 0 {
		t.Fatalf("unavailable agent was probed: %+v (calls=%d)", withCLI, runner.versionCalls())
	}
}
