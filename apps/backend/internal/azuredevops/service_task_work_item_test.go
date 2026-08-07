package azuredevops

import (
	"context"
	"testing"
)

type taskWorkItemClient struct{ fakeDetailClient }

func TestTaskWorkItemAssociation(t *testing.T) {
	client := &taskWorkItemClient{fakeDetailClient: fakeDetailClient{detail: &WorkItemDetail{WorkItem: WorkItem{ID: 101, Title: "Ship it", Project: "project-1", WebURL: "https://dev.azure.com/acme/p/_workitems/edit/101"}}}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := service.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	row, err := service.AssociateTaskWorkItem(t.Context(), "ws-1", "task-1", "project-1", 101)
	if err != nil || row.Title != "Ship it" {
		t.Fatalf("AssociateTaskWorkItem = %+v, %v", row, err)
	}
	grouped, err := service.ListTaskWorkItemsByWorkspace(t.Context(), "ws-1")
	if err != nil || len(grouped["task-1"]) != 1 || grouped["task-1"][0].ID != row.ID {
		t.Fatalf("ListTaskWorkItemsByWorkspace = %+v, %v", grouped, err)
	}
}
