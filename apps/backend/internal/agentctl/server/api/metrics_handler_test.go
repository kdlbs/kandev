package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

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
// user-visible-surface boundary: the three agentctl diagnostic metric IDs
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
		metrics.MetricAgentctlCreateReadyMs: true,
	}
	for _, sample := range snapshot.Metrics {
		if diagnostic[sample.ID] {
			t.Errorf("default snapshot unexpectedly includes diagnostic metric %q", sample.ID)
		}
	}
}

// TestHandleSystemMetrics_DiagnosticMetricsServedWhenRequested asserts the
// three diagnostic metrics are served, with the expected shape, only when
// explicitly named. newTestServer's config never goes through
// instance.Manager.CreateInstance and its process.Manager is never started,
// so agentctl_create_ready_ms and agentctl_git_poll_ms are exercised on their
// "not recorded yet" branches; agentctl_goroutines is always available.
func TestHandleSystemMetrics_DiagnosticMetricsServedWhenRequested(t *testing.T) {
	srv := newTestServer(t)

	rec := serverGet(t, srv, "/api/v1/system/metrics?metrics="+
		metrics.MetricAgentctlGoroutines+","+
		metrics.MetricAgentctlGitPollMillis+","+
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
	byID := make(map[string]metrics.MetricSample, len(snapshot.Metrics))
	for _, sample := range snapshot.Metrics {
		byID[sample.ID] = sample
	}

	goroutines, ok := byID[metrics.MetricAgentctlGoroutines]
	if !ok {
		t.Fatalf("missing %s in response %+v", metrics.MetricAgentctlGoroutines, snapshot.Metrics)
	}
	if !goroutines.Available || goroutines.Value == nil || *goroutines.Value <= 0 {
		t.Errorf("goroutines sample = %+v, want available with a positive value", goroutines)
	}

	gitPoll, ok := byID[metrics.MetricAgentctlGitPollMillis]
	if !ok {
		t.Fatalf("missing %s in response %+v", metrics.MetricAgentctlGitPollMillis, snapshot.Metrics)
	}
	if gitPoll.Available || gitPoll.Error == "" {
		t.Errorf("git poll sample = %+v, want unavailable with an error (no tracker started)", gitPoll)
	}

	createReady, ok := byID[metrics.MetricAgentctlCreateReadyMs]
	if !ok {
		t.Fatalf("missing %s in response %+v", metrics.MetricAgentctlCreateReadyMs, snapshot.Metrics)
	}
	if createReady.Available || createReady.Error == "" {
		t.Errorf("create-ready sample = %+v, want unavailable with an error (nil CreateReadyMillis)", createReady)
	}
}
