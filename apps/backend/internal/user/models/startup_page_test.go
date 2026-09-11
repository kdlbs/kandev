package models

import "testing"

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-002.5
func TestNormalizeStartupPage(t *testing.T) {
	for _, tt := range []struct{ value, want string }{
		{"threads", "threads"},
		{"last_task", "last_task"},
		{"task_overview", "task_overview"},
		{"", "task_overview"},
		{"future_value", "task_overview"},
	} {
		t.Run(tt.value, func(t *testing.T) {
			if got := NormalizeStartupPage(tt.value); got != tt.want {
				t.Fatalf("NormalizeStartupPage(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
