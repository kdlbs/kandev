package github

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// githubRepositoryGraphQLFields is the checked subset of the Repository
// object used by PR discovery. The field names are taken from GitHub's
// Repository schema, not from gh CLI JSON aliases:
// https://docs.github.com/en/graphql/reference/objects#repository
var githubRepositoryGraphQLFields = map[string]struct{}{
	"id":            {},
	"name":          {},
	"nameWithOwner": {},
	"url":           {},
}

func headRepositorySelection(query string) (map[string]struct{}, bool) {
	const prefix = "headRepository {"
	start := strings.Index(query, prefix)
	if start < 0 {
		return nil, false
	}
	start += len(prefix)
	end := strings.Index(query[start:], "}")
	if end < 0 {
		return nil, false
	}
	fields := make(map[string]struct{})
	for _, field := range strings.Fields(query[start : start+end]) {
		fields[field] = struct{}{}
	}
	return fields, true
}

func TestPRDiscoveryQueryContract(t *testing.T) {
	queries := map[string]string{}
	if query, _ := buildBatchedPRQuery([]graphQLPRRef{{Owner: "acme", Repo: "widget", Number: 1}}); query != "" {
		queries["known PR"] = query
	}
	if query, _ := buildBatchedBranchQuery([]graphQLBranchRef{{Owner: "acme", Repo: "widget", Branch: "feature"}}); query != "" {
		queries["branch"] = query
	}

	for name, query := range queries {
		fields, ok := headRepositorySelection(query)
		if !ok {
			t.Fatalf("%s query has no headRepository selection: %s", name, query)
		}
		for field := range fields {
			if _, supported := githubRepositoryGraphQLFields[field]; !supported {
				t.Errorf("%s query selects unsupported Repository field %q", name, field)
			}
		}
		for field := range githubRepositoryGraphQLFields {
			if _, selected := fields[field]; !selected {
				t.Errorf("%s query omits supported Repository field %q", name, field)
			}
		}
	}
}

func TestPRDiscoveryHeadIdentity(t *testing.T) {
	tests := []struct {
		name         string
		head         ghRepository
		headOwner    string
		wantTarget   string
		wantHead     string
		wantCloneURL string
	}{
		{
			name: "same repository",
			head: ghRepository{
				ID:            "R_same",
				Name:          "widget",
				NameWithOwner: "acme/widget",
				URL:           "https://github.com/acme/widget",
			},
			headOwner:    "acme",
			wantTarget:   "acme/widget",
			wantHead:     "acme/widget",
			wantCloneURL: "https://github.com/acme/widget.git",
		},
		{
			name: "fork keeps target distinct",
			head: ghRepository{
				ID:            "R_fork",
				Name:          "widget",
				NameWithOwner: "alice/widget",
				URL:           "https://github.com/alice/widget.git",
			},
			headOwner:    "alice",
			wantTarget:   "acme/widget",
			wantHead:     "alice/widget",
			wantCloneURL: "https://github.com/alice/widget.git",
		},
		{
			name:       "missing head repository does not invent identity",
			wantTarget: "acme/widget",
		},
		{
			name: "name with owner fills omitted owner fields",
			head: ghRepository{
				NameWithOwner: "alice/widget",
			},
			wantTarget: "acme/widget",
			wantHead:   "alice/widget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &batchedPRResult{
				HeadRepository: tt.head,
				HeadRepoOwner:  ghRepositoryOwner{Login: tt.headOwner},
			}
			status := convertBatchedPRResult(raw, "acme", "widget", 42)
			if got := status.PR.RepoOwner + "/" + status.PR.RepoName; got != tt.wantTarget {
				t.Errorf("target repository = %q, want %q", got, tt.wantTarget)
			}
			gotHead := status.PR.HeadRepoOwner
			if status.PR.HeadRepoName != "" {
				gotHead += "/" + status.PR.HeadRepoName
			}
			if gotHead != tt.wantHead {
				t.Errorf("head repository = %q, want %q", gotHead, tt.wantHead)
			}
			if status.PR.HeadRepoCloneURL != tt.wantCloneURL {
				t.Errorf("head clone URL = %q, want %q", status.PR.HeadRepoCloneURL, tt.wantCloneURL)
			}
		})
	}
}

func TestPRDiscoveryAssociationAfterQueryRecovery(t *testing.T) {
	_, service, client, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-discovery", false)
	fixedNow := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	service.prDiscoveryHealth.setNow(func() time.Time { return fixedNow })
	watch := withTestWorkspace(&PRWatch{
		SessionID: "session-discovery",
		TaskID:    "task-discovery",
		Owner:     "o",
		Repo:      "r",
		Branch:    "feature/discovery",
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}

	updated := make(chan *TaskPR, 1)
	subscription, err := service.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(_ context.Context, event *bus.Event) error {
		if taskPR, ok := event.Data.(*TaskPR); ok {
			updated <- taskPR
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to task PR updates: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	client.branchResponses = []string{
		`{"errors":[{"type":"GRAPHQL_VALIDATION_FAILED","message":"Field 'cloneUrl' doesn't exist on type 'Repository'"}]}`,
		branchAliasResponse(t, 1, map[int]string{0: openPRNode(7, "feature/discovery", "alice")}),
	}

	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("health-aware rejected discovery: %v", err)
	}
	if taskPR, err := store.GetTaskPR(ctx, "task-discovery"); err != nil {
		t.Fatalf("check failed association: %v", err)
	} else if taskPR != nil {
		t.Fatalf("query failure created task PR association: %+v", taskPR)
	}
	select {
	case event := <-updated:
		t.Fatalf("query failure published task PR event: %+v", event)
	default:
	}

	fixedNow = fixedNow.Add(PRDiscoveryRetryBase)
	if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, []*PRWatch{watch}); err != nil {
		t.Fatalf("discovery after corrected query: %v", err)
	}
	associated, err := store.GetTaskPR(ctx, "task-discovery")
	if err != nil {
		t.Fatalf("read recovered association: %v", err)
	}
	if associated == nil || associated.PRNumber != 7 {
		t.Fatalf("recovered association = %+v, want PR #7", associated)
	}
	select {
	case event := <-updated:
		if event.TaskID != "task-discovery" || event.PRNumber != 7 {
			t.Fatalf("task PR event = %+v, want task-discovery PR #7", event)
		}
	case <-time.After(time.Second):
		t.Fatal("recovered association did not publish task PR event")
	}
}
