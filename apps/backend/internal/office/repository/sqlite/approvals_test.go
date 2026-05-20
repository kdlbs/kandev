package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func TestApproval_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID: "ws-1",
		Type:        "hire_agent",
		Status:      "pending",
		Payload:     `{"agent_name":"new-agent"}`,
	}
	if err := repo.CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetApproval(ctx, approval.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != "hire_agent" {
		t.Errorf("type = %q, want %q", got.Type, "hire_agent")
	}

	approvals, err := repo.ListApprovals(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("list count = %d, want 1", len(approvals))
	}

	now := time.Now().UTC()
	approval.Status = "approved"
	approval.DecidedBy = "user-1"
	approval.DecidedAt = &now
	if err := repo.UpdateApproval(ctx, approval); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.GetApproval(ctx, approval.ID)
	if got.Status != "approved" {
		t.Errorf("status = %q, want %q", got.Status, "approved")
	}
}
