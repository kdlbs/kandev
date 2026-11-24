package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type captureCreationAttachments struct {
	mu    sync.Mutex
	calls []string
	rows  []v1.MessageAttachment
	err   error
}

func (c *captureCreationAttachments) ReadTaskCreationAttachments(
	_ context.Context, taskID string,
) ([]v1.MessageAttachment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, taskID)
	if c.err != nil {
		return nil, c.err
	}
	return append([]v1.MessageAttachment(nil), c.rows...), nil
}

func (c *captureCreationAttachments) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

type launchContextStarter struct {
	*mockTaskStarter
	launch service.LaunchContext
	calls  int
}

func (m *launchContextStarter) StartTaskWithLaunchContextReturningSession(
	_ context.Context, _ string, _ string, launch service.LaunchContext,
) (string, error) {
	m.launch = launch
	m.calls++
	return "session-creation-attachments", nil
}

func seedCreationAttachmentLaunch(t *testing.T, svc *service.Service, taskID, agentID string) {
	t.Helper()
	ctx := context.Background()
	agent := makeAgent(agentID, models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_pc"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, priority, created_at, updated_at)
		VALUES (?, 'ws-1', 'Creation attachments', 'desc', 'medium', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
}

func creationAttachmentRows() []v1.MessageAttachment {
	return []v1.MessageAttachment{
		{Type: "image", AttachmentID: "attach-1", MimeType: "image/png", Name: "one.png", SizeBytes: 4, DeliveryMode: "prompt"},
		{Type: "resource", AttachmentID: "attach-2", MimeType: "application/zip", Name: "two.zip", SizeBytes: 5, DeliveryMode: "path"},
	}
}

func TestSchedulerTickCreationAttachments(t *testing.T) {
	t.Run("direct launch receives the ordered creation set", func(t *testing.T) {
		starter := &launchContextStarter{mockTaskStarter: &mockTaskStarter{}}
		svc := newTestService(t, service.ServiceOptions{TaskStarter: starter, APIBaseURL: "http://localhost:8080/api/v1"})
		svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "test-token"})
		seedCreationAttachmentLaunch(t, svc, "task-creation-direct", "agent-creation-direct")
		reader := &captureCreationAttachments{rows: creationAttachmentRows()}

		scheduler := service.NewSchedulerIntegration(svc, time.Second)
		scheduler.SetTaskCreationAttachmentReader(reader)
		scheduler.Tick(context.Background())

		if starter.calls != 1 {
			t.Fatalf("launch calls = %d, want 1", starter.calls)
		}
		if len(starter.launch.Attachments) != 2 ||
			starter.launch.Attachments[0].AttachmentID != "attach-1" ||
			starter.launch.Attachments[1].AttachmentID != "attach-2" {
			t.Fatalf("launch attachments = %+v", starter.launch.Attachments)
		}
		if reader.callCount() != 1 || reader.calls[0] != "task-creation-direct" {
			t.Fatalf("reader calls = %+v", reader.calls)
		}
	})

	t.Run("routed launch receives the same set", func(t *testing.T) {
		starter := &launchContextStarter{mockTaskStarter: &mockTaskStarter{}}
		svc := newTestService(t, service.ServiceOptions{TaskStarter: starter, APIBaseURL: "http://localhost:8080/api/v1"})
		svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "test-token"})
		dispatcher := &captureDispatcher{launched: true}
		svc.SetRoutingDispatcher(dispatcher)
		seedCreationAttachmentLaunch(t, svc, "task-creation-routed", "agent-creation-routed")
		reader := &captureCreationAttachments{rows: creationAttachmentRows()}

		scheduler := service.NewSchedulerIntegration(svc, time.Second)
		scheduler.SetTaskCreationAttachmentReader(reader)
		scheduler.Tick(context.Background())

		if starter.calls != 0 {
			t.Fatalf("direct launch calls = %d, want 0", starter.calls)
		}
		launch := dispatcher.lastCall()
		if len(launch.Attachments) != 2 || launch.Attachments[0].AttachmentID != "attach-1" {
			t.Fatalf("routed attachments = %+v", launch.Attachments)
		}
	})

	t.Run("taskless run keeps an empty set", func(t *testing.T) {
		starter := &launchContextStarter{mockTaskStarter: &mockTaskStarter{}}
		svc := newTestService(t, service.ServiceOptions{TaskStarter: starter, APIBaseURL: "http://localhost:8080/api/v1"})
		svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "test-token"})
		ctx := context.Background()
		agent := makeAgent("agent-creation-taskless", models.AgentRoleWorker)
		agent.ExecutorPreference = `{"type":"local_pc"}`
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
			t.Fatal(err)
		}
		reader := &captureCreationAttachments{rows: creationAttachmentRows()}

		scheduler := service.NewSchedulerIntegration(svc, time.Second)
		scheduler.SetTaskCreationAttachmentReader(reader)
		scheduler.Tick(ctx)

		if reader.callCount() != 0 {
			t.Fatalf("taskless run read creation attachments: %+v", reader.calls)
		}
	})

	t.Run("reader failure prevents the launch", func(t *testing.T) {
		starter := &launchContextStarter{mockTaskStarter: &mockTaskStarter{}}
		svc := newTestService(t, service.ServiceOptions{TaskStarter: starter, APIBaseURL: "http://localhost:8080/api/v1"})
		svc.SetAgentTokenMinter(fakeAgentTokenMinter{token: "test-token"})
		seedCreationAttachmentLaunch(t, svc, "task-creation-read-fail", "agent-creation-read-fail")
		reader := &captureCreationAttachments{err: errors.New("attachment registry unavailable")}

		scheduler := service.NewSchedulerIntegration(svc, time.Second)
		scheduler.SetTaskCreationAttachmentReader(reader)
		scheduler.Tick(context.Background())

		if starter.calls != 0 {
			t.Fatalf("launch calls = %d, want 0 after a failed attachment read", starter.calls)
		}
		if reader.callCount() != 1 {
			t.Fatalf("reader calls = %d, want 1", reader.callCount())
		}
	})
}
