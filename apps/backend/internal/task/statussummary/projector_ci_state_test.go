package statussummary

import (
	"encoding/json"
	"testing"
)

type ciEventFixture struct {
	State                    string
	HeadSHA                  string
	ChecksState              string
	ChecksTotal              int
	ChecksPassing            int
	MergeableState           string
	ReviewState              string
	WorkflowAttentionState   string
	WorkflowAttentionHeadSHA string
	WorkflowAttentionStale   bool
}

func ciEvent(number int, fixture ciEventFixture) map[string]interface{} {
	event := map[string]interface{}{
		"repository_id":       "repo-1",
		"owner":               "acme",
		"repo":                "widget",
		"pr_number":           number,
		"state":               fixture.State,
		"head_sha":            fixture.HeadSHA,
		"checks_state":        fixture.ChecksState,
		"checks_total":        fixture.ChecksTotal,
		"checks_passing":      fixture.ChecksPassing,
		"mergeable_state":     fixture.MergeableState,
		"review_state":        fixture.ReviewState,
		"has_merge_conflicts": false,
	}
	if fixture.WorkflowAttentionState != "" || fixture.WorkflowAttentionHeadSHA != "" {
		event["workflow_attention"] = map[string]interface{}{
			"state":    fixture.WorkflowAttentionState,
			"head_sha": fixture.WorkflowAttentionHeadSHA,
			"stale":    fixture.WorkflowAttentionStale,
		}
	}
	return event
}

func summarizeCIEvent(t *testing.T, fixtures ...map[string]interface{}) *PullRequestSummary {
	t.Helper()
	state := newProjectionState()
	projector := &Projector{}
	for _, event := range fixtures {
		if !projector.applyPREventLocked(state, event) {
			t.Fatalf("PR event was not applied: %#v", event)
		}
	}
	return derivePullRequestSummary(state)
}

func TestCIStateProjectionMatrix(t *testing.T) {
	tests := []struct {
		name    string
		fixture ciEventFixture
		want    string
	}{
		{
			name:    "explicit failure",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "failure"},
			want:    prCIStateFailed,
		},
		{
			name:    "approval without started checks",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-a"},
			want:    prCIStateApprovalRequired,
		},
		{
			name:    "other workflow attention",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", WorkflowAttentionState: "action_required", WorkflowAttentionHeadSHA: "head-a"},
			want:    prCIStateAttention,
		},
		{
			name:    "unstable rollup",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "unstable"},
			want:    prCIStateAttention,
		},
		{
			name:    "pending checks",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "pending"},
			want:    prCIStateInProgress,
		},
		{
			name:    "successful checks",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "success"},
			want:    prCIStateNone,
		},
		{
			name:    "empty checks",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a"},
			want:    prCIStateNone,
		},
		{
			name:    "failure beats approval",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "failure", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-a"},
			want:    prCIStateFailed,
		},
		{
			name:    "approval beats other attention",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-a", ChecksState: "unstable"},
			want:    prCIStateApprovalRequired,
		},
		{
			name:    "attention beats pending",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", WorkflowAttentionState: "action_required", WorkflowAttentionHeadSHA: "head-a", ChecksState: "pending"},
			want:    prCIStateAttention,
		},
		{
			name:    "wrong-head approval is ineligible",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-b", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-a"},
			want:    prCIStateNone,
		},
		{
			name:    "closed failure",
			fixture: ciEventFixture{State: "closed", HeadSHA: "head-a", ChecksState: "failure"},
			want:    prCIStateNone,
		},
		{
			name:    "merged approval",
			fixture: ciEventFixture{State: "merged", HeadSHA: "head-a", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-a"},
			want:    prCIStateNone,
		},
		{
			name:    "draft failure stays failed",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "failure", MergeableState: "draft"},
			want:    prCIStateFailed,
		},
		{
			name:    "review state alone is not CI",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ReviewState: "changes_requested"},
			want:    prCIStateNone,
		},
		{
			name:    "mergeability alone is not CI",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", MergeableState: "dirty"},
			want:    prCIStateNone,
		},
		{
			name:    "count shortfall is not running work",
			fixture: ciEventFixture{State: "open", HeadSHA: "head-a", ChecksTotal: 4, ChecksPassing: 1},
			want:    prCIStateNone,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeCIEvent(t, ciEvent(42, test.fixture))
			if got == nil {
				t.Fatal("pull request summary is nil")
			}
			if got.CIState != test.want {
				t.Fatalf("ci_state = %q, want %q", got.CIState, test.want)
			}
			if test.want == prCIStateNone && got.CIPRNumber != 0 {
				t.Fatalf("ci_pr_number = %d for none state, want 0", got.CIPRNumber)
			}
		})
	}
}

func TestCIStateProjectionIdentityAndStaleness(t *testing.T) {
	got := summarizeCIEvent(t, ciEvent(42, ciEventFixture{
		State: "open", HeadSHA: "head-a", ChecksState: "failure",
	}))
	if got.CIState != prCIStateFailed || got.CIPRNumber != 42 || got.CIRepository != "acme/widget" {
		t.Fatalf("failure identity = %+v, want acme/widget #42", got)
	}
	if got.CIStale {
		t.Fatal("fresh failure marked stale")
	}

	stale := summarizeCIEvent(t, ciEvent(43, ciEventFixture{
		State: "open", HeadSHA: "head-b", WorkflowAttentionState: "approval_required",
		WorkflowAttentionHeadSHA: "head-b", WorkflowAttentionStale: true,
	}))
	if stale.CIState != prCIStateApprovalRequired || !stale.CIStale {
		t.Fatalf("stale approval = %+v, want stale approval", stale)
	}
}

func TestCIStateProjectionAggregatesOpenPRsAndPrioritizes(t *testing.T) {
	got := summarizeCIEvent(t,
		ciEvent(42, ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "pending"}),
		ciEvent(43, ciEventFixture{State: "open", HeadSHA: "head-b", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-b"}),
		ciEvent(44, ciEventFixture{State: "merged", HeadSHA: "head-c", ChecksState: "failure"}),
	)
	if got.CIState != prCIStateApprovalRequired || got.CIPRNumber != 43 {
		t.Fatalf("aggregate = %+v, want approval from #43", got)
	}

	failure := summarizeCIEvent(t,
		ciEvent(45, ciEventFixture{State: "open", HeadSHA: "head-d", ChecksState: "failure"}),
		ciEvent(46, ciEventFixture{State: "open", HeadSHA: "head-e", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-e"}),
	)
	if failure.CIState != prCIStateFailed || failure.CIPRNumber != 45 {
		t.Fatalf("failure aggregate = %+v, want failure from #45", failure)
	}

	tie := summarizeCIEvent(t,
		ciEvent(48, ciEventFixture{State: "open", HeadSHA: "head-f", WorkflowAttentionState: "action_required", WorkflowAttentionHeadSHA: "head-f"}),
		ciEvent(47, ciEventFixture{State: "open", HeadSHA: "head-g", WorkflowAttentionState: "action_required", WorkflowAttentionHeadSHA: "head-g"}),
	)
	if tie.CIState != prCIStateAttention || tie.CIPRNumber != 47 {
		t.Fatalf("attention tie = %+v, want #47", tie)
	}
}

func TestCIStateEventRebuildParity(t *testing.T) {
	fixtures := []ciEventFixture{
		{State: "open", HeadSHA: "head-a", ChecksState: "failure"},
		{State: "open", HeadSHA: "head-b", WorkflowAttentionState: "approval_required", WorkflowAttentionHeadSHA: "head-b", WorkflowAttentionStale: true},
		{State: "open", HeadSHA: "head-c", WorkflowAttentionState: "action_required", WorkflowAttentionHeadSHA: "head-c"},
		{State: "open", HeadSHA: "head-d", ChecksState: "unstable"},
		{State: "open", HeadSHA: "head-e", ChecksState: "pending"},
		{State: "open", HeadSHA: "head-f", ChecksState: "success"},
		{State: "closed", HeadSHA: "head-g", ChecksState: "failure"},
	}
	for index, fixture := range fixtures {
		t.Run(fixture.ChecksState+fixture.WorkflowAttentionState, func(t *testing.T) {
			number := 50 + index
			eventSummary := summarizeCIEvent(t, ciEvent(number, fixture))
			input := RebuildInput{
				PRObserved: true,
				PullRequests: []PullRequestInput{{
					Key:                      "repo-1#50",
					Owner:                    "acme",
					Repo:                     "widget",
					State:                    fixture.State,
					Number:                   number,
					ChecksState:              fixture.ChecksState,
					ChecksTotal:              fixture.ChecksTotal,
					ChecksPassing:            fixture.ChecksPassing,
					MergeableState:           fixture.MergeableState,
					ReviewState:              fixture.ReviewState,
					HeadSHA:                  fixture.HeadSHA,
					WorkflowAttentionState:   fixture.WorkflowAttentionState,
					WorkflowAttentionHeadSHA: fixture.WorkflowAttentionHeadSHA,
					WorkflowAttentionStale:   fixture.WorkflowAttentionStale,
				}},
			}
			rebuilt := BuildFromAuthoritative(input).PullRequest
			if rebuilt == nil || eventSummary == nil {
				t.Fatal("expected both summaries to be present")
			}
			if rebuilt.CIState != eventSummary.CIState ||
				rebuilt.CIPRNumber != eventSummary.CIPRNumber ||
				rebuilt.CIRepository != eventSummary.CIRepository ||
				rebuilt.CIStale != eventSummary.CIStale {
				t.Fatalf("rebuild/event CI mismatch: rebuilt=%+v event=%+v", rebuilt, eventSummary)
			}
		})
	}
}

func TestCIStateTerminalCleanup(t *testing.T) {
	state := newProjectionState()
	projector := &Projector{}
	if !projector.applyPREventLocked(state, ciEvent(42, ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "failure"})) {
		t.Fatal("initial failure event was not applied")
	}
	if got := derivePullRequestSummary(state); got == nil || got.CIState != prCIStateFailed {
		t.Fatalf("initial ci_state = %+v, want failed", got)
	}
	if !projector.applyPREventLocked(state, ciEvent(42, ciEventFixture{State: "merged", HeadSHA: "head-a", ChecksState: "failure"})) {
		t.Fatal("terminal event was not applied")
	}
	if got := derivePullRequestSummary(state); got == nil || got.CIState != prCIStateNone || got.CIPRNumber != 0 {
		t.Fatalf("terminal ci_state = %+v, want explicit none", got)
	}
}

func TestCIStateSerializesExplicitNone(t *testing.T) {
	got := summarizeCIEvent(t, ciEvent(42, ciEventFixture{State: "open", HeadSHA: "head-a", ChecksState: "success"}))
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if fields["ci_state"] != prCIStateNone {
		t.Fatalf("serialized ci_state = %#v, want %q; payload=%s", fields["ci_state"], prCIStateNone, encoded)
	}
}
