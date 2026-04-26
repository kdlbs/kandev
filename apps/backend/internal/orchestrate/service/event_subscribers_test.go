package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// newTestServiceWithBus creates a service wired to an in-memory event bus.
func newTestServiceWithBus(t *testing.T) (*service.Service, bus.EventBus) {
	t.Helper()
	svc := newTestService(t)
	log := logger.Default()
	eb := bus.NewMemoryEventBus(log)
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}
	return svc, eb
}

func TestCommentCreated_RelaysAgentComment(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	var relayed atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Set up a relay with a test HTTP client.
	relay := service.NewChannelRelayWithClient(svc, ts.Client())
	svc.SetRelay(relay)

	// Create agent + channel.
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "relay-test",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	config := `{"webhook_url":"` + ts.URL + `"}`
	channel := &models.Channel{
		WorkspaceID:     "ws-1",
		AgentInstanceID: agent.ID,
		Platform:        "webhook",
		Config:          config,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	// Create an agent comment on the channel task.
	comment := &models.TaskComment{
		TaskID:         channel.TaskID,
		AuthorType:     "agent",
		AuthorID:       agent.ID,
		Body:           "Status update from agent",
		ReplyChannelID: channel.ID,
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if !relayed.Load() {
		t.Error("expected agent comment to be relayed to webhook")
	}
}

func TestCommentCreated_UserComment_NotRelayed(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	var relayed atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	relay := service.NewChannelRelayWithClient(svc, ts.Client())
	svc.SetRelay(relay)

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "relay-test-user",
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	config := `{"webhook_url":"` + ts.URL + `"}`
	channel := &models.Channel{
		WorkspaceID:     "ws-1",
		AgentInstanceID: agent.ID,
		Platform:        "webhook",
		Config:          config,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}

	comment := &models.TaskComment{
		TaskID:         channel.TaskID,
		AuthorType:     "user",
		AuthorID:       "user-1",
		Body:           "User message",
		ReplyChannelID: channel.ID,
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if relayed.Load() {
		t.Error("user comments should not be relayed")
	}
}

func TestMovedToDone_WithExecutionPolicy_EntersReview(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	// Create reviewer and worker agents.
	createTestAgent(t, svc, "ws-1", "reviewer-1")
	createTestAgent(t, svc, "ws-1", "worker-1")

	taskID := createOrchestrateTask(t, svc, "ws-1", "worker-1")

	// Set an execution policy with a review stage.
	policy := service.ExecutionPolicy{
		Stages: []service.ExecutionStage{{
			ID:   "review",
			Type: "review",
			Participants: []service.ExecutionParticipant{
				{Type: "agent", AgentID: "reviewer-1"},
			},
			ApprovalsNeeded: 1,
		}},
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	if err := svc.SetTaskExecutionPolicy(ctx, taskID, string(raw)); err != nil {
		t.Fatalf("set execution policy: %v", err)
	}

	// Simulate a task.moved event to the "Done" step.
	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                    taskID,
		"workspace_id":               "ws-1",
		"from_step_id":               "step-1",
		"to_step_id":                 "step-done",
		"to_step_name":               "Done",
		"from_step_name":             "In Progress",
		"assignee_agent_instance_id": "worker-1",
		"parent_id":                  "",
		"execution_policy":           "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	// The reviewer should receive a wakeup for the review stage.
	wakeups, err := svc.ListWakeupRequests(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	found := false
	for _, w := range wakeups {
		if w.AgentInstanceID == "reviewer-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected reviewer-1 to receive a wakeup after task moved to Done with execution policy")
	}
}

func TestMovedToDone_WithoutExecutionPolicy_NormalCompletion(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-2")
	taskID := createOrchestrateTask(t, svc, "ws-1", "worker-2")

	// No execution policy -- normal done path.
	moveEvent := bus.NewEvent("task.moved", "test", map[string]string{
		"task_id":                    taskID,
		"workspace_id":               "ws-1",
		"from_step_id":               "step-1",
		"to_step_id":                 "step-done",
		"to_step_name":               "Done",
		"from_step_name":             "In Progress",
		"assignee_agent_instance_id": "worker-2",
		"parent_id":                  "",
		"execution_policy":           "",
	})
	if err := eb.Publish(ctx, "task.moved", moveEvent); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	// No review wakeups should be created (only the setup channel wakeup from createOrchestrateTask).
	wakeups, err := svc.ListWakeupRequests(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list wakeups: %v", err)
	}
	for _, w := range wakeups {
		if w.Reason == "task_assigned" {
			// Check it's not a review_request wakeup by examining the payload.
			var payload map[string]string
			if json.Unmarshal([]byte(w.Payload), &payload) == nil {
				if payload["stage_type"] == "review" {
					t.Error("no review wakeups should exist for tasks without execution policy")
				}
			}
		}
	}
}
