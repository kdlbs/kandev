package dto

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskPlanFromModelIncludesImplementationMarker(t *testing.T) {
	startedAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	sessionID := "session-1"
	actor := "user"

	out := TaskPlanFromModel(&models.TaskPlan{
		ID:                             "plan-1",
		TaskID:                         "task-1",
		Title:                          "Plan",
		Content:                        "Implement it",
		CreatedBy:                      "user",
		CreatedAt:                      startedAt.Add(-time.Hour),
		UpdatedAt:                      startedAt,
		ImplementationStartedAt:        &startedAt,
		ImplementationStartedSessionID: &sessionID,
		ImplementationStartedBy:        &actor,
	})

	if out.ImplementationStartedAt == nil || !out.ImplementationStartedAt.Equal(startedAt) {
		t.Fatalf("expected implementation_started_at %s, got %v", startedAt, out.ImplementationStartedAt)
	}
	if out.ImplementationStartedSessionID == nil || *out.ImplementationStartedSessionID != sessionID {
		t.Fatalf("expected implementation_started_session_id %q, got %v", sessionID, out.ImplementationStartedSessionID)
	}
	if out.ImplementationStartedBy == nil || *out.ImplementationStartedBy != actor {
		t.Fatalf("expected implementation_started_by %q, got %v", actor, out.ImplementationStartedBy)
	}
}

func TestTaskPlanRevisionFromModelComputesContentLengthInRunes(t *testing.T) {
	// "héllo wörld 日本語" has multibyte runes; RuneCountInString must be used
	// instead of len(), which would count UTF-8 bytes rather than characters.
	content := "héllo wörld 日本語"
	out := TaskPlanRevisionFromModel(&models.TaskPlanRevision{
		ID:      "rev-1",
		TaskID:  "task-1",
		Title:   "Plan",
		Content: content,
	})

	if out.ContentLength != 15 {
		t.Fatalf("expected content_length 15 (rune count), got %d", out.ContentLength)
	}
}

func TestTaskPlanRevisionMetaFromModelKeepsContentLengthAfterBlankingContent(t *testing.T) {
	out := TaskPlanRevisionMetaFromModel(&models.TaskPlanRevision{
		ID:      "rev-1",
		TaskID:  "task-1",
		Title:   "Plan",
		Content: "12345",
	})

	if out.Content != "" {
		t.Fatalf("expected content blanked for meta payload, got %q", out.Content)
	}
	if out.ContentLength != 5 {
		t.Fatalf("expected content_length 5 to survive the content blank, got %d", out.ContentLength)
	}
}

func TestTaskPlanRevisionFromModelIncludesWorkflowStepStamp(t *testing.T) {
	out := TaskPlanRevisionFromModel(&models.TaskPlanRevision{
		ID:                "rev-1",
		TaskID:            "task-1",
		Title:             "Plan",
		Content:           "hi",
		WorkflowStepID:    "step-1",
		WorkflowStepName:  "Build",
		WorkflowStepColor: "bg-blue-500",
	})

	if out.WorkflowStepID != "step-1" || out.WorkflowStepName != "Build" || out.WorkflowStepColor != "bg-blue-500" {
		t.Fatalf("expected workflow step stamp to survive DTO conversion, got %+v", out)
	}
}
