package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/webapp"
)

// bootRepositorySetsPayload drives /api/v1/app-state for one route and returns
// the hydrated repositorySets slice shape.
type bootRepositorySetsPayload struct {
	InitialState struct {
		RepositorySets struct {
			ItemsByWorkspaceID map[string][]struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				Repositories []struct {
					RepositoryID string `json:"repository_id"`
					Position     int    `json:"position"`
				} `json:"repositories"`
			} `json:"itemsByWorkspaceId"`
			LoadingByWorkspaceID map[string]bool `json:"loadingByWorkspaceId"`
			LoadedByWorkspaceID  map[string]bool `json:"loadedByWorkspaceId"`
		} `json:"repositorySets"`
	} `json:"initialState"`
	RouteData struct {
		TasksPage struct {
			RepositorySets []struct {
				ID string `json:"id"`
			} `json:"repositorySets"`
		} `json:"tasksPage"`
	} `json:"routeData"`
}

// bootActiveWorkspace returns the workspace boot selects with no cookie, query
// parameter, or saved setting: the first one the service lists.
func bootActiveWorkspace(t *testing.T, harness bootStateTestHarness) *taskmodels.Workspace {
	t.Helper()
	workspaces, err := harness.taskSvc.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) == 0 || workspaces[0] == nil {
		t.Fatal("no workspace to boot into")
	}
	return workspaces[0]
}

func decodeBootRepositorySets(t *testing.T, harness bootStateTestHarness, routePath string) bootRepositorySetsPayload {
	t.Helper()
	// The query value is escaped; ClassifyRoute takes the real path. Passing the
	// escaped form to ClassifyRoute silently classifies every route as the
	// default one, which made an earlier version of this test assert nothing.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/app-state?path="+url.QueryEscape(routePath), nil)
	payload := bootPayload(context.Background(), req, routeParams{
		taskSvc:  harness.taskSvc,
		services: &Services{Workflow: harness.workflowSvc},
	}, webapp.ClassifyRoute(routePath))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var decoded bootRepositorySetsPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return decoded
}

// TestBootPayloadHydratesRepositorySets pins the hydrated shape rather than just
// the status code: the boot mappers are explicit whitelists, so a field that is
// not listed is silently absent and the client refetches on every dialog open.
func TestBootPayloadHydratesRepositorySets(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspace := bootActiveWorkspace(t, harness)

	for _, name := range []string{"web", "gateway"} {
		if err := harness.taskRepo.CreateRepository(ctx, &taskmodels.Repository{
			ID: "repo-" + name, WorkspaceID: workspace.ID, Name: name,
			SourceType: "local", LocalPath: t.TempDir(),
		}); err != nil {
			t.Fatalf("CreateRepository %s: %v", name, err)
		}
	}
	set, err := harness.taskSvc.CreateRepositorySet(ctx, &taskservice.CreateRepositorySetRequest{
		WorkspaceID:   workspace.ID,
		Name:          "Full-stack",
		RepositoryIDs: []string{"repo-web", "repo-gateway"},
	})
	if err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	for _, routePath := range []string{"/tasks", "/"} {
		t.Run(routePath, func(t *testing.T) {
			decoded := decodeBootRepositorySets(t, harness, routePath)
			items := decoded.InitialState.RepositorySets.ItemsByWorkspaceID[workspace.ID]
			if len(items) != 1 {
				t.Fatalf("hydrated sets = %+v, want one", items)
			}
			if items[0].ID != set.ID || items[0].Name != "Full-stack" {
				t.Fatalf("hydrated set = %+v", items[0])
			}
			if len(items[0].Repositories) != 2 {
				t.Fatalf("hydrated membership = %+v", items[0].Repositories)
			}
			if items[0].Repositories[0].RepositoryID != "repo-web" ||
				items[0].Repositories[1].RepositoryID != "repo-gateway" {
				t.Fatalf("hydrated membership order = %+v", items[0].Repositories)
			}
			if decoded.InitialState.RepositorySets.LoadingByWorkspaceID[workspace.ID] {
				t.Fatal("hydrated workspace is marked loading")
			}
			if !decoded.InitialState.RepositorySets.LoadedByWorkspaceID[workspace.ID] {
				t.Fatal("hydrated workspace is not marked loaded")
			}
		})
	}
}

// TestBootPayloadDoesNotMarkAFailedRepositorySetLoadAsLoaded pins the error path.
// `useRepositorySets` skips its fallback request when the workspace reports as
// loaded, so marking a failed load as loaded hides the workspace's real sets
// until an explicit refresh.
func TestBootPayloadDoesNotMarkAFailedRepositorySetLoadAsLoaded(t *testing.T) {
	harness := newBootStateTestHarness(t)
	workspace := bootActiveWorkspace(t, harness)

	// Drop the table so ListRepositorySets fails the way a real query error would.
	if _, err := harness.db.Exec(`DROP TABLE repository_set_items`); err != nil {
		t.Fatalf("drop items table: %v", err)
	}
	if _, err := harness.db.Exec(`DROP TABLE repository_sets`); err != nil {
		t.Fatalf("drop sets table: %v", err)
	}

	for _, routePath := range []string{"/tasks", "/"} {
		t.Run(routePath, func(t *testing.T) {
			decoded := decodeBootRepositorySets(t, harness, routePath)
			// Boot still succeeds: the failure is non-fatal.
			items, present := decoded.InitialState.RepositorySets.ItemsByWorkspaceID[workspace.ID]
			if !present || len(items) != 0 {
				t.Fatalf("items = %+v, want an empty list", items)
			}
			if decoded.InitialState.RepositorySets.LoadedByWorkspaceID[workspace.ID] {
				t.Fatal("a failed load is marked loaded, so the client will never retry")
			}
		})
	}
}

// TestBootPayloadRepositorySetsShapeIsAlwaysPresent covers a workspace with no
// sets. An absent key reads as "not loaded" to the client hook, which then
// refetches on every dialog open even though boot already answered.
func TestBootPayloadRepositorySetsShapeIsAlwaysPresent(t *testing.T) {
	harness := newBootStateTestHarness(t)
	workspace := bootActiveWorkspace(t, harness)

	decoded := decodeBootRepositorySets(t, harness, "/tasks")
	items, present := decoded.InitialState.RepositorySets.ItemsByWorkspaceID[workspace.ID]
	if !present {
		t.Fatal("repositorySets key is absent for a workspace with no sets")
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want empty", items)
	}
	if !decoded.InitialState.RepositorySets.LoadedByWorkspaceID[workspace.ID] {
		t.Fatal("empty workspace is not marked loaded")
	}
	if decoded.RouteData.TasksPage.RepositorySets == nil {
		t.Fatal("routeData.tasksPage.repositorySets serialized as null, want an array")
	}
}
