package storeconformance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

const previousStableTag = "v0.93.0"

type upgradeManifest struct {
	Tag                        string              `json:"tag"`
	SourceCommit               string              `json:"source_commit"`
	Fixtures                   []upgradeFixture    `json:"fixtures"`
	KnownMissingRequiredStore  []string            `json:"known_missing_required_stores"`
	KnownMissingRequiredTables map[string][]string `json:"known_missing_required_tables"`
}

type upgradeFixture struct {
	Engine    testconformance.EngineName `json:"engine"`
	File      string                     `json:"file"`
	SHA256    string                     `json:"sha256"`
	Sentinels map[string]string          `json:"sentinels"`
	OwnerRows []ownerRowSentinel         `json:"owner_rows"`
}

type ownerRowSentinel struct {
	Owner     string            `json:"owner"`
	Table     string            `json:"table"`
	KeyColumn string            `json:"key_column"`
	KeyValue  string            `json:"key_value"`
	Values    map[string]string `json:"values"`
}

func TestUpgradeFixtureManifest(t *testing.T) {
	manifest := loadUpgradeManifest(t)
	if manifest.Tag != previousStableTag || manifest.SourceCommit == "" {
		t.Fatalf("manifest = %#v, want tagged provenance", manifest)
	}
	if len(manifest.Fixtures) != 2 {
		t.Fatalf("manifest fixtures = %d, want SQLite and PostgreSQL", len(manifest.Fixtures))
	}
	if err := validateKnownMissingRequiredTableMetadata(manifest.KnownMissingRequiredStore, manifest.KnownMissingRequiredTables); err != nil {
		t.Fatalf("manifest partial-store metadata: %v", err)
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
		if len(fixture.OwnerRows) == 0 {
			t.Errorf("fixture %s has no owner row sentinels", fixture.File)
		}
		if err := validateOwnerRowCoverage(fixture, manifest.KnownMissingRequiredStore); err != nil {
			t.Errorf("fixture %s owner row coverage: %v", fixture.File, err)
		}
		for _, row := range fixture.OwnerRows {
			if !validUpgradeIdentifier(row.Owner) || !validUpgradeIdentifier(row.Table) ||
				!validUpgradeIdentifier(row.KeyColumn) || row.KeyValue == "" || len(row.Values) == 0 {
				t.Errorf("fixture %s has invalid owner row sentinel %#v", fixture.File, row)
			}
			for column := range row.Values {
				if !validUpgradeIdentifier(column) {
					t.Errorf("fixture %s owner row %s has invalid column %q", fixture.File, row.Owner, column)
				}
			}
		}
	}
	if !seenEngines[testconformance.EngineSQLite] || !seenEngines[testconformance.EnginePostgres] {
		t.Fatalf("fixture engines = %#v, want both engines", seenEngines)
	}
	catalogIDs := make(map[string]struct{}, len(requiredstores.Catalog()))
	for _, descriptor := range requiredstores.Catalog() {
		catalogIDs[descriptor.ID] = struct{}{}
	}
	seenMissing := make(map[string]struct{}, len(manifest.KnownMissingRequiredStore))
	for _, id := range manifest.KnownMissingRequiredStore {
		if _, ok := catalogIDs[id]; !ok {
			t.Errorf("manifest lists unknown missing required store %q", id)
		}
		if _, duplicate := seenMissing[id]; duplicate {
			t.Errorf("manifest lists duplicate missing required store %q", id)
		}
		seenMissing[id] = struct{}{}
	}
	if len(seenMissing) == 0 {
		t.Fatal("manifest has no partial-store provenance")
	}
}

func validateOwnerRowCoverage(fixture upgradeFixture, knownMissing []string) error {
	missing := make(map[string]struct{}, len(knownMissing))
	for _, id := range knownMissing {
		missing[id] = struct{}{}
	}
	present := make(map[string]bool)
	catalogIDs := make(map[string]struct{}, len(requiredstores.Catalog()))
	for _, descriptor := range requiredstores.Catalog() {
		catalogIDs[descriptor.ID] = struct{}{}
	}
	for _, row := range fixture.OwnerRows {
		if _, ok := catalogIDs[row.Owner]; !ok {
			return fmt.Errorf("owner row names unknown catalog store %q", row.Owner)
		}
		if _, ok := missing[row.Owner]; ok {
			// A stable fixture can contain rows for a store whose current
			// descriptor gained an additional required table. The inventory check
			// below still proves that the older fixture lacks that table.
			continue
		}
		present[row.Owner] = true
	}
	for _, descriptor := range requiredstores.Catalog() {
		_, expectedMissing := missing[descriptor.ID]
		if expectedMissing {
			continue
		}
		if present[descriptor.ID] == expectedMissing {
			return fmt.Errorf("present store %q has no owner row", descriptor.ID)
		}
	}
	return nil
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
			if err := validateKnownMissingRequiredStores(database, manifest.KnownMissingRequiredStore, manifest.KnownMissingRequiredTables); err != nil {
				t.Fatalf("fixture store inventory: %v", err)
			}
			if err := validateKnownMissingRequiredTables(database, manifest.KnownMissingRequiredStore, manifest.KnownMissingRequiredTables); err != nil {
				t.Fatalf("fixture table inventory: %v", err)
			}
			if err := checkSentinels(database, fixture); err != nil {
				t.Fatalf("fixture sentinels before initialization: %v", err)
			}
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("current initialization: %v", err)
			}
			if err := checkSentinels(database, fixture); err != nil {
				t.Fatalf("sentinels after initialization: %v", err)
			}
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("schema replay: %v", err)
			}
			if err := checkSentinels(database, fixture); err != nil {
				t.Fatalf("sentinels after schema replay: %v", err)
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

// TestPreviousStableUpgrade_BackfillsLegacyActiveDynamicRoutes is the
// two-dialect regression test for the PR #3362 review follow-up (thread
// r3931855722): the v0.93.0 fixture predates the durable "active" dynamic
// route status, so every route a legacy install successfully launched is
// durably "starting" against an otherwise-healthy IDLE Office session. This
// seeds that exact legacy shape on top of the unmodified golden fixture (no
// fixture bytes or checksums change) and proves the one-time backfill
// migration repairs it identically on SQLite and PostgreSQL, so the startup
// orphan sweep does not misclassify it as orphaned on first restart after
// upgrade.
func TestPreviousStableUpgrade_BackfillsLegacyActiveDynamicRoutes(t *testing.T) {
	manifest := loadUpgradeManifest(t)
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		engine := engine
		t.Run(string(engine)+"/"+previousStableTag, func(t *testing.T) {
			fixture := fixtureForEngine(t, manifest, engine)
			database := testconformance.OpenEngine(t, engine, "")
			if err := applyFixture(t, database, fixture); err != nil {
				t.Fatalf("apply %s fixture: %v", engine, err)
			}
			seedLegacyDynamicRouteFixtureRows(t, database)
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("current initialization: %v", err)
			}
			assertLegacyDynamicRouteBackfilled(t, database)
		})
	}
}

// seedLegacyDynamicRouteFixtureRows adds an IDLE session with a "starting"
// dynamic route on top of the unmodified v0.93.0 fixture, referencing the
// fixture's own task row so no foreign key is left dangling.
func seedLegacyDynamicRouteFixtureRows(t *testing.T, engine testconformance.Engine) {
	t.Helper()
	ctx := context.Background()
	now := "2026-01-01 00:00:00"
	if _, err := engine.DB.ExecContext(ctx, engine.DB.Rebind(`
		INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`), "kandev_version", previousStableTag); err != nil {
		t.Fatalf("seed legacy database version: %v", err)
	}
	if _, err := engine.DB.ExecContext(ctx, engine.DB.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, route_generation, route_state, started_at, updated_at)
		VALUES (?, ?, 'IDLE', 1, 'starting', ?, ?)
	`), "fixture-v0930-dynamic-session", "fixture-v0930-task", now, now); err != nil {
		t.Fatalf("seed legacy IDLE dynamic session: %v", err)
	}
	if _, err := engine.DB.ExecContext(ctx, engine.DB.Rebind(`
		INSERT INTO dynamic_route_states (
			session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, state, updated_at
		) VALUES (?, 'dynamic-logical', 'candidate-1', 1, 1, 'starting', ?)
	`), "fixture-v0930-dynamic-session", now); err != nil {
		t.Fatalf("seed legacy starting dynamic_route_states row: %v", err)
	}
}

func assertLegacyDynamicRouteBackfilled(t *testing.T, engine testconformance.Engine) {
	t.Helper()
	ctx := context.Background()
	var routeState string
	if err := engine.DB.QueryRowxContext(ctx, engine.DB.Rebind(
		`SELECT state FROM dynamic_route_states WHERE session_id = ?`,
	), "fixture-v0930-dynamic-session").Scan(&routeState); err != nil {
		t.Fatalf("read dynamic_route_states.state: %v", err)
	}
	if routeState != "active" {
		t.Fatalf("dynamic_route_states.state = %q, want active", routeState)
	}
	var sessionRouteState string
	if err := engine.DB.QueryRowxContext(ctx, engine.DB.Rebind(
		`SELECT route_state FROM task_sessions WHERE id = ?`,
	), "fixture-v0930-dynamic-session").Scan(&sessionRouteState); err != nil {
		t.Fatalf("read task_sessions.route_state: %v", err)
	}
	if sessionRouteState != "active" {
		t.Fatalf("task_sessions.route_state = %q, want active", sessionRouteState)
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
	for _, sentinel := range fixture.OwnerRows {
		columns := make([]string, 0, len(sentinel.Values))
		for column := range sentinel.Values {
			columns = append(columns, column)
		}
		sort.Strings(columns)
		selectColumns := make([]string, 0, len(columns))
		for _, column := range columns {
			selectColumns = append(selectColumns, "CAST("+column+" AS TEXT)")
		}
		query := fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s = ?",
			strings.Join(selectColumns, ", "), sentinel.Table, sentinel.KeyColumn,
		)
		values := make([]any, len(columns))
		destinations := make([]any, len(values))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := engine.DB.QueryRowxContext(context.Background(), engine.DB.Rebind(query), sentinel.KeyValue).Scan(destinations...); err != nil {
			return fmt.Errorf("read %s owner row %s: %w", sentinel.Owner, sentinel.Table, err)
		}
		for index, column := range columns {
			got := fmt.Sprint(values[index])
			if got != sentinel.Values[column] {
				return fmt.Errorf("owner row %s.%s %s = %q, want %q", sentinel.Table, sentinel.KeyColumn, column, got, sentinel.Values[column])
			}
		}
	}
	return nil
}

func validateKnownMissingRequiredStores(engine testconformance.Engine, expected []string, partial map[string][]string) error {
	actual := make([]string, 0)
	for _, descriptor := range requiredstores.Catalog() {
		if _, hasPartialExpectation := partial[descriptor.ID]; hasPartialExpectation {
			continue
		}
		present := true
		for _, table := range descriptor.RequiredTables {
			if exists, err := requiredTableExists(engine, table); err != nil {
				return fmt.Errorf("check %s.%s: %w", descriptor.ID, table, err)
			} else if !exists {
				present = false
				break
			}
		}
		if !present {
			actual = append(actual, descriptor.ID)
		}
	}
	got := append([]string(nil), actual...)
	want := append([]string(nil), expected...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("known missing required stores = %v, want %v", got, want)
	}
	return nil
}

func validateKnownMissingRequiredTableMetadata(complete []string, partial map[string][]string) error {
	completeSet := make(map[string]struct{}, len(complete))
	for _, id := range complete {
		if _, duplicate := completeSet[id]; duplicate {
			return fmt.Errorf("manifest lists duplicate missing required store %q", id)
		}
		completeSet[id] = struct{}{}
	}
	descriptors := make(map[string]requiredstores.Descriptor, len(requiredstores.Catalog()))
	for _, descriptor := range requiredstores.Catalog() {
		descriptors[descriptor.ID] = descriptor
	}
	for id, tables := range partial {
		descriptor, ok := descriptors[id]
		if !ok {
			return fmt.Errorf("manifest lists partial tables for unknown store %q", id)
		}
		if _, complete := completeSet[id]; complete {
			return fmt.Errorf("manifest lists partial tables for complete-missing store %q", id)
		}
		if len(tables) == 0 {
			return fmt.Errorf("manifest lists no partial tables for store %q", id)
		}
		allowed := make(map[string]struct{}, len(descriptor.RequiredTables))
		for _, table := range descriptor.RequiredTables {
			allowed[table] = struct{}{}
		}
		seen := make(map[string]struct{}, len(tables))
		for _, table := range tables {
			if _, ok := allowed[table]; !ok {
				return fmt.Errorf("manifest lists unknown required table %s.%s", id, table)
			}
			if _, duplicate := seen[table]; duplicate {
				return fmt.Errorf("manifest lists duplicate required table %s.%s", id, table)
			}
			seen[table] = struct{}{}
		}
	}
	return nil
}

func validateKnownMissingRequiredTables(engine testconformance.Engine, complete []string, expected map[string][]string) error {
	completeSet := make(map[string]struct{}, len(complete))
	for _, id := range complete {
		completeSet[id] = struct{}{}
	}
	actual := make(map[string][]string)
	for _, descriptor := range requiredstores.Catalog() {
		if _, complete := completeSet[descriptor.ID]; complete {
			continue
		}
		for _, table := range descriptor.RequiredTables {
			exists, err := requiredTableExists(engine, table)
			if err != nil {
				return fmt.Errorf("check %s.%s: %w", descriptor.ID, table, err)
			}
			if !exists {
				actual[descriptor.ID] = append(actual[descriptor.ID], table)
			}
		}
	}
	for id := range actual {
		sort.Strings(actual[id])
	}
	for id := range expected {
		sort.Strings(expected[id])
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("partial missing required tables = %v, want %v", actual, expected)
	}
	for id, tables := range actual {
		want, ok := expected[id]
		if !ok || strings.Join(tables, "\x00") != strings.Join(want, "\x00") {
			return fmt.Errorf("partial missing required tables = %v, want %v", actual, expected)
		}
	}
	return nil
}

func requiredTableExists(engine testconformance.Engine, table string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = ?
	)`
	if engine.Name == testconformance.EngineSQLite {
		query = `SELECT EXISTS (
		SELECT 1 FROM sqlite_master
		WHERE type = 'table' AND name = ?
	)`
	}
	err := engine.DB.QueryRowxContext(context.Background(), engine.DB.Rebind(query), table).Scan(&exists)
	return exists, err
}

func validUpgradeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if !validUpgradeIdentifierCharacter(character) {
			return false
		}
		if index == 0 && (character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func validUpgradeIdentifierCharacter(character rune) bool {
	return character == '_' || character == '-' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}
