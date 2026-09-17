package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
)

func TestTaskCallbacksPreserveEveryResultAndDeduplicateTransitionEvents(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.ExecSQL(t, `INSERT OR IGNORE INTO workspaces(id) VALUES('ws-1')`)
	createTestAgent(t, svc, "ws-1", "coordinator")
	svc.ExecSQL(t, `INSERT INTO workspace_orchestrators(agent_id,workspace_id,role_id) VALUES('coordinator','ws-1','chief-of-staff')`)
	for _, id := range []string{"result-one", "result-two"} {
		insertTestTask(t, svc, id, "ws-1")
		svc.ExecSQL(t, `UPDATE tasks SET state='COMPLETED',metadata='{"orchestration_chief_id":"coordinator"}' WHERE id=?`, id)
		// Different events for the same committed transition must still deduplicate.
		for _, subject := range []string{events.TaskStateChanged, events.TaskMoved, events.TaskStateChanged} {
			if err := eb.Publish(ctx, subject, bus.NewEvent(subject, "test", map[string]string{"task_id": id})); err != nil {
				t.Fatal(err)
			}
		}
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, run := range runs {
		if run.Reason != "workspace_task_callback" {
			continue
		}
		var payload struct {
			TaskID   string `json:"task_id"`
			Callback struct {
				TaskID string `json:"task_id"`
				State  string `json:"state"`
			} `json:"callback"`
		}
		if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.TaskID == payload.Callback.TaskID || payload.Callback.State != "COMPLETED" {
			t.Fatalf("wrong callback destination/payload: %s", run.Payload)
		}
		if seen[payload.Callback.TaskID] {
			t.Fatalf("duplicate callback: %s", run.Payload)
		}
		reply := "Completed " + payload.Callback.TaskID
		event := bus.NewEvent(events.AgentTurnMessageSaved, "orchestrator", map[string]string{
			"task_id": payload.TaskID, "session_id": "coordinator-session",
			"agent_text": reply, "agent_id": "coordinator",
		})
		if err := eb.Publish(ctx, events.AgentTurnMessageSaved, event); err != nil {
			t.Fatal(err)
		}
		comments, err := svc.ListComments(ctx, payload.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, comment := range comments {
			if comment.Body == reply {
				found = true
			}
		}
		if !found {
			t.Fatal("callback reply did not land in the coordinator chat")
		}
		seen[payload.Callback.TaskID] = true
	}
	after, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(runs) {
		t.Fatal("callback reply triggered a self-wake loop")
	}
	if len(seen) != 2 {
		t.Fatalf("expected both task callbacks, got %v (%d runs)", seen, len(runs))
	}
}

func TestTaskCallbackPromptRequestsAChatUpdateWithoutDeclaringOtherTasksDone(t *testing.T) {
	prompt := service.BuildPrompt(&service.PromptContext{Reason: service.RunReasonWorkspaceTaskCallback, TaskID: "chief-chat", WorkspaceTaskCallback: &service.WorkspaceTaskCallback{TaskID: "delivery", WorkspaceID: "ws", Title: "Performance review", State: "REVIEW"}})
	for _, text := range []string{"kandev task inspect --id delivery", "this conversation (chief-chat)", "/t/delivery?workspaceId=ws", "REVIEW means ready for review, not fully completed", "not all workspace tasks"} {
		if !strings.Contains(prompt, text) {
			t.Fatalf("missing %q in callback prompt: %s", text, prompt)
		}
	}
	if strings.Contains(prompt, "All child tasks") {
		t.Fatalf("callback claimed every task is done: %s", prompt)
	}
}
