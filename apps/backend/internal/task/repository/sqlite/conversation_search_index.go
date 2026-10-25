package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// BackfillConversationSearch advances one durable keyset page without reading bodies.
func (r *Repository) BackfillConversationSearch(ctx context.Context, limit int) (bool, error) {
	limit = min(max(limit, 1), 512)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var upper, cursor string
	var complete int
	if err = tx.QueryRowContext(ctx, `SELECT upper_id,cursor_id,complete FROM task_conversation_index_state WHERE id=1`).Scan(&upper, &cursor, &complete); err != nil {
		return false, err
	}
	if complete == 1 {
		return true, nil
	}
	var ids []string
	err = tx.SelectContext(ctx, &ids, r.db.Rebind(`SELECT id FROM task_session_messages WHERE id > ? AND id <= ? AND type='message' AND author_type IN ('user','agent') ORDER BY id LIMIT ?`), cursor, upper, limit)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_conversation_dirty(message_id,token,origin) VALUES(?,?,'backfill') ON CONFLICT(message_id) DO NOTHING`), id, uuid.NewString())
		if err != nil {
			return false, err
		}
		cursor = id
	}
	if len(ids) < limit {
		complete = 1
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_conversation_index_state SET cursor_id=?,complete=? WHERE id=1`), cursor, complete)
	if err != nil {
		return false, err
	}
	return complete == 1, tx.Commit()
}

type conversationDocument struct {
	messageID, token                     string
	sessionID, body, author, messageType sql.NullString
	createdAt                            sql.NullTime
}

// ProcessConversationSearchBatch coalesces source updates before indexing text.
func (r *Repository) ProcessConversationSearchBatch(ctx context.Context, historical bool) (int, error) {
	origin := conversationOriginLive
	if historical {
		origin = conversationOriginBackfill
	}
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`SELECT message_id FROM task_conversation_dirty WHERE origin=? ORDER BY queued_at,message_id LIMIT 32`), origin)
	if err != nil {
		return 0, err
	}
	documents := make([]conversationDocument, 0, len(ids))
	bytes := 0
	for _, id := range ids {
		doc, err := r.readConversationDocument(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return len(documents), err
		}
		if len(documents) > 0 && bytes+len(doc.body.String) > 256*1024 {
			break
		}
		documents = append(documents, doc)
		bytes += len(doc.body.String)
		if bytes >= 256*1024 {
			break
		}
	}
	return len(documents), r.publishConversationDocuments(ctx, documents)
}

func (r *Repository) readConversationDocument(ctx context.Context, id string) (conversationDocument, error) {
	doc := conversationDocument{messageID: id}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT q.token,m.task_session_id,m.content,m.author_type,m.type,m.created_at
  FROM task_conversation_dirty q LEFT JOIN task_session_messages m ON m.id=q.message_id WHERE q.message_id=?`), id).
		Scan(&doc.token, &doc.sessionID, &doc.body, &doc.author, &doc.messageType, &doc.createdAt)
	return doc, err
}

func (r *Repository) publishConversationDocument(ctx context.Context, doc conversationDocument) error {
	return r.publishConversationDocuments(ctx, []conversationDocument{doc})
}

func (r *Repository) publishConversationDocuments(ctx context.Context, docs []conversationDocument) error {
	if len(docs) == 0 {
		return nil
	}
	normalized := make([]string, len(docs))
	keys := make([]string, len(docs))
	for i, doc := range docs {
		normalized[i] = strings.ToLower(doc.body.String)
		if dialect.IsPostgres(r.db.DriverName()) {
			keys[i] = conversationIndexKey(normalized[i])
		}
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, doc := range docs {
		if err = r.publishConversationTx(ctx, tx, doc, normalized[i], keys[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) publishConversationTx(ctx context.Context, tx *sqlx.Tx, doc conversationDocument, normalized, key string) error {
	query := `SELECT token FROM task_conversation_dirty WHERE message_id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause + " SKIP LOCKED"
	}
	var token string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), doc.messageID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if token != doc.token {
		return nil
	}
	eligible := models.MessageType(doc.messageType.String) == models.MessageTypeMessage && (models.MessageAuthorType(doc.author.String) == models.MessageAuthorUser || models.MessageAuthorType(doc.author.String) == models.MessageAuthorAgent)
	if eligible {
		query = `INSERT INTO task_conversation_documents(message_id,session_id,body,search_text,token,created_at) VALUES(?,?,?,?,?,?)
   ON CONFLICT(message_id) DO UPDATE SET session_id=excluded.session_id,body=excluded.body,search_text=excluded.search_text,token=excluded.token,created_at=excluded.created_at`
		args := []any{doc.messageID, doc.sessionID.String, doc.body.String, normalized, doc.token, doc.createdAt.Time}
		if dialect.IsPostgres(r.db.DriverName()) {
			query = `INSERT INTO task_conversation_documents(message_id,session_id,body,search_text,token,created_at,index_text) VALUES(?,?,?,?,?,?,?)
    ON CONFLICT(message_id) DO UPDATE SET session_id=excluded.session_id,body=excluded.body,search_text=excluded.search_text,token=excluded.token,created_at=excluded.created_at,index_text=excluded.index_text`
			args = append(args, key)
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(query), args...)
	} else {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_conversation_documents WHERE message_id=?`), doc.messageID)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_conversation_dirty WHERE message_id=? AND token=?`), doc.messageID, doc.token)
	return err
}

// ConversationSearchReady checks capability without scanning historical messages.
func (r *Repository) ConversationSearchReady(ctx context.Context) bool {
	var id int
	return r.ro.QueryRowContext(ctx, `SELECT id FROM task_conversation_index_state WHERE id=1`).Scan(&id) == nil
}

// ConversationSearchPending reports only the caller's workspace backlog.
func (r *Repository) ConversationSearchPending(ctx context.Context, workspaceID string) (bool, error) {
	var complete, pending int
	if err := r.ro.QueryRowContext(ctx, `SELECT complete FROM task_conversation_index_state WHERE id=1`).Scan(&complete); err != nil {
		return false, err
	}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT CASE WHEN EXISTS(
  SELECT 1 FROM task_conversation_dirty q JOIN task_session_messages m ON m.id=q.message_id
  JOIN task_sessions s ON s.id=m.task_session_id JOIN tasks t ON t.id=s.task_id WHERE t.workspace_id=?) THEN 1 ELSE 0 END`), workspaceID).Scan(&pending)
	return complete == 0 || pending == 1, err
}

const (
	conversationOriginLive     = "live"
	conversationOriginBackfill = "backfill"
)

// conversationIndexKey preserves ASCII selectivity and encodes other UTF-8 bytes
// as ASCII letters/digits so pg_trgm also works under the database C locale.
func conversationIndexKey(text string) string {
	const digits = "0123456789abcdef"
	var key strings.Builder
	key.Grow(len(text))
	for i := 0; i < len(text); i++ {
		value := text[i]
		if value < 128 {
			key.WriteByte(value)
			continue
		}
		key.WriteByte('u')
		key.WriteByte(digits[value>>4])
		key.WriteByte(digits[value&15])
	}
	return key.String()
}
