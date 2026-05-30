package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// lastTurnCompletedHadOutput returns the had_output flag of the most recently
// published turn.completed event, and whether such an event was found.
func lastTurnCompletedHadOutput(t *testing.T, eventBus *MockEventBus) (bool, bool) {
	t.Helper()
	published := eventBus.GetPublishedEvents()
	for i := len(published) - 1; i >= 0; i-- {
		ev := published[i]
		if ev.Type != events.TurnCompleted {
			continue
		}
		data, ok := ev.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("turn.completed event data is not a map: %T", ev.Data)
		}
		hadOutput, ok := data["had_output"].(bool)
		if !ok {
			t.Fatalf("turn.completed event missing had_output bool: %v", data["had_output"])
		}
		return hadOutput, true
	}
	return false, false
}

func TestCompleteTurn_PublishesHadOutput(t *testing.T) {
	tests := []struct {
		name string
		msgs []*models.Message
		want bool
	}{
		{
			name: "empty turn with only launch notices",
			msgs: []*models.Message{
				{Type: models.MessageTypeScriptExecution, Content: "Environment prepared"},
				{Type: models.MessageTypeStatus, Content: "Started agent Mock"},
			},
			want: false,
		},
		{
			name: "turn with an agent text response",
			msgs: []*models.Message{
				{Type: models.MessageTypeMessage, Content: "Here is the answer"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, eventBus, repo := createTestService(t)
			ctx := context.Background()
			setupTestTask(t, repo)
			sessionID := setupTestSession(t, repo)

			turn := &models.Turn{
				ID:            "turn-output",
				TaskSessionID: sessionID,
				TaskID:        "task-123",
				StartedAt:     time.Now().UTC(),
			}
			if err := repo.CreateTurn(ctx, turn); err != nil {
				t.Fatalf("CreateTurn: %v", err)
			}
			for i, m := range tt.msgs {
				m.ID = fmt.Sprintf("msg-%d", i)
				m.TaskSessionID = sessionID
				m.TaskID = "task-123"
				m.TurnID = turn.ID
				m.AuthorType = models.MessageAuthorAgent
				if err := repo.CreateMessage(ctx, m); err != nil {
					t.Fatalf("CreateMessage: %v", err)
				}
			}

			eventBus.ClearEvents()
			if err := svc.CompleteTurn(ctx, turn.ID); err != nil {
				t.Fatalf("CompleteTurn: %v", err)
			}

			got, found := lastTurnCompletedHadOutput(t, eventBus)
			if !found {
				t.Fatal("expected a turn.completed event to be published")
			}
			if got != tt.want {
				t.Errorf("had_output = %v, want %v", got, tt.want)
			}
		})
	}
}

// AbandonOpenTurns sweeps orphan turns on resume; it must report had_output=true
// so the frontend never shows an empty-turn notice for a swept orphan.
func TestAbandonOpenTurns_PublishesHadOutputTrue(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	turn := &models.Turn{
		ID:            "turn-orphan",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	eventBus.ClearEvents()
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	got, found := lastTurnCompletedHadOutput(t, eventBus)
	if !found {
		t.Fatal("expected a turn.completed event to be published")
	}
	if !got {
		t.Error("had_output = false, want true (orphan sweep must suppress the notice)")
	}
}

func TestTurnHadAgentOutput(t *testing.T) {
	const turnID = "turn-1"

	agentMsg := func(t models.MessageType, content string) *models.Message {
		return &models.Message{TurnID: turnID, AuthorType: models.MessageAuthorAgent, Type: t, Content: content}
	}

	tests := []struct {
		name string
		msgs []*models.Message
		want bool
	}{
		{
			name: "no messages",
			msgs: nil,
			want: false,
		},
		{
			name: "only a user prompt",
			msgs: []*models.Message{
				{TurnID: turnID, AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "/pr-fixup"},
			},
			want: false,
		},
		{
			name: "agent text response",
			msgs: []*models.Message{agentMsg(models.MessageTypeMessage, "Here is the answer")},
			want: true,
		},
		{
			name: "agent default-typed text (empty type)",
			msgs: []*models.Message{agentMsg("", "Done")},
			want: true,
		},
		{
			name: "agent message with only whitespace content",
			msgs: []*models.Message{agentMsg(models.MessageTypeMessage, "   \n  ")},
			want: false,
		},
		{
			name: "agent tool call",
			msgs: []*models.Message{agentMsg(models.MessageTypeToolCall, "")},
			want: true,
		},
		{
			name: "agent content chunk",
			msgs: []*models.Message{agentMsg(models.MessageTypeContent, "partial")},
			want: true,
		},
		{
			name: "agent plan",
			msgs: []*models.Message{agentMsg(models.MessageTypeAgentPlan, "")},
			want: true,
		},
		{
			name: "todo is visible output",
			msgs: []*models.Message{agentMsg(models.MessageTypeTodo, "")},
			want: true,
		},
		{
			name: "a permission request is visible output",
			msgs: []*models.Message{agentMsg(models.MessageTypePermissionRequest, "")},
			want: true,
		},
		{
			name: "a clarification request is visible output",
			msgs: []*models.Message{agentMsg(models.MessageTypeClarificationRequest, "")},
			want: true,
		},
		{
			name: "lifecycle status notices are not output",
			msgs: []*models.Message{agentMsg(models.MessageTypeStatus, "Started agent Mock")},
			want: false,
		},
		{
			name: "script execution notices are not output",
			msgs: []*models.Message{agentMsg(models.MessageTypeScriptExecution, "Environment prepared")},
			want: false,
		},
		{
			name: "only thinking is not output",
			msgs: []*models.Message{agentMsg(models.MessageTypeThinking, "hmm")},
			want: false,
		},
		{
			name: "only a log line is not output",
			msgs: []*models.Message{agentMsg(models.MessageTypeLog, "debug")},
			want: false,
		},
		{
			name: "only progress is not output",
			msgs: []*models.Message{agentMsg(models.MessageTypeProgress, "50%")},
			want: false,
		},
		{
			name: "an empty first turn with only launch notices",
			msgs: []*models.Message{
				{TurnID: turnID, AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "/e2e:empty-turn"},
				agentMsg(models.MessageTypeScriptExecution, "Environment prepared"),
				agentMsg(models.MessageTypeStatus, "Started agent Mock"),
			},
			want: false,
		},
		{
			name: "output belongs to a different turn",
			msgs: []*models.Message{
				{TurnID: "other-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "elsewhere"},
			},
			want: false,
		},
		{
			name: "noise plus real output",
			msgs: []*models.Message{
				agentMsg(models.MessageTypeThinking, "let me think"),
				agentMsg(models.MessageTypeLog, "log"),
				agentMsg(models.MessageTypeToolCall, ""),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := turnHadAgentOutput(tt.msgs, turnID); got != tt.want {
				t.Errorf("turnHadAgentOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}
