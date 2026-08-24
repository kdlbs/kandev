package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func mustMarshalPlanPayload(t *testing.T, v map[string]any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(data)
}

func revisionWithNumber(t *testing.T, revisions []*models.TaskPlanRevision, number int) *models.TaskPlanRevision {
	t.Helper()
	for _, rev := range revisions {
		if rev.RevisionNumber == number {
			return rev
		}
	}
	t.Fatalf("no revision with number %d among %d revisions", number, len(revisions))
	return nil
}

// TestMCPPlanTruncationGuard_WarnsAndPreservesHistory pins the defect this
// card fixes: a write that drops the majority of a substantial plan today
// returns plain success with no signal that anything shrank (WO-38, task
// 809498b3, measured two incidents dropping 76-77% of a 40k+ char plan).
//
// This asserts against a response shape (plan_write_warning,
// prior_revision_number) that does not exist yet, so it fails first.
func TestMCPPlanTruncationGuard_WarnsAndPreservesHistory(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	large := strings.Repeat("x", 40000)
	small := strings.Repeat("y", 10000) // ~25% retained, matching the WO-38 magnitude

	createOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"title":   "Ship it",
			"content": large,
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	created := decodeMCPPlanPayload(t, createOut)
	if warning, ok := created["plan_write_warning"]; ok {
		t.Errorf("unexpected warning on initial create (no prior plan to truncate): %v", warning)
	}

	updateOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID,
			"content": small,
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, updateOut)
	if updated["content"] != small {
		t.Errorf("content = %v, want the new (truncated) content", updated["content"])
	}

	warning, _ := updated["plan_write_warning"].(string)
	if warning == "" {
		t.Fatal("expected a truncation warning naming the byte drop, got none")
	}
	if !strings.Contains(warning, "40000") || !strings.Contains(warning, "10000") {
		t.Errorf("warning does not name the byte drop (40000 -> 10000): %q", warning)
	}
	if !strings.Contains(strings.ToLower(warning), "entire") && !strings.Contains(strings.ToLower(warning), "whole document") {
		t.Errorf("warning does not explain the write replaced the whole document: %q", warning)
	}

	priorRev, ok := updated["prior_revision_number"].(float64)
	if !ok || int(priorRev) != 1 {
		t.Errorf("prior_revision_number = %v, want 1", updated["prior_revision_number"])
	}

	// The truncating write must NOT coalesce: revision 1 must survive as its
	// own row with the full pre-truncation content, not be overwritten
	// in-place by mergeRevisionInTx.
	revisions, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 separate revisions (no coalesce), got %d", len(revisions))
	}
	rev1 := revisionWithNumber(t, revisions, 1)
	fullRev1, err := h.planService.GetRevision(ctx, rev1.ID)
	if err != nil {
		t.Fatalf("GetRevision(rev1): %v", err)
	}
	if fullRev1.Content != large {
		t.Errorf("revision 1 content was mutated by the truncating write; got len=%d, want len=%d",
			len(fullRev1.Content), len(large))
	}
}

// TestMCPPlanTruncationGuard_SmallDropsAreQuiet pins the two false-positive
// guards from the threshold: a small plan (under the 2,000-char floor) and a
// modest, legitimate shrink (well above the 50% retain line) must not warn.
func TestMCPPlanTruncationGuard_SmallDropsAreQuiet(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	t.Run("small plan under the floor", func(t *testing.T) {
		_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID,
				"content": "a short plan",
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanTaskID,
				"content": "x",
			})))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		updated := decodeMCPPlanPayload(t, out)
		if warning, ok := updated["plan_write_warning"]; ok {
			t.Errorf("unexpected warning for a sub-floor plan: %v", warning)
		}
	})

	t.Run("legitimate prune retains more than half", func(t *testing.T) {
		large := strings.Repeat("x", 40000)
		retained := strings.Repeat("y", 25000) // 62.5% retained, above the 50% line
		_, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID,
				"content": large,
			})))
		if err != nil {
			t.Fatalf("handleCreateTaskPlan: %v", err)
		}
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			mustMarshalPlanPayload(t, map[string]any{
				"task_id": mcpPlanlessID,
				"content": retained,
			})))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		updated := decodeMCPPlanPayload(t, out)
		if warning, ok := updated["plan_write_warning"]; ok {
			t.Errorf("unexpected warning for a legitimate >50%% retain: %v", warning)
		}
	})
}
