package startup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/i18n"
)

// TestRegisteredIdentifiersMatchAC005_3 is completeness test 2: the registered
// identifier set equals AC-PLATFORM-STARTUP-PROGRESS-005.3's list exactly, in
// both directions.
func TestRegisteredIdentifiersMatchAC005_3(t *testing.T) {
	want := map[StepID]bool{
		StepDatabaseBackup:            true,
		StepStoresRepositories:        true,
		StepStoresServices:            true,
		StepPromptSeqBackfill:         true,
		StepMessageTimestampsBackfill: true,
		StepSubagentContextBackfill:   true,
		StepSessionsRecovery:          true,
	}

	got := map[StepID]bool{}
	for _, spec := range Registry() {
		got[spec.ID] = true
	}

	for id := range want {
		if !got[id] {
			t.Errorf("AC-005.3 identifier %q is not registered", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("registered identifier %q is absent from the AC-005.3 list", id)
		}
	}
}

// TestEveryLabelKeyResolvesInEveryLocale is completeness test 3: every
// LabelKey resolves in every supported backend locale and every supported web
// locale. The backend half asserts the key is defined in the `en` catalog;
// i18n's own TestCatalogsHaveMatchingKeys already asserts every other backend
// locale carries an identical key set, so that transitively covers pt-pt,
// zh-cn, zh-tw, zh-hk, and pseudo. The web half reads the `startup` namespace's
// `en` catalog directly (the LabelKey with its leading "startup." stripped,
// matching the web resolution convention `t(key, { ns: "startup" })`); the
// repo's own i18n:check gate transitively covers the other web locales the
// same way.
func TestEveryLabelKeyResolvesInEveryLocale(t *testing.T) {
	backendKeys := map[string]bool{}
	for _, key := range i18n.Keys() {
		backendKeys[key] = true
	}
	webCatalog := readWebStartupCatalog(t)

	for _, spec := range Registry() {
		if !backendKeys[spec.LabelKey] {
			t.Errorf("step %q label key %q is missing from the backend en catalog", spec.ID, spec.LabelKey)
		}
		webKey := strings.TrimPrefix(spec.LabelKey, "startup.")
		if _, ok := webCatalog[webKey]; !ok {
			t.Errorf("step %q label key %q (web key %q) is missing from apps/web/src/locales/en/startup.json", spec.ID, spec.LabelKey, webKey)
		}
	}
}

func readWebStartupCatalog(t *testing.T) map[string]string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate startup package")
	}
	// internal/startup -> internal -> backend -> apps -> repo root
	repoRoot := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..")
	path := filepath.Join(repoRoot, "apps", "web", "src", "locales", "en", "startup.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web startup catalog: %v", err)
	}
	catalog := map[string]string{}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse web startup catalog: %v", err)
	}
	return catalog
}

// TestRegisteredStepsDeclarePhaseMatchesCallSite is completeness test 4:
// each step's declared Phase equals the phase active at its production
// BeginStep call site (AC-PLATFORM-STARTUP-PROGRESS-005.7). The right-hand
// side of callSitePhase is derived from each call site's governing SetPhase
// transition, not copied from the registry, so a call site moving to a
// different phase without a matching registry update (or vice versa) fails
// here instead of only warning silently at runtime.
func TestRegisteredStepsDeclarePhaseMatchesCallSite(t *testing.T) {
	callSitePhase := map[StepID]Phase{
		// persistence/provider.go:101 sets backing_up_database immediately
		// before :108's BeginStep.
		StepDatabaseBackup: BackingUpDatabase,
		// backendapp/storage.go:62 sets applying_migrations immediately
		// before :63's BeginStep.
		StepStoresRepositories: ApplyingMigrations,
		// backendapp/storage.go:218 sets initializing_services immediately
		// before :219's BeginStep.
		StepStoresServices: InitializingServices,
		// task/repository/sqlite/base_migrations.go:211/:477 and
		// subagent_context_backfill.go:68, reached from the same
		// runMigrations call chain during repository construction.
		StepMessageTimestampsBackfill: ApplyingMigrations,
		StepPromptSeqBackfill:         ApplyingMigrations,
		StepSubagentContextBackfill:   ApplyingMigrations,
		// agent/runtime/lifecycle/manager_lifecycle.go:56, inside
		// Manager.Start, called from backendapp/main.go's
		// startAgentInfrastructure immediately after that function's own
		// SetPhase(RecoveringSessions) transition (not the later
		// bindListeners closure in startGatewayAndServe, which runs after
		// recovery has already completed).
		StepSessionsRecovery: RecoveringSessions,
	}

	for _, spec := range Registry() {
		want, ok := callSitePhase[spec.ID]
		if !ok {
			t.Errorf("step %q has no call-site phase recorded in this test's table", spec.ID)
			continue
		}
		if spec.Phase != want {
			t.Errorf("step %q declares Phase=%q but its call site runs under %q", spec.ID, spec.Phase, want)
		}
	}
	if len(callSitePhase) != len(Registry()) {
		t.Errorf("call-site phase table has %d entries, want %d (one per registered step)", len(callSitePhase), len(Registry()))
	}
}

// TestRetiredIdentifiersCannotBeReRegistered is completeness test 5: the three
// identifiers retired with their work by 2eee90d38 are recorded in the
// append-only retired list, are absent from the registry, and panic through
// init()'s own guard if re-registered (AC-PLATFORM-STARTUP-PROGRESS-005.6).
func TestRetiredIdentifiersCannotBeReRegistered(t *testing.T) {
	wantRetired := []StepID{
		"task.journal.turns.backfill",
		"task.journal.messages.backfill",
		"plugins.session_events.mirror",
	}

	if len(retiredStepIDs) != len(wantRetired) {
		t.Errorf("retiredStepIDs has %d entries, want %d: %v", len(retiredStepIDs), len(wantRetired), retiredStepIDs)
	}

	registered := map[StepID]bool{}
	for _, spec := range Registry() {
		registered[spec.ID] = true
	}

	for _, id := range wantRetired {
		if !IsRetired(id) {
			t.Errorf("IsRetired(%q) = false, want true", id)
		}
		if registered[id] {
			t.Errorf("retired identifier %q is still registered", id)
		}
		if !panicsOnRegistration(id) {
			t.Errorf("re-registering retired identifier %q did not panic", id)
		}
	}
}

// panicsOnRegistration reports whether init()'s production guard rejects a
// registry that reuses id. It calls assertNoRetiredRegistrations rather than
// re-implementing the check, so the test cannot drift from the guard.
func panicsOnRegistration(id StepID) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	assertNoRetiredRegistrations([]StepSpec{{ID: id}})
	return false
}

// TestRegistryDeclarationsMatchMeasure is completeness test 6: a counting or
// counted entry declares a non-empty Corpus; an opaque entry declares none of
// Corpus/Outstanding/Order; a non-empty Outstanding requires Measure==counted.
func TestRegistryDeclarationsMatchMeasure(t *testing.T) {
	for _, spec := range Registry() {
		switch spec.Measure {
		case MeasureOpaque:
			if spec.Corpus != "" || spec.Outstanding != "" || len(spec.Order) != 0 {
				t.Errorf("step %q is opaque but declares Corpus=%q Outstanding=%q Order=%v", spec.ID, spec.Corpus, spec.Outstanding, spec.Order)
			}
		case MeasureCounting, MeasureCounted:
			if spec.Corpus == "" {
				t.Errorf("step %q has measure %q but declares no Corpus", spec.ID, spec.Measure)
			}
		default:
			t.Errorf("step %q has unknown measure %q", spec.ID, spec.Measure)
		}
		if spec.Outstanding != "" && spec.Measure != MeasureCounted {
			t.Errorf("step %q declares Outstanding but measure is %q, want counted", spec.ID, spec.Measure)
		}
	}
}
