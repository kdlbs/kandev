package github

import (
	"expvar"
	"strconv"
	"strings"
	"testing"
)

// readOutcomeCounter walks the expvar map looking for a key that matches the
// supplied prefix. Returns 0 when no key matches. The prefix match keeps the
// assertion robust against process-wide test pollution (other tests in the
// package may push entries with the same label under -count=1 reruns).
func readOutcomeCounter(t *testing.T, m *expvar.Map, prefix string) int64 {
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

func TestOutcomeMetricLabel(t *testing.T) {
	cases := []struct {
		name  string
		pairs []string
		want  string
	}{
		{"single_pair", []string{"populated", "true"}, "populated=true"},
		{"two_pairs", []string{"action", "set", "outcome", "unknown"}, "action=set;outcome=unknown"},
		{"odd_args_returns_empty", []string{"populated"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := outcomeMetricLabel(tc.pairs...); got != tc.want {
				t.Errorf("outcomeMetricLabel(%v) = %q, want %q", tc.pairs, got, tc.want)
			}
		})
	}
}

func TestBoolLabel(t *testing.T) {
	if got := boolLabel(true); got != "true" {
		t.Errorf("boolLabel(true) = %q, want true", got)
	}
	if got := boolLabel(false); got != "false" {
		t.Errorf("boolLabel(false) = %q, want false", got)
	}
}

// TestIncTaskPROutcomeSyncRecordsOnExpvarMap covers AC-38: a sync's populated
// state is recorded on the outcome-syncs expvar map under the label
// incTaskPROutcomeSync builds, so the "did the writer stop populating
// outcome fields" canary has raw material to read.
func TestIncTaskPROutcomeSyncRecordsOnExpvarMap(t *testing.T) {
	labelTrue := outcomeMetricLabel("populated", boolLabel(true))
	beforeTrue := readOutcomeCounter(t, taskPROutcomeSyncsTotal, labelTrue)
	incTaskPROutcomeSync(true)
	afterTrue := readOutcomeCounter(t, taskPROutcomeSyncsTotal, labelTrue)
	if afterTrue-beforeTrue != 1 {
		t.Errorf("populated=true counter delta = %d, want 1", afterTrue-beforeTrue)
	}

	labelFalse := outcomeMetricLabel("populated", boolLabel(false))
	beforeFalse := readOutcomeCounter(t, taskPROutcomeSyncsTotal, labelFalse)
	incTaskPROutcomeSync(false)
	afterFalse := readOutcomeCounter(t, taskPROutcomeSyncsTotal, labelFalse)
	if afterFalse-beforeFalse != 1 {
		t.Errorf("populated=false counter delta = %d, want 1", afterFalse-beforeFalse)
	}
}

// TestIncTaskPROutcomeDispositionRecordsOnExpvarMap covers AC-38: a
// disposition write's action ("set" or "clear") is recorded on the
// dispositions expvar map, distinguishing the two per the doc comment on
// incTaskPROutcomeDisposition.
func TestIncTaskPROutcomeDispositionRecordsOnExpvarMap(t *testing.T) {
	labelSet := outcomeMetricLabel("action", "set")
	beforeSet := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, labelSet)
	incTaskPROutcomeDisposition("set")
	afterSet := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, labelSet)
	if afterSet-beforeSet != 1 {
		t.Errorf("action=set counter delta = %d, want 1", afterSet-beforeSet)
	}

	labelClear := outcomeMetricLabel("action", "clear")
	beforeClear := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, labelClear)
	incTaskPROutcomeDisposition("clear")
	afterClear := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, labelClear)
	if afterClear-beforeClear != 1 {
		t.Errorf("action=clear counter delta = %d, want 1", afterClear-beforeClear)
	}
}

// TestOutcomeExpvarMapsPublishedAtKnownNames guards the /debug/vars names
// AC-38 promises: a rename here silently breaks any external dashboard
// scraping the names directly.
func TestOutcomeExpvarMapsPublishedAtKnownNames(t *testing.T) {
	expected := []string{
		"github_task_pr_outcome_syncs_total",
		"github_task_pr_outcome_dispositions_total",
	}
	for _, name := range expected {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q not published — /debug/vars consumers will miss it", name)
		}
	}
}
