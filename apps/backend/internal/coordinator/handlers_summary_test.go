package coordinator

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type listSummary struct {
	WatchScope      string `json:"watch_scope"`
	WatchedCount    int    `json:"watched_count"`
	ApprovalActions int    `json:"approval_actions"`
	ActiveOrders    int    `json:"active_orders"`
}

func listFirstSummary(t *testing.T, h *Handlers) (*listSummary, map[string]any) {
	t.Helper()
	rec := runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams(""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Coordinators []struct {
			Summary *listSummary `json:"summary"`
		} `json:"coordinators"`
	}
	decodeBody(t, rec, &list)
	var raw struct {
		Coordinators []map[string]any `json:"coordinators"`
	}
	decodeBody(t, rec, &raw)
	if len(list.Coordinators) == 0 {
		t.Fatalf("list empty: %s", rec.Body.String())
	}
	return list.Coordinators[0].Summary, raw.Coordinators[0]
}

func TestListSummary_AllSelectedAndEmptyEffectiveSet(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	h := &Handlers{service: svc, logger: newTestLogger(t)}
	ctx := context.Background()
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", c.WorkspaceID)

	sum, _ := listFirstSummary(t, h)
	if sum == nil || sum.WatchScope != "all" || sum.WatchedCount != 0 || sum.ApprovalActions != 1 || sum.ActiveOrders != 0 {
		t.Fatalf("all summary = %+v", sum)
	}

	if _, err := svc.AddStandingOrder(ctx, c.WorkspaceID, c.ID, AddStandingOrderInput{Text: "Keep the queue short"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, wf := range []string{"wf-a", "wf-gone"} {
		if _, err := store.db.Exec(store.db.Rebind(`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, ?, ?, ?)`), c.ID, wf, c.WorkspaceID, now); err != nil {
			t.Fatal(err)
		}
	}
	sum, _ = listFirstSummary(t, h)
	if sum.WatchScope != "selected" || sum.WatchedCount != 1 || sum.ActiveOrders != 1 {
		t.Fatalf("selected summary counts the effective set only: %+v", sum)
	}

	dropWorkflow(t, store, "wf-a")
	sum, _ = listFirstSummary(t, h)
	if sum.WatchScope != "selected" || sum.WatchedCount != 0 {
		t.Fatalf("empty effective set summary = %+v", sum)
	}
}

func TestListSummary_AbsentWhilePhase2Off(t *testing.T) {
	_, _, svc := phase2Fixture(t, false)
	h := &Handlers{service: svc, logger: newTestLogger(t)}
	sum, raw := listFirstSummary(t, h)
	if _, ok := raw["summary"]; ok || sum != nil {
		t.Fatalf("summary present with phase 2 off: %+v", raw)
	}
}
