package registry

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Provide creates and loads the agent registry.
//
// KANDEV_MOCK_AGENT controls mock-agent availability:
//   - "only"  → E2E mode: only register mock agent, skip all others
//   - "true"  → Dev mode: load all agents AND enable mock agent
//   - unset   → Production: load all agents, mock agent disabled
//
// KANDEV_MOCK_PROVIDERS is a comma-separated list of canonical routing
// provider IDs registered as additional MockAgent instances. This
// unblocks coverage of the office provider-routing feature: routing
// config validation requires real-looking provider IDs to be present
// in the registry, while every launch still routes through the mock
// binary. Entries not in RoutableProviderIDs are skipped with a
// warning. It is honoured only when explicitly set, in both dev and
// E2E modes, and is never honoured in production.
//
// For the routing UI without real CLIs installed, set
// `KANDEV_MOCK_PROVIDERS=claude-acp,codex-acp,opencode-acp` when
// launching `make dev`.
func Provide(log *logger.Logger) (*Registry, func() error, error) {
	reg := NewRegistry(log)

	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	mockProviders := os.Getenv("KANDEV_MOCK_PROVIDERS")
	if mockMode == "only" {
		// E2E mode: only register mock agent — skip agent discovery for all others
		_ = reg.Register(agents.NewMockAgent())
		configureMockAgent(reg, "mock-agent", log)
		registerExtraMockProviders(reg, log, mockProviders)
		validateMockProviders(reg, mockProviders, log)
	} else {
		reg.LoadDefaults()
		if mockMode == "true" {
			// Dev mode: enable the base mock agent alongside the real
			// agents. KANDEV_MOCK_PROVIDERS is opt-in — when set, the
			// listed canonicals are replaced with MockAgent aliases so
			// the routing UI is exercisable without the real CLIs.
			configureMockAgent(reg, "mock-agent", log)
			registerExtraMockProviders(reg, log, mockProviders)
			validateMockProviders(reg, mockProviders, log)
		}
		registerRealProvidersProber(reg, log)
	}

	return reg, func() error { return nil }, nil
}

// validateMockProviders logs an error if any provider listed in the raw
// comma-separated string is missing from the registry after
// registerExtraMockProviders. A missing provider means the routing
// catalogue will reject every non-empty provider_order, causing 400s
// in the office-routing-* E2E specs. Failing loudly here surfaces the
// root cause instead of leaving it to cascade into opaque HTTP errors.
func validateMockProviders(reg *Registry, raw string, log *logger.Logger) {
	if raw == "" {
		return
	}
	for _, entry := range strings.Split(raw, ",") {
		id := strings.TrimSpace(entry)
		if id == "" || !IsRoutableProviderID(id) {
			continue
		}
		if !reg.Exists(id) {
			log.Error("KANDEV_MOCK_PROVIDERS: provider missing from registry after registration — routing catalogue will be empty",
				zap.String("id", id))
		}
	}
}

// registerRealProvidersProber wires the shared ACP probe to each
// routable real provider. The probe spawns the agent's CLI via
// BuildCommand, performs a JSON-RPC initialize, and tears down — no
// session ever starts. Called once at boot in non-mock mode so the
// HTTP /routing/retry endpoint can flip a degraded provider back to
// healthy without waiting for the next real launch.
func registerRealProvidersProber(reg *Registry, log *logger.Logger) {
	resolver := func(providerID string) ([]string, map[string]string, bool) {
		ag, ok := reg.Get(providerID)
		if !ok || !ag.Enabled() {
			return nil, nil, false
		}
		cmd := ag.BuildCommand(agents.CommandOptions{})
		if cmd.IsEmpty() {
			return nil, nil, false
		}
		var env map[string]string
		if rt := ag.Runtime(); rt != nil && len(rt.Env) > 0 {
			env = make(map[string]string, len(rt.Env))
			for k, v := range rt.Env {
				env[k] = v
			}
		}
		return cmd.Args(), env, true
	}
	probe := routingerr.NewACPProbe(resolver, log)
	for _, id := range RoutableProviderIDs {
		// Skip ids that are not actually loaded (e.g. an agent type
		// that was removed from LoadDefaults but still appears in the
		// routable list). The probe is harmless to register against a
		// missing agent — but registering only loaded ones keeps the
		// boot log noise minimal.
		if !reg.Exists(id) {
			continue
		}
		routingerr.RegisterProber(id, probe)
	}
}

// registerExtraMockProviders parses the supplied comma-separated list
// of provider IDs and registers each accepted entry as a MockAgent
// alias under that canonical provider ID.
//
// In KANDEV_MOCK_AGENT=only (E2E) mode the canonicals are not loaded by
// LoadDefaults so each entry is a fresh registration. In
// KANDEV_MOCK_AGENT=true (dev) mode the real agents were just loaded —
// we Replace the entry with a MockAgent so the routing UI exercises
// the mock binary instead of requiring the real CLI to be installed
// on the developer's machine.
func registerExtraMockProviders(reg *Registry, log *logger.Logger, raw string) {
	if raw == "" {
		return
	}
	for _, entry := range strings.Split(raw, ",") {
		id := strings.TrimSpace(entry)
		if id == "" {
			continue
		}
		if !IsRoutableProviderID(id) {
			log.Warn("KANDEV_MOCK_PROVIDERS: skipping unknown provider id",
				zap.String("id", id),
				zap.Strings("allowed", RoutableProviderIDs))
			continue
		}
		displayName := mockDisplayNameFor(id)
		ag := agents.NewMockAgentWithID(id, displayName, displayName)
		if err := reg.Replace(ag); err != nil {
			log.Warn("KANDEV_MOCK_PROVIDERS: register failed",
				zap.String("id", id), zap.Error(err))
			continue
		}
		configureMockAgent(reg, id, log)
		// Register an E2E-only prober so /routing/retry can mark the
		// provider healthy without depending on a follow-up launch.
		// The prober consults the live injection map: if KANDEV_PROVIDER_
		// FAILURES still names this provider, the probe fails with that
		// code; otherwise it succeeds (provider flips healthy).
		routingerr.RegisterProber(id, &mockProvProber{providerID: id})
	}
}

// mockProvProber is the E2E-only ProviderProber wired in
// registerExtraMockProviders. It mirrors how a real prober for
// claude-acp/codex-acp would behave: a cheap availability check that
// returns the injected error when the test has armed one, and nil
// when the injection has been cleared.
type mockProvProber struct{ providerID string }

func (p *mockProvProber) Probe(_ context.Context, _ routingerr.ProbeInput) *routingerr.Error {
	if _, ok := routingerr.InjectedCode(p.providerID); ok {
		return routingerr.Classify(routingerr.Input{
			Phase:      routingerr.PhaseProcessStart,
			ProviderID: p.providerID,
		})
	}
	return nil
}

// mockDisplayNameFor returns a friendly display name for a routable
// provider id (e.g. "Mock Claude" for "claude-acp"). Falls back to the
// id itself when the prefix is unknown.
func mockDisplayNameFor(id string) string {
	switch id {
	case "claude-acp":
		return "Mock Claude"
	case "codex-acp":
		return "Mock Codex"
	case "opencode-acp":
		return "Mock OpenCode"
	case "copilot-acp":
		return "Mock Copilot"
	case "amp-acp":
		return "Mock Amp"
	default:
		return "Mock " + id
	}
}

// configureMockAgent enables and configures the mock agent binary path and capabilities.
// KANDEV_MOCK_AGENT_MCP=false disables MCP support (defaults to enabled).
func configureMockAgent(reg *Registry, id string, log *logger.Logger) {
	ag, ok := reg.Get(id)
	if !ok {
		return
	}
	mock, ok := ag.(*agents.MockAgent)
	if !ok {
		return
	}
	mock.SetEnabled(true)
	if os.Getenv("KANDEV_MOCK_AGENT_MCP") == "false" {
		mock.SetSupportsMCP(false)
	}
	// Resolve binary path: same directory as the running executable.
	// On Windows the binary is mock-agent.exe — exec.Command on Windows
	// only auto-appends .exe for PATH lookups, not absolute paths, so
	// the basename must include the extension here.
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	binaryName := "mock-agent"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(filepath.Dir(exePath), binaryName)
	mock.SetBinaryPath(binaryPath)
	log.Info("mock agent enabled",
		zap.String("id", id),
		zap.String("cmd", binaryPath),
		zap.Bool("supports_mcp", mock.SupportsMCPEnabled()))
}
