package service

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func permissionRow(pendingID, status string, createdAt time.Time) *models.Message {
	metadata := map[string]interface{}{
		"pending_id":   pendingID,
		"tool_call_id": "call-9",
		"action_type":  "execute",
		"options": []interface{}{
			map[string]interface{}{"option_id": "allow", "name": "Allow once", "kind": "allow_once"},
			map[string]interface{}{"option_id": "deny", "name": "Deny", "kind": "reject_once"},
			map[string]interface{}{"name": "Malformed option without an id"},
		},
	}
	if status != "" {
		metadata["status"] = status
	}
	return &models.Message{
		ID: "msg-" + pendingID, TaskSessionID: "session-1", TaskID: "task-1", TurnID: "turn-1",
		Type: models.MessageTypePermissionRequest, Content: "Run rm -rf /tmp/x?",
		Metadata: metadata, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func clarificationRow(
	id, pendingID, questionID, status string, index int, disconnected bool, at time.Time,
) *models.Message {
	metadata := map[string]interface{}{
		"pending_id":     pendingID,
		"question_id":    questionID,
		"question_index": index,
		"context":        "shared context",
		"status":         status,
		"question": map[string]interface{}{
			"id":     questionID,
			"title":  "Title " + questionID,
			"prompt": "Prompt " + questionID,
			"options": []interface{}{
				map[string]interface{}{"option_id": "a", "label": "A", "description": "first"},
				map[string]interface{}{"option_id": "b", "label": "B"},
			},
		},
	}
	if disconnected {
		metadata["agent_disconnected"] = true
	}
	return &models.Message{
		ID: id, TaskSessionID: "session-2", TaskID: "task-2", TurnID: "turn-2",
		Type: models.MessageTypeClarificationRequest, Content: "Prompt " + questionID,
		Metadata: metadata, CreatedAt: at, UpdatedAt: at,
	}
}

func TestAssembleInteractionsBuildsPermission(t *testing.T) {
	at := time.Date(2026, time.August, 21, 8, 0, 0, 0, time.UTC)
	got := assembleInteractions([]*models.Message{permissionRow("pending-1", "", at)})
	if len(got) != 1 {
		t.Fatalf("interactions = %d, want 1", len(got))
	}
	interaction := got[0]
	if interaction.ID != "pending-1" || interaction.Kind != models.InteractionKindPermission {
		t.Fatalf("id/kind = %q/%q", interaction.ID, interaction.Kind)
	}
	if interaction.Status != models.InteractionStatusPending {
		t.Fatalf("status = %q, want pending (absent metadata.status is the pending sentinel)", interaction.Status)
	}
	if interaction.Title != "Run rm -rf /tmp/x?" || interaction.ToolCallID != "call-9" || interaction.ActionType != "execute" {
		t.Fatalf("permission fields = %+v", interaction)
	}
	// The malformed third option carries no option_id, so no response could
	// ever name it; surfacing it would offer a choice the agent cannot accept.
	if len(interaction.Options) != 2 {
		t.Fatalf("options = %+v, want the two id-carrying options", interaction.Options)
	}
	if interaction.Options[0].ID != "allow" || interaction.Options[0].Label != "Allow once" ||
		interaction.Options[0].Kind != "allow_once" {
		t.Fatalf("option[0] = %+v (label must come from the persisted ACP \"name\")", interaction.Options[0])
	}
	if interaction.Options[1].Kind != "reject_once" {
		t.Fatalf("option[1].Kind = %q, want reject_once", interaction.Options[1].Kind)
	}
}

func TestAssembleInteractionsCollapsesClarificationBundle(t *testing.T) {
	at := time.Date(2026, time.August, 21, 9, 0, 0, 0, time.UTC)
	rows := []*models.Message{
		// Deliberately out of question_index order: the bundle must be
		// reassembled by index, not by row order.
		clarificationRow("m2", "pending-2", "q2", "pending", 1, true, at.Add(time.Minute)),
		clarificationRow("m1", "pending-2", "q1", "answered", 0, false, at),
	}
	got := assembleInteractions(rows)
	if len(got) != 1 {
		t.Fatalf("interactions = %d, want 1 (bundle collapses to one interaction)", len(got))
	}
	interaction := got[0]
	if interaction.Kind != models.InteractionKindClarification {
		t.Fatalf("kind = %q", interaction.Kind)
	}
	if len(interaction.Questions) != 2 ||
		interaction.Questions[0].ID != "q1" || interaction.Questions[1].ID != "q2" {
		t.Fatalf("questions = %+v, want q1 then q2", interaction.Questions)
	}
	// The agent stays blocked until EVERY question is answered, so one
	// answered question must not make the bundle read as resolved.
	if interaction.Status != models.InteractionStatusPending {
		t.Fatalf("status = %q, want pending while any question is unanswered", interaction.Status)
	}
	if !interaction.AgentDisconnected {
		t.Fatal("AgentDisconnected = false, want true when any row is detached")
	}
	if !interaction.CreatedAt.Equal(at) || !interaction.UpdatedAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("timestamps = %v..%v, want the bundle span", interaction.CreatedAt, interaction.UpdatedAt)
	}
	if interaction.Context != "shared context" || interaction.Title != "Prompt q1" {
		t.Fatalf("context/title = %q/%q", interaction.Context, interaction.Title)
	}
	if len(interaction.Questions[0].Options) != 2 || interaction.Questions[0].Options[0].Description != "first" {
		t.Fatalf("question options = %+v", interaction.Questions[0].Options)
	}
}

func TestAssembleInteractionsTerminalBundle(t *testing.T) {
	at := time.Date(2026, time.August, 21, 10, 0, 0, 0, time.UTC)
	got := assembleInteractions([]*models.Message{
		clarificationRow("m1", "pending-3", "q1", "answered", 0, false, at),
		clarificationRow("m2", "pending-3", "q2", "answered", 1, false, at),
	})
	if len(got) != 1 {
		t.Fatalf("interactions = %d, want 1 (bundle collapses to one interaction)", len(got))
	}
	if got[0].Status != models.InteractionStatusAnswered {
		t.Fatalf("status = %q, want answered once every question is", got[0].Status)
	}
	if !got[0].Status.IsTerminal() {
		t.Fatal("answered bundle must report terminal")
	}
}

func TestAssembleInteractionsGroupsAndSkipsUnkeyedRows(t *testing.T) {
	at := time.Date(2026, time.August, 21, 11, 0, 0, 0, time.UTC)
	orphan := clarificationRow("orphan", "", "q1", "pending", 0, false, at)
	delete(orphan.Metadata, "pending_id")
	got := assembleInteractions([]*models.Message{
		permissionRow("pending-a", "", at),
		orphan,
		clarificationRow("m1", "pending-b", "q1", "pending", 0, false, at),
		permissionRow("pending-c", "approved", at),
	})
	if len(got) != 3 {
		t.Fatalf("interactions = %d, want 3 (a row with no pending_id has no id to respond to)", len(got))
	}
	if got[0].ID != "pending-a" || got[1].ID != "pending-b" || got[2].ID != "pending-c" {
		t.Fatalf("ids = %q/%q/%q, want first-seen order preserved", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[2].Status != models.InteractionStatusApproved {
		t.Fatalf("terminal permission status = %q, want approved", got[2].Status)
	}
}

func TestInteractionStatusFromMetadataPreservesUnknownTerminalValues(t *testing.T) {
	if got := models.InteractionStatusFromMetadata(nil); got != models.InteractionStatusPending {
		t.Fatalf("nil metadata = %q, want pending", got)
	}
	if got := models.InteractionStatusFromMetadata(map[string]interface{}{"status": ""}); got != models.InteractionStatusPending {
		t.Fatalf("empty status = %q, want pending", got)
	}
	// An unrecognized value must NOT collapse to pending: reporting an unknown
	// terminal state as "still owed a response" is the false positive this
	// whole projection exists to prevent.
	got := models.InteractionStatusFromMetadata(map[string]interface{}{"status": "some_future_state"})
	if got != models.InteractionStatus("some_future_state") || !got.IsTerminal() {
		t.Fatalf("unknown status = %q (terminal=%v), want it preserved and terminal", got, got.IsTerminal())
	}
}
