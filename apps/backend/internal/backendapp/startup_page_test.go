package backendapp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/startup"
)

func int64Ptr(v int64) *int64 { return &v }

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.13
func TestBuildStartupPageViewNoStep(t *testing.T) {
	view := buildStartupPageView("en", startup.Snapshot{Phase: startup.ApplyingMigrations, ElapsedMS: 4200})
	if view.HasStep {
		t.Fatal("no-step snapshot must not render a step")
	}
	if view.PhaseLabel != "Applying migrations" {
		t.Fatalf("phase label = %q, want %q", view.PhaseLabel, "Applying migrations")
	}
	if view.Elapsed != "5 seconds elapsed" {
		t.Fatalf("elapsed = %q, want the rounded-up elapsed string", view.Elapsed)
	}
}

func TestBuildStartupPageViewOpaqueStep(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.prompt_seq",
			Measure:  startup.MeasureOpaque,
			Unit:     startup.UnitRows,
		},
	}
	view := buildStartupPageView("en", snap)
	if !view.HasStep {
		t.Fatal("want a step")
	}
	if view.Step.Label != "Prompt sequence numbers" {
		t.Fatalf("step label = %q", view.Step.Label)
	}
	if view.Step.Progress != "Progress cannot be measured for this step." {
		t.Fatalf("opaque progress = %q", view.Step.Progress)
	}
	if view.Step.HasBar || view.Step.HasETA {
		t.Fatal("opaque step must render no bar and no ETA")
	}
}

func TestBuildStartupPageViewCountingStep(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.BackingUpDatabase,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.database_backup",
			Measure:  startup.MeasureCounting,
			Unit:     startup.UnitBytes,
			Done:     int64Ptr(1),
		},
	}
	view := buildStartupPageView("en", snap)
	if view.Step.Progress != "1 byte" {
		t.Fatalf("counting progress = %q, want singular unit", view.Step.Progress)
	}
	if view.Step.HasBar {
		t.Fatal("counting step (no total) must render no bar")
	}
}

func TestBuildStartupPageViewCountedStepWithETA(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.stores_repositories",
			Measure:  startup.MeasureCounted,
			Unit:     startup.UnitTurns,
			Done:     int64Ptr(5),
			Total:    int64Ptr(10),
			ETAMS:    int64Ptr(30000),
		},
	}
	view := buildStartupPageView("en", snap)
	if view.Step.Progress != "5 of 10 turns" {
		t.Fatalf("counted progress = %q, want plural unit from total", view.Step.Progress)
	}
	if !view.Step.HasBar || view.Step.PercentDoneStr != "50.00" {
		t.Fatalf("bar = %v %q, want 50.00%%", view.Step.HasBar, view.Step.PercentDoneStr)
	}
	if !view.Step.HasETA || view.Step.ETA != "About 30 seconds remaining" {
		t.Fatalf("eta = %v %q", view.Step.HasETA, view.Step.ETA)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-004.6
func TestBuildStartupPageViewStalledStep(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey:       "startup.step.stores_repositories",
			Measure:        startup.MeasureCounted,
			Unit:           startup.UnitTurns,
			Done:           int64Ptr(5),
			Total:          int64Ptr(10),
			SinceAdvanceMS: int64Ptr(125000),
			Stalled:        true,
		},
	}
	view := buildStartupPageView("en", snap)
	if !view.Step.Stalled {
		t.Fatal("want the step view to report stalled")
	}
	if view.Step.StalledText != "Stalled for 3 minutes" {
		t.Fatalf("stalled text = %q, want the rounded-up duration", view.Step.StalledText)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-004.5
func TestBuildStartupPageViewRunningStepHasNoStalledText(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey:       "startup.step.stores_repositories",
			Measure:        startup.MeasureCounted,
			Unit:           startup.UnitTurns,
			Done:           int64Ptr(5),
			Total:          int64Ptr(10),
			SinceAdvanceMS: int64Ptr(500),
			Stalled:        false,
		},
	}
	view := buildStartupPageView("en", snap)
	if view.Step.Stalled || view.Step.StalledText != "" {
		t.Fatalf("running step must render no stalled state, got Stalled=%v StalledText=%q", view.Step.Stalled, view.Step.StalledText)
	}
}

func TestBuildStartupPageViewCountedStepSingularTotal(t *testing.T) {
	snap := startup.Snapshot{
		Phase: startup.ApplyingMigrations,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.stores_repositories",
			Measure:  startup.MeasureCounted,
			Unit:     startup.UnitTurns,
			Done:     int64Ptr(1),
			Total:    int64Ptr(1),
		},
	}
	view := buildStartupPageView("en", snap)
	if view.Step.Progress != "1 of 1 turn" {
		t.Fatalf("counted progress = %q, want singular unit from total=1", view.Step.Progress)
	}
}

func TestWriteStartupPageHeadersAndStatus(t *testing.T) {
	w := httptest.NewRecorder()
	writeStartupPage(w, httptest.NewRequest("GET", "/", nil), startup.Snapshot{Phase: startup.OpeningDatabase})

	if w.Code != 503 {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Opening database") {
		t.Fatalf("body missing phase label: %s", body)
	}
	if !strings.Contains(body, `role="status" aria-live="polite"`) {
		t.Fatal("body missing the live region required by AC-PLATFORM-STARTUP-PROGRESS-003.11")
	}
	if !strings.Contains(body, "prefers-reduced-motion") {
		t.Fatal("body missing the reduced-motion accommodation required by AC-PLATFORM-STARTUP-PROGRESS-003.11")
	}

	dataStart := strings.Index(body, `<script id="startup-data" type="application/json">`)
	if dataStart < 0 {
		t.Fatal("body missing the startup-data JSON island")
	}
	dataStart += len(`<script id="startup-data" type="application/json">`)
	dataEnd := strings.Index(body[dataStart:], "</script>")
	if dataEnd < 0 {
		t.Fatal("unterminated startup-data script")
	}
	var island struct {
		Translations map[string]string `json:"translations"`
		Snapshot     startup.Snapshot  `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(body[dataStart:dataStart+dataEnd]), &island); err != nil {
		t.Fatalf("startup-data island did not parse as JSON: %v", err)
	}
	if island.Snapshot.Phase != startup.OpeningDatabase {
		t.Fatalf("embedded snapshot phase = %q, want %q", island.Snapshot.Phase, startup.OpeningDatabase)
	}
	if island.Translations["startup.phase.opening_database"] != "Opening database" {
		t.Fatalf("embedded translations missing phase key, got %#v", island.Translations)
	}
}

// @covers AC-PLATFORM-STARTUP-PROGRESS-003.3
func TestNewBootstrapHandlerServesStartupPageForHTMLPreferringSPARequest(t *testing.T) {
	handler := newBootstrapHandler("test")
	req := httptest.NewRequest("GET", "/tasks", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html for an HTML-preferring SPA route request", got)
	}
}

func TestNewBootstrapHandlerKeepsJSONForReadyEvenWhenHTMLPreferred(t *testing.T) {
	handler := newBootstrapHandler("test")
	req := httptest.NewRequest("GET", "/ready", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON: /ready must keep answering the snapshot even when HTML is preferred", got)
	}
}

func TestNewBootstrapHandlerKeepsJSONForNonSPARoute(t *testing.T) {
	handler := newBootstrapHandler("test")
	req := httptest.NewRequest("GET", "/api/v1/features", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON for a non-application route", got)
	}
}

func TestNewBootstrapHandlerKeepsJSONWhenJSONPreferred(t *testing.T) {
	handler := newBootstrapHandler("test")
	req := httptest.NewRequest("GET", "/tasks", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON when the client prefers it", got)
	}
}

func TestNewBootstrapHandlerKeepsJSONForNonGETMethod(t *testing.T) {
	handler := newBootstrapHandler("test")
	req := httptest.NewRequest("POST", "/tasks", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON for a non-GET method", got)
	}
}
