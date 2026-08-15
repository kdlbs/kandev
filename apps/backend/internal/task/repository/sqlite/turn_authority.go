package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// turnAuthorityPredicate excludes an empty successor reservation until prompt
// dispatch is attempted or published. An attempt marker or referencing message
// is durable evidence that the prompt may have been accepted before a crash, so
// that turn remains current.
func turnAuthorityPredicate(driverName, turnAlias string) string {
	pending := turnDispatchPendingPredicate(driverName, turnAlias)
	attempted := turnDispatchAttemptedPredicate(driverName, turnAlias)
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
	value := dialect.JSONExtract(
		driverName,
		turnAlias+".metadata",
		models.TurnMetaKeyPromptDispatchPending,
	)
	if dialect.IsPostgres(driverName) {
		return fmt.Sprintf("COALESCE(%s, '') IN ('true', '1')", value)
	}
	return fmt.Sprintf("COALESCE(%s, 0) = 1", value)
}

func turnDispatchAttemptedPredicate(driverName, turnAlias string) string {
	value := dialect.JSONExtract(
		driverName,
		turnAlias+".metadata",
		models.TurnMetaKeyPromptDispatchAttempted,
	)
	if dialect.IsPostgres(driverName) {
		return fmt.Sprintf("COALESCE(%s, '') IN ('true', '1')", value)
	}
	return fmt.Sprintf("COALESCE(%s, 0) = 1", value)
}
