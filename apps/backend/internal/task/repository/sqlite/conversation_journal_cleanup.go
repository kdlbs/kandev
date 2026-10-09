package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// cleanupLegacyConversationJournal removes the payload journal after the
// source revision schema is ready. All DDL runs in one transaction so an
// interrupted upgrade leaves the old journal available for a later retry.
func (r *Repository) cleanupLegacyConversationJournal() error {
	tx, err := r.db.BeginTxx(r.migrationContext(), nil)
	if err != nil {
		return fmt.Errorf("begin conversation journal cleanup: %w", err)
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}

	statements := legacyConversationJournalCleanupSQL(r.db.DriverName())
	for index, statement := range statements {
		if r.failConversationJournalCleanupAfter != "" &&
			r.failConversationJournalCleanupAfter == fmt.Sprintf("statement-%d", index) {
			return rollback(fmt.Errorf("conversation journal cleanup failpoint at statement-%d", index))
		}
		if _, err := tx.ExecContext(r.migrationContext(), statement); err != nil {
			return rollback(fmt.Errorf("conversation journal cleanup statement %d: %w", index, err))
		}
	}
	if r.failConversationJournalCleanupAfter == "commit" {
		return rollback(fmt.Errorf("conversation journal cleanup failpoint before commit"))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conversation journal cleanup: %w", err)
	}
	return nil
}

func legacyConversationJournalCleanupSQL(driver string) []string {
	if dialect.IsPostgres(driver) {
		return []string{
			`DROP TRIGGER IF EXISTS conversation_message_journal_trigger ON task_session_messages`,
			`DROP TRIGGER IF EXISTS conversation_turn_journal_trigger ON task_session_turns`,
			`DROP TRIGGER IF EXISTS conversation_session_delete_trigger ON task_sessions`,
			`DROP FUNCTION IF EXISTS conversation_message_journal()`,
			`DROP FUNCTION IF EXISTS conversation_turn_journal()`,
			`DROP FUNCTION IF EXISTS conversation_session_delete_journal()`,
			`DROP FUNCTION IF EXISTS conversation_next_sequence(TEXT, BOOLEAN)`,
			`DROP FUNCTION IF EXISTS conversation_safe_jsonb(TEXT)`,
			`DROP FUNCTION IF EXISTS conversation_visible_content(TEXT)`,
			`DROP TABLE IF EXISTS conversation_session_events`,
			`DROP TABLE IF EXISTS conversation_message_versions`,
			`DROP TABLE IF EXISTS conversation_turn_versions`,
			`DROP TABLE IF EXISTS conversation_session_streams`,
			`DROP TABLE IF EXISTS conversation_journal_meta`,
		}
	}
	return []string{
		`DROP TRIGGER IF EXISTS conversation_message_insert`,
		`DROP TRIGGER IF EXISTS conversation_message_update`,
		`DROP TRIGGER IF EXISTS conversation_message_delete`,
		`DROP TRIGGER IF EXISTS conversation_turn_insert`,
		`DROP TRIGGER IF EXISTS conversation_turn_complete`,
		`DROP TRIGGER IF EXISTS conversation_turn_delete`,
		`DROP TRIGGER IF EXISTS conversation_session_delete`,
		`DROP TABLE IF EXISTS conversation_session_events`,
		`DROP TABLE IF EXISTS conversation_message_versions`,
		`DROP TABLE IF EXISTS conversation_turn_versions`,
		`DROP TABLE IF EXISTS conversation_session_streams`,
		`DROP TABLE IF EXISTS conversation_journal_meta`,
	}
}
