package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// TestIncomingDiff_EmptyDB_NewFSAgents shows added/created when DB is empty
// and FS has agents.
func TestIncomingDiff_EmptyDB_NewFSAgents(t *testing.T) {
	svc, _ := newTestServiceWithConfig(t)
	ctx := context.Background()

	// Write an agent to FS via ApplyOutgoing on a temporary in-memory bundle is
	// not available here; use the writer directly through ApplyOutgoing after
	// seeding the DB, then nuke the DB row to simulate "FS only".
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "default", Name: "fs-agent", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.ApplyOutgoing(ctx, "default"); err != nil {
		t.Fatalf("apply outgoing: %v", err)
	}
	// Wipe DB
	agents, _ := svc.ListAgentsFromConfig(ctx, "")
	for _, a := range agents {
		if err := svc.DeleteAgentInstance(ctx, a.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}

	diff, err := svc.IncomingDiff(ctx, "default")
	if err != nil {
		t.Fatalf("incoming diff: %v", err)
	}
	if got := len(diff.Preview.Agents.Created); got != 1 {
		t.Errorf("created = %d, want 1", got)
	}
}

// TestIncomingDiff_EmptyFS_DeletedAgents shows existing DB rows in Deleted
// when FS has nothing.
func TestIncomingDiff_EmptyFS_DeletedAgents(t *testing.T) {
	svc, _ := newTestServiceWithConfig(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "default", Name: "ghost", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	diff, err := svc.IncomingDiff(ctx, "default")
	if err != nil {
		t.Fatalf("incoming diff: %v", err)
	}
	if got := len(diff.Preview.Agents.Deleted); got != 1 {
		t.Errorf("deleted = %d, want 1", got)
	}
}

// TestOutgoingDiff_EmptyFS_NewDBAgents shows DB→FS adds when FS is empty.
func TestOutgoingDiff_EmptyFS_NewDBAgents(t *testing.T) {
	svc, _ := newTestServiceWithConfig(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "default", Name: "to-export", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	diff, err := svc.OutgoingDiff(ctx, "default")
	if err != nil {
		t.Fatalf("outgoing diff: %v", err)
	}
	if got := len(diff.Preview.Agents.Created); got != 1 {
		t.Errorf("outgoing created = %d, want 1", got)
	}
}

// TestApplyOutgoing_WritesToFS verifies a roundtrip write to disk.
func TestApplyOutgoing_WritesToFS(t *testing.T) {
	svc, _ := newTestServiceWithConfig(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "default", Name: "roundtrip", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.ApplyOutgoing(ctx, "default"); err != nil {
		t.Fatalf("apply outgoing: %v", err)
	}
	bundle, _, err := svc.ScanFilesystem(ctx, "default")
	if err != nil {
		t.Fatalf("scan fs: %v", err)
	}
	if len(bundle.Agents) != 1 || bundle.Agents[0].Name != "roundtrip" {
		t.Errorf("bundle = %+v, want one agent named roundtrip", bundle.Agents)
	}
}

// TestApplyIncoming_DeletesMissingDBRows verifies that rows missing from FS
// are removed from the DB after apply.
func TestApplyIncoming_DeletesMissingDBRows(t *testing.T) {
	svc, _ := newTestServiceWithConfig(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		WorkspaceID: "default", Name: "to-be-deleted", Role: models.AgentRoleWorker,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ApplyIncoming(ctx, "default"); err != nil {
		t.Fatalf("apply incoming: %v", err)
	}
	got, err := svc.ListAgentsFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("agents after apply = %d, want 0", len(got))
	}
}
