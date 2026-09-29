package github

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type reviewWatchSearchClient struct {
	*MockClient
	searchPR   *PR
	detailPR   *PR
	detailErr  error
	getPRCalls int
	getPROwner string
	getPRRepo  string
	getPRNum   int
}

func (c *reviewWatchSearchClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	return []*PR{c.searchPR}, nil
}

func (c *reviewWatchSearchClient) GetPR(_ context.Context, owner, repo string, number int) (*PR, error) {
	c.getPRCalls++
	c.getPROwner = owner
	c.getPRRepo = repo
	c.getPRNum = number
	return c.detailPR, c.detailErr
}

type reviewPRDetails struct {
	headBranch          string
	headSHA             string
	baseBranch          string
	baseSHA             string
	additions           int
	deletions           int
	mergeable           bool
	mergeableState      string
	headRepoID          int64
	headRepoNodeID      string
	headRepoOwner       string
	headRepoName        string
	headRepoCloneURL    string
	baseRepoID          int64
	baseRepoOwner       string
	baseRepoName        string
	baseDefaultBranch   string
	maintainerCanModify bool
}

func reviewPRDetailsFrom(pr *PR) reviewPRDetails {
	return reviewPRDetails{
		headBranch:          pr.HeadBranch,
		headSHA:             pr.HeadSHA,
		baseBranch:          pr.BaseBranch,
		baseSHA:             pr.BaseSHA,
		additions:           pr.Additions,
		deletions:           pr.Deletions,
		mergeable:           pr.Mergeable,
		mergeableState:      pr.MergeableState,
		headRepoID:          pr.HeadRepoID,
		headRepoNodeID:      pr.HeadRepoNodeID,
		headRepoOwner:       pr.HeadRepoOwner,
		headRepoName:        pr.HeadRepoName,
		headRepoCloneURL:    pr.HeadRepoCloneURL,
		baseRepoID:          pr.BaseRepoID,
		baseRepoOwner:       pr.BaseRepoOwner,
		baseRepoName:        pr.BaseRepoName,
		baseDefaultBranch:   pr.BaseDefaultBranch,
		maintainerCanModify: pr.MaintainerCanModify,
	}
}

func TestCheckReviewWatchEnrichesSearchResultsBeforePublishingEvent(t *testing.T) {
	tests := []struct {
		name        string
		detailPR    *PR
		detailErr   error
		wantDetails reviewPRDetails
	}{
		{
			name: "same repository identity",
			detailPR: &PR{
				RepoOwner: "acme", RepoName: "widget",
				HeadBranch: "feature/detail", HeadSHA: "head-sha-detail",
				BaseBranch: "main-detail", BaseSHA: "base-sha-detail",
				Additions: 7, Deletions: 3, Mergeable: true, MergeableState: "clean",
				HeadRepoID: 101, HeadRepoNodeID: "H_repo", HeadRepoOwner: "acme",
				HeadRepoName: "widget", HeadRepoCloneURL: "https://github.com/acme/widget.git",
				BaseRepoID: 202, BaseRepoOwner: "acme", BaseRepoName: "widget",
				BaseDefaultBranch: "main", MaintainerCanModify: true,
			},
			wantDetails: reviewPRDetails{
				headBranch: "feature/detail", headSHA: "head-sha-detail",
				baseBranch: "main-detail", baseSHA: "base-sha-detail",
				additions: 7, deletions: 3, mergeable: true, mergeableState: "clean",
				headRepoID: 101, headRepoNodeID: "H_repo", headRepoOwner: "acme",
				headRepoName: "widget", headRepoCloneURL: "https://github.com/acme/widget.git",
				baseRepoID: 202, baseRepoOwner: "acme", baseRepoName: "widget",
				baseDefaultBranch: "main", maintainerCanModify: true,
			},
		},
		{
			name: "fork identity",
			detailPR: &PR{
				HeadBranch: "contrib/feature", HeadSHA: "fork-head-sha",
				BaseBranch: "main", BaseSHA: "base-sha",
				HeadRepoID: 303, HeadRepoNodeID: "H_fork", HeadRepoOwner: "contributor",
				HeadRepoName: "widget-fork", HeadRepoCloneURL: "https://github.com/contributor/widget-fork.git",
				BaseRepoID: 202, BaseRepoOwner: "acme", BaseRepoName: "widget",
				BaseDefaultBranch: "main", MaintainerCanModify: false,
				Mergeable: false, MergeableState: "blocked", Additions: 4, Deletions: 2,
			},
			wantDetails: reviewPRDetails{
				headBranch: "contrib/feature", headSHA: "fork-head-sha",
				baseBranch: "main", baseSHA: "base-sha", additions: 4, deletions: 2,
				mergeable: false, mergeableState: "blocked",
				headRepoID: 303, headRepoNodeID: "H_fork", headRepoOwner: "contributor",
				headRepoName: "widget-fork", headRepoCloneURL: "https://github.com/contributor/widget-fork.git",
				baseRepoID: 202, baseRepoOwner: "acme", baseRepoName: "widget",
				baseDefaultBranch: "main", maintainerCanModify: false,
			},
		},
		{
			name:      "detail lookup failure keeps identity unavailable",
			detailErr: errors.New("GitHub detail lookup failed"),
			wantDetails: reviewPRDetails{
				headBranch: "feature/search", baseBranch: "main", additions: 1, deletions: 2,
			},
		},
		{
			name: "nil detail response keeps identity unavailable",
			wantDetails: reviewPRDetails{
				headBranch: "feature/search", baseBranch: "main", additions: 1, deletions: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			searchPR := &PR{
				Number: 42, Title: "Review this change", HTMLURL: "https://github.com/acme/widget/pull/42",
				RepoOwner: "acme", RepoName: "widget",
				HeadBranch: "feature/search", BaseBranch: "main", Additions: 1, Deletions: 2,
			}
			client := &reviewWatchSearchClient{
				MockClient: NewMockClient(), searchPR: searchPR, detailPR: tt.detailPR, detailErr: tt.detailErr,
			}
			eventBus := bus.NewMemoryEventBus(testLogger(t))
			t.Cleanup(eventBus.Close)
			var emitted *NewReviewPREvent
			subscription, err := eventBus.Subscribe(events.GitHubNewReviewPR, func(_ context.Context, event *bus.Event) error {
				payload, ok := event.Data.(*NewReviewPREvent)
				if !ok {
					return errors.New("event data is not a NewReviewPREvent")
				}
				emitted = payload
				return nil
			})
			if err != nil {
				t.Fatalf("subscribe to review event: %v", err)
			}
			t.Cleanup(func() { _ = subscription.Unsubscribe() })

			service := NewService(client, AuthMethodPAT, nil, store, eventBus, testLogger(t))
			t.Cleanup(service.Stop)
			configureTestWorkspaceAuth(t, service, client, "workspace-1")
			watch := &ReviewWatch{
				ID: "watch-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
				WorkflowStepID: "step-1", Repos: []RepoFilter{{Owner: "acme", Name: "widget"}},
				Enabled: true,
			}
			if err := store.CreateReviewWatch(ctx, watch); err != nil {
				t.Fatalf("create review watch: %v", err)
			}

			prs, err := service.TriggerReviewWatch(ctx, watch)
			if err != nil {
				t.Fatalf("TriggerReviewWatch: %v", err)
			}
			if len(prs) != 1 {
				t.Fatalf("new review PR count = %d, want 1", len(prs))
			}
			if client.getPRCalls != 1 || client.getPROwner != "acme" || client.getPRRepo != "widget" || client.getPRNum != 42 {
				t.Fatalf("GetPR calls = (%d, %q, %q, %d), want one lookup for acme/widget#42",
					client.getPRCalls, client.getPROwner, client.getPRRepo, client.getPRNum)
			}
			if got := reviewPRDetailsFrom(prs[0]); got != tt.wantDetails {
				t.Errorf("enriched PR details = %+v, want %+v", got, tt.wantDetails)
			}
			if prs[0].RepoOwner != "acme" || prs[0].RepoName != "widget" {
				t.Errorf("search target repository = %s/%s, want acme/widget", prs[0].RepoOwner, prs[0].RepoName)
			}
			if emitted == nil || emitted.PR == nil {
				t.Fatal("review watch did not publish a PR event")
			}
			if got := reviewPRDetailsFrom(emitted.PR); got != tt.wantDetails {
				t.Errorf("event PR details = %+v, want %+v", got, tt.wantDetails)
			}
			if emitted.PR.RepoOwner != "acme" || emitted.PR.RepoName != "widget" {
				t.Errorf("event target repository = %s/%s, want acme/widget", emitted.PR.RepoOwner, emitted.PR.RepoName)
			}
		})
	}
}
