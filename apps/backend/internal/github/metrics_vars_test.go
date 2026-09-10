package github

import (
	"context"
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

func TestPRWatchCardinalityCountsCurrentCanonicalState(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`CREATE TABLE task_repositories (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, repository_id TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create task_repositories: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO task_repositories (id, task_id, repository_id)
		VALUES ('binding-active', 'task-active', 'repo-attached')`); err != nil {
		t.Fatalf("seed task_repositories: %v", err)
	}
	seedTask(t, store, "task-active", false)
	seedTask(t, store, "task-archived", true)

	watches := []*PRWatch{
		withTestWorkspace(&PRWatch{ID: "watch-searching", TaskID: "task-active", Branch: "feature/a"}),
		withTestWorkspace(&PRWatch{ID: "watch-discovered", TaskID: "task-active", Branch: "feature/b", PRNumber: 42}),
		withTestWorkspace(&PRWatch{ID: "watch-archived", TaskID: "task-archived", Branch: "feature/c"}),
		withTestWorkspace(&PRWatch{ID: "watch-orphan", TaskID: "task-missing", Branch: "feature/d"}),
		withTestWorkspace(&PRWatch{ID: "watch-detached", TaskID: "task-active", RepositoryID: "repo-detached", Branch: "feature/e"}),
		withTestWorkspace(&PRWatch{ID: "watch-attached", TaskID: "task-active", RepositoryID: "repo-attached", Branch: "feature/f"}),
	}
	for _, watch := range watches {
		watch.Owner = "owner"
		watch.Repo = "repo"
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("CreatePRWatch(%s): %v", watch.ID, err)
		}
	}

	got, err := store.PRWatchCardinality(ctx)
	if err != nil {
		t.Fatalf("PRWatchCardinality: %v", err)
	}
	want := (PRWatchCardinality{Active: 4, Searching: 3, Duplicates: 0, Orphans: 2})
	if got != want {
		t.Fatalf("PRWatchCardinality = %+v, want %+v", got, want)
	}
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

// TestOutcomeExpvarMapsPublishedAtKnownNames guards the /debug/vars names
// AC-38 promises: a rename here silently breaks any external dashboard
// scraping the names directly.
func TestOutcomeExpvarMapsPublishedAtKnownNames(t *testing.T) {
	expected := []string{
		"github_task_pr_outcome_syncs_total",
		"github_pr_watch_active",
		"github_pr_watch_searching",
		"github_pr_watch_duplicates",
		"github_pr_watch_orphans",
		"github_pr_watch_canonical_poll_requests_total",
	}
	for _, name := range expected {
		if expvar.Get(name) == nil {
			t.Errorf("expvar %q not published — /debug/vars consumers will miss it", name)
		}
	}
}

func TestPRWatchOperationalMetricsRecordAggregateValues(t *testing.T) {
	recordPRWatchCardinality(PRWatchCardinality{Active: 7, Searching: 3, Duplicates: 1, Orphans: 2})
	if got := prWatchActive.Value(); got != 7 {
		t.Fatalf("active watch gauge = %d, want 7", got)
	}
	if got := prWatchSearching.Value(); got != 3 {
		t.Fatalf("searching watch gauge = %d, want 3", got)
	}
	if got := prWatchDuplicates.Value(); got != 1 {
		t.Fatalf("duplicate watch gauge = %d, want 1", got)
	}
	if got := prWatchOrphans.Value(); got != 2 {
		t.Fatalf("orphan watch gauge = %d, want 2", got)
	}

	before := canonicalPollRequests.Value()
	incCanonicalPollRequests(4)
	if got := canonicalPollRequests.Value() - before; got != 4 {
		t.Fatalf("canonical poll request delta = %d, want 4", got)
	}
}
