package plugins

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/store"
)

func bptr(b bool) *bool { return &b }

func TestEffectiveAutoUpdate(t *testing.T) {
	cases := []struct {
		name     string
		override *bool
		def      bool
		want     bool
	}{
		{"no override inherits default off", nil, false, false},
		{"no override inherits default on", nil, true, true},
		{"override on wins over default off", bptr(true), false, true},
		{"override off wins over default on", bptr(false), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveAutoUpdate(tc.override, tc.def); got != tc.want {
				t.Fatalf("effectiveAutoUpdate(%v, %v) = %v, want %v", tc.override, tc.def, got, tc.want)
			}
		})
	}
}

func TestSetPluginAutoUpdatePersistsOverride(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")

	rec, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", bptr(true))
	if err != nil {
		t.Fatalf("SetPluginAutoUpdate(true): %v", err)
	}
	if rec.AutoUpdate == nil || !*rec.AutoUpdate {
		t.Fatalf("returned record AutoUpdate = %v, want *true", rec.AutoUpdate)
	}
	onDisk, err := fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("store.Get(): %v", err)
	}
	if onDisk.AutoUpdate == nil || !*onDisk.AutoUpdate {
		t.Fatalf("persisted AutoUpdate = %v, want *true", onDisk.AutoUpdate)
	}

	// Clearing the override (nil) restores inheritance of the global default.
	if _, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", nil); err != nil {
		t.Fatalf("SetPluginAutoUpdate(nil): %v", err)
	}
	onDisk, err = fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("store.Get(): %v", err)
	}
	if onDisk.AutoUpdate != nil {
		t.Fatalf("persisted AutoUpdate = %v, want nil after clear", onDisk.AutoUpdate)
	}
}

func TestSetPluginAutoUpdateUnknownPluginReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.SetPluginAutoUpdate("nope", bptr(true)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetPluginAutoUpdate(unknown) err = %v, want store.ErrNotFound", err)
	}
}

// TestAutoUpdateOverrideSurvivesUpgrade pins that an in-place upgrade (which
// rebuilds the record from the new package's manifest) carries the operator's
// per-plugin override forward rather than resetting it.
func TestAutoUpdateOverrideSurvivesUpgrade(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack") // v1.0.0
	if _, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", bptr(false)); err != nil {
		t.Fatalf("SetPluginAutoUpdate(false): %v", err)
	}

	rec2, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.1.0", false))
	if err != nil {
		t.Fatalf("upgrade Install(): %v", err)
	}
	if rec2.Version != "1.1.0" {
		t.Fatalf("upgraded version = %q, want 1.1.0", rec2.Version)
	}
	if rec2.AutoUpdate == nil || *rec2.AutoUpdate {
		t.Fatalf("upgraded record AutoUpdate = %v, want *false (override preserved)", rec2.AutoUpdate)
	}
	onDisk, err := fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("store.Get(): %v", err)
	}
	if onDisk.AutoUpdate == nil || *onDisk.AutoUpdate {
		t.Fatalf("persisted AutoUpdate after upgrade = %v, want *false", onDisk.AutoUpdate)
	}
}

func TestAutoUpdateCandidatesFiltersByStatusAndOptIn(t *testing.T) {
	svc, _, _ := newTestService(t)
	installTestPlugin(t, svc, "plugin-a") // active
	installTestPlugin(t, svc, "plugin-b") // active
	if err := svc.Disable("plugin-b"); err != nil {
		t.Fatalf("Disable(plugin-b): %v", err)
	}

	// Default on: only the active plugin is a candidate; the disabled one is
	// excluded even though it inherits the same "on" default.
	got := svc.autoUpdateCandidates(true)
	if !got["plugin-a"] || got["plugin-b"] {
		t.Fatalf("candidates(def=true) = %v, want only plugin-a", got)
	}

	// Per-plugin override off on the active plugin removes it despite default on.
	if _, err := svc.SetPluginAutoUpdate("plugin-a", bptr(false)); err != nil {
		t.Fatalf("SetPluginAutoUpdate(plugin-a,false): %v", err)
	}
	if got := svc.autoUpdateCandidates(true); len(got) != 0 {
		t.Fatalf("candidates(def=true, plugin-a opted out) = %v, want empty", got)
	}

	// Default off, per-plugin override on re-adds the active plugin.
	if _, err := svc.SetPluginAutoUpdate("plugin-a", bptr(true)); err != nil {
		t.Fatalf("SetPluginAutoUpdate(plugin-a,true): %v", err)
	}
	if got := svc.autoUpdateCandidates(false); !got["plugin-a"] || len(got) != 1 {
		t.Fatalf("candidates(def=false, plugin-a opted in) = %v, want only plugin-a", got)
	}
}

// TestEligibleForAutoUpdate pins the authoritative install-time gate that
// re-checks a plugin's current state (guarding the window between the candidate
// snapshot / catalog fetch and the actual install).
func TestEligibleForAutoUpdate(t *testing.T) {
	svc, _, ss := newAutoUpdateService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack") // active
	if err := ss.SetAutoUpdateDefault(true); err != nil {
		t.Fatalf("SetAutoUpdateDefault(true): %v", err)
	}

	if !svc.eligibleForAutoUpdate("kandev-plugin-slack") {
		t.Fatal("active, globally-opted-in plugin should be eligible")
	}

	// An unknown plugin is never eligible.
	if svc.eligibleForAutoUpdate("nope") {
		t.Fatal("unknown plugin must not be eligible")
	}

	// A per-plugin opt-out removes eligibility despite the global default.
	if _, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", bptr(false)); err != nil {
		t.Fatalf("SetPluginAutoUpdate(false): %v", err)
	}
	if svc.eligibleForAutoUpdate("kandev-plugin-slack") {
		t.Fatal("opted-out plugin must not be eligible")
	}

	// Clear the override (inherits the on default again), then disabling it
	// removes eligibility — the install-time gate never reactivates a plugin the
	// operator turned off mid-sweep.
	if _, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", nil); err != nil {
		t.Fatalf("SetPluginAutoUpdate(nil): %v", err)
	}
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}
	if svc.eligibleForAutoUpdate("kandev-plugin-slack") {
		t.Fatal("disabled plugin must not be eligible")
	}
}

// --- RunAutoUpdatePass integration (marketplace + install-from-URL) ---

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newAutoUpdateService returns a Service wired with a settings store and a
// marketplace whose only source advertises kandev-plugin-slack v1.1.0 (a newer
// version than the v1.0.0 the tests install locally), served from a local
// httptest tarball server.
func newAutoUpdateService(t *testing.T) (*Service, *fakeRuntime, *settingsStore) {
	t.Helper()
	svc, _, _, rt := newTestServiceWithDir(t)

	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	pool := db.NewPool(conn, conn)

	ss, err := newSettingsStore(pool)
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	svc.SetSettings(ss)

	pkgSrv := serveBytes(t, testPackage(t, "kandev-plugin-slack", "1.1.0", false).Bytes())
	body := `{"schema_version":1,"source":{"name":"Test","url":""},"plugins":[` +
		`{"id":"kandev-plugin-slack","name":"Slack","version":"1.1.0","package_url":"` +
		pkgSrv.URL + `/slack.tar.gz"}]}`
	idxSrv := serveBytes(t, []byte(body))

	srcStore, err := marketplace.NewSourceStore(pool)
	if err != nil {
		t.Fatalf("new source store: %v", err)
	}
	if err := srcStore.EnsureBuiltin("Test Source", idxSrv.URL); err != nil {
		t.Fatalf("EnsureBuiltin: %v", err)
	}
	svc.SetMarketplace(marketplace.NewService(srcStore, testLogger(t)))

	return svc, rt, ss
}

func TestRunAutoUpdatePassUpgradesActiveOptedInPlugin(t *testing.T) {
	svc, rt, ss := newAutoUpdateService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack") // v1.0.0, active
	if err := ss.SetAutoUpdateDefault(true); err != nil {
		t.Fatalf("SetAutoUpdateDefault(true): %v", err)
	}

	outcome, err := svc.RunAutoUpdatePass(context.Background())
	if err != nil {
		t.Fatalf("RunAutoUpdatePass(): %v", err)
	}
	if len(outcome.Updated) != 1 || outcome.Updated[0].ID != "kandev-plugin-slack" ||
		outcome.Updated[0].From != "1.0.0" || outcome.Updated[0].To != "1.1.0" {
		t.Fatalf("outcome.Updated = %+v, want one 1.0.0->1.1.0 update", outcome.Updated)
	}
	rec, err := svc.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if rec.Version != "1.1.0" {
		t.Fatalf("installed version = %q, want 1.1.0 after auto-update", rec.Version)
	}
	if rec.Status != StatusActive || !rt.Running("kandev-plugin-slack") {
		t.Fatalf("plugin not active/running after auto-update: status=%q running=%v", rec.Status, rt.Running("kandev-plugin-slack"))
	}
}

func TestRunAutoUpdatePassRespectsPerPluginOverrideWhenDefaultOff(t *testing.T) {
	svc, _, ss := newAutoUpdateService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := ss.SetAutoUpdateDefault(false); err != nil {
		t.Fatalf("SetAutoUpdateDefault(false): %v", err)
	}
	if _, err := svc.SetPluginAutoUpdate("kandev-plugin-slack", bptr(true)); err != nil {
		t.Fatalf("SetPluginAutoUpdate(true): %v", err)
	}

	outcome, err := svc.RunAutoUpdatePass(context.Background())
	if err != nil {
		t.Fatalf("RunAutoUpdatePass(): %v", err)
	}
	if len(outcome.Updated) != 1 {
		t.Fatalf("outcome.Updated = %+v, want the overridden plugin to update", outcome.Updated)
	}
	rec, _ := svc.Get("kandev-plugin-slack")
	if rec.Version != "1.1.0" {
		t.Fatalf("version = %q, want 1.1.0 (per-plugin override on)", rec.Version)
	}
}

func TestRunAutoUpdatePassSkipsDisabledPlugin(t *testing.T) {
	svc, _, ss := newAutoUpdateService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	if err := svc.Disable("kandev-plugin-slack"); err != nil {
		t.Fatalf("Disable(): %v", err)
	}
	if err := ss.SetAutoUpdateDefault(true); err != nil { // opted in globally
		t.Fatalf("SetAutoUpdateDefault(true): %v", err)
	}

	outcome, err := svc.RunAutoUpdatePass(context.Background())
	if err != nil {
		t.Fatalf("RunAutoUpdatePass(): %v", err)
	}
	if len(outcome.Updated) != 0 {
		t.Fatalf("outcome.Updated = %+v, want empty (disabled plugins do not auto-update)", outcome.Updated)
	}
	rec, _ := svc.Get("kandev-plugin-slack")
	if rec.Version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0 (disabled plugin must stay put)", rec.Version)
	}
}

func TestRunAutoUpdatePassNoOpWhenNoneOptedIn(t *testing.T) {
	svc, _, _ := newAutoUpdateService(t)
	installTestPlugin(t, svc, "kandev-plugin-slack")
	// Default is off and there is no per-plugin override.

	outcome, err := svc.RunAutoUpdatePass(context.Background())
	if err != nil {
		t.Fatalf("RunAutoUpdatePass(): %v", err)
	}
	if len(outcome.Updated) != 0 || len(outcome.Failed) != 0 {
		t.Fatalf("outcome = %+v, want empty when nothing is opted in", outcome)
	}
	rec, _ := svc.Get("kandev-plugin-slack")
	if rec.Version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0 (nothing opted in)", rec.Version)
	}
}

func TestRunAutoUpdatePassNoMarketplaceIsNoOp(t *testing.T) {
	svc, _, _ := newTestService(t) // no marketplace, no settings attached
	installTestPlugin(t, svc, "kandev-plugin-slack")

	outcome, err := svc.RunAutoUpdatePass(context.Background())
	if err != nil {
		t.Fatalf("RunAutoUpdatePass() with no marketplace should be a no-op, got err: %v", err)
	}
	if len(outcome.Updated) != 0 {
		t.Fatalf("outcome.Updated = %+v, want empty with no marketplace", outcome.Updated)
	}
}
