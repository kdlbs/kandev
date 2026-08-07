package azuredevops

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestTriggerWorkItemWatchPublishesOnlyNewMatches(t *testing.T) {
	client := &fakeReadClient{work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101, Title: "Fix build", WebURL: "https://azure/item/101"}}}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	watch := &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT [System.Id] FROM WorkItems"}
	if err := store.CreateWorkItemWatch(t.Context(), watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	service.SetEventBus(eventBus)
	seen := make(chan *WorkItemWatchEvent, 1)
	_, err := eventBus.Subscribe(events.AzureDevOpsWorkItemWatchMatch, func(_ context.Context, event *bus.Event) error {
		payload, ok := event.Data.(*WorkItemWatchEvent)
		if !ok {
			t.Fatalf("event payload = %T", event.Data)
		}
		seen <- payload
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	result, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Matched != 1 {
		t.Fatalf("trigger result = %+v err=%v", result, err)
	}
	select {
	case event := <-seen:
		if event.WorkItem.ID != 101 || event.WatchGeneration != watch.Generation {
			t.Fatalf("watch event = %+v", event)
		}
	default:
		t.Fatal("expected work-item watch event")
	}
	result, err = service.TriggerWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Matched != 0 {
		t.Fatalf("duplicate trigger result = %+v err=%v", result, err)
	}
}

func TestTriggerPullRequestWatchPublishesOnlyNewMatches(t *testing.T) {
	client := &fakeReadClient{prPage: &PullRequestPage{Items: []PullRequest{{ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1", WebURL: "https://azure/pr/42", Title: "Ship it"}}}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	watch := &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1", Status: "active"}
	if err := store.CreatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	service.SetEventBus(eventBus)
	seen := make(chan *PullRequestWatchEvent, 1)
	if _, err := eventBus.Subscribe(events.AzureDevOpsPullRequestWatchMatch, func(_ context.Context, event *bus.Event) error {
		seen <- event.Data.(*PullRequestWatchEvent)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	result, err := service.TriggerPullRequestWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Matched != 1 {
		t.Fatalf("trigger result = %+v err=%v", result, err)
	}
	select {
	case event := <-seen:
		if event.PullRequest.ID != 42 || event.AzureRepositoryID != "azure-repo-1" {
			t.Fatalf("watch event = %+v", event)
		}
	default:
		t.Fatal("expected pull-request watch event")
	}
}

func TestAzurePollerRunsEnabledWatchers(t *testing.T) {
	client := &fakeReadClient{
		work:   &WorkItemSearchResult{Items: []WorkItem{{ID: 101, WebURL: "https://azure/item/101"}}},
		prPage: &PullRequestPage{Items: []PullRequest{{ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1", WebURL: "https://azure/pr/42"}}},
	}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	workWatch := &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT [System.Id] FROM WorkItems"}
	prWatch := &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1"}
	if err := store.CreateWorkItemWatch(t.Context(), workWatch); err != nil {
		t.Fatalf("create work watch: %v", err)
	}
	if err := store.CreatePullRequestWatch(t.Context(), prWatch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	poller := NewPoller(service, logger.Default())
	if matched := poller.runWatchChecks(t.Context()); matched != 2 {
		t.Fatalf("matched = %d, want 2", matched)
	}
}
