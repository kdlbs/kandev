package docknet

import (
	"expvar"
	"strings"
	"sync"
	"time"
)

// expvar maps published at package init, exposed via stdlib's /debug/vars
// handler in dev mode. Counters only, "k1=v1;k2=v2" label keys so a future
// Prometheus translation can split without re-shaping storage, mirroring the
// idiom in internal/office/scheduler/metrics_vars.go.
var (
	networksClassifiedTotal = expvar.NewMap("docknet_networks_classified_total")
	networksRemovedTotal    = expvar.NewMap("docknet_networks_removed_total")
	probeRunsTotal          = expvar.NewMap("docknet_probe_runs_total")
)

var lastRunTimestamp = expvar.NewString("docknet_last_run_unix")

// metricsMu guards the lastRun string under concurrent provider runs; expvar
// types are individually safe but the read-modify sequence here is not.
var metricsMu sync.Mutex

func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func recordClassification(class Classification) {
	networksClassifiedTotal.Add(string(class), 1)
}

func recordRemoval(outcome string) {
	networksRemovedTotal.Add(metricLabel("outcome", outcome), 1)
}

func recordProbe(passed bool) {
	probeRunsTotal.Add(metricLabel("passed", boolLabel(passed)), 1)
}

func recordRunTimestamp(at time.Time) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	lastRunTimestamp.Set(strings.TrimSpace(at.UTC().Format(time.RFC3339)))
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// classifiedCounts summarizes a census for run results and metrics.
func classifiedCounts(items []ClassifiedNetwork) map[string]int {
	counts := make(map[string]int, 6)
	for _, item := range items {
		counts[string(item.Class)]++
		recordClassification(item.Class)
	}
	return counts
}
