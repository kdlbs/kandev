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
	LaunchErrorCategoryWorkspaceCheckoutFailed = "workspace_checkout_failed"
	LaunchErrorCategoryGenericLaunchFailure    = "generic_launch_failure"
)

// Recovery actions are stable wire values shared by backend and frontend.
const (
	RecoveryActionRetryDefault   = "retry_default"
	RecoveryActionPickBaseBranch = "pick_base_branch"
	RecoveryActionMarkReviewDone = "mark_review_done"
	RecoveryActionRetryLaunch    = "retry_launch"
)

const (
	maxLaunchErrorRecoveryActions = 3
	maxLaunchErrorMessageBytes    = 4096
	maxLaunchErrorStampBytes      = 256
	maxLaunchErrorCategoryBytes   = 64
	maxLaunchErrorDetailsBytes    = 4096
	maxTaskRepositoryIDBytes      = 256
	maxLaunchErrorIDBytes         = 256
	maxAgentErrorCauseDetailBytes = 1024
	maxAgentErrorCauses           = 2
)

const (
	LaunchErrorPhaseBootstrap = "bootstrap"

	AgentErrorCauseOperationResume           = "resume"
	AgentErrorCauseOperationRestoreWorkspace = "restore_workspace"

	AgentErrorCauseCodeAuthenticationRequired = "authentication_required"
	AgentErrorCauseCodePermissionDenied       = "permission_denied"
	AgentErrorCauseCodeDestinationInvalid     = "destination_invalid"
	AgentErrorCauseCodeSourceBranchMissing    = "source_branch_missing"
	AgentErrorCauseCodeTransportUnavailable   = "transport_unavailable"
	AgentErrorCauseCodeTimeout                = "timeout"
	AgentErrorCauseCodeUnknown                = "unknown"
)

// AgentErrorCause keeps the bounded, operation-specific explanation for one
// recovery attempt. It is intentionally smaller than LastAgentError so a
// resume failure and a workspace fallback can remain distinct without
// persisting provider transport payloads.
type AgentErrorCause struct {
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
}

// NormalizeAgentErrorCauses removes malformed, duplicate, and excess causes
// before a session error crosses a persistence or transport boundary.
func NormalizeAgentErrorCauses(causes []AgentErrorCause) []AgentErrorCause {
	result := make([]AgentErrorCause, 0, min(len(causes), maxAgentErrorCauses))
	seen := make(map[string]struct{}, len(causes))
	for _, cause := range causes {
		cause.Operation = strings.TrimSpace(cause.Operation)
		cause.Code = strings.TrimSpace(cause.Code)
		if !isKnownAgentErrorCauseOperation(cause.Operation) || !isKnownAgentErrorCauseCode(cause.Code) {
			continue
		}
		cause.Detail = truncateUTF8Bytes(cause.Detail, maxAgentErrorCauseDetailBytes)
		identity := cause.Operation + "\x00" + cause.Code + "\x00" + cause.Detail
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, cause)
		if len(result) == maxAgentErrorCauses {
			break
		}
	}
	return result
}

// NormalizeAgentErrorDetails keeps the legacy details and cause details in
// the one existing details budget. Cause details are retained first because
// they are the structured recovery fields used to explain separate attempts.
func NormalizeAgentErrorDetails(details string, causes []AgentErrorCause) string {
	causes = NormalizeAgentErrorCauses(causes)
	remaining := maxLaunchErrorDetailsBytes
	for _, cause := range causes {
		remaining -= len(cause.Detail)
	}
	if remaining < 0 {
		remaining = 0
	}
	return truncateUTF8Bytes(details, remaining)
}

func isKnownAgentErrorCauseOperation(operation string) bool {
	return operation == AgentErrorCauseOperationResume || operation == AgentErrorCauseOperationRestoreWorkspace
}

func isKnownAgentErrorCauseCode(code string) bool {
	switch code {
	case AgentErrorCauseCodeAuthenticationRequired,
		AgentErrorCauseCodePermissionDenied,
		AgentErrorCauseCodeDestinationInvalid,
		AgentErrorCauseCodeSourceBranchMissing,
		AgentErrorCauseCodeTransportUnavailable,
		AgentErrorCauseCodeTimeout,
		AgentErrorCauseCodeUnknown,
		LaunchErrorCategoryBaseBranchMissing,
		LaunchErrorCategoryDefaultBranchUnresolved,
		LaunchErrorCategoryWorkspaceCheckoutFailed,
		LaunchErrorCategoryGenericLaunchFailure:
		return true
	default:
		return false
	}
}

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

// NormalizeRecoveryActionsForCategory retains only the actions that are
// meaningful for a typed launch-failure category. Unknown categories keep the
// legacy generic normalization so unrelated runtime errors remain compatible.
func NormalizeRecoveryActionsForCategory(category string, actions []string) []string {
	normalized := NormalizeRecoveryActions(actions)
	var allowed map[string]struct{}
	switch category {
	case LaunchErrorCategoryBaseBranchMissing:
		allowed = map[string]struct{}{
			RecoveryActionRetryDefault: {}, RecoveryActionPickBaseBranch: {},
		}
	case LaunchErrorCategoryDefaultBranchUnresolved:
		allowed = map[string]struct{}{RecoveryActionPickBaseBranch: {}}
	case LaunchErrorCategoryWorkspaceCheckoutFailed, LaunchErrorCategoryGenericLaunchFailure:
		if len(normalized) == 0 {
			return nil
		}
		return []string{RecoveryActionRetryLaunch}
	case LaunchErrorCategoryPRAlreadyClosed:
		allowed = map[string]struct{}{RecoveryActionMarkReviewDone: {}}
	default:
		return normalized
	}

	result := make([]string, 0, len(normalized))
	for _, action := range normalized {
		if _, ok := allowed[action]; ok {
			result = append(result, action)
		}
	}
	return result
}

func isKnownRecoveryAction(action string) bool {
	switch action {
	case RecoveryActionRetryDefault, RecoveryActionPickBaseBranch, RecoveryActionMarkReviewDone, RecoveryActionRetryLaunch:
		return true
	default:
		return false
	}
}

func normalizeLastAgentError(value LastAgentError) LastAgentError {
	value.Message = truncateUTF8Bytes(value.Message, maxLaunchErrorMessageBytes)
	value.Code = truncateUTF8Bytes(value.Code, maxLaunchErrorCategoryBytes)
	value.RecoveryActions = NormalizeRecoveryActionsForCategory(value.Code, value.RecoveryActions)
	value.TaskRepositoryID = truncateUTF8Bytes(value.TaskRepositoryID, maxTaskRepositoryIDBytes)
	value.AgentExecutionID = truncateUTF8Bytes(value.AgentExecutionID, maxLaunchErrorIDBytes)
	value.ExecutionID = truncateUTF8Bytes(value.ExecutionID, maxLaunchErrorIDBytes)
	if value.ExecutionID == "" {
		value.ExecutionID = value.AgentExecutionID
	}
	if value.AgentExecutionID == "" {
		value.AgentExecutionID = value.ExecutionID
	}
	if value.Phase != LaunchErrorPhaseBootstrap {
		value.Phase = ""
	}
	value.AttemptID = truncateUTF8Bytes(value.AttemptID, maxLaunchErrorIDBytes)
	value.StampValue = boundedLaunchErrorStamp(value.StampValue)
	value.Causes = NormalizeAgentErrorCauses(value.Causes)
	value.Details = NormalizeAgentErrorDetails(value.Details, value.Causes)
	return value
}

func normalizeTaskLaunchError(value TaskLaunchError) TaskLaunchError {
	value.Message = truncateUTF8Bytes(value.Message, maxLaunchErrorMessageBytes)
	value.Code = truncateUTF8Bytes(value.Code, maxLaunchErrorCategoryBytes)
	value.RecoveryActions = NormalizeRecoveryActionsForCategory(value.Code, value.RecoveryActions)
	value.TaskRepositoryID = truncateUTF8Bytes(value.TaskRepositoryID, maxTaskRepositoryIDBytes)
	value.StampValue = boundedLaunchErrorStamp(value.StampValue)
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
