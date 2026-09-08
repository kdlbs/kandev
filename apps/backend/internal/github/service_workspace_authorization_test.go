package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestReviewWatchOperationsAuthorizeWorkspaceAtServiceBoundary(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	watch := &ReviewWatch{WorkspaceID: "victim-workspace", Prompt: "private prompt"}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("workspace not found")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "victim-workspace" {
			t.Fatalf("workspaceID = %q, want victim workspace", workspaceID)
		}
		return denied
	})

	if _, err := svc.ListReviewWatches(context.Background(), "victim-workspace"); !errors.Is(err, denied) {
		t.Fatalf("ListReviewWatches() error = %v, want authorization denial", err)
	}
	if _, err := svc.GetReviewWatch(context.Background(), watch.ID); !errors.Is(err, denied) {
		t.Fatalf("GetReviewWatch() error = %v, want authorization denial", err)
	}
	if err := svc.UpdateReviewWatch(context.Background(), watch.ID, &UpdateReviewWatchRequest{}); !errors.Is(err, denied) {
		t.Fatalf("UpdateReviewWatch() error = %v, want authorization denial", err)
	}
	if err := svc.DeleteReviewWatch(context.Background(), watch.ID); !errors.Is(err, denied) {
		t.Fatalf("DeleteReviewWatch() error = %v, want authorization denial", err)
	}
}

func TestTaskCIOptionsResolveAndAuthorizeOwningWorkspace(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-a", WorkspaceID: "workspace-a"},
	})
	var authorized string
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		authorized = workspaceID
		return nil
	})

	response, err := svc.GetTaskCIOptionsResponse(context.Background(), "task-a")
	if err != nil {
		t.Fatal(err)
	}
	if authorized != "workspace-a" {
		t.Fatalf("authorized workspace = %q, want workspace-a", authorized)
	}
	if response.WorkspaceID != "workspace-a" || response.GetWorkspaceID() != "workspace-a" {
		t.Fatalf("response workspace = %q, want workspace-a", response.WorkspaceID)
	}
}

func TestIssueAndPRWatchOperationsAuthorizeStoredWorkspace(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	ctx := context.Background()
	issueWatch := &IssueWatch{WorkspaceID: "victim-workspace"}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatal(err)
	}
	prWatch := &PRWatch{WorkspaceID: "victim-workspace", SessionID: "session-1", TaskID: "task-1", Owner: "acme", Repo: "private", Branch: "main"}
	if err := store.CreatePRWatch(ctx, prWatch); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("workspace not found")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "victim-workspace" {
			t.Fatalf("workspaceID = %q, want victim workspace", workspaceID)
		}
		return denied
	})

	if _, err := svc.GetIssueWatch(ctx, issueWatch.ID); !errors.Is(err, denied) {
		t.Fatalf("GetIssueWatch() error = %v, want authorization denial", err)
	}
	if _, err := svc.ListIssueWatches(ctx, issueWatch.WorkspaceID); !errors.Is(err, denied) {
		t.Fatalf("ListIssueWatches() error = %v, want authorization denial", err)
	}
	if err := svc.UpdateIssueWatch(ctx, issueWatch.ID, &UpdateIssueWatchRequest{}); !errors.Is(err, denied) {
		t.Fatalf("UpdateIssueWatch() error = %v, want authorization denial", err)
	}
	if err := svc.DeleteIssueWatch(ctx, issueWatch.ID); !errors.Is(err, denied) {
		t.Fatalf("DeleteIssueWatch() error = %v, want authorization denial", err)
	}
	if err := svc.DeletePRWatch(ctx, prWatch.ID); !errors.Is(err, denied) {
		t.Fatalf("DeletePRWatch() error = %v, want authorization denial", err)
	}
}

func TestWorkspaceSettingsAndActionPresetsAuthorizeBodyWorkspace(t *testing.T) {
	svc, _, _ := setupSyncTest(t)
	denied := errors.New("workspace not found")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "victim-workspace" {
			t.Fatalf("workspaceID = %q, want victim workspace", workspaceID)
		}
		return denied
	})
	ctx := context.Background()

	if _, err := svc.GetWorkspaceSettings(ctx, "victim-workspace"); !errors.Is(err, denied) {
		t.Fatalf("GetWorkspaceSettings() error = %v, want authorization denial", err)
	}
	if _, err := svc.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{WorkspaceID: "victim-workspace"}); !errors.Is(err, denied) {
		t.Fatalf("UpdateWorkspaceSettings() error = %v, want authorization denial", err)
	}
	if _, err := svc.GetActionPresets(ctx, "victim-workspace"); !errors.Is(err, denied) {
		t.Fatalf("GetActionPresets() error = %v, want authorization denial", err)
	}
	if _, err := svc.UpdateActionPresets(ctx, &UpdateActionPresetsRequest{WorkspaceID: "victim-workspace"}); !errors.Is(err, denied) {
		t.Fatalf("UpdateActionPresets() error = %v, want authorization denial", err)
	}
}

func TestCopyWorkspaceSettingsAuthorizesSourceAndTarget(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		svc := newCopyTestService(t)
		denied := errors.New("source denied")
		svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
			if workspaceID == "source" {
				return denied
			}
			return nil
		})
		if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "source", "target"); !errors.Is(err, denied) {
			t.Fatalf("copy error = %v, want source denial", err)
		}
	})
	t.Run("target", func(t *testing.T) {
		svc := newCopyTestService(t)
		denied := errors.New("target denied")
		svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
			if workspaceID == "target" {
				return denied
			}
			return nil
		})
		if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "source", "target"); !errors.Is(err, denied) {
			t.Fatalf("copy error = %v, want target denial", err)
		}
	})
	t.Run("both", func(t *testing.T) {
		svc := newCopyTestService(t)
		var authorized []string
		svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
			authorized = append(authorized, workspaceID)
			return nil
		})
		if _, err := svc.CopyWorkspaceSettingsToWorkspace(context.Background(), "source", "target"); err != nil {
			t.Fatal(err)
		}
		if len(authorized) != 2 || authorized[0] != "source" || authorized[1] != "target" {
			t.Fatalf("authorized = %#v, want source then target", authorized)
		}
	})
}

func TestGetPRForWorkspaceWithoutConnectionUsesAnonymousClient(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":1,"title":"anonymous","state":"open","user":{"login":"octo"},"head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
	}))
	t.Cleanup(api.Close)
	oldBase := anonymousAPIBase
	anonymousAPIBase = api.URL
	t.Cleanup(func() { anonymousAPIBase = oldBase })

	svc := NewService(&stubClient{getPRFunc: func(context.Context, string, string, int) (*PR, error) {
		return &PR{Title: "ambient credential"}, nil
	}}, AuthMethodPAT, nil, nil, nil, testLogger(t))
	pr, err := svc.GetPRForWorkspace(context.Background(), "workspace-without-connection", "user-1", "acme", "private", 1)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Title != "anonymous" {
		t.Fatalf("PR title = %q, want anonymous lookup without ambient credential", pr.Title)
	}
}

func TestGetPRForAutomationUsesWorkspaceCredential(t *testing.T) {
	ambient := &stubClient{getPRFunc: func(context.Context, string, string, int) (*PR, error) {
		return &PR{Title: "ambient credential"}, nil
	}}
	workspace := &stubClient{getPRFunc: func(context.Context, string, string, int) (*PR, error) {
		return &PR{Title: "workspace credential", BaseBranch: "main"}, nil
	}}
	svc := NewService(ambient, AuthMethodPAT, nil, nil, nil, testLogger(t))
	configureTestWorkspaceAuth(t, svc, workspace, "workspace-1")

	pr, err := svc.GetPRForAutomation(context.Background(), "workspace-1", "acme", "widgets", 42)
	if err != nil {
		t.Fatalf("GetPRForAutomation() error: %v", err)
	}
	if pr.Title != "workspace credential" {
		t.Fatalf("PR title = %q, want workspace credential", pr.Title)
	}
}

func TestListAllReviewWatchesRetainsIdentitylessInternalUse(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		if err := store.CreateReviewWatch(context.Background(), &ReviewWatch{WorkspaceID: workspaceID}); err != nil {
			t.Fatal(err)
		}
	}
	watches, err := svc.ListAllReviewWatches(context.Background())
	if err != nil || len(watches) != 2 {
		t.Fatalf("identityless internal list = %#v, %v; want two watches", watches, err)
	}
}

func TestIssueAndPRAllWorkspaceListsRetainIdentitylessInternalUse(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	ctx := context.Background()
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		if err := store.CreateIssueWatch(ctx, &IssueWatch{WorkspaceID: workspaceID}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, "task-"+workspaceID, workspaceID); err != nil {
			t.Fatal(err)
		}
		if err := store.CreatePRWatch(ctx, &PRWatch{WorkspaceID: workspaceID, SessionID: "session-" + workspaceID, TaskID: "task-" + workspaceID, Owner: "acme", Repo: "repo", Branch: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	if watches, err := svc.ListAllIssueWatches(ctx); err != nil || len(watches) != 2 {
		t.Fatalf("identityless issue list = %#v, %v; want two watches", watches, err)
	}
	if watches, err := svc.ListActivePRWatches(ctx); err != nil || len(watches) != 2 {
		t.Fatalf("identityless PR list = %#v, %v; want two watches", watches, err)
	}
}
