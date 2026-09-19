package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

func insertBudgetInboxActivity(t *testing.T, deps *testDeps, action string) {
	t.Helper()
	if err := deps.repo.CreateActivityEntry(context.Background(), &models.ActivityEntry{
		ID:          "inbox-" + action,
		WorkspaceID: "ws1",
		ActorType:   models.ActivityActorSystem,
		ActorID:     "budget_checker",
		Action:      models.ActivityAction(action),
		TargetType:  models.ActivityTargetProject,
		TargetID:    "project-1",
		Details:     "budget details",
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("insert %s activity: %v", action, err)
	}
}

func TestGetInbox_IncludesPermissionRequestItems(t *testing.T) {
	deps := newTestDeps(t)

	lister := &stubPermissionLister{
		items: []shared.PendingPermission{
			{
				PendingID: "perm-1",
				SessionID: "session-1",
				TaskID:    "task-1",
				Prompt:    "Allow bash execution?",
				Context:   "tool permission",
				CreatedAt: time.Now(),
			},
		},
	}
	deps.svc.SetPermissionLister(lister)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/inbox", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.InboxResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		if item.Type == "permission_request" && item.EntityID == "perm-1" {
			found = true
			if item.Status != "pending" {
				t.Errorf("status = %q, want pending", item.Status)
			}
		}
	}
	if !found {
		t.Error("expected permission_request inbox item to be present")
	}
}

func TestGetInboxCount_IncludesPermissionRequests(t *testing.T) {
	deps := newTestDeps(t)

	lister := &stubPermissionLister{
		items: []shared.PendingPermission{
			{PendingID: "perm-a", SessionID: "s1", CreatedAt: time.Now()},
			{PendingID: "perm-b", SessionID: "s2", CreatedAt: time.Now()},
		},
	}
	deps.svc.SetPermissionLister(lister)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/inbox?count=true", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.InboxCountResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Count < 2 {
		t.Errorf("expected count >= 2 (including 2 permission requests), got %d", resp.Count)
	}
}

func TestGetInbox_IncludesBudgetExceededActivity(t *testing.T) {
	deps := newTestDeps(t)
	insertBudgetInboxActivity(t, deps, "budget.exceeded")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/inbox", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.InboxResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, item := range resp.Items {
		if item.ID == "inbox-budget.exceeded" {
			if item.Type != "budget_alert" {
				t.Errorf("type = %q, want budget_alert", item.Type)
			}
			if item.Title != "Budget exceeded" {
				t.Errorf("title = %q, want Budget exceeded", item.Title)
			}
			return
		}
	}
	t.Fatal("expected budget.exceeded activity in inbox")
}

func TestGetInboxCount_IncludesBudgetExceededActivity(t *testing.T) {
	deps := newTestDeps(t)
	insertBudgetInboxActivity(t, deps, "budget.exceeded")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws1/inbox?count=true", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp dashboard.InboxCountResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count < 1 {
		t.Fatalf("expected count >= 1 for budget.exceeded activity, got %d", resp.Count)
	}
}
