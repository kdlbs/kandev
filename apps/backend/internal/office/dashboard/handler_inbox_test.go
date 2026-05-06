package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/shared"
)

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
