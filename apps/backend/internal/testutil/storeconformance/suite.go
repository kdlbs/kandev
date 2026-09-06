package storeconformance

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
)

// ScenarioContext is shared by all engine-specific and behavior callbacks.
type ScenarioContext struct {
	Context context.Context
	Engine  EngineName
	StoreID string
	DB      *sqlx.DB
}

// Scenario is one assertion against an initialized store.
type Scenario func(ScenarioContext) error

// CapabilityScenario binds exactly one catalog capability to one assertion.
type CapabilityScenario struct {
	Capability requiredstores.Capability
	Run        Scenario
}

// Scenarios contains the common behavior assertions. Fresh and Replay are
// engine callbacks because schema initialization can use dialect-specific SQL;
// CRUD and capability assertions are shared across engines.
type Scenarios struct {
	CRUD         Scenario
	Capabilities []CapabilityScenario
}

// EngineAdapter supplies fresh and replay schema initialization for one engine.
type EngineAdapter struct {
	Fresh  Scenario
	Replay Scenario
}

// Adapter binds one catalog entry to both database engines.
type Adapter struct {
	ID        string
	Engines   map[EngineName]EngineAdapter
	Scenarios Scenarios
}

// RunOptions controls which engines a test invokes. The default runs both
// engines. Completeness validation always runs before any engine is opened.
type RunOptions struct {
	Engines     []EngineName
	PostgresDSN string
}

// ValidateAdapters compares fixed adapter metadata with the authoritative
// catalog. It is deliberately independent of database setup so a skipped
// PostgreSQL test cannot hide an incomplete adapter set.
func ValidateAdapters(catalog []requiredstores.Descriptor, adapters []Adapter) error {
	if err := requiredstores.ValidateCatalog(catalog); err != nil {
		return fmt.Errorf("validate catalog: %w", err)
	}
	byID := make(map[string]Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter.ID == "" {
			return fmt.Errorf("adapter has empty ID")
		}
		if _, exists := byID[adapter.ID]; exists {
			return fmt.Errorf("duplicate store adapter %q", adapter.ID)
		}
		byID[adapter.ID] = adapter
	}
	for _, descriptor := range catalog {
		adapter, exists := byID[descriptor.ID]
		if !exists {
			return fmt.Errorf("missing store adapter %q", descriptor.ID)
		}
		if err := validateAdapter(descriptor, adapter); err != nil {
			return err
		}
		delete(byID, descriptor.ID)
	}
	if len(byID) > 0 {
		unknown := make([]string, 0, len(byID))
		for id := range byID {
			unknown = append(unknown, id)
		}
		sort.Strings(unknown)
		return fmt.Errorf("unknown store adapter %q", unknown[0])
	}
	return nil
}

func validateAdapter(descriptor requiredstores.Descriptor, adapter Adapter) error {
	if err := validateAdapterEngines(adapter); err != nil {
		return err
	}
	if adapter.Scenarios.CRUD == nil {
		return fmt.Errorf("store adapter %q has no CRUD scenario", adapter.ID)
	}
	return validateAdapterCapabilities(descriptor, adapter)
}

func validateAdapterEngines(adapter Adapter) error {
	for _, name := range []EngineName{EngineSQLite, EnginePostgres} {
		engine, ok := adapter.Engines[name]
		if !ok {
			return fmt.Errorf("store adapter %q has no %s engine", adapter.ID, name)
		}
		if engine.Fresh == nil || engine.Replay == nil {
			return fmt.Errorf("store adapter %q has incomplete %s schema callbacks", adapter.ID, name)
		}
	}
	for name := range adapter.Engines {
		if err := validateEngine(name); err != nil {
			return fmt.Errorf("store adapter %q: %w", adapter.ID, err)
		}
	}
	return nil
}

func validateAdapterCapabilities(descriptor requiredstores.Descriptor, adapter Adapter) error {
	want := make(map[requiredstores.Capability]struct{}, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		want[capability] = struct{}{}
	}
	got := make(map[requiredstores.Capability]struct{}, len(adapter.Scenarios.Capabilities))
	for _, capability := range adapter.Scenarios.Capabilities {
		if capability.Run == nil {
			return fmt.Errorf("store adapter %q has nil %s capability callback", adapter.ID, capability.Capability)
		}
		if _, exists := got[capability.Capability]; exists {
			return fmt.Errorf("store adapter %q has duplicate %s capability callback", adapter.ID, capability.Capability)
		}
		got[capability.Capability] = struct{}{}
	}
	return validateCapabilitySets(adapter.ID, want, got)
}

func validateCapabilitySets(id string, want, got map[requiredstores.Capability]struct{}) error {
	for capability := range want {
		if _, exists := got[capability]; !exists {
			return fmt.Errorf("store adapter %q is missing %s capability callback", id, capability)
		}
	}
	for capability := range got {
		if _, exists := want[capability]; !exists {
			return fmt.Errorf("store adapter %q declares unexpected %s capability callback", id, capability)
		}
	}
	return nil
}

// Run executes all common and capability scenarios with stable subtest names.
func Run(t testingT, catalog []requiredstores.Descriptor, adapters []Adapter, options RunOptions) {
	t.Helper()
	if err := ValidateAdapters(catalog, adapters); err != nil {
		t.Fatalf("store conformance metadata: %v", err)
	}
	byID := make(map[string]Adapter, len(adapters))
	for _, adapter := range adapters {
		byID[adapter.ID] = adapter
	}
	engines := options.Engines
	if len(engines) == 0 {
		engines = []EngineName{EngineSQLite, EnginePostgres}
	}
	for _, engineName := range engines {
		if err := validateEngine(engineName); err != nil {
			t.Fatalf("store conformance options: %v", err)
		}
		for _, descriptor := range catalog {
			adapter := byID[descriptor.ID]
			t.Run(string(engineName)+"/"+descriptor.ID, func(t *testing.T) {
				engine := openEngine(t, engineName, options.PostgresDSN)
				callbacks := adapter.Engines[engineName]
				runScenario(t, "fresh", ScenarioContext{Context: context.Background(), Engine: engineName, StoreID: adapter.ID, DB: engine.DB}, callbacks.Fresh)
				runScenario(t, "replay", ScenarioContext{Context: context.Background(), Engine: engineName, StoreID: adapter.ID, DB: engine.DB}, callbacks.Replay)
				runScenario(t, "crud", ScenarioContext{Context: context.Background(), Engine: engineName, StoreID: adapter.ID, DB: engine.DB}, adapter.Scenarios.CRUD)
				for _, capability := range adapter.Scenarios.Capabilities {
					capabilityName := capabilityName(capability.Capability)
					runScenario(t, capabilityName, ScenarioContext{Context: context.Background(), Engine: engineName, StoreID: adapter.ID, DB: engine.DB}, capability.Run)
				}
			})
		}
	}
}

// testingT keeps Run usable with testing.T while making its contract obvious.
type testingT interface {
	Helper()
	Fatalf(format string, args ...any)
	Run(name string, f func(*testing.T)) bool
}

func runScenario(t *testing.T, name string, context ScenarioContext, scenario Scenario) {
	t.Run(name, func(t *testing.T) {
		if err := scenario(context); err != nil {
			t.Fatalf("%s scenario failed: %v", name, err)
		}
	})
}

func capabilityName(capability requiredstores.Capability) string {
	switch capability {
	case requiredstores.CapabilityBoolean:
		return "boolean"
	case requiredstores.CapabilityTimestamp:
		return "timestamp"
	case requiredstores.CapabilityConflict:
		return "conflict"
	case requiredstores.CapabilityTransaction:
		return "transaction"
	default:
		return string(capability)
	}
}
