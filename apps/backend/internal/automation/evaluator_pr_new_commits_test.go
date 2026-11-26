package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// followUpPR is one open pull request served by followUpTransport.
type followUpPR struct {
	Number    int
	Head      string
	Base      string
	Author    string
	Draft     bool
	Labels    []string
	Reviewers []string
	Teams     []string
	// CreatedAt is omitted from the listing when zero.
	CreatedAt time.Time
}

// followUpTransport serves GET /repos/{owner}/{repo}/pulls from a mutable
// in-memory list. With partial set, it advertises a second page and fails it.
type followUpTransport struct {
	t        *testing.T
	mu       sync.Mutex
	prs      map[string][]followUpPR
	partial  bool
	listings int
	// ended maps a PR number to the state GET /pulls/{n} reports
	// ("merged" or "closed"); an absent number fails the read.
	ended map[int]string
}

func (r *followUpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	header := make(http.Header)
	if len(parts) == 5 {
		return r.singlePR(req, parts[4], header), nil
	}
	require.Len(r.t, parts, 4, "path = %q", req.URL.Path)
	require.Equal(r.t, "pulls", parts[3])
	if req.URL.Query().Get("page") == "2" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: header,
			Body: io.NopCloser(strings.NewReader(`{"message":"boom"}`)), Request: req}, nil
	}
	r.listings++
	if r.partial {
		header.Set("Link", `<https://api.github.com/repos/`+parts[1]+"/"+parts[2]+`/pulls?state=open&per_page=100&page=2>; rel="next"`)
	}
	items := make([]map[string]any, 0)
	for _, pr := range r.prs[parts[1]+"/"+parts[2]] {
		items = append(items, followUpPRJSON(parts[1], parts[2], pr))
	}
	body, err := json.Marshal(items)
	require.NoError(r.t, err)
	return &http.Response{StatusCode: http.StatusOK, Header: header,
		Body: io.NopCloser(strings.NewReader(string(body))), Request: req}, nil
}

func (r *followUpTransport) singlePR(req *http.Request, number string, header http.Header) *http.Response {
	var n int
	_, _ = fmt.Sscanf(number, "%d", &n)
	state, ok := r.ended[n]
	if !ok {
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: header,
			Body: io.NopCloser(strings.NewReader(`{"message":"boom"}`)), Request: req}
	}
	mergedAt := "null"
	if state == "merged" {
		mergedAt = `"2026-11-24T10:00:00Z"`
	}
	body := fmt.Sprintf(`{"number":%d,"state":"closed","merged_at":%s,"head":{"sha":"x"},"base":{"ref":"main"},"user":{"login":"alice"}}`, n, mergedAt)
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func followUpPRJSON(owner, repo string, pr followUpPR) map[string]any {
	base := pr.Base
	if base == "" {
		base = "main"
	}
	author := pr.Author
	if author == "" {
		author = "alice"
	}
	labels := make([]map[string]string, 0, len(pr.Labels))
	for _, l := range pr.Labels {
		labels = append(labels, map[string]string{"name": l})
	}
	reviewers := make([]map[string]string, 0, len(pr.Reviewers))
	for _, login := range pr.Reviewers {
		reviewers = append(reviewers, map[string]string{"login": login})
	}
	teams := make([]map[string]string, 0, len(pr.Teams))
	for _, slug := range pr.Teams {
		teams = append(teams, map[string]string{"slug": slug})
	}
	item := map[string]any{
		"number": pr.Number, "title": fmt.Sprintf("PR %d", pr.Number), "state": "open", "draft": pr.Draft,
		"html_url": fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, pr.Number),
		"head":     map[string]string{"ref": "feature", "sha": pr.Head},
		"base":     map[string]string{"ref": base}, "user": map[string]string{"login": author},
		"labels": labels, "requested_reviewers": reviewers, "requested_teams": teams,
	}
	if !pr.CreatedAt.IsZero() {
		item["created_at"] = pr.CreatedAt.UTC().Format(time.RFC3339)
	}
	return item
}

type prFollowUpFixture struct {
	prTriggerFixtureBase
	transport *followUpTransport
	clock     time.Time
}

func newPRFollowUpFixture(t *testing.T, prs ...followUpPR) *prFollowUpFixture {
	t.Helper()
	transport := &followUpTransport{t: t, prs: map[string][]followUpPR{"acme/widget": prs}}
	f := &prFollowUpFixture{prTriggerFixtureBase: newPRTriggerFixtureBase(t, transport), transport: transport,
		clock: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)}
	f.evaluator.now = func() time.Time { return f.clock }
	unlimited := 0
	_, err := f.svc.UpdateAutomation(context.Background(), f.automation.ID, &UpdateAutomationRequest{MaxConcurrentRuns: &unlimited})
	require.NoError(t, err)
	return f
}

func (f *prFollowUpFixture) addTrigger(t *testing.T, extra string) *AutomationTrigger {
	t.Helper()
	trigger, err := f.svc.AddTrigger(context.Background(), &AddTriggerRequest{
		AutomationID: f.automation.ID, Type: TriggerTypeGitHubPR, Enabled: true,
		Config: json.RawMessage(`{` + eventSelectionRepo + extra + `}`),
	})
	require.NoError(t, err)
	return trigger
}

func (f *prFollowUpFixture) setPRs(prs ...followUpPR) {
	f.transport.mu.Lock()
	defer f.transport.mu.Unlock()
	f.transport.prs["acme/widget"] = prs
}

func (f *prFollowUpFixture) evaluate() { f.evaluator.evaluatePRTriggers(context.Background()) }

// runs returns admitted and skipped runs, oldest first.
func (f *prFollowUpFixture) runs(t *testing.T) []*AutomationRun {
	t.Helper()
	runs, err := f.svc.Store().ListRuns(context.Background(), f.automation.ID, 100)
	require.NoError(t, err)
	out := make([]*AutomationRun, 0, len(runs))
	for i := len(runs) - 1; i >= 0; i-- {
		out = append(out, runs[i])
	}
	return out
}

func (f *prFollowUpFixture) observation(t *testing.T, trigger *AutomationTrigger, number int) *PRObservation {
	t.Helper()
	observations, err := f.svc.Store().ListPRObservations(context.Background(), trigger.ID, "acme", "widget")
	require.NoError(t, err)
	return observations[number]
}

func runData(t *testing.T, run *AutomationRun) map[string]any {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal(run.TriggerData, &data))
	return data
}

const newCommitsEvents = `,"events":["new_commits"]`

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.1
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.2
func TestNewCommitsBaselineDoesNotFire(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, newCommitsEvents)

	f.evaluate()

	require.Equal(t, 1, f.transport.listings, "new_commits alone must poll")
	require.Empty(t, f.runs(t))
	obs := f.observation(t, trigger, 17)
	require.NotNil(t, obs)
	require.Equal(t, "aaa", obs.HeadSHA)

	// A restarted evaluator reads the stored baseline instead of re-baselining.
	f.evaluator = NewGitHubEvaluator(f.svc, f.evaluator.githubService, f.svc.logger)
	f.evaluate()
	require.Empty(t, f.runs(t))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.3
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.3
func TestNewCommitsFiresOncePerHead(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, newCommitsEvents)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.evaluate()
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 1, "unchanged head must neither fire nor record a skip")
	require.Equal(t, "pr:acme/widget#17@bbb", runs[0].DedupKey)
	data := runData(t, runs[0])
	require.Equal(t, PREventNewCommits, data["event"])
	require.Equal(t, "bbb", data["head_sha"])
	require.Equal(t, "aaa", data["previous_head_sha"])
	require.Equal(t, "acme/widget#17", data["pr_subject"])
	require.Equal(t, "bbb", f.observation(t, trigger, 17).HeadSHA)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.4
func TestNewCommitsCoalescesToLatestHead(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, newCommitsEvents)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.setPRs(followUpPR{Number: 17, Head: "ccc"})
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 1)
	require.Equal(t, "ccc", runData(t, runs[0])["head_sha"])
	require.Equal(t, "aaa", runData(t, runs[0])["previous_head_sha"])
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.5
func TestNewCommitsQuietPeriod(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	f.addTrigger(t, newCommitsEvents+`,"quiet_period_minutes":10`)
	f.evaluate()
	start := f.clock

	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.evaluate()
	f.clock = start.Add(5 * time.Minute)
	f.evaluate()
	require.Empty(t, f.runs(t), "head has not settled")

	f.clock = start.Add(6 * time.Minute)
	f.setPRs(followUpPR{Number: 17, Head: "ccc"})
	f.evaluate()
	f.clock = start.Add(15 * time.Minute)
	f.evaluate()
	require.Empty(t, f.runs(t), "a newer head restarts the wait")

	// The pending head survives an evaluator restart.
	f.evaluator = NewGitHubEvaluator(f.svc, f.evaluator.githubService, f.svc.logger)
	f.evaluator.now = func() time.Time { return start.Add(16 * time.Minute) }
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 1)
	require.Equal(t, "ccc", runData(t, runs[0])["head_sha"])
	require.Equal(t, "aaa", runData(t, runs[0])["previous_head_sha"])
}

func TestQuietPeriodValidation(t *testing.T) {
	f := newPRFollowUpFixture(t)
	for _, value := range []string{"-1", "1441", `"ten"`} {
		_, err := f.svc.AddTrigger(context.Background(), &AddTriggerRequest{
			AutomationID: f.automation.ID, Type: TriggerTypeGitHubPR, Enabled: true,
			Config: json.RawMessage(`{"events":["new_commits"],` + eventSelectionRepo + `,"quiet_period_minutes":` + value + `}`),
		})
		require.Error(t, err, "quiet_period_minutes=%s", value)
	}
	f.addTrigger(t, newCommitsEvents+`,"quiet_period_minutes":1440`)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.6
func TestNewCommitsOutcomeAdvancesHead(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"}, followUpPR{Number: 18, Head: "aaa"})
	trigger := f.addTrigger(t, newCommitsEvents)
	f.evaluate()
	one := 1
	_, err := f.svc.UpdateAutomation(context.Background(), f.automation.ID, &UpdateAutomationRequest{MaxConcurrentRuns: &one})
	require.NoError(t, err)

	f.setPRs(followUpPR{Number: 17, Head: "bbb"}, followUpPR{Number: 18, Head: "bbb"})
	f.evaluate()

	runs := f.runs(t)
	require.Len(t, runs, 2)
	require.Equal(t, RunStatusTriggered, runs[0].Status)
	require.Equal(t, RunStatusSkipped, runs[1].Status, "the concurrency cap skips the second PR")
	require.Equal(t, "bbb", f.observation(t, trigger, 17).HeadSHA, "admitted")
	require.Equal(t, "bbb", f.observation(t, trigger, 18).HeadSHA, "capacity skipped")

	// A duplicate firing also counts as processed and records no new run.
	obs := f.observation(t, trigger, 17)
	obs.HeadSHA = "aaa"
	require.NoError(t, f.svc.Store().UpsertPRObservation(context.Background(), obs))
	f.evaluate()
	require.Len(t, f.runs(t), 2)
	require.Equal(t, "bbb", f.observation(t, trigger, 17).HeadSHA, "duplicate")
}

func TestFiringAdvancesObservationOutcomeRule(t *testing.T) {
	require.True(t, firingAdvancesObservation(FireResult{RunID: "run"}, nil), "admitted")
	require.True(t, firingAdvancesObservation(FireResult{Skipped: true, Duplicate: true}, nil), "duplicate")
	require.True(t, firingAdvancesObservation(FireResult{Skipped: true, Reason: "cap"}, nil), "capacity")
	require.False(t, firingAdvancesObservation(FireResult{Skipped: true, Deferred: true}, nil), "deferred")
	require.False(t, firingAdvancesObservation(FireResult{}, fmt.Errorf("boom")), "error")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.7
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.10
func TestOpenedAndNewCommitsFireOncePerPoll(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, `,"events":["opened","new_commits"]`)

	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 1)
	require.Equal(t, "pr:acme/widget#17", runs[0].DedupKey)
	require.Equal(t, PREventOpened, runData(t, runs[0])["event"])
	require.Equal(t, "", runData(t, runs[0])["previous_head_sha"])
	require.Equal(t, "aaa", f.observation(t, trigger, 17).HeadSHA)

	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.evaluate()
	runs = f.runs(t)
	require.Len(t, runs, 2, "opened stays deduplicated and new_commits fires")
	require.Equal(t, "pr:acme/widget#17@bbb", runs[1].DedupKey)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.8
func TestNewCommitsFilteredPRAdvancesSilently(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, newCommitsEvents+`,"authors":["bob"]`)
	f.evaluate()

	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.evaluate()
	require.Empty(t, f.runs(t))
	require.Equal(t, "bbb", f.observation(t, trigger, 17).HeadSHA)

	cfg := json.RawMessage(`{` + eventSelectionRepo + newCommitsEvents + `,"authors":["alice"]}`)
	require.NoError(t, f.svc.UpdateTrigger(context.Background(), trigger.ID, &UpdateTriggerRequest{Config: &cfg}))
	f.evaluate()
	require.Empty(t, f.runs(t), "heads pushed while filtered out do not fire later")

	f.setPRs(followUpPR{Number: 17, Head: "ccc"})
	f.evaluate()
	require.Len(t, f.runs(t), 1)
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.9
func TestNewCommitsPrunesClosedPRsOnlyOnCompleteListing(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"}, followUpPR{Number: 18, Head: "aaa"})
	trigger := f.addTrigger(t, newCommitsEvents)
	f.evaluate()

	f.transport.partial = true
	f.setPRs(followUpPR{Number: 18, Head: "aaa"})
	f.evaluate()
	require.NotNil(t, f.observation(t, trigger, 17), "a partial listing never prunes")

	f.transport.partial = false
	f.evaluate()
	require.Nil(t, f.observation(t, trigger, 17), "closed PR observation is pruned")
	require.NotNil(t, f.observation(t, trigger, 18))

	f.setPRs(followUpPR{Number: 17, Head: "zzz"}, followUpPR{Number: 18, Head: "aaa"})
	f.evaluate()
	require.Empty(t, f.runs(t), "a reopened PR starts from a new baseline")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.1
func TestPRFiltersApplyBaseBranchLabelsDraftAuthor(t *testing.T) {
	cases := []struct {
		name, extra string
		pr          followUpPR
		firings     int
	}{
		{"label missing", `,"labels":["needs-review"]`, followUpPR{Number: 17, Head: "a"}, 0},
		{"label present", `,"labels":["needs-review","other"]`, followUpPR{Number: 17, Head: "a", Labels: []string{"needs-review"}}, 1},
		{"base branch match", `,"branches":["main"]`, followUpPR{Number: 17, Head: "a"}, 1},
		{"base branch glob", `,"branches":["release/*"]`, followUpPR{Number: 17, Head: "a", Base: "release/2"}, 1},
		{"base branch mismatch", `,"branches":["release/*"]`, followUpPR{Number: 17, Head: "a"}, 0},
		{"draft excluded", `,"exclude_draft":true`, followUpPR{Number: 17, Head: "a", Draft: true}, 0},
		{"author mismatch", `,"authors":["bob"]`, followUpPR{Number: 17, Head: "a"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPRFollowUpFixture(t, tc.pr)
			f.addTrigger(t, `,"events":["opened"]`+tc.extra)
			f.evaluate()
			require.Len(t, f.runs(t), tc.firings)
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001.2
func TestNewCommitsAddedLaterStartsFromBaseline(t *testing.T) {
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "aaa"})
	trigger := f.addTrigger(t, reviewRequestedEvents)
	f.evaluate()
	f.setPRs(followUpPR{Number: 17, Head: "bbb"})
	f.evaluate()

	cfg := json.RawMessage(`{` + eventSelectionRepo + `,"events":["review_requested","new_commits"]}`)
	require.NoError(t, f.svc.UpdateTrigger(context.Background(), trigger.ID, &UpdateTriggerRequest{Config: &cfg}))
	f.evaluate()
	f.evaluate()
	require.Empty(t, f.runs(t), "a push made before new_commits was selected does not fire")

	f.setPRs(followUpPR{Number: 17, Head: "ccc"})
	f.evaluate()
	runs := f.runs(t)
	require.Len(t, runs, 1)
	require.Equal(t, "bbb", runData(t, runs[0])["previous_head_sha"])
}
