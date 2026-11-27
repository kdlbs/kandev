package backendapp

import (
	"context"
	"errors"
	"go/ast"
	"os"
	"path/filepath"
	"runtime"
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

type openCodeBootstrapSettings struct {
	values map[string][]byte
}

func (s *openCodeBootstrapSettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, found := s.values[key]
	return append([]byte(nil), value...), found, nil
}

func (s *openCodeBootstrapSettings) Save(_ context.Context, key string, value []byte) error {
	if s.values == nil {
		s.values = make(map[string][]byte)
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *openCodeBootstrapSettings) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
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
	if len(got) != 1 {
		t.Fatalf("managed generations = %#v, want only generic managed agents", got)
	}
	if got[0].AgentID != "claude-acp" {
		t.Fatalf("managed generation order = %#v, want only claude-acp", got)
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
	if err := reg.Register(agents.NewClaudeACP()); err != nil {
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

	_, _, err = provideServices(context.Background(), cfg, log, repos, pool, bus.NewMemoryEventBus(log), agentRegistry, "test-managed-runtime-defaults")
	if err == nil || !strings.Contains(err.Error(), "OpenCode runtime selection") {
		t.Fatalf("provideServices error = %v, want selection failure before readiness", err)
	}
}

func TestProvideServicesReconcilesManagedRuntimeDefaultsBeforeDiscovery(t *testing.T) {
	if !callsFunction(t, "services.go", "provideServices", "initManagedRuntimeAndDiscovery") {
		t.Fatal("provideServices does not call initManagedRuntimeAndDiscovery")
	}

	// initManagedRuntimeAndDiscovery is what actually reconciles managed
	// runtime defaults and loads the discovery registry now; inspect its body
	// for the ordering guarantee.
	provideFn := findFuncDecl(t, "services.go", "initManagedRuntimeAndDiscovery")
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
	bootstrapIndex := find("bootstrapOpenCodeSelection")
	reconcileIndex := find("reconcileManagedRuntimeDefaults")
	discoveryIndex := find("LoadRegistry")
	if bootstrapIndex < 0 {
		t.Fatal("initManagedRuntimeAndDiscovery does not bootstrap OpenCode selection")
	}
	if reconcileIndex < 0 {
		t.Fatal("initManagedRuntimeAndDiscovery does not reconcile managed runtime defaults")
	}
	if discoveryIndex < 0 {
		t.Fatal("initManagedRuntimeAndDiscovery does not load the discovery registry")
	}
	if bootstrapIndex > reconcileIndex || reconcileIndex > discoveryIndex {
		t.Fatalf("startup order = bootstrap %d, reconcile %d, discovery %d", bootstrapIndex, reconcileIndex, discoveryIndex)
	}
}

func TestOpenCodeFilesystemEvidenceUsesOnlyConfigurationAndSessionDatabase(t *testing.T) {
	home := t.TempDir()
	if hasOpenCodeFilesystemEvidence(home, "") {
		t.Fatal("empty home has OpenCode evidence")
	}

	authPath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatalf("create auth directory: %v", err)
	}
	if err := os.WriteFile(authPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write auth fixture: %v", err)
	}
	if hasOpenCodeFilesystemEvidence(home, "") {
		t.Fatal("credentials alone count as prior OpenCode use")
	}

	databasePath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.WriteFile(databasePath, []byte(""), 0o600); err != nil {
		t.Fatalf("write database fixture: %v", err)
	}
	if !hasOpenCodeFilesystemEvidence(home, "") {
		t.Fatal("OpenCode session database did not count as prior use")
	}
}

func TestCollectOpenCodeBootstrapEvidenceTreatsNativeDetectionFailuresAsPriorUse(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "unsupported version", err: errors.New("native OpenCode major 3 is not supported")},
		{name: "detector failure", err: errors.New("native OpenCode executable failed")},
		{name: "detector timeout", err: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence, err := collectOpenCodeBootstrapEvidence(
				context.Background(), nil, nil, t.TempDir(), "",
				func(context.Context) (agents.OpenCodeNativeRuntime, bool, error) {
					return agents.OpenCodeNativeRuntime{}, true, tt.err
				},
			)
			if err != nil {
				t.Fatalf("collect evidence: %v", err)
			}
			if !evidence.PriorUse || evidence.NativeFamily != "" {
				t.Fatalf("evidence = %+v, want prior use without a guessed native family", evidence)
			}
		})
	}
}

func TestCollectOpenCodeBootstrapEvidencePropagatesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := collectOpenCodeBootstrapEvidence(
		ctx, nil, nil, t.TempDir(), "",
		func(ctx context.Context) (agents.OpenCodeNativeRuntime, bool, error) {
			return agents.OpenCodeNativeRuntime{}, false, ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("collect evidence error = %v, want caller cancellation", err)
	}
}

func TestOpenCodeBootstrapDoesNotInspectNativeCLIWhenSelectionExists(t *testing.T) {
	defaults, err := openCodeRuntimeDefaults()
	if err != nil {
		t.Fatalf("openCodeRuntimeDefaults: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("native executable fixture uses a POSIX shell")
	}
	for _, tc := range []struct {
		name   string
		script string
	}{
		{name: "detection error", script: "#!/bin/sh\nexit 1\n"},
		{name: "detection timeout", script: "#!/bin/sh\nexec /bin/sleep 10\n"},
		{name: "unsupported major", script: "#!/bin/sh\nprintf 'OpenCode 3.0.0\\n'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := &openCodeBootstrapSettings{}
			store := managedruntime.NewStore(settings)
			selection := managedruntime.OpenCodeSelection{
				SchemaVersion:         1,
				Family:                managedruntime.OpenCodeFamilyV2,
				Source:                managedruntime.OpenCodeSourceManaged,
				Package:               defaults.V2Package,
				SelectedVersion:       defaults.V2Version,
				AppliedDefaultVersion: defaults.V2Version,
				Revision:              1,
			}
			if err := store.SaveOpenCodeSelection(context.Background(), 0, selection); err != nil {
				t.Fatalf("save persisted selection: %v", err)
			}
			detectionMarker := filepath.Join(t.TempDir(), "native-detection-called")
			t.Setenv("OPENCODE_DETECTION_MARKER", detectionMarker)
			binDir := t.TempDir()
			script := "#!/bin/sh\n: > \"$OPENCODE_DETECTION_MARKER\"\n" + tc.script
			if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
				t.Fatalf("write native OpenCode fixture: %v", err)
			}
			t.Setenv("PATH", binDir)

			if err := bootstrapOpenCodeSelection(context.Background(), store, &Repositories{}, nil, nil); err != nil {
				t.Fatalf("bootstrapOpenCodeSelection: %v", err)
			}
			got, found, err := store.GetOpenCodeSelection(context.Background())
			if err != nil || !found || got != selection {
				t.Fatalf("persisted selection after bootstrap = %+v, found=%t err=%v", got, found, err)
			}
			if _, err := os.Stat(detectionMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("native CLI detector was called for persisted selection; marker stat error = %v", err)
			}
		})
	}
}
