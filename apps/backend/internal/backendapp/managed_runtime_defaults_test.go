package backendapp

import (
	"context"
	"errors"
	"go/ast"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events/bus"
)

type managedRuntimeDefaultSettings struct {
	getErr error
}

func (s managedRuntimeDefaultSettings) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, s.getErr
}

func (s managedRuntimeDefaultSettings) Save(context.Context, string, []byte) error {
	return nil
}

func (s managedRuntimeDefaultSettings) Delete(context.Context, string) error {
	return nil
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.1 and AC-AGENTS-RUNTIME-UPDATES-002.2
func TestManagedRuntimeDefaultGenerationsUseRegisteredManagedAgents(t *testing.T) {
	log := newTestLogger()
	reg := registry.NewRegistry(log)
	for _, agent := range []agents.Agent{
		agents.NewOpenCodeACP(),
		agents.NewClaudeACP(),
		agents.NewDynamicAgent(),
		agents.NewMockAgent(),
	} {
		if err := reg.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.ID(), err)
		}
	}

	got := managedRuntimeDefaultGenerations(reg)
	if len(got) != 2 {
		t.Fatalf("managed generations = %#v, want two built-in managed agents", got)
	}
	if got[0].AgentID != "claude-acp" || got[1].AgentID != "opencode-acp" {
		t.Fatalf("managed generation order = %#v, want claude-acp then opencode-acp", got)
	}
	for _, generation := range got {
		if generation.Package == "" || generation.Version == "" {
			t.Fatalf("incomplete managed generation = %#v", generation)
		}
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.6
func TestReconcileManagedRuntimeDefaultsReturnsErrorBeforeServicesAreReady(t *testing.T) {
	wantErr := errors.New("settings read failed")
	reg := registry.NewRegistry(newTestLogger())
	if err := reg.Register(agents.NewOpenCodeACP()); err != nil {
		t.Fatalf("register managed agent: %v", err)
	}
	store := managedruntime.NewStore(managedRuntimeDefaultSettings{getErr: wantErr})

	if err := reconcileManagedRuntimeDefaults(context.Background(), store, reg, newTestLogger()); !errors.Is(err, wantErr) {
		t.Fatalf("reconciliation error = %v, want %v", err, wantErr)
	}
}

// @covers AC-AGENTS-RUNTIME-UPDATES-002.6
func TestProvideServicesStopsWhenManagedRuntimeReconciliationFails(t *testing.T) {
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	log := newTestLogger()
	pool, repos, cleanups, err := provideRepositories(context.Background(), cfg, log, "test-managed-runtime-defaults")
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})
	agentRegistry, registryCleanup, err := registry.Provide(log)
	if err != nil {
		t.Fatalf("registry.Provide: %v", err)
	}
	t.Cleanup(func() {
		if registryCleanup != nil {
			_ = registryCleanup()
		}
	})
	if err := pool.Reader().Close(); err != nil {
		t.Fatalf("close settings reader: %v", err)
	}

	_, _, err = provideServices(cfg, log, repos, pool, bus.NewMemoryEventBus(log), agentRegistry, "test-managed-runtime-defaults")
	if err == nil || !strings.Contains(err.Error(), "reconcile managed runtime defaults") {
		t.Fatalf("provideServices error = %v, want reconciliation failure before readiness", err)
	}
}

func TestProvideServicesReconcilesManagedRuntimeDefaultsBeforeDiscovery(t *testing.T) {
	provideFn := findFuncDecl(t, "services.go", "provideServices")
	callOrder := []string{}
	ast.Inspect(provideFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			callOrder = append(callOrder, fn.Name)
		case *ast.SelectorExpr:
			callOrder = append(callOrder, fn.Sel.Name)
		}
		return true
	})

	find := func(name string) int {
		for i, call := range callOrder {
			if call == name {
				return i
			}
		}
		return -1
	}
	reconcileIndex := find("reconcileManagedRuntimeDefaults")
	discoveryIndex := find("LoadRegistry")
	if reconcileIndex < 0 {
		t.Fatal("provideServices does not reconcile managed runtime defaults")
	}
	if discoveryIndex < 0 {
		t.Fatal("provideServices does not load the discovery registry")
	}
	if reconcileIndex > discoveryIndex {
		t.Fatalf("managed runtime reconciliation call index %d occurs after discovery index %d", reconcileIndex, discoveryIndex)
	}
}
