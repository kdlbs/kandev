package storeconformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

const previousStableTag = "v0.93.0"

type upgradeManifest struct {
	Tag                       string           `json:"tag"`
	SourceCommit              string           `json:"source_commit"`
	Fixtures                  []upgradeFixture `json:"fixtures"`
	KnownMissingRequiredStore []string         `json:"known_missing_required_stores"`
}

type upgradeFixture struct {
	Engine    testconformance.EngineName `json:"engine"`
	File      string                     `json:"file"`
	SHA256    string                     `json:"sha256"`
	Sentinels map[string]string          `json:"sentinels"`
}

func TestUpgradeFixtureManifest(t *testing.T) {
	manifest := loadUpgradeManifest(t)
	if manifest.Tag != previousStableTag || manifest.SourceCommit == "" {
		t.Fatalf("manifest = %#v, want tagged provenance", manifest)
	}
	if len(manifest.Fixtures) != 2 {
		t.Fatalf("manifest fixtures = %d, want SQLite and PostgreSQL", len(manifest.Fixtures))
	}
	seenEngines := make(map[testconformance.EngineName]bool)
	for _, fixture := range manifest.Fixtures {
		if seenEngines[fixture.Engine] {
			t.Fatalf("duplicate fixture engine %q", fixture.Engine)
		}
		seenEngines[fixture.Engine] = true
		if len(fixture.SHA256) != sha256.Size*2 {
			t.Fatalf("fixture %q has invalid checksum %q", fixture.File, fixture.SHA256)
		}
		path := filepath.Join("testdata", "upgrades", previousStableTag, fixture.File)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixture %s: %v", path, err)
		}
		for _, table := range []string{"kandev_meta", "tasks", "workspaces"} {
			schema := string(contents)
			if !strings.Contains(schema, "CREATE TABLE "+table) &&
				!strings.Contains(schema, `CREATE TABLE "`+table+`"`) {
				t.Errorf("fixture %s does not contain stable core table %s", fixture.File, table)
			}
		}
		checksum := sha256.Sum256(contents)
		if got := hex.EncodeToString(checksum[:]); got != fixture.SHA256 {
			t.Errorf("fixture %s checksum = %s, want %s", fixture.File, got, fixture.SHA256)
		}
	}
	if !seenEngines[testconformance.EngineSQLite] || !seenEngines[testconformance.EnginePostgres] {
		t.Fatalf("fixture engines = %#v, want both engines", seenEngines)
	}
	if len(manifest.KnownMissingRequiredStore) == 0 {
		t.Fatal("manifest has no partial-store provenance")
	}
}

func TestPreviousStableUpgrade(t *testing.T) {
	manifest := loadUpgradeManifest(t)
	if err := testconformance.ValidateAdapters(requiredstores.Catalog(), Adapters()); err != nil {
		t.Fatalf("validate adapters before fixture setup: %v", err)
	}
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		engine := engine
		t.Run(string(engine)+"/"+previousStableTag, func(t *testing.T) {
			fixture := fixtureForEngine(t, manifest, engine)
			database := testconformance.OpenEngine(t, engine, "")
			if err := applyFixture(t, database, fixture); err != nil {
				t.Fatalf("apply %s fixture: %v", engine, err)
			}
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("current initialization: %v", err)
			}
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("schema replay: %v", err)
			}
			if err := runCurrentScenarios(database); err != nil {
				t.Fatalf("conformance scenarios: %v", err)
			}
			if err := checkSentinels(database, fixture); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func loadUpgradeManifest(t *testing.T) upgradeManifest {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "upgrades", previousStableTag, "manifest.json"))
	if err != nil {
		t.Fatalf("read upgrade manifest: %v", err)
	}
	var manifest upgradeManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatalf("parse upgrade manifest: %v", err)
	}
	return manifest
}

func fixtureForEngine(t *testing.T, manifest upgradeManifest, engine testconformance.EngineName) upgradeFixture {
	t.Helper()
	for _, fixture := range manifest.Fixtures {
		if fixture.Engine == engine {
			return fixture
		}
	}
	t.Fatalf("manifest has no %s fixture", engine)
	return upgradeFixture{}
}

func applyFixture(t *testing.T, engine testconformance.Engine, fixture upgradeFixture) error {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "upgrades", previousStableTag, fixture.File))
	if err != nil {
		return err
	}
	for _, statement := range strings.Split(string(contents), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := engine.DB.ExecContext(context.Background(), statement); err != nil {
			return err
		}
	}
	return nil
}

func runCurrentInitialization(engine testconformance.Engine) error {
	adapters := Adapters()
	for _, adapter := range adapters {
		callbacks := adapter.Engines[engine.Name]
		scenario := testconformance.ScenarioContext{
			Context: context.Background(), Engine: engine.Name, StoreID: adapter.ID, DB: engine.DB,
		}
		if err := callbacks.Fresh(scenario); err != nil {
			return fmt.Errorf("%s fresh: %w", adapter.ID, err)
		}
		if err := callbacks.Replay(scenario); err != nil {
			return fmt.Errorf("%s replay: %w", adapter.ID, err)
		}
	}
	return nil
}

func runCurrentScenarios(engine testconformance.Engine) error {
	for _, adapter := range Adapters() {
		scenario := testconformance.ScenarioContext{
			Context: context.Background(), Engine: engine.Name, StoreID: adapter.ID, DB: engine.DB,
		}
		if err := adapter.Scenarios.CRUD(scenario); err != nil {
			return fmt.Errorf("%s CRUD: %w", adapter.ID, err)
		}
		for _, capability := range adapter.Scenarios.Capabilities {
			if err := capability.Run(scenario); err != nil {
				return fmt.Errorf("%s %s: %w", adapter.ID, capability.Capability, err)
			}
		}
	}
	return nil
}

func checkSentinels(engine testconformance.Engine, fixture upgradeFixture) error {
	for key, want := range fixture.Sentinels {
		var got string
		if err := engine.DB.QueryRowxContext(context.Background(), engine.DB.Rebind(
			`SELECT value FROM kandev_meta WHERE key = ?`,
		), key).Scan(&got); err != nil {
			return fmt.Errorf("read sentinel %q: %w", key, err)
		}
		if got != want {
			return fmt.Errorf("sentinel %q = %q, want %q", key, got, want)
		}
	}
	return nil
}
