package service_test

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// withObservedWarnLogs swaps the package-level default logger (the only
// logger dedup.go's reporting functions use — dedupLogger() always calls
// logger.Default()) for one backed by a zaptest/observer core, and restores
// the original on cleanup. Not safe for t.Parallel(): the swap is process-
// global for the duration of the test.
func withObservedWarnLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	observed, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	original := logger.Default()
	logger.SetDefault(observed)
	t.Cleanup(func() { logger.SetDefault(original) })
	return logs
}

// TestReportAssignmentRateLimitRefused_LogFields covers
// AC-OFFICE-ASSIGN-RATE-003.3's full field set: task id, assignee profile
// id, acting agent id, observed count and allowance are all present —
// every argument this function receives is logged.
func TestReportAssignmentRateLimitRefused_LogFields(t *testing.T) {
	logs := withObservedWarnLogs(t)

	runsservice.ReportAssignmentRateLimitRefused("task-log-1", "assignee-log-1", "actor-log-1", 5, 5)

	entries := logs.FilterMessage("assignment wake refused (rate limit)").All()
	if len(entries) != 1 {
		t.Fatalf("warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != "task-log-1" ||
		fields["assignee_agent_profile_id"] != "assignee-log-1" ||
		fields["acting_agent_id"] != "actor-log-1" ||
		fields["observed_count"] != int64(5) ||
		fields["allowance"] != int64(5) {
		t.Fatalf("fields = %#v, want task_id/assignee_agent_profile_id/acting_agent_id/observed_count/allowance all present", fields)
	}
}

// TestReportAssignmentRateLimitCountReadFailed_LogFields covers
// AC-OFFICE-ASSIGN-RATE-002.2: task id, assignee profile id, acting agent
// id, allowance and the error are present, but observed_count is
// deliberately absent — a failed count read never produced one.
func TestReportAssignmentRateLimitCountReadFailed_LogFields(t *testing.T) {
	logs := withObservedWarnLogs(t)

	runsservice.ReportAssignmentRateLimitCountReadFailed(
		"task-log-2", "assignee-log-2", "actor-log-2", 5, errors.New("db unavailable"))

	entries := logs.FilterMessage("assignment wake admitted (rate limit count read failed)").All()
	if len(entries) != 1 {
		t.Fatalf("warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != "task-log-2" ||
		fields["assignee_agent_profile_id"] != "assignee-log-2" ||
		fields["acting_agent_id"] != "actor-log-2" ||
		fields["allowance"] != int64(5) {
		t.Fatalf("fields = %#v, want task_id/assignee_agent_profile_id/acting_agent_id/allowance all present", fields)
	}
	if errVal, ok := fields["error"]; !ok || errVal == "" {
		t.Fatalf("fields = %#v, want a non-empty error field", fields)
	}
	if _, present := fields["observed_count"]; present {
		t.Fatalf("fields = %#v, want observed_count absent — a failed count read never produced one", fields)
	}
}

// TestReportAssignmentRateLimitUnattributed_LogFields covers
// AC-OFFICE-ASSIGN-RATE-002.3 with a non-empty acting agent id: reason,
// allowance and acting_agent_id are present, but task_id is deliberately
// absent — the wake's task could not be determined at all.
func TestReportAssignmentRateLimitUnattributed_LogFields(t *testing.T) {
	logs := withObservedWarnLogs(t)

	runsservice.ReportAssignmentRateLimitUnattributed("actor-log-3", 5)

	entries := logs.FilterMessage("assignment wake admitted (task unattributed)").All()
	if len(entries) != 1 {
		t.Fatalf("warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["reason"] != "task_unattributed" || fields["allowance"] != int64(5) || fields["acting_agent_id"] != "actor-log-3" {
		t.Fatalf("fields = %#v, want reason/allowance/acting_agent_id all present", fields)
	}
	if _, present := fields["task_id"]; present {
		t.Fatalf("fields = %#v, want task_id absent — the wake's task could not be determined", fields)
	}
}

// TestReportAssignmentRateLimitUnattributed_LogFields_EmptyActorID covers
// the same function's other deliberate omission: an empty acting agent id
// is dropped from the fields entirely rather than logged as "".
func TestReportAssignmentRateLimitUnattributed_LogFields_EmptyActorID(t *testing.T) {
	logs := withObservedWarnLogs(t)

	runsservice.ReportAssignmentRateLimitUnattributed("", 5)

	entries := logs.FilterMessage("assignment wake admitted (task unattributed)").All()
	if len(entries) != 1 {
		t.Fatalf("warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["reason"] != "task_unattributed" || fields["allowance"] != int64(5) {
		t.Fatalf("fields = %#v, want reason/allowance present", fields)
	}
	if _, present := fields["acting_agent_id"]; present {
		t.Fatalf("fields = %#v, want acting_agent_id absent for an empty caller", fields)
	}
	if _, present := fields["task_id"]; present {
		t.Fatalf("fields = %#v, want task_id absent — the wake's task could not be determined", fields)
	}
}
