package models

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// ErrUnansweredAdmissionUnknown means no existing receipt proves the named
// late-answer message was admitted for the requested task and session.
var ErrUnansweredAdmissionUnknown = errors.New("late answer admission could not be confirmed")

// Workspace defaults and bounds for the unanswered-clarification inbox.
const (
	DefaultClarificationUnansweredRetentionDays = 14
	MinClarificationUnansweredRetentionDays     = 1
	MaxClarificationUnansweredRetentionDays     = 90
)

// Clarification message metadata keys that close or hide an unanswered
// (superseded, still-pending) clarification bundle. They never change the
// bundle's status, so the active-clarification rules that read status and
// turn ownership are unaffected.
const (
	ClarificationMetaClosedReason        = "closed_reason"
	ClarificationMetaClosedAt            = "closed_at"
	ClarificationMetaLateAnsweredAt      = "late_answered_at"
	ClarificationMetaLateAnswerMessageID = "late_answer_message_id"
	ClarificationMetaLateAnswerSessionID = "late_answer_session_id"
	ClarificationMetaMergedInto          = "merged_into"
)

// ClarificationClosedReason is the closed_reason value of a closed
// unanswered clarification bundle.
type ClarificationClosedReason string

const (
	// ClarificationClosedAnsweredLate records an accepted late-answer message.
	ClarificationClosedAnsweredLate ClarificationClosedReason = "answered_late"
	// ClarificationClosedByUser records a user closing the question without
	// sending an answer.
	ClarificationClosedByUser ClarificationClosedReason = "closed_by_user"
	// ClarificationClosedExpiredUnanswered records retention expiry.
	ClarificationClosedExpiredUnanswered ClarificationClosedReason = "expired_unanswered"
)

// ListUnansweredClarificationsOptions filters and paginates
// ListUnansweredClarificationBundles. Only workspaces whose
// clarification_unanswered_inbox_enabled setting is on can contribute rows.
type ListUnansweredClarificationsOptions struct {
	// AnyWorkspace admits every enabled workspace. When false, WorkspaceIDs
	// is the closed set of candidate workspaces; an empty set lists nothing.
	AnyWorkspace bool
	WorkspaceIDs []string
	// TaskID optionally narrows to one task.
	TaskID string
	// CreatedSince optionally excludes bundles created before this instant.
	CreatedSince *time.Time
	// CursorCreatedAt/CursorPendingID are the last returned (created_at,
	// pending_id) pair. An empty CursorPendingID means the first page.
	CursorCreatedAt time.Time
	CursorPendingID string
	// Limit is the page size. Must be >= 1.
	Limit int
	// Sidecar optionally applies one operator's dismiss/snooze rows.
	Sidecar *ClarificationSidecarFilter
	// Now is the instant retention is evaluated against. Zero means the
	// repository clock.
	Now time.Time
}

// CloseUnansweredClarification describes one terminal metadata write on an
// unanswered bundle.
type CloseUnansweredClarification struct {
	Reason ClarificationClosedReason
	At     time.Time
	// MessageID and SessionID identify the accepted late-answer message.
	// Both are required for ClarificationClosedAnsweredLate and ignored
	// otherwise.
	MessageID string
	SessionID string
	// SourceSessionID is the authorized session that asked the question.
	SourceSessionID string
}

// ClarificationQuestionIdentity is the part of a question compared when a
// newer bundle replaces an older unanswered one.
type ClarificationQuestionIdentity struct {
	ID     string
	Prompt string
}

// positionalQuestionID matches the identifiers Kandev assigns when the agent
// omits one ("q1", "q2", ...). They identify a position inside one call, not
// a question, so they never prove two bundles ask the same thing.
var positionalQuestionID = regexp.MustCompile(`^q[0-9]+$`)

// NormalizeClarificationQuestionText trims, collapses internal whitespace and
// lower-cases a question prompt. It is a textual comparison only.
func NormalizeClarificationQuestionText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// SameClarificationQuestion reports whether two questions are duplicates: the
// same agent-chosen question_id, or the same normalized prompt.
func SameClarificationQuestion(a, b ClarificationQuestionIdentity) bool {
	if a.ID != "" && a.ID == b.ID && !positionalQuestionID.MatchString(a.ID) {
		return true
	}
	na, nb := NormalizeClarificationQuestionText(a.Prompt), NormalizeClarificationQuestionText(b.Prompt)
	return na != "" && na == nb
}

// ClampClarificationUnansweredRetentionDays maps a stored value onto the
// supported range so a legacy or hand-edited row cannot disable retention.
func ClampClarificationUnansweredRetentionDays(days int) int {
	switch {
	case days <= 0:
		return DefaultClarificationUnansweredRetentionDays
	case days < MinClarificationUnansweredRetentionDays:
		return MinClarificationUnansweredRetentionDays
	case days > MaxClarificationUnansweredRetentionDays:
		return MaxClarificationUnansweredRetentionDays
	default:
		return days
	}
}
