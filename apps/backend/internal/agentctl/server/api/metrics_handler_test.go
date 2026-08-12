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
