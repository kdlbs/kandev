package storeconformance

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
)

func TestValidateAdaptersRejectsMissingCapability(t *testing.T) {
	catalog := []requiredstores.Descriptor{{
		ID: "store", OwnerPackage: "test/store", RequiredTables: []string{"values_table"},
		Capabilities: []requiredstores.Capability{requiredstores.CapabilityBoolean},
	}}
	adapter := Adapter{
		ID: "store",
		Engines: map[EngineName]EngineAdapter{
			EngineSQLite:   {Fresh: noop, Replay: noop},
			EnginePostgres: {Fresh: noop, Replay: noop},
		},
		Scenarios: Scenarios{CRUD: noop},
	}
	if err := ValidateAdapters(catalog, []Adapter{adapter}); err == nil {
		t.Fatal("ValidateAdapters() error = nil, want missing boolean callback")
	}
}

func TestValidateAdaptersRejectsMissingEngine(t *testing.T) {
	catalog := []requiredstores.Descriptor{{ID: "store", OwnerPackage: "test/store", RequiredTables: []string{"values_table"}}}
	adapter := Adapter{
		ID:        "store",
		Engines:   map[EngineName]EngineAdapter{EngineSQLite: {Fresh: noop, Replay: noop}},
		Scenarios: Scenarios{CRUD: noop},
	}
	if err := ValidateAdapters(catalog, []Adapter{adapter}); err == nil {
		t.Fatal("ValidateAdapters() error = nil, want missing PostgreSQL engine")
	}
}

func TestRunUsesStableEngineStoreScenarioNames(t *testing.T) {
	catalog := []requiredstores.Descriptor{{ID: "store", OwnerPackage: "test/store", RequiredTables: []string{"values_table"}}}
	seen := make(chan string, 3)
	callback := func(name string) Scenario {
		return func(s ScenarioContext) error {
			seen <- fmt.Sprintf("%s/%s/%s", s.Engine, s.StoreID, name)
			return nil
		}
	}
	adapter := Adapter{
		ID: "store",
		Engines: map[EngineName]EngineAdapter{
			EngineSQLite:   {Fresh: callback("fresh"), Replay: callback("replay")},
			EnginePostgres: {Fresh: noop, Replay: noop},
		},
		Scenarios: Scenarios{CRUD: callback("crud")},
	}
	Run(t, catalog, []Adapter{adapter}, RunOptions{Engines: []EngineName{EngineSQLite}})
	close(seen)
	want := map[string]bool{"sqlite3/store/fresh": false, "sqlite3/store/replay": false, "sqlite3/store/crud": false}
	for got := range seen {
		if _, ok := want[got]; !ok {
			t.Errorf("unexpected callback %q", got)
			continue
		}
		want[got] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("callback %q was not run", name)
		}
	}
}

func TestCapabilityScenarioUsesDatabase(t *testing.T) {
	catalog := []requiredstores.Descriptor{{
		ID: "store", OwnerPackage: "test/store", RequiredTables: []string{"values_table"},
		Capabilities: []requiredstores.Capability{requiredstores.CapabilityTransaction},
	}}
	adapter := Adapter{
		ID: "store",
		Engines: map[EngineName]EngineAdapter{
			EngineSQLite: {Fresh: func(s ScenarioContext) error {
				_, err := s.DB.ExecContext(s.Context, "CREATE TABLE values_table (value TEXT NOT NULL)")
				return err
			}, Replay: noop},
			EnginePostgres: {Fresh: noop, Replay: noop},
		},
		Scenarios: Scenarios{
			CRUD: noop,
			Capabilities: []CapabilityScenario{{
				Capability: requiredstores.CapabilityTransaction,
				Run: func(s ScenarioContext) error {
					_, err := s.DB.ExecContext(s.Context, "INSERT INTO values_table(value) VALUES (?)", "committed")
					return err
				},
			}},
		},
	}
	Run(t, catalog, []Adapter{adapter}, RunOptions{Engines: []EngineName{EngineSQLite}})
}

func noop(ScenarioContext) error { return nil }
