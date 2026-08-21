package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadLastAgentErrorUsesExplicitStampAndNormalizesRecoveryFields(t *testing.T) {
	metadata := map[string]interface{}{
		SessionMetaKeyLastAgentError: map[string]interface{}{
			"message":            "base branch is missing",
			"occurred_at":        "2026-08-19T10:00:00Z",
			"stamp":              "pr-state-1",
			"task_repository_id": "task-repo-1",
			"recovery_actions":   []string{"retry_default", "retry_default", "unknown", "pick_base_branch", "mark_review_done"},
		},
	}

	lastError, ok := LoadLastAgentError(metadata)
	require.True(t, ok)
	require.Equal(t, "pr-state-1", lastError.Stamp())
	require.Equal(t, "task-repo-1", lastError.TaskRepositoryID)
	require.Equal(t, []string{
		RecoveryActionRetryDefault,
		RecoveryActionPickBaseBranch,
		RecoveryActionMarkReviewDone,
	}, lastError.RecoveryActions)
}

func TestTaskLaunchErrorStoreIsNoOpForSameStamp(t *testing.T) {
	firstTime := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Minute)
	metadata := map[string]interface{}{}

	first := TaskLaunchError{
		Message:          "base branch is missing",
		OccurredAt:       firstTime,
		StampValue:       "pr-state-1",
		TaskRepositoryID: "task-repo-1",
		RecoveryActions:  []string{RecoveryActionRetryDefault},
	}
	require.True(t, SetTaskLaunchError(metadata, first))

	second := first
	second.OccurredAt = secondTime
	second.Message = "a newer message for the same PR state"
	require.False(t, SetTaskLaunchError(metadata, second))

	stored, ok := LoadTaskLaunchError(metadata)
	require.True(t, ok)
	require.Equal(t, firstTime, stored.OccurredAt)
	require.Equal(t, first.Message, stored.Message)
	require.Equal(t, first.StampValue, stored.Stamp())
}

func TestTaskLaunchErrorClearRequiresMatchingStamp(t *testing.T) {
	metadata := map[string]interface{}{}
	require.True(t, SetTaskLaunchError(metadata, TaskLaunchError{
		Message:    "base branch is missing",
		OccurredAt: time.Now().UTC(),
		StampValue: "pr-state-1",
	}))

	require.False(t, ClearTaskLaunchError(metadata, "pr-state-old"))
	require.True(t, ClearTaskLaunchError(metadata, "pr-state-1"))
	_, ok := LoadTaskLaunchError(metadata)
	require.False(t, ok)
}

func TestStableLaunchErrorStampIsDeterministicAndBounded(t *testing.T) {
	first := StableLaunchErrorStamp("repo-1", "123", "open")
	second := StableLaunchErrorStamp("repo-1", "123", "open")
	require.Equal(t, first, second)
	require.Len(t, first, 32)
	require.NotEqual(t, first, StableLaunchErrorStamp("repo-1", "123", "closed"))
}

func TestLaunchErrorNormalizationBoundsPersistedMessages(t *testing.T) {
	longMessage := strings.Repeat("x", maxLaunchErrorMessageBytes+100)

	taskMetadata := map[string]interface{}{
		MetaKeyLastLaunchError: TaskLaunchError{Message: longMessage},
	}
	taskError, ok := LoadTaskLaunchError(taskMetadata)
	require.True(t, ok)
	require.Len(t, taskError.Message, maxLaunchErrorMessageBytes)

	sessionMetadata := map[string]interface{}{
		SessionMetaKeyLastAgentError: LastAgentError{Message: longMessage},
	}
	sessionError, ok := LoadLastAgentError(sessionMetadata)
	require.True(t, ok)
	require.Len(t, sessionError.Message, maxLaunchErrorMessageBytes)
}

func TestLaunchErrorMatchesStampDoesNotFallBackWhenExplicitStampExists(t *testing.T) {
	occurredAt := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	legacyStamp := occurredAt.Format(time.RFC3339Nano) + ":boom"

	taskError := TaskLaunchError{Message: "boom", OccurredAt: occurredAt, StampValue: "explicit-task"}
	require.True(t, taskError.MatchesStamp("explicit-task"))
	require.False(t, taskError.MatchesStamp(legacyStamp))

	sessionError := LastAgentError{Message: "boom", OccurredAt: occurredAt, StampValue: "explicit-session"}
	require.True(t, sessionError.MatchesStamp("explicit-session"))
	require.False(t, sessionError.MatchesStamp(legacyStamp))
}
