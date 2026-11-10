package backendapp

import (
	"context"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type handoffCompletionTasksFake struct {
	origin   *taskmodels.Task
	delivery *taskmodels.Task
	grant    *taskmodels.WorkspaceCollaborationGrant
	session  *taskmodels.TaskSession
}

func (f handoffCompletionTasksFake) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if id == f.origin.ID {
		return f.origin, nil
	}
	return f.delivery, nil
}

func (f handoffCompletionTasksFake) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	if f.session != nil {
		return f.session, nil
	}
	return &taskmodels.TaskSession{TaskID: "source-task", AgentProfileID: "source-agent"}, nil
}

func (f handoffCompletionTasksFake) GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*taskmodels.WorkspaceCollaborationGrant, error) {
	return f.grant, nil
}

type handoffCompletionOfficeFake struct {
	agent *officemodels.AgentInstance
}

func (f handoffCompletionOfficeFake) GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error) {
	return f.agent, nil
}

func TestHandoffCompletionPolicyUsesDurableTaskProvenance(t *testing.T) {
	tasks := handoffCompletionTasksFake{
		origin: &taskmodels.Task{
			ID: "source-task", WorkspaceID: "source-workspace", IsFromOffice: true,
			Metadata: map[string]interface{}{
				taskmodels.MetaKeyHandoffs: []interface{}{map[string]interface{}{"task_id": "delivery-task"}},
			},
		},
		delivery: &taskmodels.Task{
			ID: "delivery-task", WorkspaceID: "delivery-workspace", State: v1.TaskStateCompleted,
		},
		grant: &taskmodels.WorkspaceCollaborationGrant{Report: true},
	}
	office := handoffCompletionOfficeFake{
		agent: &officemodels.AgentInstance{ID: "source-agent", WorkspaceID: "source-workspace"},
	}
	policy := newOfficeHandoffCompletionPolicy(true, tasks, office)
	authorization, err := policy(context.Background(), officeservice.HandoffCompletionSource{
		DeliveryTaskID: "delivery-task", DeliveryWorkspaceID: "delivery-workspace", DeliveryState: "COMPLETED",
		SourceTaskID: "source-task", SourceWorkspaceID: "source-workspace", SourceSessionID: "source-session",
		SourceAgentProfileID: "source-agent", SourceAssigneeAgentProfileID: "source-agent",
	})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if !authorization.Report || authorization.Prompt {
		t.Fatalf("authorization = %+v, want report-only permission from the current grant", authorization)
	}
}

func TestHandoffCompletionPolicyRequiresSourceReverseLink(t *testing.T) {
	tasks := handoffCompletionTasksFake{
		origin: &taskmodels.Task{
			ID: "source-task", WorkspaceID: "source-workspace", IsFromOffice: true,
			Metadata: map[string]interface{}{},
		},
		delivery: &taskmodels.Task{ID: "delivery-task", WorkspaceID: "delivery-workspace", State: v1.TaskStateCompleted},
		grant:    &taskmodels.WorkspaceCollaborationGrant{Report: true, Prompt: true},
	}
	office := handoffCompletionOfficeFake{
		agent: &officemodels.AgentInstance{ID: "source-agent", WorkspaceID: "source-workspace"},
	}
	policy := newOfficeHandoffCompletionPolicy(true, tasks, office)
	authorization, err := policy(context.Background(), officeservice.HandoffCompletionSource{
		DeliveryTaskID: "delivery-task", DeliveryWorkspaceID: "delivery-workspace", DeliveryState: "COMPLETED",
		SourceTaskID: "source-task", SourceWorkspaceID: "source-workspace", SourceSessionID: "source-session",
		SourceAgentProfileID: "source-agent",
	})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if authorization.Report || authorization.Prompt {
		t.Fatalf("authorization = %+v, want no permission without the source task reverse link", authorization)
	}
}

func TestHandoffCompletionPolicyRequiresSourceAgentSessionBinding(t *testing.T) {
	tasks := handoffCompletionTasksFake{
		origin: &taskmodels.Task{
			ID: "source-task", WorkspaceID: "source-workspace", IsFromOffice: true,
			Metadata: map[string]interface{}{
				taskmodels.MetaKeyHandoffs: []interface{}{map[string]interface{}{"task_id": "delivery-task"}},
			},
		},
		delivery: &taskmodels.Task{ID: "delivery-task", WorkspaceID: "delivery-workspace", State: v1.TaskStateCompleted},
		grant:    &taskmodels.WorkspaceCollaborationGrant{Report: true, Prompt: true},
		session:  &taskmodels.TaskSession{TaskID: "source-task", AgentProfileID: "different-agent"},
	}
	office := handoffCompletionOfficeFake{
		agent: &officemodels.AgentInstance{ID: "source-agent", WorkspaceID: "source-workspace"},
	}
	policy := newOfficeHandoffCompletionPolicy(true, tasks, office)
	authorization, err := policy(context.Background(), officeservice.HandoffCompletionSource{
		DeliveryTaskID: "delivery-task", DeliveryWorkspaceID: "delivery-workspace", DeliveryState: "COMPLETED",
		SourceTaskID: "source-task", SourceWorkspaceID: "source-workspace", SourceSessionID: "source-session",
		SourceAgentProfileID: "source-agent",
	})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if authorization.Report || authorization.Prompt {
		t.Fatalf("authorization = %+v, want no permission for a session owned by another agent", authorization)
	}
}
