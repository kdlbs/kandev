package envmetrics

import (
	"expvar"
	"strconv"
	"strings"
	"testing"
)

// readCounter walks the expvar map looking for a key that matches the
// supplied prefix. Returns 0 when no key matches. The prefix match keeps the
// assertion robust against process-wide test pollution (other tests in this
// package may push entries with different labels).
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
		{"single_pair", []string{"winning_origin", "executor profile"}, "winning_origin=executor profile"},
		{"two_pairs", []string{"winning_origin", "executor profile", "losing_origin", "agent profile"},
			"winning_origin=executor profile;losing_origin=agent profile"},
		{"odd_args_returns_empty", []string{"winning_origin"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metricLabel(tc.pairs...); got != tc.want {
				t.Errorf("metricLabel(%v) = %q, want %q", tc.pairs, got, tc.want)
			}
		})
	}
}

func TestRecordOverrideApplied(t *testing.T) {
	label := metricLabel("winning_origin", "executor profile", "losing_origin", "agent profile-metrics-test")

	before := readCounter(t, environmentOverrideApplied, label)
	RecordOverrideApplied("executor profile", "agent profile-metrics-test")
	after := readCounter(t, environmentOverrideApplied, label)

	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}
