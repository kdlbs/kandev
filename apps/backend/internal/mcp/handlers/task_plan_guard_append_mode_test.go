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

// TestPlanTruncationWarningNeverDeniesAppendMode pins AC-TASKS-PLAN-APPEND-
// 006.7/006.9's negative half on both rendered branches of the truncation
// warning: it must no longer claim no partial update or append mode exists,
// having shipped exactly that claim before this capability existed.
func TestPlanTruncationWarningNeverDeniesAppendMode(t *testing.T) {
	deniedPhrases := []string{"no partial update", "no append mode"}

	t.Run("revision unknown", func(t *testing.T) {
		warning := planTruncationWarning(1000, 100, 0)
		for _, phrase := range deniedPhrases {
			if strings.Contains(warning, phrase) {
				t.Errorf("warning still denies append mode (%q): %q", phrase, warning)
			}
		}
	})

	t.Run("revision known", func(t *testing.T) {
		warning := planTruncationWarning(1000, 100, 3)
		for _, phrase := range deniedPhrases {
			if strings.Contains(warning, phrase) {
				t.Errorf("warning still denies append mode (%q): %q", phrase, warning)
			}
		}
	})
}
