package statussummary

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestQueuedPullRequestIsProjectedByLiveAndRestartPaths(t *testing.T) {
	input := PullRequestInput{
		Key: "repo-1#42", State: prStateOpen, Number: 42,
		URL: "https://example.test/42", MergeQueueState: "queued",
		ChecksState: prStateSuccess,
	}
	rebuilt := BuildFromAuthoritative(RebuildInput{PRObserved: true, PullRequests: []PullRequestInput{input}})
	if rebuilt.PullRequest == nil || rebuilt.PullRequest.AggregateState != prStateQueued {
		t.Fatalf("rebuilt pull request summary = %+v, want queued", rebuilt.PullRequest)
	}

	projector, store, _, _, _ := newProjectorTest(t)
	if err := projector.HandleEvent(context.Background(), bus.NewEvent(events.GitHubTaskPRUpdated, "test", map[string]interface{}{
		"task_id":           "task-queue-live",
		"repository_id":     "repo-1",
		"pr_number":         42,
		"state":             prStateOpen,
		"pr_url":            input.URL,
		"merge_queue_state": "queued",
		"checks_state":      prStateSuccess,
	})); err != nil {
		t.Fatalf("live PR event: %v", err)
	}
	live := store.summary("task-queue-live")
	if live == nil || live.PullRequest == nil || live.PullRequest.AggregateState != prStateQueued {
		t.Fatalf("live pull request summary = %+v, want queued", live)
	}
}

func TestQueuedPullRequestRanksBelowMoreAttentionWorthySibling(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{
		PRObserved: true,
		PullRequests: []PullRequestInput{
			{Key: "queued", State: prStateOpen, Number: 1, MergeQueueState: "queued"},
			{Key: "failed", State: prStateOpen, Number: 2, ChecksState: prStateFailure},
		},
	})
	if got.PullRequest == nil || got.PullRequest.AggregateState != prStateFailure {
		t.Fatalf("aggregate state = %+v, want failure over queued sibling", got.PullRequest)
	}
}

func TestQueuedPullRequestTakesPrecedenceOverItsOtherNonTerminalStates(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{
		PRObserved: true,
		PullRequests: []PullRequestInput{{
			Key: "queued", State: prStateOpen, Number: 1,
			MergeQueueState: "queued", MergeableState: "dirty",
			ReviewState: prStateChanges, ChecksState: prStateFailure,
		}},
	})
	if got.PullRequest == nil || got.PullRequest.AggregateState != prStateQueued {
		t.Fatalf("aggregate state = %+v, want queued", got.PullRequest)
	}
}

func TestQueuedPullRequestTakesPrecedenceOverDraftMergeability(t *testing.T) {
	got := BuildFromAuthoritative(RebuildInput{
		PRObserved: true,
		PullRequests: []PullRequestInput{{
			Key: "queued", State: prStateOpen, Number: 1,
			MergeQueueState: "queued", MergeableState: prStateDraft,
		}},
	})
	if got.PullRequest == nil || got.PullRequest.AggregateState != prStateQueued {
		t.Fatalf("aggregate state = %+v, want queued", got.PullRequest)
	}
}
