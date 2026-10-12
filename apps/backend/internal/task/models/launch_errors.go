package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MetaKeyLastLaunchError stores the task-owned launch failure. Unlike the
// session error, this record survives session creation and remains visible
// when a launch never created a session.
const MetaKeyLastLaunchError = "last_launch_error"

// Launch error categories are stable wire and persistence values. Keep these
// values independent from human-readable messages.
const (
	LaunchErrorCategoryBaseBranchMissing              = "base_branch_missing"
	LaunchErrorCategoryPRAlreadyClosed                = "pr_already_closed"
	LaunchErrorCategoryDefaultBranchUnresolved        = "default_branch_unresolved"
	LaunchErrorCategoryWorkspaceCheckoutFailed        = "workspace_checkout_failed"
	LaunchErrorCategoryGenericLaunchFailure           = "generic_launch_failure"
	LaunchErrorCategoryManagedCloneRelocationRequired = "managed_clone_relocation_required"
)

// Recovery actions are stable wire values shared by backend and frontend.
const (
	RecoveryActionRetryDefault      = "retry_default"
	RecoveryActionPickBaseBranch    = "pick_base_branch"
	RecoveryActionMarkReviewDone    = "mark_review_done"
	RecoveryActionRetryLaunch       = "retry_launch"
	RecoveryActionRelocateAndResume = "relocate_and_resume"
	RecoveryActionResumeNewBranch   = "resume_new_branch"
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
	maxAgentErrorSelectorBytes    = 256
	maxAgentErrorCauses           = 2
)

const (
	LaunchErrorPhaseBootstrap = "bootstrap"

	// Error scopes identify the owner of a failure. Session failures belong in
	// the session transcript; task failures belong to the task shell and remain
	// visible while the task changes tabs or sessions.
	ErrorScopeSession = "session"
	ErrorScopeTask    = "task"

	AgentErrorCauseOperationResume           = "resume"
	AgentErrorCauseOperationRestoreWorkspace = "restore_workspace"
	AgentErrorCauseOperationStart            = "start"

	AgentErrorCauseCodeAuthenticationRequired    = "authentication_required"
	AgentErrorCauseCodePermissionDenied          = "permission_denied"
	AgentErrorCauseCodeDestinationInvalid        = "destination_invalid"
	AgentErrorCauseCodeSourceBranchMissing       = "source_branch_missing"
	AgentErrorCauseCodeTransportUnavailable      = "transport_unavailable"
	AgentErrorCauseCodeTimeout                   = "timeout"
	AgentErrorCauseCodeUnknown                   = "unknown"
	AgentErrorCauseCodeModelUnavailable          = "model_unavailable"
	AgentErrorCauseCodeModelSelectionFailed      = "model_selection_failed"
	AgentErrorCauseCodePermissionModeFailed      = "permission_mode_failed"
	AgentErrorCauseCodePermissionModeUnconfirmed = "permission_mode_unconfirmed"
	AgentErrorCauseCodePermissionModeMismatch    = "permission_mode_mismatch"

	AgentErrorCauseReasonRequestedNotAdvertised = "requested_not_advertised"
	AgentErrorCauseReasonCatalogEmpty           = "catalog_empty"
	AgentErrorCauseReasonSelectionUnsupported   = "selection_unsupported"
	AgentErrorCauseReasonApplicationFailed      = "application_failed"
	AgentErrorCauseReasonSelectionMissing       = "selection_missing"
	AgentErrorCauseReasonClientUnavailable      = "client_unavailable"
	AgentErrorCauseReasonConfirmationMissing    = "confirmation_missing"
	AgentErrorCauseReasonEffectiveMismatch      = "effective_mismatch"
)

// AgentErrorCause keeps the bounded, operation-specific explanation for one
// recovery attempt. It is intentionally smaller than LastAgentError so a
// resume failure and a workspace fallback can remain distinct without
// persisting provider transport payloads.
type AgentErrorCause struct {
	Operation      string `json:"operation"`
	Code           string `json:"code"`
	Detail         string `json:"detail,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RequestedModel string `json:"requested_model,omitempty"`
	EffectiveModel string `json:"effective_model,omitempty"`
	AttemptedModel string `json:"attempted_model,omitempty"`
	RequestedMode  string `json:"requested_mode,omitempty"`
	EffectiveMode  string `json:"effective_mode,omitempty"`
	PromptNotSent  *bool  `json:"prompt_not_sent,omitempty"`
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
		normalizeAgentErrorCauseEvidence(&cause)
		cause.Detail = truncateUTF8Bytes(cause.Detail, maxAgentErrorCauseDetailBytes)
		identity := cause.Operation + "\x00" + cause.Code + "\x00" + cause.Detail +
			"\x00" + cause.Reason + "\x00" + cause.RequestedModel + "\x00" + cause.EffectiveModel +
			"\x00" + cause.AttemptedModel + "\x00" + cause.RequestedMode + "\x00" + cause.EffectiveMode
		switch {
		case cause.PromptNotSent == nil:
			identity += "\x00unknown"
		case *cause.PromptNotSent:
			identity += "\x00true"
		default:
			identity += "\x00false"
		}
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

// AgentErrorCausesEqual compares normalized cause values, including the
// distinction between an omitted prompt observation and an explicit false.
func AgentErrorCausesEqual(left, right []AgentErrorCause) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a.Operation != b.Operation || a.Code != b.Code || a.Detail != b.Detail ||
			a.Reason != b.Reason || a.RequestedModel != b.RequestedModel ||
			a.EffectiveModel != b.EffectiveModel || a.AttemptedModel != b.AttemptedModel ||
			a.RequestedMode != b.RequestedMode ||
			a.EffectiveMode != b.EffectiveMode {
			return false
		}
		if (a.PromptNotSent == nil) != (b.PromptNotSent == nil) {
			return false
		}
		if a.PromptNotSent != nil && *a.PromptNotSent != *b.PromptNotSent {
			return false
		}
	}
	return true
}

func normalizeAgentErrorCauseEvidence(cause *AgentErrorCause) {
	if cause == nil {
		return
	}
	cause.Reason = strings.TrimSpace(cause.Reason)
	switch cause.Code {
	case AgentErrorCauseCodeModelUnavailable, AgentErrorCauseCodeModelSelectionFailed:
		if !isKnownAgentErrorCauseReason(cause.Code, cause.Reason) {
			clearAgentErrorCauseEvidence(cause)
			return
		}
		cause.RequestedModel = safeAgentErrorSelector(cause.RequestedModel)
		cause.EffectiveModel = safeAgentErrorSelector(cause.EffectiveModel)
		cause.AttemptedModel = safeAgentErrorSelector(cause.AttemptedModel)
		cause.RequestedMode = ""
		cause.EffectiveMode = ""
	case AgentErrorCauseCodePermissionModeFailed,
		AgentErrorCauseCodePermissionModeUnconfirmed,
		AgentErrorCauseCodePermissionModeMismatch:
		if !isKnownAgentErrorCauseReason(cause.Code, cause.Reason) {
			clearAgentErrorCauseEvidence(cause)
			return
		}
		cause.RequestedMode = safeAgentErrorSelector(cause.RequestedMode)
		cause.EffectiveMode = safeAgentErrorSelector(cause.EffectiveMode)
		cause.RequestedModel = ""
		cause.EffectiveModel = ""
		cause.AttemptedModel = ""
	default:
		clearAgentErrorCauseEvidence(cause)
	}
	if cause.PromptNotSent != nil {
		value := *cause.PromptNotSent
		cause.PromptNotSent = &value
	}
}

func clearAgentErrorCauseEvidence(cause *AgentErrorCause) {
	cause.Reason = ""
	cause.RequestedModel = ""
	cause.EffectiveModel = ""
	cause.AttemptedModel = ""
	cause.RequestedMode = ""
	cause.EffectiveMode = ""
	cause.PromptNotSent = nil
}

func isKnownAgentErrorCauseReason(code, reason string) bool {
	switch code {
	case AgentErrorCauseCodeModelUnavailable:
		return reason == AgentErrorCauseReasonRequestedNotAdvertised
	case AgentErrorCauseCodeModelSelectionFailed:
		return reason == AgentErrorCauseReasonCatalogEmpty ||
			reason == AgentErrorCauseReasonSelectionUnsupported ||
			reason == AgentErrorCauseReasonApplicationFailed ||
			reason == AgentErrorCauseReasonSelectionMissing
	case AgentErrorCauseCodePermissionModeFailed:
		return reason == AgentErrorCauseReasonClientUnavailable ||
			reason == AgentErrorCauseReasonApplicationFailed
	case AgentErrorCauseCodePermissionModeUnconfirmed:
		return reason == AgentErrorCauseReasonConfirmationMissing
	case AgentErrorCauseCodePermissionModeMismatch:
		return reason == AgentErrorCauseReasonEffectiveMismatch
	default:
		return false
	}
}

func safeAgentErrorSelector(value string) string {
	if value == "" || len(value) > maxAgentErrorSelectorBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return ""
	}
	if hasUnsafeAgentErrorSelectorContent(value) || hasUnsafeAgentErrorSelectorSegments(value) {
		return ""
	}
	return value
}

// SafeAgentErrorSelector returns a bounded selector only when it passes the
// same validation used for durable startup evidence.
func SafeAgentErrorSelector(value string) string {
	return safeAgentErrorSelector(value)
}

func hasUnsafeAgentErrorSelectorContent(value string) bool {
	lower := strings.ToLower(value)
	return strings.ContainsAny(value, "\\=@") || strings.Contains(value, "://") ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		containsCredentialLikeAgentErrorSelector(lower)
}

func containsCredentialLikeAgentErrorSelector(lower string) bool {
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "apikey") || strings.Contains(lower, "bearer ") ||
		strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "ghp_") ||
		strings.HasPrefix(lower, "github_pat_") || strings.HasPrefix(lower, "kandev_pat_")
}

func hasUnsafeAgentErrorSelectorSegments(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) || unicode.IsSpace(char) {
			return true
		}
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "." || segment == ".." || strings.HasPrefix(segment, ".") || looksOpaqueAgentErrorSelectorSegment(segment) {
			return true
		}
	}
	if len(segments) > 3 || (len(value) >= 3 && value[1] == ':' && (value[2] == '/' || value[2] == '\\')) {
		return true
	}
	return false
}

func looksOpaqueAgentErrorSelectorSegment(value string) bool {
	if len(value) < 32 {
		return false
	}
	for _, char := range value {
		if !isOpaqueAgentErrorSelectorRune(char) {
			return false
		}
	}
	return true
}

func isOpaqueAgentErrorSelectorRune(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' || strings.ContainsRune("+/=_-", char)
}

// NormalizeAgentErrorDetails keeps the legacy details and cause details in
// the one existing details budget. Cause details are retained first because
// they are the structured recovery fields used to explain separate attempts.
func NormalizeAgentErrorDetails(details string, causes []AgentErrorCause) string {
	causes = NormalizeAgentErrorCauses(causes)
	remaining := maxLaunchErrorDetailsBytes
	for _, cause := range causes {
		remaining -= agentErrorCauseEvidenceBytes(cause)
	}
	if remaining < 0 {
		remaining = 0
	}
	return truncateUTF8Bytes(details, remaining)
}

func agentErrorCauseEvidenceBytes(cause AgentErrorCause) int {
	total := len(cause.Operation) + len(cause.Code) + len(cause.Detail) + len(cause.Reason) +
		len(cause.RequestedModel) + len(cause.EffectiveModel) + len(cause.AttemptedModel) +
		len(cause.RequestedMode) + len(cause.EffectiveMode)
	if cause.PromptNotSent != nil {
		total++
	}
	return total
}

func isKnownAgentErrorCauseOperation(operation string) bool {
	return operation == AgentErrorCauseOperationStart ||
		operation == AgentErrorCauseOperationResume ||
		operation == AgentErrorCauseOperationRestoreWorkspace
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
		AgentErrorCauseCodeModelUnavailable,
		AgentErrorCauseCodeModelSelectionFailed,
		AgentErrorCauseCodePermissionModeFailed,
		AgentErrorCauseCodePermissionModeUnconfirmed,
		AgentErrorCauseCodePermissionModeMismatch,
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
	Scope            string    `json:"scope,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
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
	case LaunchErrorCategoryManagedCloneRelocationRequired:
		allowed = map[string]struct{}{RecoveryActionRelocateAndResume: {}}
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
	case RecoveryActionRetryDefault, RecoveryActionPickBaseBranch, RecoveryActionMarkReviewDone, RecoveryActionRetryLaunch, RecoveryActionRelocateAndResume:
		return true
	default:
		return false
	}
}

func normalizeLastAgentError(value LastAgentError) LastAgentError {
	value.Scope = normalizeErrorScope(value.Scope, ErrorScopeSession)
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
	value.Scope = normalizeErrorScope(value.Scope, ErrorScopeTask)
	value.Message = truncateUTF8Bytes(value.Message, maxLaunchErrorMessageBytes)
	value.Code = truncateUTF8Bytes(value.Code, maxLaunchErrorCategoryBytes)
	value.SessionID = truncateUTF8Bytes(value.SessionID, maxLaunchErrorIDBytes)
	value.RecoveryActions = NormalizeRecoveryActionsForCategory(value.Code, value.RecoveryActions)
	value.TaskRepositoryID = truncateUTF8Bytes(value.TaskRepositoryID, maxTaskRepositoryIDBytes)
	value.StampValue = boundedLaunchErrorStamp(value.StampValue)
	value.Details = truncateUTF8Bytes(value.Details, maxLaunchErrorDetailsBytes)
	return value
}

func normalizeErrorScope(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case ErrorScopeSession:
		return ErrorScopeSession
	case ErrorScopeTask:
		return ErrorScopeTask
	default:
		return fallback
	}
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
