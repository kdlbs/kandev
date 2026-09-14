package statussummary

import "testing"

// @covers AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1
// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.5
func TestPullRequestAggregateStatePrecedence(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequestInput
		want string
	}{
		{
			name: "pending CI outranks blocked mergeability",
			pr: PullRequestInput{
				State:          prStateOpen,
				ChecksState:    prStatePending,
				MergeableState: prStateBlocked,
			},
			want: prStatePending,
		},
		{
			name: "failed CI outranks blocked mergeability",
			pr: PullRequestInput{
				State:          prStateOpen,
				ChecksState:    prStateFailure,
				MergeableState: prStateBlocked,
			},
			want: prStateFailure,
		},
		{
			name: "pending review outranks blocked mergeability",
			pr: PullRequestInput{
				State:              prStateOpen,
				ReviewState:        prStatePending,
				ChecksState:        prStateSuccess,
				MergeableState:     prStateBlocked,
				RequiredReviews:    2,
				PendingReviewCount: 1,
			},
			want: prStateAwaiting,
		},
		{
			name: "blocked mergeability remains visible after CI passes",
			pr: PullRequestInput{
				State:          prStateOpen,
				ChecksState:    prStateSuccess,
				MergeableState: prStateBlocked,
			},
			want: prStateBlocked,
		},
		{
			name: "pending review without CI signal remains yellow",
			pr: PullRequestInput{
				State:          prStateOpen,
				ReviewState:    prStatePending,
				MergeableState: prStateBlocked,
			},
			want: prStatePending,
		},
		{
			name: "dirty mergeability does not hide pending CI",
			pr: PullRequestInput{
				State:          prStateOpen,
				ChecksState:    prStatePending,
				MergeableState: prStateDirty,
			},
			want: prStatePending,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BuildFromAuthoritative(RebuildInput{
				PRObserved:   true,
				PullRequests: []PullRequestInput{test.pr},
			})
			if got.PullRequest == nil {
				t.Fatal("pull request summary is nil")
			}
			if got.PullRequest.AggregateState != test.want {
				t.Fatalf("aggregate state = %q, want %q", got.PullRequest.AggregateState, test.want)
			}
		})
	}
}

// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.5
// @covers AC-UI-PR-TASK-STATUS-SUMMARY-001.8
func TestPullRequestAggregateStateAcrossPullRequests(t *testing.T) {
	tests := []struct {
		name string
		prs  []PullRequestInput
		want string
	}{
		{
			name: "pending outranks blocked",
			prs: []PullRequestInput{
				{Key: "pending", State: prStateOpen, ChecksState: prStatePending},
				{Key: "blocked", State: prStateOpen, ChecksState: prStateSuccess, MergeableState: prStateBlocked},
			},
			want: prStatePending,
		},
		{
			name: "awaiting review outranks blocked",
			prs: []PullRequestInput{
				{
					Key:                "awaiting",
					State:              prStateOpen,
					ReviewState:        prStatePending,
					ChecksState:        prStateSuccess,
					PendingReviewCount: 1,
				},
				{Key: "blocked", State: prStateOpen, ChecksState: prStateSuccess, MergeableState: prStateBlocked},
			},
			want: prStateAwaiting,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BuildFromAuthoritative(RebuildInput{
				PRObserved:   true,
				PullRequests: test.prs,
			})
			if got.PullRequest == nil {
				t.Fatal("pull request summary is nil")
			}
			if got.PullRequest.AggregateState != test.want {
				t.Fatalf("aggregate state = %q, want %q", got.PullRequest.AggregateState, test.want)
			}
		})
	}
}
