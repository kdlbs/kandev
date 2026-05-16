package summary_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/summary"
)

func TestBuildSummary_Sections(t *testing.T) {
	at := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		in       summary.BuildInputs
		contains []string
	}{
		{
			name: "empty inputs lands fallback content",
			in:   summary.BuildInputs{},
			contains: []string{
				"## Active focus",
				"Workspace coordination",
				"## Open blockers",
				"None.",
				"## Recent decisions",
				"None recorded.",
				"## Next action",
				"Continue monitoring.",
			},
		},
		{
			name: "all sections populated",
			in: summary.BuildInputs{
				ActiveFocus: "Driving the OAuth rollout to staging.",
				Activity: summary.ActivityStats{
					CompletedRuns: 3, FailedRuns: 1, OpenTasks: 4, InProgress: 2,
				},
				Blockers: []summary.BlockerInput{
					{Title: "Awaiting infra approval", Reason: "k8s namespace", SurfacedAt: at},
				},
				Decisions: []summary.DecisionInput{
					{Text: "Pivot to Postgres for staging", At: at},
				},
				NextAction: "Confirm namespace approval and unblock the rollout.",
			},
			contains: []string{
				"Driving the OAuth rollout to staging.",
				"3 completed run(s), 1 failed run(s), 2 in-progress, 4 open task(s).",
				"- Awaiting infra approval — k8s namespace (surfaced 2026-05-01)",
				"- [2026-05-01] Pivot to Postgres for staging",
				"Confirm namespace approval and unblock the rollout.",
			},
		},
		{
			name: "next-action falls back when blockers exist",
			in: summary.BuildInputs{
				Blockers: []summary.BlockerInput{{Title: "Stuck"}},
			},
			contains: []string{
				"Resolve open blockers before assigning new work.",
			},
		},
		{
			name: "next-action calls out failed runs",
			in: summary.BuildInputs{
				Activity: summary.ActivityStats{FailedRuns: 2},
			},
			contains: []string{
				"Triage recent failed runs and assign follow-ups.",
			},
		},
		{
			name: "previous summary feeds active focus when current is empty",
			in: summary.BuildInputs{
				PreviousSummary: "## Active focus\nMonitoring CI flakiness across PRs.\n",
			},
			contains: []string{
				"Monitoring CI flakiness across PRs.",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summary.BuildSummary(tc.in)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q\n--- got ---\n%s", want, got)
				}
			}
		})
	}
}

func TestBuildSummary_DecisionsSortedNewestFirstAndCappedAtFive(t *testing.T) {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	in := summary.BuildInputs{
		Decisions: []summary.DecisionInput{
			{Text: "old1", At: base},
			{Text: "newest", At: base.Add(7 * 24 * time.Hour)},
			{Text: "old2", At: base.Add(time.Hour)},
			{Text: "old3", At: base.Add(2 * time.Hour)},
			{Text: "old4", At: base.Add(3 * time.Hour)},
			{Text: "old5", At: base.Add(4 * time.Hour)},
			{Text: "should-be-dropped", At: base.Add(-time.Hour)},
		},
	}
	got := summary.BuildSummary(in)
	if !strings.Contains(got, "newest") {
		t.Errorf("newest decision should be present:\n%s", got)
	}
	if strings.Contains(got, "should-be-dropped") {
		t.Errorf("oldest decision should be capped out:\n%s", got)
	}
	idxNewest := strings.Index(got, "newest")
	idxOld5 := strings.Index(got, "old5")
	if idxNewest < 0 || idxOld5 < 0 || idxNewest >= idxOld5 {
		t.Errorf("decisions not sorted newest-first:\n%s", got)
	}
}

func TestBuildSummary_TruncatesAtCap(t *testing.T) {
	// Stuff active focus past the 8 KB cap and confirm we cut at the
	// boundary (the storage layer also caps; the builder caps as a
	// defensive duplicate).
	huge := strings.Repeat("x", 9000)
	got := summary.BuildSummary(summary.BuildInputs{ActiveFocus: huge})
	if len(got) > 8192 {
		t.Errorf("expected output capped at 8192 bytes, got %d", len(got))
	}
}

func TestBuildSummary_DeterministicForSameInputs(t *testing.T) {
	in := summary.BuildInputs{
		ActiveFocus: "Same as last time.",
		Blockers:    []summary.BlockerInput{{Title: "X"}},
	}
	first := summary.BuildSummary(in)
	second := summary.BuildSummary(in)
	if first != second {
		t.Errorf("BuildSummary should be deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestBuildSummary_FailedRunStatusInfersInvestigateAction(t *testing.T) {
	in := summary.BuildInputs{LastRunStatus: "failed"}
	got := summary.BuildSummary(in)
	if !strings.Contains(got, "Investigate the previous run failure") {
		t.Errorf("expected failure-driven next action, got:\n%s", got)
	}
}

func TestBuildSummary_OpenTasksDriveDispatchAction(t *testing.T) {
	in := summary.BuildInputs{
		Activity: summary.ActivityStats{OpenTasks: 5},
	}
	got := summary.BuildSummary(in)
	if !strings.Contains(got, "Review the open task queue") {
		t.Errorf("expected dispatch-next next action, got:\n%s", got)
	}
}
