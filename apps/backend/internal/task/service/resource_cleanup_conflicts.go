package service

import (
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	cleanupConditionReconciliationRequired = "reconciliation_required"
	cleanupRetryPolicyBounded              = "bounded"
	cleanupRetryPolicyUnlimited            = "unlimited"
	cleanupConflictMaxNodes                = 256
	cleanupConflictMaxDiagnostics          = 32
	cleanupConflictFieldMaxBytes           = 64
)

type cleanupResourceConflict struct {
	WorktreeID       string
	Stage            string
	Reason           string
	Comparison       string
	Action           string
	RecordedEvidence bool
	ObservedEvidence bool
}

// collectCleanupResourceConflicts traverses the joined cleanup error once per
// attempt, deduplicating repeated references to the same typed resource.
func collectCleanupResourceConflicts(err error) ([]cleanupResourceConflict, bool) {
	if err == nil {
		return nil, false
	}
	stack := []error{err}
	seen := make(map[string]struct{})
	conflicts := make([]cleanupResourceConflict, 0, 4)
	nodes := 0
	overflow := false
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		nodes++
		if nodes > cleanupConflictMaxNodes {
			overflow = true
			break
		}
		if inspection, ok := node.(*worktree.CleanupInspectionError); ok &&
			worktree.IsCleanupConflictReason(inspection.Reason) {
			conflict := cleanupResourceConflict{
				WorktreeID:       boundedCleanupConflictValue(inspection.WorktreeID, cleanupConflictFieldMaxBytes),
				Stage:            boundedCleanupConflictValue(inspection.Stage, cleanupConflictFieldMaxBytes),
				Reason:           boundedCleanupConflictValue(inspection.Reason, cleanupConflictFieldMaxBytes),
				Comparison:       boundedCleanupConflictValue(inspection.Comparison, cleanupConflictFieldMaxBytes),
				Action:           boundedCleanupConflictValue(inspection.Action, cleanupConflictFieldMaxBytes),
				RecordedEvidence: inspection.RecordedEvidence,
				ObservedEvidence: inspection.ObservedEvidence,
			}
			key := strings.Join([]string{
				conflict.WorktreeID, conflict.Stage, conflict.Reason,
				conflict.Comparison, conflict.Action,
			}, "\x00")
			if _, duplicate := seen[key]; !duplicate {
				if len(conflicts) >= cleanupConflictMaxDiagnostics {
					overflow = true
				} else {
					seen[key] = struct{}{}
					conflicts = append(conflicts, conflict)
				}
			}
		}
		switch nested := node.(type) {
		case interface{ Unwrap() []error }:
			stack = append(stack, nested.Unwrap()...)
		case interface{ Unwrap() error }:
			if next := nested.Unwrap(); next != nil {
				stack = append(stack, next)
			}
		}
	}
	return conflicts, overflow
}

func boundedCleanupConflictValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

func (s *Service) logCleanupConflictDiagnostics(
	job *models.TaskResourceCleanupJob,
	state models.TaskResourceCleanupState,
	nextAttempt *time.Time,
	cleanupErr error,
) {
	conflicts, overflow := collectCleanupResourceConflicts(cleanupErr)
	for _, conflict := range conflicts {
		fields := []zap.Field{
			zap.String("cleanup_condition", cleanupConditionReconciliationRequired),
			zap.String("job_id", job.ID),
			zap.String("task_id", job.TaskID),
			zap.String("worktree_id", conflict.WorktreeID),
			zap.String("stage", conflict.Stage),
			zap.String("reason", conflict.Reason),
			zap.String("comparison", conflict.Comparison),
			zap.String("action", conflict.Action),
			zap.Bool("recorded_evidence", conflict.RecordedEvidence),
			zap.Bool("observed_evidence", conflict.ObservedEvidence),
			zap.Int("attempt", job.Attempts),
			zap.String("state", string(state)),
			zap.String("trigger", string(job.Trigger)),
		}
		fields = append(fields, cleanupRetryPolicyFields(job)...)
		if nextAttempt != nil {
			fields = append(fields, zap.Time("next_attempt_at", nextAttempt.UTC()))
		}
		s.logger.Warn("task resource cleanup requires reconciliation", fields...)
	}
	if overflow {
		s.logger.Warn("task resource cleanup conflict diagnostics truncated",
			zap.String("cleanup_condition", cleanupConditionReconciliationRequired),
			zap.String("job_id", job.ID),
			zap.String("task_id", job.TaskID),
			zap.Int("attempt", job.Attempts),
			zap.Int("max_nodes", cleanupConflictMaxNodes),
			zap.Int("max_diagnostics", cleanupConflictMaxDiagnostics),
			zap.Bool("diagnostics_truncated", true),
		)
	}
}

// cleanupRetryPolicyFields describes the effective attempt policy separately
// from the always-capped delay schedule.
func cleanupRetryPolicyFields(job *models.TaskResourceCleanupJob) []zap.Field {
	policy := cleanupRetryPolicyBounded
	fields := []zap.Field{
		zap.String("retry_policy", policy),
		zap.Int64("retry_delay_cap_seconds", int64(taskResourceCleanupRetryDelays[len(taskResourceCleanupRetryDelays)-1]/time.Second)),
	}
	if cleanupRetryIsBounded(job) {
		return append(fields, zap.Int("max_attempts", taskResourceCleanupMaxAttempts))
	}
	fields[0] = zap.String("retry_policy", cleanupRetryPolicyUnlimited)
	return append(fields, zap.Bool("attempt_limit_unlimited", true))
}
