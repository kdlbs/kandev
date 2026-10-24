package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) initMessageShellOutputSchema() error {
	_, err := r.db.Exec(fmt.Sprintf(`
 CREATE TABLE IF NOT EXISTS task_message_shell_output (
  message_id TEXT PRIMARY KEY REFERENCES task_session_messages(id) ON DELETE CASCADE,
  identity TEXT NOT NULL,
  header TEXT NOT NULL,
  stdout_start BIGINT NOT NULL,
  stdout_end BIGINT NOT NULL,
  stderr_start BIGINT NOT NULL,
  stderr_end BIGINT NOT NULL,
  last_sequence BIGINT NOT NULL
 );
 CREATE TABLE IF NOT EXISTS task_message_shell_output_chunks (
  message_id TEXT NOT NULL REFERENCES task_message_shell_output(message_id) ON DELETE CASCADE,
  field TEXT NOT NULL,
  ordinal BIGINT NOT NULL,
  content %s NOT NULL,
  PRIMARY KEY(message_id,field,ordinal)
 );`, dialect.BlobType(r.db.DriverName())))
	return err
}
