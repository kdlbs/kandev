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
