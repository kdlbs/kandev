package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/system/metrics"
)

// TestSplitMetrics covers the query-list parser. Blank entries have to be
// dropped rather than passed through, because an empty metric ID reaches the
// collector as an unknown metric.
func TestSplitMetrics(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"single", "cpu", []string{"cpu"}},
		{"several with spaces", " cpu , memory ,disk ", []string{"cpu", "memory", "disk"}},
		{"blank entries dropped", "cpu,,memory,", []string{"cpu", "memory"}},
		{"only separators", ",,,", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitMetrics(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitMetrics(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestHandleSystemMetrics_StampsExecutionIdentity asserts the snapshot is
// labelled as this executor before it leaves the handler. The dashboard merges
// host and executor snapshots into one list keyed by ID, so an unstamped
// snapshot would collide with the host's.
func TestHandleSystemMetrics_StampsExecutionIdentity(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		ID      string           `json:"id"`
		Label   string           `json:"label"`
		Kind    string           `json:"kind"`
		Metrics []map[string]any `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}
	if snapshot.ID != "agentctl" {
		t.Errorf("id = %q, want %q", snapshot.ID, "agentctl")
	}
	if snapshot.Label != "Execution" {
		t.Errorf("label = %q, want %q", snapshot.Label, "Execution")
	}
	if snapshot.Kind != "execution" {
		t.Errorf("kind = %q, want %q", snapshot.Kind, "execution")
	}
	if len(snapshot.Metrics) != len(metrics.DefaultSettings().Metrics) {
		t.Errorf("sampled %d metrics, want the %d defaults",
			len(snapshot.Metrics), len(metrics.DefaultSettings().Metrics))
	}
}

// TestHandleSystemMetrics_HonoursRequestedMetrics pins the selection: asking
// for one metric must sample exactly that one, not the default set.
func TestHandleSystemMetrics_HonoursRequestedMetrics(t *testing.T) {
	srv := newTestServer(t)
	wanted := metrics.DefaultSettings().Metrics[0]

	rec := serverGet(t, srv, "/api/v1/system/metrics?metrics="+wanted)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}
	if len(snapshot.Metrics) != 1 || snapshot.Metrics[0].ID != wanted {
		t.Errorf("metrics = %+v, want exactly [%s]", snapshot.Metrics, wanted)
	}
}

// TestHandleSystemMetrics_DiagnosticMetricsAbsentByDefault guards the
// user-visible-surface boundary: the agentctl diagnostic metric IDs
// must never appear unless a caller names them explicitly, because they are
// deliberately excluded from metrics.isKnownMetric and must never leak into
// what looks like the persisted/broadcast metric set.
func TestHandleSystemMetrics_DiagnosticMetricsAbsentByDefault(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}
	diagnostic := map[string]bool{
		metrics.MetricAgentctlGoroutines:    true,
		metrics.MetricAgentctlGitPollMillis: true,
		metrics.MetricAgentctlMonitorMillis: true,
		metrics.MetricAgentctlCreateReadyMs: true,
	}
	for _, sample := range snapshot.Metrics {
		if diagnostic[sample.ID] {
			t.Errorf("default snapshot unexpectedly includes diagnostic metric %q", sample.ID)
		}
	}
}

// TestHandleSystemMetrics_DiagnosticMetricsServedWhenRequested asserts the
// diagnostic metrics are served, with the expected shape, only when
// explicitly named. newTestServer's config never goes through
// instance.Manager.CreateInstance and its process.Manager is never started,
// so agentctl_create_ready_ms and agentctl_git_poll_ms are exercised on their
// "not recorded yet" branches; agentctl_goroutines is always available.
func TestHandleSystemMetrics_DiagnosticMetricsServedWhenRequested(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics?metrics="+
		metrics.MetricAgentctlGoroutines+","+
		metrics.MetricAgentctlGitPollMillis+","+
		metrics.MetricAgentctlMonitorMillis+","+
		metrics.MetricAgentctlCreateReadyMs)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []metrics.MetricSample `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}

	// Assert on the list itself, not a map keyed by ID: a collector that
	// doesn't recognize a diagnostic ID emits its own "unknown metric" sample
	// alongside the handler's correct one for the same ID, and collapsing
	// into a map (last-write-wins) hides that duplication instead of
	// catching it.
	wantIDs := []string{
		metrics.MetricAgentctlGoroutines,
		metrics.MetricAgentctlGitPollMillis,
		metrics.MetricAgentctlMonitorMillis,
		metrics.MetricAgentctlCreateReadyMs,
	}
	if len(snapshot.Metrics) != len(wantIDs) {
		t.Fatalf("got %d metrics %+v, want exactly %d (one per requested ID)",
			len(snapshot.Metrics), snapshot.Metrics, len(wantIDs))
	}
	counts := make(map[string]int, len(snapshot.Metrics))
	for _, sample := range snapshot.Metrics {
		counts[sample.ID]++
	}
	for _, id := range wantIDs {
		if counts[id] != 1 {
			t.Fatalf("id %q appears %d times in %+v, want exactly 1", id, counts[id], snapshot.Metrics)
		}
	}

	byID := make(map[string]metrics.MetricSample, len(snapshot.Metrics))
	for _, sample := range snapshot.Metrics {
		byID[sample.ID] = sample
	}

	goroutines := byID[metrics.MetricAgentctlGoroutines]
	if !goroutines.Available || goroutines.Value == nil || *goroutines.Value <= 0 {
		t.Errorf("goroutines sample = %+v, want available with a positive value", goroutines)
	}

	gitPoll := byID[metrics.MetricAgentctlGitPollMillis]
	if gitPoll.Available || gitPoll.Error == "" {
		t.Errorf("git poll sample = %+v, want unavailable with an error (no tracker started)", gitPoll)
	}

	monitorPoll := byID[metrics.MetricAgentctlMonitorMillis]
	if monitorPoll.Available || monitorPoll.Error == "" {
		t.Errorf("monitor poll sample = %+v, want unavailable with an error (no tracker started)", monitorPoll)
	}

	createReady := byID[metrics.MetricAgentctlCreateReadyMs]
	if createReady.Available || createReady.Error == "" {
		t.Errorf("create-ready sample = %+v, want unavailable with an error (nil CreateReadyMillis)", createReady)
	}
}

func TestHandleSystemMetrics_CreateReadyIsAvailableAfterInstanceStartup(t *testing.T) {
	log := newTestLogger()
	readyMillis := &atomic.Int64{}
	readyMillis.Store(42)
	cfg := &config.InstanceConfig{
		Port:              0,
		WorkDir:           t.TempDir(),
		CreateReadyMillis: readyMillis,
	}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	rec := serverGet(t, server, "/api/v1/system/metrics?metrics="+metrics.MetricAgentctlCreateReadyMs)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []metrics.MetricSample `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}
	if len(snapshot.Metrics) != 1 {
		t.Fatalf("metrics = %+v, want one sample", snapshot.Metrics)
	}
	sample := snapshot.Metrics[0]
	if sample.ID != metrics.MetricAgentctlCreateReadyMs || !sample.Available || sample.Value == nil || *sample.Value != 42 {
		t.Fatalf("create-ready sample = %+v, want available value 42", sample)
	}
}

// TestHandleSystemMetrics_MixedMetricsPreserveRequestOrder pins the
// interleaving behaviour when a request mixes a standard (collector-served)
// metric ID with a diagnostic (handler-served) one: the response must come
// back in the same order they were requested in, not with every diagnostic
// sample trailing every collector sample regardless of query order.
func TestHandleSystemMetrics_MixedMetricsPreserveRequestOrder(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics?metrics="+
		metrics.MetricAgentctlGoroutines+","+
		metrics.MetricCPUPercent+","+
		metrics.MetricAgentctlGitPollMillis)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []metrics.MetricSample `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}

	wantOrder := []string{
		metrics.MetricAgentctlGoroutines,
		metrics.MetricCPUPercent,
		metrics.MetricAgentctlGitPollMillis,
	}
	if len(snapshot.Metrics) != len(wantOrder) {
		t.Fatalf("got %d metrics %+v, want exactly %d (one per requested ID)",
			len(snapshot.Metrics), snapshot.Metrics, len(wantOrder))
	}
	for i, id := range wantOrder {
		if snapshot.Metrics[i].ID != id {
			t.Errorf("metrics[%d].ID = %q, want %q (request order not preserved): got order %+v",
				i, snapshot.Metrics[i].ID, id, snapshot.Metrics)
		}
	}
}

// TestHandleSystemMetrics_GenuinelyUnknownMetricStillReportsUnknown pins the
// collector's fallback behaviour for an ID that is neither a known backend
// metric nor an agentctl diagnostic one: it must still come back as exactly
// one "unknown metric" sample, not zero (collectorMetricIDs must not filter
// out anything besides the diagnostic IDs) and not two (the diagnostic
// path must not also emit something for it).
func TestHandleSystemMetrics_GenuinelyUnknownMetricStillReportsUnknown(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics?metrics=nonsense")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		Metrics []metrics.MetricSample `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot %q: %v", rec.Body.String(), err)
	}
	if len(snapshot.Metrics) != 1 {
		t.Fatalf("got %d metrics %+v, want exactly 1", len(snapshot.Metrics), snapshot.Metrics)
	}
	got := snapshot.Metrics[0]
	if got.ID != "nonsense" || got.Available || got.Error != "unknown metric" {
		t.Errorf("sample = %+v, want {ID: nonsense, Available: false, Error: unknown metric}", got)
	}
}
