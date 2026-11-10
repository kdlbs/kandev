package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// The Office service is a pass-through for agent task relations: whatever the
// runtime validated reaches the task creator unchanged, on both paths.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.1
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.1
func TestCreateOfficeTaskAsAgent_ForwardsRelations(t *testing.T) {
	mock := &mockSubtaskCreator{}
	svc := newTestService(t, service.ServiceOptions{TaskCreator: mock})
	relations := shared.AgentTaskRelations{BlockedBy: []string{"a", "b"}, WorkspaceMode: "new_workspace"}

	if _, err := svc.CreateOfficeTaskAsAgent(
		context.Background(), "", "ws-1", "project-1", "", "Join", "", "", relations,
	); err != nil {
		t.Fatalf("CreateOfficeTaskAsAgent: %v", err)
	}
	if len(mock.agentCalls) != 1 {
		t.Fatalf("root calls = %d, want 1", len(mock.agentCalls))
	}
	assertForwardedRelations(t, mock.agentCalls[0].Relations, relations)

	if _, err := svc.CreateOfficeSubtaskAsAgent(
		context.Background(), "", "parent-1", "", "Batch", "", "", relations,
	); err != nil {
		t.Fatalf("CreateOfficeSubtaskAsAgent: %v", err)
	}
	if len(mock.subtaskCalls) != 1 {
		t.Fatalf("subtask calls = %d, want 1", len(mock.subtaskCalls))
	}
	assertForwardedRelations(t, mock.subtaskCalls[0].Relations, relations)
}

func assertForwardedRelations(t *testing.T, got, want shared.AgentTaskRelations) {
	t.Helper()
	if got.WorkspaceMode != want.WorkspaceMode || strings.Join(got.BlockedBy, ",") != strings.Join(want.BlockedBy, ",") {
		t.Fatalf("forwarded relations = %#v, want %#v", got, want)
	}
}
