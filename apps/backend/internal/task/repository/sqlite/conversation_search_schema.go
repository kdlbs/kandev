package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db/dialect"
	"go.uber.org/zap"
)

// Conversation search is optional when the database lacks its native text index.
func (r *Repository) initConversationSearch() error {
	ctx := r.migrationContext()
	if err := r.createConversationSearchSchema(ctx); err != nil {
		reportConversationSearchSetupFailure(ctx, r.log, err)
	}
	return nil
}

func (r *Repository) createConversationSearchSchema(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	idType := "INTEGER PRIMARY KEY"
	if dialect.IsPostgres(r.db.DriverName()) {
		idType = "BIGSERIAL PRIMARY KEY"
		if _, err = tx.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public`); err != nil {
			return err
		}
	}
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS task_conversation_documents (
   document_id %s, message_id TEXT NOT NULL UNIQUE, session_id TEXT NOT NULL,
   body TEXT NOT NULL, search_text TEXT NOT NULL, token TEXT NOT NULL,
   created_at TIMESTAMP NOT NULL)`, idType),
		`CREATE INDEX IF NOT EXISTS task_conversation_documents_session ON task_conversation_documents(session_id)`,
		`CREATE TABLE IF NOT EXISTS task_conversation_dirty (
   message_id TEXT PRIMARY KEY, token TEXT NOT NULL, origin TEXT NOT NULL,
   queued_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS task_conversation_dirty_order ON task_conversation_dirty(origin,queued_at,message_id)`,
		`CREATE TABLE IF NOT EXISTS task_conversation_index_state (
   id INTEGER PRIMARY KEY, upper_id TEXT NOT NULL, cursor_id TEXT NOT NULL DEFAULT '', complete INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		err = createConversationPostgresTriggers(ctx, tx)
	} else {
		err = createConversationSQLiteTriggers(ctx, tx)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_conversation_index_state(id,upper_id)
  SELECT 1,COALESCE(MAX(id),'') FROM task_session_messages WHERE true ON CONFLICT(id) DO NOTHING`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func createConversationSQLiteTriggers(ctx context.Context, tx *sqlx.Tx) error {
	statements := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS task_conversation_fts USING fts5(search_text, content='task_conversation_documents', content_rowid='document_id', tokenize='trigram case_sensitive 1')`,
		`CREATE TRIGGER IF NOT EXISTS task_conversation_doc_insert AFTER INSERT ON task_conversation_documents BEGIN
    INSERT INTO task_conversation_fts(rowid,search_text) VALUES(new.document_id,new.search_text); END`,
		`CREATE TRIGGER IF NOT EXISTS task_conversation_doc_delete AFTER DELETE ON task_conversation_documents BEGIN
    INSERT INTO task_conversation_fts(task_conversation_fts,rowid,search_text) VALUES('delete',old.document_id,old.search_text); END`,
		`CREATE TRIGGER IF NOT EXISTS task_conversation_doc_update AFTER UPDATE ON task_conversation_documents BEGIN
    INSERT INTO task_conversation_fts(task_conversation_fts,rowid,search_text) VALUES('delete',old.document_id,old.search_text);
    INSERT INTO task_conversation_fts(rowid,search_text) VALUES(new.document_id,new.search_text); END`,
	}
	for _, event := range []struct{ name, timing, id, predicate string }{
		{"insert", "INSERT", conversationTriggerNew, "new.type='message' AND new.author_type IN ('user','agent')"},
		{"update", "UPDATE OF content,type,author_type,task_session_id", conversationTriggerNew, "(new.type='message' AND new.author_type IN ('user','agent')) OR (old.type='message' AND old.author_type IN ('user','agent'))"},
		{"delete", "DELETE", conversationTriggerOld, "1"},
	} {
		cleanup := ""
		if event.name == "delete" {
			cleanup = `DELETE FROM task_conversation_documents WHERE message_id=old.id;`
		}
		statements = append(statements, fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS task_conversation_message_%s AFTER %s ON task_session_messages WHEN %s BEGIN
   INSERT INTO task_conversation_dirty(message_id,token,origin) VALUES(%s.id,lower(hex(randomblob(16))),'live')
   ON CONFLICT(message_id) DO UPDATE SET token=excluded.token,origin='live'; %s END`, event.name, event.timing, event.predicate, event.id, cleanup))
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func createConversationPostgresTriggers(ctx context.Context, tx *sqlx.Tx) error {
	statements := []string{
		`ALTER TABLE task_conversation_documents ADD COLUMN IF NOT EXISTS index_text TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS task_conversation_text ON task_conversation_documents USING gin(index_text public.gin_trgm_ops)`,
		`CREATE OR REPLACE FUNCTION task_conversation_track() RETURNS trigger LANGUAGE plpgsql AS $$
   DECLARE target TEXT; eligible BOOLEAN;
   BEGIN
    IF TG_OP='DELETE' THEN target:=OLD.id; eligible:=TRUE;
    ELSIF TG_OP='INSERT' THEN target:=NEW.id; eligible:=NEW.type='message' AND NEW.author_type IN ('user','agent');
    ELSE target:=NEW.id; eligible:=(NEW.type='message' AND NEW.author_type IN ('user','agent')) OR (OLD.type='message' AND OLD.author_type IN ('user','agent')); END IF;
    IF eligible THEN
     INSERT INTO task_conversation_dirty(message_id,token,origin) VALUES(target,gen_random_uuid()::text,'live')
     ON CONFLICT(message_id) DO UPDATE SET token=EXCLUDED.token,origin='live';
    END IF;
    IF TG_OP='DELETE' THEN DELETE FROM task_conversation_documents WHERE message_id=target; RETURN OLD; END IF;
    RETURN NEW;
   END $$`,
		`DROP TRIGGER IF EXISTS task_conversation_message_change ON task_session_messages`,
		`CREATE TRIGGER task_conversation_message_change AFTER INSERT OR DELETE OR UPDATE OF content,type,author_type,task_session_id ON task_session_messages FOR EACH ROW EXECUTE FUNCTION task_conversation_track()`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

const (
	conversationTriggerNew = "new"
	conversationTriggerOld = "old"
)

func reportConversationSearchSetupFailure(ctx context.Context, log *logger.Logger, err error) {
	if err == nil || log == nil || ctx.Err() != nil {
		return
	}
	log.Warn("conversation search unavailable", zap.String("reason", conversationSearchSetupReason(err)))
}

func conversationSearchSetupReason(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42501":
			return "postgres_extension_permission_denied"
		case "0A000", "58P01":
			return "postgres_extension_unavailable"
		}
		if strings.HasPrefix(pgErr.Code, "08") {
			return "database_unavailable"
		}
	}
	if strings.Contains(err.Error(), "no such module: fts5") {
		return "sqlite_fts5_unavailable"
	}
	if strings.Contains(err.Error(), "no such tokenizer: trigram") {
		return "sqlite_trigram_unavailable"
	}
	return "index_setup_failed"
}
