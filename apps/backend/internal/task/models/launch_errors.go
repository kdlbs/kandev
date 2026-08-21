package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

// MetaKeyLastLaunchError stores the task-owned launch failure. Unlike the
// session error, this record survives session creation and remains visible
// when a launch never created a session.
const MetaKeyLastLaunchError = "last_launch_error"

// Launch error categories are stable wire and persistence values. Keep these
// values independent from human-readable messages.
const (
	LaunchErrorCategoryBaseBranchMissing       = "base_branch_missing"
	LaunchErrorCategoryPRAlreadyClosed         = "pr_already_closed"
	LaunchErrorCategoryDefaultBranchUnresolved = "default_branch_unresolved"
	LaunchErrorCategoryGenericLaunchFailure    = "generic_launch_failure"
)

// Recovery actions are stable wire values shared by backend and frontend.
const (
	RecoveryActionRetryDefault   = "retry_default"
	RecoveryActionPickBaseBranch = "pick_base_branch"
	RecoveryActionMarkReviewDone = "mark_review_done"
)

const (
	maxLaunchErrorRecoveryActions = 3
	maxLaunchErrorMessageBytes    = 4096
	maxLaunchErrorStampBytes      = 256
	maxLaunchErrorCategoryBytes   = 64
	maxLaunchErrorDetailsBytes    = 4096
	maxTaskRepositoryIDBytes      = 256
)

// TaskLaunchError is persisted under Task.Metadata[MetaKeyLastLaunchError].
// It is intentionally bounded because task metadata is returned in boot and
// task-list payloads.
type TaskLaunchError struct {
	Message          string    `json:"message"`
	OccurredAt       time.Time `json:"occurred_at"`
	Code             string    `json:"code,omitempty"`
	Details          string    `json:"details,omitempty"`
	RecoveryActions  []string  `json:"recovery_actions,omitempty"`
	TaskRepositoryID string    `json:"task_repository_id,omitempty"`
	StampValue       string    `json:"stamp,omitempty"`
}

// NormalizeRecoveryActions removes unknown, duplicate, and excess actions
// before a record crosses a persistence or WebSocket boundary.
func NormalizeRecoveryActions(actions []string) []string {
	seen := make(map[string]struct{}, len(actions))
	result := make([]string, 0, min(len(actions), maxLaunchErrorRecoveryActions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if !isKnownRecoveryAction(action) {
			continue
		}
		if _, exists := seen[action]; exists {
			continue
		}
		seen[action] = struct{}{}
		result = append(result, action)
		if len(result) == maxLaunchErrorRecoveryActions {
			break
		}
	}
	return result
}

func isKnownRecoveryAction(action string) bool {
	switch action {
	case RecoveryActionRetryDefault, RecoveryActionPickBaseBranch, RecoveryActionMarkReviewDone:
		return true
	default:
		return false
	}
}

func normalizeLastAgentError(value LastAgentError) LastAgentError {
	value.Message = truncateUTF8Bytes(value.Message, maxLaunchErrorMessageBytes)
	value.RecoveryActions = NormalizeRecoveryActions(value.RecoveryActions)
	value.TaskRepositoryID = truncateUTF8Bytes(value.TaskRepositoryID, maxTaskRepositoryIDBytes)
	value.StampValue = boundedLaunchErrorStamp(value.StampValue)
	value.Details = truncateUTF8Bytes(value.Details, maxLaunchErrorDetailsBytes)
	return value
}

func normalizeTaskLaunchError(value TaskLaunchError) TaskLaunchError {
	value.Message = truncateUTF8Bytes(value.Message, maxLaunchErrorMessageBytes)
	value.RecoveryActions = NormalizeRecoveryActions(value.RecoveryActions)
	value.TaskRepositoryID = truncateUTF8Bytes(value.TaskRepositoryID, maxTaskRepositoryIDBytes)
	value.StampValue = boundedLaunchErrorStamp(value.StampValue)
	value.Code = truncateUTF8Bytes(value.Code, maxLaunchErrorCategoryBytes)
	value.Details = truncateUTF8Bytes(value.Details, maxLaunchErrorDetailsBytes)
	return value
}

// LoadTaskLaunchError reads and validates the task-owned launch error.
func LoadTaskLaunchError(metadata map[string]interface{}) (TaskLaunchError, bool) {
	if metadata == nil {
		return TaskLaunchError{}, false
	}
	raw, ok := metadata[MetaKeyLastLaunchError]
	if !ok || raw == nil {
		return TaskLaunchError{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return TaskLaunchError{}, false
	}
	var value TaskLaunchError
	if err := json.Unmarshal(data, &value); err != nil || strings.TrimSpace(value.Message) == "" {
		return TaskLaunchError{}, false
	}
	return normalizeTaskLaunchError(value), true
}

// SetTaskLaunchError writes a task launch error unless the current record has
// the same stamp. The no-op preserves the first occurrence time for a PR
// state while repeated lifecycle callbacks race.
func SetTaskLaunchError(metadata map[string]interface{}, value TaskLaunchError) bool {
	if metadata == nil || strings.TrimSpace(value.Message) == "" {
		return false
	}
	value = normalizeTaskLaunchError(value)
	if current, ok := LoadTaskLaunchError(metadata); ok && value.Stamp() != "" && current.MatchesStamp(value.Stamp()) {
		return false
	}
	metadata[MetaKeyLastLaunchError] = value
	return true
}

// ClearTaskLaunchError removes the current record only when expectedStamp
// still identifies it. This protects a newer launch failure from an older
// successful retry callback.
func ClearTaskLaunchError(metadata map[string]interface{}, expectedStamp string) bool {
	if metadata == nil || strings.TrimSpace(expectedStamp) == "" {
		return false
	}
	current, ok := LoadTaskLaunchError(metadata)
	if !ok || !current.MatchesStamp(expectedStamp) {
		return false
	}
	delete(metadata, MetaKeyLastLaunchError)
	return true
}

// Stamp returns the explicit bounded stamp, or the legacy computed form for
// records written before the explicit field existed.
func (e TaskLaunchError) Stamp() string {
	if stamp := boundedLaunchErrorStamp(e.StampValue); stamp != "" {
		return stamp
	}
	return e.OccurredAt.UTC().Format(time.RFC3339Nano) + ":" + e.Message
}

// MatchesStamp reports whether stamp identifies this launch error.
func (e TaskLaunchError) MatchesStamp(stamp string) bool {
	if stamp == e.Stamp() {
		return true
	}
	if boundedLaunchErrorStamp(e.StampValue) != "" {
		return false
	}
	suffix := ":" + e.Message
	if !strings.HasSuffix(stamp, suffix) {
		return false
	}
	rawOccurredAt := strings.TrimSuffix(stamp, suffix)
	if rawOccurredAt == "" {
		return e.OccurredAt.IsZero()
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, rawOccurredAt)
	return err == nil && occurredAt.Equal(e.OccurredAt)
}

// StableLaunchErrorStamp creates a short deterministic identity for a
// normalized set of external states, such as repository and PR state.
func StableLaunchErrorStamp(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	sum := hash.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func boundedLaunchErrorStamp(value string) string {
	return truncateUTF8Bytes(strings.TrimSpace(value), maxLaunchErrorStampBytes)
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
