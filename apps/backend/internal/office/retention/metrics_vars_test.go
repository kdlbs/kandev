package retention

import (
	"errors"
	"expvar"
	"strconv"
	"strings"
	"testing"
)

// readCounter walks the expvar map looking for a key that matches the
// supplied prefix. Returns 0 when no key matches. The prefix match keeps
// the assertion robust against process-wide test pollution.
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
		{"single_pair", []string{"table", "runs"}, "table=runs"},
		{"odd_args_returns_empty", []string{"table"}, ""},
		{"two_pairs", []string{"table", "runs", "outcome", "completed"}, "table=runs;outcome=completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metricLabel(tc.pairs...); got != tc.want {
				t.Errorf("metricLabel(%v) = %q, want %q", tc.pairs, got, tc.want)
			}
		})
	}
}

func TestIncSweepCompletedAndSkipped(t *testing.T) {
	beforeCompleted := readCounter(t, retentionSweepTotal, metricLabel("outcome", "completed"))
	incSweepCompleted()
	afterCompleted := readCounter(t, retentionSweepTotal, metricLabel("outcome", "completed"))
	if afterCompleted-beforeCompleted != 1 {
		t.Errorf("sweep completed delta = %d, want 1", afterCompleted-beforeCompleted)
	}

	beforeSkipped := readCounter(t, retentionSweepTotal, metricLabel("outcome", "skipped"))
	incSweepSkipped()
	afterSkipped := readCounter(t, retentionSweepTotal, metricLabel("outcome", "skipped"))
	if afterSkipped-beforeSkipped != 1 {
		t.Errorf("sweep skipped delta = %d, want 1", afterSkipped-beforeSkipped)
	}
}

func TestIncDeleted_ZeroIsNotRecorded(t *testing.T) {
	label := metricLabel("table", "test_table_zero")
	before := readCounter(t, retentionDeletedTotal, label)
	incDeleted("test_table_zero", 0)
	after := readCounter(t, retentionDeletedTotal, label)
	if after != before {
		t.Errorf("delta = %d, want 0 (a zero delete must not create a counter entry)", after-before)
	}
}

func TestIncDeleted_PositiveIsRecorded(t *testing.T) {
	label := metricLabel("table", "test_table_positive")
	before := readCounter(t, retentionDeletedTotal, label)
	incDeleted("test_table_positive", 7)
	after := readCounter(t, retentionDeletedTotal, label)
	if after-before != 7 {
		t.Errorf("delta = %d, want 7", after-before)
	}
}

func TestIncCensus_NilErrorIsFresh(t *testing.T) {
	label := metricLabel("table", "test_census_fresh", "outcome", "fresh")
	before := readCounter(t, retentionCensusTotal, label)
	incCensus("test_census_fresh", nil)
	after := readCounter(t, retentionCensusTotal, label)
	if after-before != 1 {
		t.Errorf("fresh delta = %d, want 1", after-before)
	}
}

func TestIncCensus_ErrorIsStale(t *testing.T) {
	label := metricLabel("table", "test_census_stale", "outcome", "stale")
	before := readCounter(t, retentionCensusTotal, label)
	incCensus("test_census_stale", errors.New("boom"))
	after := readCounter(t, retentionCensusTotal, label)
	if after-before != 1 {
		t.Errorf("stale delta = %d, want 1", after-before)
	}
}

func TestExpvarMapsPublishedAtKnownNames(t *testing.T) {
	expected := []string{
		"office_retention_sweep_total",
		"office_retention_deleted_total",
		"office_retention_census_total",
	}
	for _, name := range expected {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q not published — /debug/vars consumers will miss it", name)
		}
	}
}
