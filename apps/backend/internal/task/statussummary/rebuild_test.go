package statussummary

import (
	"testing"
	"time"
)

func TestBuildFromAuthoritativeAggregatesDurableSources(t *testing.T) {
	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	got := BuildFromAuthoritative(RebuildInput{
		Sessions: []RebuildSession{
			{
				ID:                  "primary",
				State:               "RUNNING",
				IsPrimary:           true,
				ForegroundActivity:  activityGenerating,
				ActiveSubagentCount: 2,
			},
			{
				ID:                  "worker",
				State:               "WAITING_FOR_INPUT",
				ForegroundActivity:  activityBackground,
				ActiveSubagentCount: 1,
				ActiveError: &ActiveErrorSummary{
					SessionID:  "worker",
					Stamp:      "worker-error",
					OccurredAt: now.Add(-time.Minute),
					Preview:    "the worker failed",
				},
			},
		},
		PendingActions: map[string]string{
			"worker":  string(pendingClarification),
			"primary": string(pendingPermission),
		},
		ActivityObserved: true,
		GitObserved:      true,
		Git: []RebuildGit{
			{Repository: "repo-a", Summary: GitSummary{Additions: 3, Deletions: 1, ChangedFiles: 2, Ahead: 2}},
			{Repository: "repo-b", Summary: GitSummary{Additions: 4, Deletions: 2, ChangedFiles: 1, Behind: 5}},
		},
		PRObserved: true,
		PullRequests: []PullRequestInput{
			{
				Key:             "pr-open",
				State:           prStateOpen,
				Number:          12,
				URL:             "https://github.test/pr/12",
				ReviewState:     prStateChanges,
				ChecksState:     prStateFailure,
				ChecksTotal:     3,
				ChecksPassing:   1,
				RequiredReviews: 1,
			},
			{Key: "pr-merged", State: prStateMerged, Number: 7, ReviewState: prStateApproved},
		},
		Now: now,
	})

	if got.PrimarySession == nil || got.PrimarySession.ID != "primary" || got.PrimarySession.State != "RUNNING" {
		t.Fatalf("primary session = %+v", got.PrimarySession)
	}
	if got.ForegroundActivity != activityGenerating || got.ActiveSubagentCount != 3 {
		t.Fatalf("activity summary = %+v", got)
	}
	if got.PendingAction != pendingPermission {
		t.Fatalf("pending action = %q, want %q", got.PendingAction, pendingPermission)
	}
	if got.ActiveError == nil || got.ActiveError.SessionID != "worker" || got.ActiveError.Preview != "the worker failed" {
		t.Fatalf("active error = %+v", got.ActiveError)
	}
	if got.Git == nil {
		t.Fatal("Git summary is nil")
	}
	if want := (GitSummary{Additions: 7, Deletions: 3, ChangedFiles: 3, Ahead: 2, Behind: 5}); *got.Git != want {
		t.Fatalf("Git summary = %+v, want %+v", *got.Git, want)
	}
	if got.PullRequest == nil {
		t.Fatal("pull request summary is nil")
	}
	if got.PullRequest.Count != 2 || got.PullRequest.OpenCount != 1 || !got.PullRequest.Attention {
		t.Fatalf("pull request aggregate = %+v", *got.PullRequest)
	}
	if got.PullRequest.State != prStateOpen || got.PullRequest.Number != 12 || got.PullRequest.AggregateState != prStateFailure {
		t.Fatalf("pull request representative = %+v", *got.PullRequest)
	}
}

func TestDeriveForegroundActivityPrefersGeneratingOverBackground(t *testing.T) {
	got := deriveForegroundActivity([]string{activityBackground, activityGenerating})
	if got != activityGenerating {
		t.Fatalf("foreground activity = %q, want %q", got, activityGenerating)
	}
}

func TestBuildFromAuthoritativeLeavesUnavailableOptionalSourcesEmpty(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{
		Sessions: []RebuildSession{{ID: "session-1", State: "COMPLETED", IsPrimary: true}},
	})

	if got.PrimarySession == nil || got.PrimarySession.ID != "session-1" {
		t.Fatalf("primary session = %+v", got.PrimarySession)
	}
	if got.Git != nil {
		t.Fatalf("Git summary = %+v, want nil when source was not observed", got.Git)
	}
	if got.PullRequest != nil {
		t.Fatalf("pull request summary = %+v, want nil when source was not observed", got.PullRequest)
	}
}

func TestPullRequestChecksWithAnIncompleteUnknownRollupArePending(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{
		PRObserved: true,
		PullRequests: []PullRequestInput{{
			Key:           "repo-a#42",
			State:         prStateOpen,
			Number:        42,
			ChecksTotal:   3,
			ChecksPassing: 1,
			ChecksState:   "",
		}},
	})

	if got.PullRequest == nil || got.PullRequest.AggregateState != prStatePending {
		t.Fatalf("pull request summary = %+v, want pending while checks are incomplete", got.PullRequest)
	}
}
