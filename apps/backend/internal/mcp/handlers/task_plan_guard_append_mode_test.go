package handlers

import (
	"strings"
	"testing"
)

// TestPlanTruncationWarningMentionsAppendMode pins REQ-TASKS-PLAN-APPEND-006:
// the truncation warning must point a caller at update_task_plan_kandev's
// mode="append" as the way to avoid resubmitting the whole document, on both
// the known-revision and unknown-revision branches. This is the primary
// vector for the guard's own past claim that no append mode existed.
func TestPlanTruncationWarningMentionsAppendMode(t *testing.T) {
	t.Run("revision unknown", func(t *testing.T) {
		warning := planTruncationWarning(1000, 100, 0)
		if !strings.Contains(warning, `mode="append"`) {
			t.Errorf("warning does not mention mode=\"append\": %q", warning)
		}
		if !strings.Contains(warning, "update_task_plan_kandev") {
			t.Errorf("warning does not name update_task_plan_kandev: %q", warning)
		}
	})

	t.Run("revision known", func(t *testing.T) {
		warning := planTruncationWarning(1000, 100, 3)
		if !strings.Contains(warning, `mode="append"`) {
			t.Errorf("warning does not mention mode=\"append\": %q", warning)
		}
		if !strings.Contains(warning, "update_task_plan_kandev") {
			t.Errorf("warning does not name update_task_plan_kandev: %q", warning)
		}
	})
}
