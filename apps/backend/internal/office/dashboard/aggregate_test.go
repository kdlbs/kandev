package dashboard_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// stubWorkspaceLister is a minimal dashboard.WorkspaceLister for the aggregate
// tests; it returns a fixed workspace list so the test controls exactly which
// workspaces the caller can see.
type stubWorkspaceLister struct {
	workspaces []*taskmodels.Workspace
	err        error
}

func (s *stubWorkspaceLister) ListWorkspaces(_ context.Context) ([]*taskmodels.Workspace, error) {
	return s.workspaces, s.err
}

func insertAggregateTask(t *testing.T, deps *testDeps, id, wsID, state string) {
	t.Helper()
	_, err := deps.db.Exec(
		`INSERT INTO tasks (id, workspace_id, title, state) VALUES (?, ?, ?, ?)`,
		id, wsID, id, state,
	)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

func insertAggregateApproval(t *testing.T, deps *testDeps, id, wsID, status string) {
	t.Helper()
	_, err := deps.db.Exec(
		`INSERT INTO office_approvals (id, workspace_id, type, status, created_at, updated_at)
		 VALUES (?, ?, 'hire_agent', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, wsID, status,
	)
	if err != nil {
		t.Fatalf("insert approval %s: %v", id, err)
	}
}

func insertAggregateActivity(t *testing.T, deps *testDeps, id, wsID, action string) {
	t.Helper()
	_, err := deps.db.Exec(
		`INSERT INTO office_activity_log (id, workspace_id, actor_type, actor_id, action, created_at)
		 VALUES (?, ?, 'agent', 'agent-1', ?, CURRENT_TIMESTAMP)`,
		id, wsID, action,
	)
	if err != nil {
		t.Fatalf("insert activity %s: %v", id, err)
	}
}

// TestGetWorkspacesAggregateUnavailable pins that the endpoint surfaces a 503
// shape (ErrWorkspaceAggregateUnavailable) when no lister is wired.
func TestGetWorkspacesAggregateUnavailable(t *testing.T) {
	deps := newTestDeps(t)
	_, err := deps.svc.GetWorkspacesAggregate(context.Background())
	if !errors.Is(err, dashboard.ErrWorkspaceAggregateUnavailable) {
		t.Fatalf("err = %v, want ErrWorkspaceAggregateUnavailable", err)
	}
}

// TestGetWorkspacesAggregateBuildsPerWorkspaceCounts drives the aggregate over
// two workspaces and checks the task breakdown, pending approvals, ordering,
// and merged activity feed.
func TestGetWorkspacesAggregateBuildsPerWorkspaceCounts(t *testing.T) {
	deps := newTestDeps(t)
	deps.agents.instances = []*models.AgentInstance{{
		ID: "agent-1", WorkspaceID: "ws-1", Name: "Alpha Agent", Status: models.AgentStatusWorking,
	}}
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{workspaces: []*taskmodels.Workspace{
		{ID: "ws-1", Name: "Alpha", OfficeWorkflowID: "office-workflow"},
		{ID: "ws-2", Name: "Beta", OfficeWorkflowID: "office-workflow"},
	}})

	// ws-1: 2 open (todo + backlog-ish default), 1 in_progress, 1 blocked,
	// 1 done -> total 5. ws-2: 1 open only.
	insertAggregateTask(t, deps, "t1", "ws-1", "TODO")
	insertAggregateTask(t, deps, "t2", "ws-1", "IN_PROGRESS")
	insertAggregateTask(t, deps, "t3", "ws-1", "BLOCKED")
	insertAggregateTask(t, deps, "t4", "ws-1", "COMPLETED")
	insertAggregateTask(t, deps, "t5", "ws-1", "TODO")
	insertAggregateTask(t, deps, "t6", "ws-2", "TODO")

	insertAggregateApproval(t, deps, "a1", "ws-1", "pending")
	insertAggregateApproval(t, deps, "a2", "ws-1", "pending")
	insertAggregateApproval(t, deps, "a3", "ws-1", "approved")
	insertAggregateApproval(t, deps, "a4", "ws-2", "pending")

	insertAggregateActivity(t, deps, "act-1", "ws-1", "task.completed")
	insertAggregateActivity(t, deps, "act-2", "ws-2", "task.created")
	if _, err := deps.db.Exec(`UPDATE office_activity_log SET target_type = 'task', target_id = 't4' WHERE id = 'act-1'`); err != nil {
		t.Fatalf("set activity target: %v", err)
	}

	resp, err := deps.svc.GetWorkspacesAggregate(context.Background())
	if err != nil {
		t.Fatalf("GetWorkspacesAggregate: %v", err)
	}

	if len(resp.Workspaces) != 2 {
		t.Fatalf("workspace count = %d, want 2", len(resp.Workspaces))
	}
	// Sorted by name: Alpha (ws-1) before Beta (ws-2).
	alpha, beta := resp.Workspaces[0], resp.Workspaces[1]
	if alpha.WorkspaceID != "ws-1" || alpha.Name != "Alpha" {
		t.Fatalf("first entry = %+v, want ws-1 Alpha", alpha)
	}
	if beta.WorkspaceID != "ws-2" || beta.Name != "Beta" {
		t.Fatalf("second entry = %+v, want ws-2 Beta", beta)
	}

	if alpha.TaskCount != 5 || alpha.OpenTasks != 2 || alpha.InProgressTasks != 1 ||
		alpha.BlockedTasks != 1 || alpha.DoneTasks != 1 {
		t.Fatalf("ws-1 breakdown = %+v, want total 5 (2 open, 1 in-progress, 1 blocked, 1 done)", alpha)
	}
	if alpha.PendingApprovals != 2 {
		t.Fatalf("ws-1 pending approvals = %d, want 2", alpha.PendingApprovals)
	}
	if alpha.AgentCount != 1 || alpha.RunningAgents != 1 {
		t.Fatalf("ws-1 agent counts = %d total, %d running; want 1 and 1", alpha.AgentCount, alpha.RunningAgents)
	}
	if beta.TaskCount != 1 || beta.OpenTasks != 1 || beta.PendingApprovals != 1 {
		t.Fatalf("ws-2 = %+v, want total 1 open and 1 pending approval", beta)
	}
	if beta.AgentCount != 0 || beta.RunningAgents != 0 {
		t.Fatalf("ws-2 agent counts = %d total, %d running; want 0 and 0", beta.AgentCount, beta.RunningAgents)
	}

	if len(resp.RecentActivity) != 2 {
		t.Fatalf("recent activity count = %d, want 2", len(resp.RecentActivity))
	}
	for _, entry := range resp.RecentActivity {
		if entry.WorkspaceID == "ws-1" && (entry.ActorName != "Alpha Agent" || entry.TargetName != "t4") {
			t.Fatalf("ws-1 activity labels = actor %q target %q, want Alpha Agent and t4", entry.ActorName, entry.TargetName)
		}
	}
}

// TestGetWorkspacesAggregateEmptyList pins that no workspaces yields an empty
// (not nil) workspace slice and an empty activity feed.
func TestGetWorkspacesAggregateEmptyList(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{workspaces: []*taskmodels.Workspace{}})

	resp, err := deps.svc.GetWorkspacesAggregate(context.Background())
	if err != nil {
		t.Fatalf("GetWorkspacesAggregate: %v", err)
	}
	if resp.Workspaces == nil || len(resp.Workspaces) != 0 {
		t.Fatalf("workspaces = %#v, want non-nil empty slice", resp.Workspaces)
	}
	if resp.RecentActivity == nil || len(resp.RecentActivity) != 0 {
		t.Fatalf("recent activity = %#v, want non-nil empty slice", resp.RecentActivity)
	}
}

func TestGetWorkspacesAggregateIncludesOnlyOfficeWorkspaces(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{workspaces: []*taskmodels.Workspace{
		{ID: "office", Name: "Office workspace", OfficeWorkflowID: "office-workflow"},
		{ID: "kanban", Name: "Kanban workspace"},
	}})

	resp, err := deps.svc.GetWorkspacesAggregate(context.Background())
	if err != nil {
		t.Fatalf("GetWorkspacesAggregate: %v", err)
	}
	if len(resp.Workspaces) != 1 || resp.Workspaces[0].WorkspaceID != "office" {
		t.Fatalf("workspaces = %+v, want only the Office workspace", resp.Workspaces)
	}
}

func TestGetWorkspacesAggregateReturnsAgentReadFailures(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{workspaces: []*taskmodels.Workspace{
		{ID: "office", Name: "Office workspace", OfficeWorkflowID: "office-workflow"},
	}})
	wantErr := errors.New("agent store unavailable")
	deps.agents.listErr = wantErr

	if _, err := deps.svc.GetWorkspacesAggregate(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("GetWorkspacesAggregate error = %v, want %v", err, wantErr)
	}
}

func TestGetWorkspacesAggregateBatchesLargeWorkspaceLists(t *testing.T) {
	deps := newTestDeps(t)
	const workspaceCount = 33000
	workspaces := make([]*taskmodels.Workspace, workspaceCount)
	for i := range workspaces {
		workspaces[i] = &taskmodels.Workspace{
			ID:               fmt.Sprintf("ws-%05d", i),
			Name:             fmt.Sprintf("Workspace %05d", i),
			OfficeWorkflowID: "office-workflow",
		}
	}
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{workspaces: workspaces})

	firstID := workspaces[0].ID
	lastID := workspaces[len(workspaces)-1].ID
	insertAggregateTask(t, deps, "last-task", lastID, "BLOCKED")
	insertAggregateApproval(t, deps, "last-approval", lastID, "pending")
	insertAggregateActivity(t, deps, "old-activity", firstID, "task.created")
	insertAggregateActivity(t, deps, "new-activity", lastID, "task.blocked")
	if _, err := deps.db.Exec(`UPDATE office_activity_log SET created_at = datetime('now', '-1 minute') WHERE id = 'old-activity'`); err != nil {
		t.Fatalf("set old activity time: %v", err)
	}
	if _, err := deps.db.Exec(`UPDATE office_activity_log SET created_at = datetime('now', '+1 minute') WHERE id = 'new-activity'`); err != nil {
		t.Fatalf("set new activity time: %v", err)
	}

	resp, err := deps.svc.GetWorkspacesAggregate(context.Background())
	if err != nil {
		t.Fatalf("GetWorkspacesAggregate with %d workspaces: %v", workspaceCount, err)
	}
	if len(resp.Workspaces) != workspaceCount {
		t.Fatalf("workspace count = %d, want %d", len(resp.Workspaces), workspaceCount)
	}
	var last dashboard.WorkspaceAggregateEntry
	for _, entry := range resp.Workspaces {
		if entry.WorkspaceID == lastID {
			last = entry
			break
		}
	}
	if last.BlockedTasks != 1 || last.PendingApprovals != 1 {
		t.Fatalf("last workspace counts = %+v, want one blocked task and approval", last)
	}
	if len(resp.RecentActivity) != 2 || resp.RecentActivity[0].ID != "new-activity" {
		t.Fatalf("recent activity = %+v, want newest activity first across batches", resp.RecentActivity)
	}
}

// TestGetWorkspacesAggregateListerError pins that a lister failure propagates.
func TestGetWorkspacesAggregateListerError(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkspaceLister(&stubWorkspaceLister{err: errors.New("boom")})

	if _, err := deps.svc.GetWorkspacesAggregate(context.Background()); err == nil {
		t.Fatal("expected lister error to propagate, got nil")
	}
}
