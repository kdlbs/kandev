package backendapp

import (
	"context"
	"database/sql"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeHandoffCompletionTasks struct {
	tasks    map[string]*taskmodels.Task
	sessions map[string]*taskmodels.TaskSession
	grants   map[string]*taskmodels.WorkspaceCollaborationGrant
}

func (f *fakeHandoffCompletionTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	task, ok := f.tasks[id]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return task, nil
}

func (f *fakeHandoffCompletionTasks) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	session, ok := f.sessions[id]
	if !ok {
		return nil, taskmodels.ErrTaskSessionNotFound
	}
	return session, nil
}

func (f *fakeHandoffCompletionTasks) GetWorkspaceCollaborationGrant(
	_ context.Context, taskID, sourceID, targetID string,
) (*taskmodels.WorkspaceCollaborationGrant, error) {
	return f.grants[taskID+"|"+sourceID+"|"+targetID], nil
}

type fakeHandoffCompletionOffice struct {
	runMatches bool
	agents     map[string]string
}

func (f *fakeHandoffCompletionOffice) HasHandoffSourceRun(context.Context, string, string, string) (bool, error) {
	return f.runMatches, nil
}

func (f *fakeHandoffCompletionOffice) GetAgentInstance(_ context.Context, id string) (*officemodels.AgentInstance, error) {
	workspaceID, ok := f.agents[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &officemodels.AgentInstance{ID: id, WorkspaceID: workspaceID}, nil
}

func handoffCompletionFixture() (
	officeservice.HandoffCompletionSource, *fakeHandoffCompletionTasks, *fakeHandoffCompletionOffice,
) {
	source := officeservice.HandoffCompletionSource{
		DeliveryTaskID: "delivery-1", DeliveryWorkspaceID: "ws-delivery", DeliveryState: "COMPLETED",
		SourceTaskID: "source-1", SourceWorkspaceID: "ws-source", SourceSessionID: "session-1",
		SourceAgentProfileID: "source-agent", SourceAssigneeAgentProfileID: "source-agent",
	}
	tasks := &fakeHandoffCompletionTasks{
		tasks: map[string]*taskmodels.Task{
			"source-1":   {ID: "source-1", WorkspaceID: "ws-source", IsFromOffice: true, State: "IN_PROGRESS"},
			"delivery-1": {ID: "delivery-1", WorkspaceID: "ws-delivery", State: "COMPLETED"},
		},
		sessions: map[string]*taskmodels.TaskSession{
			"session-1": {ID: "session-1", TaskID: "source-1"},
		},
		grants: map[string]*taskmodels.WorkspaceCollaborationGrant{
			"delivery-1|ws-delivery|ws-source": {
				CallerTaskID: "delivery-1", SourceWorkspaceID: "ws-delivery",
				TargetWorkspaceID: "ws-source", Report: true,
			},
		},
	}
	office := &fakeHandoffCompletionOffice{
		runMatches: true,
		agents: map[string]string{
			"source-agent": "ws-source",
		},
	}
	return source, tasks, office
}

func TestOfficeHandoffCompletionPolicy_AllowsExactRelationship(t *testing.T) {
	source, tasks, office := handoffCompletionFixture()
	policy := newOfficeHandoffCompletionPolicy(true, tasks, office)
	if policy == nil {
		t.Fatal("enabled policy must not be nil")
	}
	allowed, err := policy(context.Background(), source)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if !allowed {
		t.Fatal("policy must allow the stored relationship with a report grant")
	}
}

func TestOfficeHandoffCompletionPolicy_DisabledIsNil(t *testing.T) {
	_, tasks, office := handoffCompletionFixture()
	if policy := newOfficeHandoffCompletionPolicy(false, tasks, office); policy != nil {
		t.Fatal("disabled policy must be nil so the feature stays off")
	}
}

func TestOfficeHandoffCompletionPolicy_Denies(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*officeservice.HandoffCompletionSource, *fakeHandoffCompletionTasks, *fakeHandoffCompletionOffice)
	}{
		{
			name: "missing report grant",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				tasks.grants = nil
			},
		},
		{
			name: "report not allowed",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				tasks.grants["delivery-1|ws-delivery|ws-source"].Report = false
			},
		},
		{
			name: "no claimed source run",
			mutate: func(_ *officeservice.HandoffCompletionSource, _ *fakeHandoffCompletionTasks, office *fakeHandoffCompletionOffice) {
				office.runMatches = false
			},
		},
		{
			name: "stale delivery state",
			mutate: func(source *officeservice.HandoffCompletionSource, _ *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				source.DeliveryState = "FAILED"
			},
		},
		{
			name: "source task not office",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				tasks.tasks["source-1"].IsFromOffice = false
			},
		},
		{
			name: "source session belongs to another task",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				tasks.sessions["session-1"].TaskID = "other-task"
			},
		},
		{
			name: "source agent outside source workspace",
			mutate: func(_ *officeservice.HandoffCompletionSource, _ *fakeHandoffCompletionTasks, office *fakeHandoffCompletionOffice) {
				office.agents["source-agent"] = "ws-elsewhere"
			},
		},
		{
			name: "current assignee outside source workspace",
			mutate: func(source *officeservice.HandoffCompletionSource, _ *fakeHandoffCompletionTasks, office *fakeHandoffCompletionOffice) {
				source.SourceAssigneeAgentProfileID = "other-agent"
				office.agents["other-agent"] = "ws-elsewhere"
			},
		},
		{
			name: "delivery in the source workspace",
			mutate: func(source *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				source.DeliveryWorkspaceID = "ws-source"
				tasks.tasks["delivery-1"].WorkspaceID = "ws-source"
			},
		},
		{
			name: "missing source task",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				delete(tasks.tasks, "source-1")
			},
		},
		{
			name: "missing delivery task",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				delete(tasks.tasks, "delivery-1")
			},
		},
		{
			name: "missing source session",
			mutate: func(_ *officeservice.HandoffCompletionSource, tasks *fakeHandoffCompletionTasks, _ *fakeHandoffCompletionOffice) {
				delete(tasks.sessions, "session-1")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, tasks, office := handoffCompletionFixture()
			tc.mutate(&source, tasks, office)
			policy := newOfficeHandoffCompletionPolicy(true, tasks, office)
			allowed, err := policy(context.Background(), source)
			if err != nil {
				t.Fatalf("policy: %v", err)
			}
			if allowed {
				t.Fatal("policy must deny this relationship")
			}
		})
	}
}
