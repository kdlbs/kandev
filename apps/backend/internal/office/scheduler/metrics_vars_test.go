package scheduler

import (
	"expvar"
	"strconv"
	"strings"
	"testing"
)

// readCounter walks the expvar map looking for a key that matches the
// supplied prefix. Returns 0 when no key matches. The prefix match
// keeps the assertion robust against process-wide test pollution
// (other tests in the package may push entries with different
// workspace IDs).
func readCounter(t *testing.T, m *expvar.Map, prefix string) int64 {
	t.Helper()
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if !strings.HasPrefix(kv.Key, prefix) {
			return
		}
		n, err := strconv.ParseInt(kv.Value.String(), 10, 64)
		if err != nil {
			t.Fatalf("counter %q value not int: %s", kv.Key, kv.Value.String())
		}
		total += n
	})
	return total
}

func TestMetricLabel(t *testing.T) {
	cases := []struct {
		name  string
		pairs []string
		want  string
	}{
		{"single_pair", []string{"workspace", "ws-1"}, "workspace=ws-1"},
		{"empty_workspace", []string{"workspace", "", "provider", "claude"},
			"workspace=;provider=claude"},
		{"odd_args_returns_empty", []string{"workspace"}, ""},
		{"three_pairs",
			[]string{"workspace", "w", "provider", "p", "outcome", "success"},
			"workspace=w;provider=p;outcome=success"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metricLabel(tc.pairs...); got != tc.want {
				t.Errorf("metricLabel(%v) = %q, want %q", tc.pairs, got, tc.want)
			}
		})
	}
}

func TestIncrementersRecordOnExpvarMaps(t *testing.T) {
	const ws = "ws-metrics-test"

	before := readCounter(t, routingRouteAttemptsTotal, metricLabel(
		"workspace", ws, "provider", "claude-acp", "outcome", "success"))
	incRouteAttempt(ws, "claude-acp", "success")
	after := readCounter(t, routingRouteAttemptsTotal, metricLabel(
		"workspace", ws, "provider", "claude-acp", "outcome", "success"))
	if after-before != 1 {
		t.Errorf("route_attempt counter delta = %d, want 1", after-before)
	}

	beforeFB := readCounter(t, routingFallbackTotal, metricLabel(
		"workspace", ws, "from", "claude-acp", "to", "codex-acp", "code", ""))
	incRouteFallback(ws, "claude-acp", "codex-acp", "")
	afterFB := readCounter(t, routingFallbackTotal, metricLabel(
		"workspace", ws, "from", "claude-acp", "to", "codex-acp", "code", ""))
	if afterFB-beforeFB != 1 {
		t.Errorf("route_fallback counter delta = %d, want 1", afterFB-beforeFB)
	}

	beforePark := readCounter(t, routingParkedRunsTotal, metricLabel(
		"workspace", ws, "status", "waiting_for_provider_capacity"))
	incRouteParked(ws, "waiting_for_provider_capacity")
	afterPark := readCounter(t, routingParkedRunsTotal, metricLabel(
		"workspace", ws, "status", "waiting_for_provider_capacity"))
	if afterPark-beforePark != 1 {
		t.Errorf("parked counter delta = %d, want 1", afterPark-beforePark)
	}

	beforeDeg := readCounter(t, routingProviderDegraded, metricLabel(
		"workspace", ws, "provider", "claude-acp", "code", "quota_exceeded"))
	incProviderDegraded(ws, "claude-acp", "quota_exceeded")
	afterDeg := readCounter(t, routingProviderDegraded, metricLabel(
		"workspace", ws, "provider", "claude-acp", "code", "quota_exceeded"))
	if afterDeg-beforeDeg != 1 {
		t.Errorf("provider_degraded counter delta = %d, want 1", afterDeg-beforeDeg)
	}

	beforeRec := readCounter(t, routingProviderRecovered, metricLabel(
		"workspace", ws, "provider", "claude-acp"))
	incProviderRecovered(ws, "claude-acp")
	afterRec := readCounter(t, routingProviderRecovered, metricLabel(
		"workspace", ws, "provider", "claude-acp"))
	if afterRec-beforeRec != 1 {
		t.Errorf("provider_recovered counter delta = %d, want 1", afterRec-beforeRec)
	}
}

func TestExpvarMapsPublishedAtKnownNames(t *testing.T) {
	expected := []string{
		"routing_fallback_total",
		"routing_route_attempts_total",
		"routing_provider_degraded_total",
		"routing_provider_recovered_total",
		"routing_parked_runs_total",
	}
	for _, name := range expected {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q not published — /debug/vars consumers will miss it", name)
		}
	}
}
