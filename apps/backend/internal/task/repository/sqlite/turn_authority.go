package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// turnAuthorityPredicate excludes an empty successor reservation until prompt
// dispatch is attempted or published. An attempt marker or referencing message
// is durable evidence that the prompt may have been accepted before a crash, so
// that turn remains current. The orchestrator keeps its live reservation private
// from ready handling until dispatch resolution.
//
// Startup recovery removes stale empty reservations before prompt admission
// resumes. A new live unattempted reservation remains intentionally excluded
// until dispatch resolution or durable attempt evidence makes it authoritative.
func turnAuthorityPredicate(driverName, turnAlias string) string {
	pending := turnDispatchPendingPredicate(driverName, turnAlias)
	attempted := turnDispatchAttemptedPredicate(driverName, turnAlias)
	// Positive invariant: a turn is authoritative once it is not a pure
	// reservation, or durable attempt/message evidence proves it was started.
	return fmt.Sprintf(`(
		NOT (%s)
		OR (%s)
		OR EXISTS (
			SELECT 1
			FROM task_session_messages turn_authority_message
			WHERE turn_authority_message.turn_id = %s.id
		)
	)`, pending, attempted, turnAlias)
}

// turnHistoryPredicate keeps live prompt reservations out of client history.
// An attempt marker is authority for crash recovery, but not publication: the
// reservation can still be rejected and rolled back without a turn event.
func turnHistoryPredicate(driverName, turnAlias string) string {
	pending := turnDispatchPendingPredicate(driverName, turnAlias)
	return fmt.Sprintf(`(
		NOT (%s)
		OR EXISTS (
			SELECT 1
			FROM task_session_messages turn_history_message
			WHERE turn_history_message.turn_id = %s.id
		)
	)`, pending, turnAlias)
}

func turnDispatchPendingPredicate(driverName, turnAlias string) string {
	return turnMetadataFlagPredicate(
		driverName, turnAlias, models.TurnMetaKeyPromptDispatchPending,
	)
}

func turnDispatchAttemptedPredicate(driverName, turnAlias string) string {
	return turnMetadataFlagPredicate(
		driverName, turnAlias, models.TurnMetaKeyPromptDispatchAttempted,
	)
}

// currentTurnAuthority composes turnAuthorityPredicate with R1's lifecycle
// exclusion into predicate, and D1's total order into orderBy. Every
// current-turn resolution site takes both values from this one call and
// hand-writes neither (AC-3).
//
// predicate is a single parenthesized conjunct for `WHERE ... AND %s`.
// orderBy is a bare, turnAlias-qualified expression list with no ORDER BY
// keyword and no LIMIT, so it drops into both `ORDER BY %s` and
// `ROW_NUMBER() OVER (... ORDER BY %s)`. Its first key ranks an open turn
// (completed_at IS NULL) ahead of a completed one regardless of timestamps;
// the remaining keys break ties by started_at, created_at, then id — all
// descending, with id (the primary key) providing a total order.
func currentTurnAuthority(driverName, turnAlias string) (predicate, orderBy string) {
	authority := turnAuthorityPredicate(driverName, turnAlias)
	lifecycle := turnLifecycleOnlyPredicate(driverName, turnAlias)
	predicate = fmt.Sprintf("(%s AND NOT (%s))", authority, lifecycle)
	orderBy = fmt.Sprintf(
		"CASE WHEN %s.completed_at IS NULL THEN 0 ELSE 1 END ASC, %s.started_at DESC, %s.created_at DESC, %s.id DESC",
		turnAlias, turnAlias, turnAlias, turnAlias,
	)
	return predicate, orderBy
}

func turnLifecycleOnlyPredicate(driverName, turnAlias string) string {
	return turnMetadataFlagPredicate(driverName, turnAlias, models.TurnMetaKeyLifecycleOnly)
}

func turnMetadataFlagPredicate(driverName, turnAlias, key string) string {
	value := dialect.JSONExtract(
		driverName,
		turnAlias+".metadata",
		key,
	)
	// Metadata normally stores JSON booleans, but older/manual rows can contain
	// equivalent string values. Normalizing to text keeps both dialects tolerant.
	return fmt.Sprintf("CAST(COALESCE(%s, '') AS TEXT) IN ('true', '1')", value)
}
