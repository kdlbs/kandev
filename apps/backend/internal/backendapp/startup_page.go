package backendapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/kandev/kandev/internal/i18n"
	"github.com/kandev/kandev/internal/startup"
)

// writeStartupPage renders the self-contained HTML startup page
// (AC-PLATFORM-STARTUP-PROGRESS-003.1) directly from the in-process
// reporter snapshot. It embeds no bundle: markup, styling, and the polling
// script that keeps the page live all live in startupPageTemplate. The
// caller (newBootstrapHandler) has already decided the request qualifies
// per AC-003.3.
func writeStartupPage(w http.ResponseWriter, r *http.Request, snap startup.Snapshot) {
	locale := i18n.FromRequest(r)
	view := buildStartupPageView(locale, snap)

	var buf bytes.Buffer
	if err := startupPageTemplate.Execute(&buf, view); err != nil {
		writeBootstrapJSON(w, http.StatusServiceUnavailable, map[string]any{
			statusKey:       startingStatus,
			serviceFieldKey: kandevName,
		})
		return
	}

	// AC-PLATFORM-STARTUP-PROGRESS-003.12: no-store so no intermediary keeps
	// serving this page after startup completes. Status stays 503 throughout,
	// matching every other bootstrap-handler branch.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write(buf.Bytes())
}

// startupStepView is the rendered form of one StepSnapshot, resolved for a
// single locale so the template stays free of measure-branching logic.
type startupStepView struct {
	Label          string
	Progress       string
	ETA            string
	HasETA         bool
	HasBar         bool
	PercentDoneStr string
	Stalled        bool
	StalledText    string
}

// startupPageView is the html/template data for startupPageTemplate.
type startupPageView struct {
	Locale     string
	Title      string
	PhaseLabel string
	Elapsed    string
	HasStep    bool
	Step       startupStepView
	DataJSON   template.JS
}

func buildStartupPageView(locale string, snap startup.Snapshot) startupPageView {
	view := startupPageView{
		Locale:     locale,
		Title:      i18n.T(locale, "startup.page.title"),
		PhaseLabel: i18n.T(locale, "startup.phase."+string(snap.Phase)),
		Elapsed:    formatElapsed(locale, snap.ElapsedMS),
	}
	if snap.Step != nil {
		view.HasStep = true
		view.Step = buildStepView(locale, snap.Step)
	}
	view.DataJSON = marshalStartupData(locale, snap)
	return view
}

func buildStepView(locale string, step *startup.StepSnapshot) startupStepView {
	view := startupStepView{Label: i18n.T(locale, step.LabelKey)}
	switch step.Measure {
	case startup.MeasureOpaque:
		view.Progress = i18n.T(locale, "startup.page.progressUnavailable")
	case startup.MeasureCounting:
		view.Progress = countingProgressText(locale, step)
	case startup.MeasureCounted:
		applyCountedProgress(locale, step, &view)
	}
	if step.Stalled && step.SinceAdvanceMS != nil {
		view.Stalled = true
		view.StalledText = i18n.Tf(locale, "startup.page.stalled", map[string]any{tfVarDuration: formatDuration(locale, *step.SinceAdvanceMS)})
	}
	return view
}

// Tf variable names, factored out because the i18n placeholder names
// ("{{count}}", "{{total}}", ...) that the catalog strings interpolate
// against must match these literals exactly.
const (
	tfVarCount    = "count"
	tfVarTotal    = "total"
	tfVarDone     = "done"
	tfVarUnit     = "unit"
	tfVarDuration = "duration"
)

func countingProgressText(locale string, step *startup.StepSnapshot) string {
	done := stepDone(step)
	unit := i18n.Tf(locale, "startup.unit."+string(step.Unit), map[string]any{tfVarCount: done})
	return i18n.Tf(locale, "startup.page.countingProgress", map[string]any{tfVarDone: done, tfVarUnit: unit})
}

func applyCountedProgress(locale string, step *startup.StepSnapshot, view *startupStepView) {
	done, total := stepDone(step), stepTotal(step)
	unit := i18n.Tf(locale, "startup.unit."+string(step.Unit), map[string]any{tfVarCount: total})
	view.Progress = i18n.Tf(locale, "startup.page.countedProgress", map[string]any{
		tfVarDone: done, tfVarTotal: total, tfVarUnit: unit,
	})
	if total > 0 {
		view.HasBar = true
		view.PercentDoneStr = fmt.Sprintf("%.2f", percentDone(done, total))
	}
	if step.ETAMS != nil {
		view.HasETA = true
		view.ETA = i18n.Tf(locale, "startup.page.eta", map[string]any{tfVarDuration: formatDuration(locale, *step.ETAMS)})
	}
}

func stepDone(step *startup.StepSnapshot) int64 {
	if step.Done == nil {
		return 0
	}
	return *step.Done
}

func stepTotal(step *startup.StepSnapshot) int64 {
	if step.Total == nil {
		return 0
	}
	return *step.Total
}

func percentDone(done, total int64) float64 {
	pct := float64(done) / float64(total) * 100
	switch {
	case pct < 0:
		return 0
	case pct > 100:
		return 100
	default:
		return pct
	}
}

// formatDuration mirrors startup.EstimateComponents's rounding rule
// (AC-PLATFORM-STARTUP-PROGRESS-003.10): whole seconds rounded up with a
// floor of one second at or below 60000 ms, whole minutes rounded up above
// that. The embedded poll script (startupPageScript) re-implements the same
// rule in JavaScript for subsequent reads, since the page has no bundle to
// share Go code with.
func formatDuration(locale string, ms int64) string {
	value, minutes := startup.EstimateComponents(ms)
	key := "startup.duration.seconds"
	if minutes {
		key = "startup.duration.minutes"
	}
	return i18n.Tf(locale, key, map[string]any{tfVarCount: value})
}

func formatElapsed(locale string, ms int64) string {
	return i18n.Tf(locale, "startup.page.elapsed", map[string]any{tfVarDuration: formatDuration(locale, ms)})
}

// startupPageKeys lists every catalog key the page's initial render or its
// client-side re-poll can need. Keeping it explicit (rather than embedding
// the whole catalog) means the translations island carries only what this
// page uses.
func startupPageKeys() []string {
	keys := []string{
		"startup.page.title", "startup.page.waiting", "startup.page.lastKnown",
		"startup.page.progressUnavailable", "startup.page.countedProgress",
		"startup.page.countingProgress", "startup.page.elapsed", "startup.page.eta",
		"startup.page.stalled",
		"startup.phase.opening_database", "startup.phase.backing_up_database",
		"startup.phase.applying_migrations", "startup.phase.initializing_services",
		"startup.phase.recovering_sessions", "startup.phase.ready",
		"startup.duration.seconds_one", "startup.duration.seconds_other",
		"startup.duration.minutes_one", "startup.duration.minutes_other",
	}
	for _, unit := range []startup.Unit{
		startup.UnitRows, startup.UnitMessages, startup.UnitTurns,
		startup.UnitSessions, startup.UnitBytes, startup.UnitStores,
	} {
		keys = append(keys, "startup.unit."+string(unit)+"_one", "startup.unit."+string(unit)+"_other")
	}
	for _, spec := range startup.Registry() {
		keys = append(keys, spec.LabelKey)
	}
	return keys
}

func translationsFor(locale string) map[string]string {
	out := make(map[string]string, len(startupPageKeys()))
	for _, key := range startupPageKeys() {
		out[key] = i18n.T(locale, key)
	}
	return out
}

// marshalStartupData embeds the resolved translations plus the initial
// snapshot as a JSON island the poll script reads on load, so every
// subsequent /ready read can be rendered client-side without another
// server round trip for markup. The "</" escape defends against a
// catalog string that could otherwise prematurely close the script tag.
func marshalStartupData(locale string, snap startup.Snapshot) template.JS {
	payload := map[string]any{
		"translations": translationsFor(locale),
		"snapshot":     snap,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte(`{"translations":{},"snapshot":null}`)
	}
	return template.JS(strings.ReplaceAll(string(encoded), "</", "<\\/")) //nolint:gosec // JSON-only, no HTML
}

var startupPageTemplate = template.Must(template.New("startup-page").Parse(startupPageTemplateSource))

const startupPageTemplateSource = `<!DOCTYPE html>
<html lang="{{.Locale}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}}</title>
<style>
  :root { color-scheme: light dark; }
  body {
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh; margin: 0; padding: 1.5rem; box-sizing: border-box;
    background: Canvas; color: CanvasText;
  }
  main { max-width: 26rem; width: 100%; text-align: center; }
  h1 { font-size: 1.05rem; font-weight: 600; margin: 0 0 1rem; }
  p { margin: 0.35rem 0; opacity: 0.85; }
  .bar { height: 6px; border-radius: 3px; background: color-mix(in srgb, CanvasText 15%, transparent); overflow: hidden; margin-top: 1rem; }
  .bar > span { display: block; height: 100%; background: #6c9eff; transition: width 300ms ease; }
  @media (prefers-reduced-motion: reduce) { .bar > span { transition: none; } }
  .last-known { font-size: 0.8rem; opacity: 0.6; margin-top: 0.5rem; }
  .hidden { display: none; }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<div id="startup-status" role="status" aria-live="polite">
  <p id="startup-phase">{{.PhaseLabel}}</p>
  <p id="startup-elapsed">{{.Elapsed}}</p>
  <p id="startup-step-label" class="{{if not .HasStep}}hidden{{end}}">{{.Step.Label}}</p>
  <p id="startup-progress" class="{{if not .HasStep}}hidden{{end}}">{{.Step.Progress}}</p>
  <p id="startup-eta" class="{{if not .Step.HasETA}}hidden{{end}}">{{.Step.ETA}}</p>
  <div id="startup-bar" class="bar {{if not .Step.HasBar}}hidden{{end}}"><span id="startup-bar-fill" style="width:{{.Step.PercentDoneStr}}%"></span></div>
  <p id="startup-stalled" class="{{if not .Step.Stalled}}hidden{{end}}">{{.Step.StalledText}}</p>
  <p id="startup-last-known" class="last-known hidden"></p>
</div>
</main>
<script id="startup-data" type="application/json">{{.DataJSON}}</script>
<script>` + startupPageScript + `</script>
</body>
</html>
`

// startupPageScript keeps the page live after the initial server render.
// It polls /ready on the AC-PLATFORM-STARTUP-PROGRESS-003.2 cadence — one
// request in flight, abandoned after 5000 ms, any tick skipped while one is
// outstanding — and never gives up while the page is open. It reloads once
// the polled snapshot itself reports the ready phase (not merely a
// successful HTTP status, since AC-PLATFORM-STARTUP-PROGRESS-002.3 allows a
// post-startup degraded response that still carries a ready snapshot).
const startupPageScript = `
(function () {
  "use strict";
  var POLL_MS = 1000;
  var TIMEOUT_MS = 5000;
  var dataEl = document.getElementById("startup-data");
  var data = JSON.parse(dataEl.textContent);
  var t = data.translations;
  var inFlight = false;

  var phases = {
    opening_database: true,
    backing_up_database: true,
    applying_migrations: true,
    initializing_services: true,
    recovering_sessions: true,
    ready: true
  };
  var measures = { opaque: true, counting: true, counted: true };
  var units = { rows: true, messages: true, turns: true, sessions: true, bytes: true, stores: true };

  function nonNegativeInteger(value) {
    return typeof value === "number" && isFinite(value) && value >= 0 && Math.floor(value) === value;
  }

  function nonNegativeNumber(value) {
    return typeof value === "number" && isFinite(value) && value >= 0;
  }

  function validStartupStep(step) {
    if (!step || typeof step !== "object" ||
        typeof step.id !== "string" || !step.id ||
        typeof step.label_key !== "string" || !step.label_key ||
        !measures[step.measure] || !units[step.unit] ||
        !nonNegativeInteger(step.elapsed_ms) || typeof step.stalled !== "boolean") {
      return false;
    }
    for (var i = 0; i < ["done", "total", "eta_ms", "since_advance_ms"].length; i++) {
      var key = ["done", "total", "eta_ms", "since_advance_ms"][i];
      if (step[key] !== undefined && !nonNegativeInteger(step[key])) return false;
    }
    return step.rate_per_second === undefined || nonNegativeNumber(step.rate_per_second);
  }

  function validStartupSnapshot(snapshot) {
    if (!snapshot || typeof snapshot !== "object" || !phases[snapshot.phase] ||
        !nonNegativeInteger(snapshot.boot) || !nonNegativeInteger(snapshot.seq) ||
        !nonNegativeInteger(snapshot.elapsed_ms) || !nonNegativeInteger(snapshot.phase_elapsed_ms)) {
      return false;
    }
    return snapshot.step === undefined || validStartupStep(snapshot.step);
  }

  function el(id) { return document.getElementById(id); }
  function setText(id, text) { var e = el(id); if (e) e.textContent = text || ""; }
  function toggle(id, show) { var e = el(id); if (e) e.classList.toggle("hidden", !show); }

  function interpolate(message, vars) {
    if (!message) return "";
    return message.replace(/\{\{(\w+)\}\}/g, function (_, name) {
      return Object.prototype.hasOwnProperty.call(vars, name) ? vars[name] : "";
    });
  }

  function pluralLookup(base, count) {
    var key = base + (count === 1 ? "_one" : "_other");
    return t[key] || t[base] || "";
  }

  function ceilDiv(numerator, denominator) {
    if (numerator <= 0) return 0;
    return Math.floor((numerator + denominator - 1) / denominator);
  }

  function estimateComponents(ms) {
    if (ms <= 60000) {
      var seconds = ceilDiv(ms, 1000);
      if (seconds < 1) seconds = 1;
      return { value: seconds, minutes: false };
    }
    return { value: ceilDiv(ms, 60000), minutes: true };
  }

  function durationLabel(ms) {
    var c = estimateComponents(ms);
    var key = c.minutes ? "startup.duration.minutes" : "startup.duration.seconds";
    return interpolate(pluralLookup(key, c.value), { count: c.value });
  }

  function unitLabel(unit, count) {
    return pluralLookup("startup.unit." + unit, count);
  }

  function progressText(step) {
    if (step.measure === "opaque") return t["startup.page.progressUnavailable"];
    if (step.measure === "counting") {
      var done = step.done || 0;
      return interpolate(t["startup.page.countingProgress"], { done: done, unit: unitLabel(step.unit, done) });
    }
    var counted = step.done || 0;
    var total = step.total || 0;
    return interpolate(t["startup.page.countedProgress"], { done: counted, total: total, unit: unitLabel(step.unit, total) });
  }

  function renderStep(step) {
    toggle("startup-step-label", !!step);
    toggle("startup-progress", !!step);
    toggle("startup-eta", false);
    toggle("startup-bar", false);
    toggle("startup-stalled", false);
    if (!step) return;
    setText("startup-step-label", t[step.label_key]);
    setText("startup-progress", progressText(step));
    if (step.measure === "counted" && step.eta_ms != null) {
      setText("startup-eta", interpolate(t["startup.page.eta"], { duration: durationLabel(step.eta_ms) }));
      toggle("startup-eta", true);
    }
    if (step.measure === "counted" && step.total) {
      var pct = Math.max(0, Math.min(100, (step.done || 0) / step.total * 100));
      var fill = el("startup-bar-fill");
      if (fill) fill.style.width = pct + "%";
      toggle("startup-bar", true);
    }
    if (step.stalled && step.since_advance_ms != null) {
      setText("startup-stalled", interpolate(t["startup.page.stalled"], { duration: durationLabel(step.since_advance_ms) }));
      toggle("startup-stalled", true);
    }
  }

  function render(snapshot) {
    setText("startup-phase", t["startup.phase." + snapshot.phase]);
    setText("startup-elapsed", interpolate(t["startup.page.elapsed"], { duration: durationLabel(snapshot.elapsed_ms) }));
    renderStep(snapshot.step || null);
  }

  function markLastKnown(isLastKnown) {
    setText("startup-last-known", isLastKnown ? t["startup.page.lastKnown"] : "");
    toggle("startup-last-known", isLastKnown);
  }

  function handleBody(ok, body) {
    var snapshot = body && body.startup;
    if (!validStartupSnapshot(snapshot)) {
      // AC-PLATFORM-STARTUP-PROGRESS-002.6: no parseable snapshot means the
      // status code is authoritative.
      if (ok) { window.location.reload(); return; }
      markLastKnown(true);
      return;
    }
    markLastKnown(false);
    if (snapshot.phase === "ready") { window.location.reload(); return; }
    render(snapshot);
  }

  function poll() {
    if (inFlight) return;
    inFlight = true;
    var controller = ("AbortController" in window) ? new AbortController() : null;
    var timeoutId = controller ? setTimeout(function () { controller.abort(); }, TIMEOUT_MS) : null;
    fetch("/ready", { cache: "no-store", signal: controller ? controller.signal : undefined })
      .then(function (res) {
        return res
          .json()
          .then(function (body) { return { ok: res.ok, body: body }; })
          .catch(function () { return { ok: res.ok, body: null }; });
      })
      .then(function (result) {
        if (timeoutId) clearTimeout(timeoutId);
        inFlight = false;
        handleBody(result.ok, result.body);
      })
      .catch(function () {
        if (timeoutId) clearTimeout(timeoutId);
        inFlight = false;
        markLastKnown(true);
      });
  }

  setInterval(poll, POLL_MS);
})();
`
