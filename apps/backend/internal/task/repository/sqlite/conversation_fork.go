package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	conversationForkMaxSourceRows     = 10000
	conversationForkMaxSourceBytes    = 4 << 20
	conversationForkCandidatePageSize = 100
)

func (r *Repository) initConversationForkSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS task_conversation_forks (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			source_task_id TEXT NOT NULL,
			source_session_id TEXT NOT NULL,
			source_message_id TEXT NOT NULL,
			start_message_id TEXT NOT NULL DEFAULT '',
			source_revision BIGINT NOT NULL,
			source_task_title TEXT NOT NULL DEFAULT '',
			source_session_name TEXT NOT NULL DEFAULT '',
			compiler_version TEXT NOT NULL,
			compiled_text TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			selection_json TEXT NOT NULL,
			omissions_json TEXT NOT NULL,
			attachments_json TEXT NOT NULL DEFAULT '[]',
			estimate_json TEXT NOT NULL DEFAULT '{}',
			message_count INTEGER NOT NULL,
			text_bytes INTEGER NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			state TEXT NOT NULL CHECK (state IN ('draft', 'attached', 'discarded')),
			destination_kind TEXT NOT NULL DEFAULT '',
			destination_complete BOOLEAN NOT NULL DEFAULT FALSE,
			destination_task_id TEXT NOT NULL DEFAULT '',
			destination_session_id TEXT NOT NULL DEFAULT '',
			draft_request_id TEXT,
			draft_request_fingerprint TEXT NOT NULL DEFAULT '',
			destination_request_id TEXT,
			destination_request_fingerprint TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)`)
	if err != nil {
		return fmt.Errorf("create task conversation forks: %w", err)
	}
	if err := r.migrateConversationForkDestinationComplete(); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS task_conversation_fork_owners (
			owner_id TEXT PRIMARY KEY
		)`); err != nil {
		return fmt.Errorf("create conversation fork owner locks: %w", err)
	}
	if _, err := r.db.ExecContext(r.migrationContext(), `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_task_conversation_forks_draft_request
		ON task_conversation_forks(owner_id, draft_request_id)
		WHERE draft_request_id IS NOT NULL`); err != nil {
		return fmt.Errorf("create conversation fork draft request index: %w", err)
	}
	if _, err := r.db.ExecContext(r.migrationContext(), `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_task_conversation_forks_destination_request
		ON task_conversation_forks(owner_id, destination_request_id)
		WHERE destination_request_id IS NOT NULL`); err != nil {
		return fmt.Errorf("create conversation fork destination request index: %w", err)
	}
	if _, err := r.db.ExecContext(r.migrationContext(), `
		CREATE INDEX IF NOT EXISTS idx_task_conversation_forks_expiry
		ON task_conversation_forks(state, expires_at)`); err != nil {
		return fmt.Errorf("create conversation fork expiry index: %w", err)
	}
	return nil
}

func (r *Repository) migrateConversationForkDestinationComplete() error {
	tx, err := r.db.BeginTxx(r.migrationContext(), nil)
	if err != nil {
		return fmt.Errorf("begin conversation fork destination completion migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := r.columnExists(tx, "task_conversation_forks", "destination_complete")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.ExecContext(r.migrationContext(), `ALTER TABLE task_conversation_forks ADD COLUMN destination_complete BOOLEAN NOT NULL DEFAULT FALSE`); err != nil {
			return fmt.Errorf("add conversation fork destination completion: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conversation fork destination completion migration: %w", err)
	}
	return nil
}

func (r *Repository) ReadConversationForkSource(ctx context.Context, req models.ConversationForkSourceRequest) (models.ConversationForkSource, error) {
	return r.readConversationForkSourceWithLimits(ctx, req, conversationForkMaxSourceRows, conversationForkMaxSourceBytes)
}

func (r *Repository) readConversationForkSourceWithLimits(ctx context.Context, req models.ConversationForkSourceRequest, maxRows, maxBytes int) (models.ConversationForkSource, error) {
	tx, err := r.beginConversationSourceRead(ctx)
	if err != nil {
		return models.ConversationForkSource{}, err
	}
	defer func() { _ = tx.Rollback() }()
	source, err := r.readConversationForkSourceTx(ctx, tx, req, maxRows, maxBytes, nil)
	if err != nil {
		return models.ConversationForkSource{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationForkSource{}, fmt.Errorf("commit conversation fork source read: %w", err)
	}
	return source, nil
}

func (r *Repository) readConversationForkSourceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	req models.ConversationForkSourceRequest,
	maxRows, maxBytes int,
	afterRevision func(),
) (models.ConversationForkSource, error) {
	if req.SessionID == "" || req.CutoffMessageID == "" || maxRows < 1 || maxBytes < 1 {
		return models.ConversationForkSource{}, models.ErrConversationForkCutoffUnavailable
	}
	source, start, cutoff, err := r.readConversationForkSourceHeader(ctx, tx, req, afterRevision)
	if err != nil {
		return models.ConversationForkSource{}, err
	}
	source.Messages, err = r.readConversationForkMessages(ctx, tx, req, start, cutoff, maxRows, maxBytes)
	if err != nil {
		return models.ConversationForkSource{}, err
	}
	if err := r.readConversationForkAttachments(ctx, tx, req, &source, start, cutoff); err != nil {
		return models.ConversationForkSource{}, err
	}
	return source, nil
}

func (r *Repository) readConversationForkSourceHeader(
	ctx context.Context,
	tx *sqlx.Tx,
	req models.ConversationForkSourceRequest,
	afterRevision func(),
) (models.ConversationForkSource, *conversationForkEndpoint, *conversationForkEndpoint, error) {
	state, err := readConversationState(ctx, tx, r.db.DriverName(), req.SessionID)
	if err != nil {
		return models.ConversationForkSource{}, nil, nil, err
	}
	if !state.Exists {
		return models.ConversationForkSource{}, nil, nil, models.ErrConversationForkSourceUnavailable
	}
	if afterRevision != nil {
		afterRevision()
	}
	source := models.ConversationForkSource{SessionID: req.SessionID, Revision: state.Revision}
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT session_row.task_id, task_row.workspace_id, task_row.title
		FROM task_sessions session_row
		JOIN tasks task_row ON task_row.id = session_row.task_id
		WHERE session_row.id = ?
	`), req.SessionID).Scan(&source.TaskID, &source.WorkspaceID, &source.TaskTitle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ConversationForkSource{}, nil, nil, models.ErrConversationForkSourceUnavailable
		}
		return models.ConversationForkSource{}, nil, nil, fmt.Errorf("read conversation fork task: %w", err)
	}
	cutoff, err := readConversationForkEndpoint(ctx, tx, r.db.DriverName(), req.SessionID, req.CutoffMessageID)
	if err != nil {
		return models.ConversationForkSource{}, nil, nil, fmt.Errorf("read conversation fork cutoff: %w", err)
	}
	if !validConversationForkCutoff(cutoff) {
		return models.ConversationForkSource{}, nil, nil, models.ErrConversationForkCutoffUnavailable
	}
	source.CutoffTurnComplete = cutoff.turnComplete
	var start *conversationForkEndpoint
	if req.StartMessageID != "" {
		start, err = readConversationForkEndpoint(ctx, tx, r.db.DriverName(), req.SessionID, req.StartMessageID)
		if err != nil {
			return models.ConversationForkSource{}, nil, nil, fmt.Errorf("read conversation fork start: %w", err)
		}
		if forkEndpointAfter(start, cutoff) {
			return models.ConversationForkSource{}, nil, nil, models.ErrConversationForkCutoffUnavailable
		}
	}
	return source, start, cutoff, nil
}

func (r *Repository) readConversationForkAttachments(
	ctx context.Context,
	tx *sqlx.Tx,
	req models.ConversationForkSourceRequest,
	source *models.ConversationForkSource,
	start, cutoff *conversationForkEndpoint,
) error {
	attachmentWhere := conversationForkRangeWhereForAlias(r.db.DriverName(), "message_row", req.SessionID, start, cutoff)
	attachmentArgs := conversationForkRangeArgs(req.SessionID, start, cutoff)
	if len(req.SelectedAttachmentIDs) > models.MaxMessageAttachmentCount {
		return models.ErrConversationForkLimitExceeded
	}
	if len(req.SelectedAttachmentIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(req.SelectedAttachmentIDs)), ",")
		attachmentWhere += ` AND attachment.id IN (` + placeholders + `)`
		for _, id := range req.SelectedAttachmentIDs {
			attachmentArgs = append(attachmentArgs, id)
		}
	} else if req.AttachmentCursor != "" {
		attachmentWhere += ` AND attachment.id > ?`
		attachmentArgs = append(attachmentArgs, req.AttachmentCursor)
	}
	attachmentLimit := conversationForkCandidatePageSize + 1
	if len(req.SelectedAttachmentIDs) > 0 {
		attachmentLimit = len(req.SelectedAttachmentIDs) + 1
	}
	attachmentArgs = append(attachmentArgs, attachmentLimit)
	attachmentRows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT attachment.id, attachment.owner_id, attachment.workspace_id, attachment.task_id,
		       attachment.session_id, attachment.message_id, attachment.queue_id,
		       attachment.name, attachment.mime_type, attachment.kind, attachment.delivery_mode,
		       attachment.size_bytes, attachment.storage_key, attachment.state, attachment.expires_at,
		       attachment.created_at, attachment.updated_at
		FROM task_message_attachments attachment
		JOIN task_session_messages message_row ON message_row.id = attachment.message_id
		WHERE `+attachmentWhere+`
		  AND attachment.state = 'claimed'
		ORDER BY attachment.id ASC
		LIMIT ?
	`), attachmentArgs...)
	if err != nil {
		return fmt.Errorf("read conversation fork attachment candidates: %w", err)
	}
	for attachmentRows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := attachmentRows.StructScan(attachment); err != nil {
			_ = attachmentRows.Close()
			return fmt.Errorf("scan conversation fork attachment candidate: %w", err)
		}
		source.Attachments = append(source.Attachments, attachment)
	}
	if err := attachmentRows.Err(); err != nil {
		_ = attachmentRows.Close()
		return fmt.Errorf("iterate conversation fork attachment candidates: %w", err)
	}
	if err := attachmentRows.Close(); err != nil {
		return fmt.Errorf("close conversation fork attachment candidates: %w", err)
	}
	if len(req.SelectedAttachmentIDs) > 0 {
		if len(source.Attachments) != len(req.SelectedAttachmentIDs) {
			return models.ErrConversationForkAttachmentMissing
		}
	} else if len(source.Attachments) > conversationForkCandidatePageSize {
		source.AttachmentsHasMore = true
		source.Attachments = source.Attachments[:conversationForkCandidatePageSize]
		source.AttachmentCursor = source.Attachments[len(source.Attachments)-1].ID
	}
	return nil
}

func (r *Repository) readConversationForkMessages(
	ctx context.Context,
	tx *sqlx.Tx,
	req models.ConversationForkSourceRequest,
	start, cutoff *conversationForkEndpoint,
	maxRows, maxBytes int,
) ([]*models.Message, error) {
	where, args := conversationForkRangeWhere(r.db.DriverName(), req.SessionID, start, cutoff)
	rows, byteCount, err := countConversationForkSourceRows(ctx, tx, r.db.DriverName(), where, args, maxRows, req.IncludeToolEvidence)
	if err != nil {
		return nil, err
	}
	if rows > maxRows || byteCount > int64(maxBytes) {
		return nil, models.ErrConversationForkLimitExceeded
	}
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content,
		       requests_input, type, metadata, created_at, updated_at,
		       COALESCE(payload_digest, ''), COALESCE(payload_size, 0)
		FROM task_session_messages
		WHERE ` + where + `
		ORDER BY ` + dialect.NormalizedMicrosecond(r.db.DriverName(), "created_at") + ` ASC, id ASC`
	messageRows, err := tx.QueryxContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("read bounded conversation fork messages: %w", err)
	}
	messages, err := scanConversationForkMessages(messageRows)
	closeErr := messageRows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close conversation fork rows: %w", closeErr)
	}
	if len(messages) != rows {
		return nil, models.ErrConversationForkSourceUnavailable
	}
	if req.IncludeToolEvidence {
		if err := rehydrateConversationForkMessages(ctx, tx, r.db.DriverName(), messages); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func rehydrateConversationForkMessages(ctx context.Context, tx *sqlx.Tx, driver string, messages []*models.Message) error {
	for _, message := range messages {
		if message.PayloadDigest == "" {
			continue
		}
		if err := rehydrateConversationForkToolPayload(ctx, tx, driver, message); err != nil {
			if errors.Is(err, errConversationForkPayloadUnavailable) {
				message.PayloadUnavailable = true
				continue
			}
			return fmt.Errorf("rehydrate selected conversation fork tool evidence: %w", err)
		}
	}
	return nil
}

var errConversationForkPayloadUnavailable = errors.New("conversation fork tool payload is unavailable")

func rehydrateConversationForkToolPayload(ctx context.Context, tx *sqlx.Tx, driver string, message *models.Message) error {
	uncompressedSize, compressedSize, actualCompressedSize, err := readConversationForkPayloadSizes(ctx, tx, driver, message.PayloadDigest)
	if err != nil {
		return err
	}
	if uncompressedSize < 0 || uncompressedSize > maxMessagePayloadRehydrateBytes || compressedSize < 0 || compressedSize > maxMessagePayloadRehydrateBytes || actualCompressedSize != compressedSize || message.PayloadSize != uncompressedSize {
		return errConversationForkPayloadUnavailable
	}
	compressed, err := readConversationForkPayloadBytes(ctx, tx, message.PayloadDigest)
	if err != nil {
		return err
	}
	if int64(len(compressed)) != compressedSize {
		return errConversationForkPayloadUnavailable
	}
	payloadBytes, err := gzipDecompressExpectedSize(compressed, maxMessagePayloadRehydrateBytes, uncompressedSize)
	if err != nil {
		return errConversationForkPayloadUnavailable
	}
	if int64(len(payloadBytes)) != uncompressedSize || sha256Hex(payloadBytes) != message.PayloadDigest {
		return errConversationForkPayloadUnavailable
	}
	var output models.ShellExecOutputSnapshot
	if err := json.Unmarshal(payloadBytes, &output); err != nil {
		return errConversationForkPayloadUnavailable
	}
	if rehydrated, ok := models.RehydrateShellOutput(message.Metadata, output); ok {
		message.Metadata = rehydrated
	}
	message.PayloadDigest = ""
	message.PayloadSize = 0
	return nil
}

func readConversationForkPayloadSizes(ctx context.Context, tx *sqlx.Tx, driver, digest string) (int64, int64, int64, error) {
	var uncompressedSize, compressedSize, actualCompressedSize int64
	actualCompressedSizeExpr := "length(compressed_content)"
	if dialect.IsPostgres(driver) {
		actualCompressedSizeExpr = "octet_length(compressed_content)"
	}
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT uncompressed_size, compressed_size, `+actualCompressedSizeExpr+`
		FROM task_message_payloads WHERE digest = ?
	`), digest).Scan(&uncompressedSize, &compressedSize, &actualCompressedSize); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, 0, errConversationForkPayloadUnavailable
		}
		return 0, 0, 0, fmt.Errorf("load payload sizes: %w", err)
	}
	return uncompressedSize, compressedSize, actualCompressedSize, nil
}

func readConversationForkPayloadBytes(ctx context.Context, tx *sqlx.Tx, digest string) ([]byte, error) {
	var compressed []byte
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT compressed_content FROM task_message_payloads WHERE digest = ?`), digest).Scan(&compressed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errConversationForkPayloadUnavailable
		}
		return nil, fmt.Errorf("load payload bytes: %w", err)
	}
	return compressed, nil
}

type conversationForkEndpoint struct {
	id           string
	turnID       string
	authorType   string
	messageType  string
	content      string
	orderKey     string
	turnComplete bool
}

func readConversationForkEndpoint(ctx context.Context, tx *sqlx.Tx, driver, sessionID, messageID string) (*conversationForkEndpoint, error) {
	var endpoint conversationForkEndpoint
	var createdAt time.Time
	err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT message_row.id, message_row.turn_id, message_row.author_type, message_row.type,
		       message_row.content, message_row.created_at,
	       COALESCE(turn_row.completed_at IS NOT NULL, FALSE)
		FROM task_session_messages message_row
		LEFT JOIN task_session_turns turn_row
		  ON turn_row.id = message_row.turn_id AND turn_row.task_session_id = message_row.task_session_id
		WHERE message_row.task_session_id = ? AND message_row.id = ?
	`), sessionID, messageID).Scan(&endpoint.id, &endpoint.turnID, &endpoint.authorType, &endpoint.messageType, &endpoint.content, &createdAt, &endpoint.turnComplete)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrConversationForkCutoffUnavailable
	}
	if err != nil {
		return nil, err
	}
	if dialect.IsPostgres(driver) {
		endpoint.orderKey = formatPromptKey(createdAt)
	} else if err := tx.QueryRowxContext(ctx, tx.Rebind(fmt.Sprintf(
		`SELECT %s FROM task_session_messages WHERE id = ?`, dialect.NormalizedMicrosecond(driver, "created_at"),
	)), messageID).Scan(&endpoint.orderKey); err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func validConversationForkCutoff(endpoint *conversationForkEndpoint) bool {
	if endpoint == nil || (endpoint.messageType != "" && endpoint.messageType != string(models.MessageTypeMessage) && endpoint.messageType != string(models.MessageTypeContent)) {
		return false
	}
	if endpoint.authorType == string(models.MessageAuthorUser) {
		return true
	}
	return endpoint.authorType == string(models.MessageAuthorAgent) && endpoint.turnComplete && strings.TrimSpace(endpoint.content) != ""
}

func forkEndpointAfter(left, right *conversationForkEndpoint) bool {
	if left.orderKey == right.orderKey {
		return left.id > right.id
	}
	return left.orderKey > right.orderKey
}

func conversationForkRangeWhere(driver, sessionID string, start, cutoff *conversationForkEndpoint) (string, []interface{}) {
	return conversationForkRangeWhereForAlias(driver, "", sessionID, start, cutoff), conversationForkRangeArgs(sessionID, start, cutoff)
}

func conversationForkRangeWhereForAlias(driver, table, sessionID string, start, cutoff *conversationForkEndpoint) string {
	createdAt := "created_at"
	id := "id"
	session := "task_session_id"
	if table != "" {
		createdAt = table + ".created_at"
		id = table + ".id"
		session = table + ".task_session_id"
	}
	normalized := dialect.NormalizedMicrosecond(driver, createdAt)
	bound := "?"
	if dialect.IsPostgres(driver) {
		bound = conversationTimestampParameter
	}
	where := session + ` = ? AND (` + normalized + ` < ` + bound + ` OR (` + normalized + ` = ` + bound + ` AND ` + id + ` <= ?))`
	if start != nil {
		where += ` AND (` + normalized + ` > ` + bound + ` OR (` + normalized + ` = ` + bound + ` AND ` + id + ` >= ?))`
	}
	return where
}

func conversationForkRangeArgs(sessionID string, start, cutoff *conversationForkEndpoint) []interface{} {
	args := []interface{}{sessionID, cutoff.orderKey, cutoff.orderKey, cutoff.id}
	if start != nil {
		args = append(args, start.orderKey, start.orderKey, start.id)
	}
	return args
}

func countConversationForkSourceRows(ctx context.Context, tx *sqlx.Tx, driver, where string, args []interface{}, maxRows int, includeToolEvidence bool) (int, int64, error) {
	contentBytes := "length(CAST(COALESCE(content, '') AS BLOB))"
	metadataBytes := "length(CAST(COALESCE(metadata, '') AS BLOB))"
	payloadBytes := "0"
	if includeToolEvidence {
		payloadBytes = "COALESCE(payload_size, 0)"
	}
	if dialect.IsPostgres(driver) {
		contentBytes = "octet_length(COALESCE(content, ''))"
		metadataBytes = "octet_length(COALESCE(metadata::text, ''))"
	}
	query := `
		SELECT COUNT(*), COALESCE(SUM(source_bytes), 0)
		FROM (
			SELECT ` + contentBytes + ` + ` + metadataBytes + ` + ` + payloadBytes + ` AS source_bytes
			FROM task_session_messages
			WHERE ` + where + `
			ORDER BY ` + dialect.NormalizedMicrosecond(driver, "created_at") + ` ASC, id ASC
			LIMIT ?
		) bounded_source
	`
	queryArgs := append(append([]interface{}{}, args...), maxRows+1)
	var rows int
	var byteCount int64
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), queryArgs...).Scan(&rows, &byteCount); err != nil {
		return 0, 0, fmt.Errorf("count bounded conversation fork source: %w", err)
	}
	return rows, byteCount, nil
}

func scanConversationForkMessages(rows *sqlx.Rows) ([]*models.Message, error) {
	var messages []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var (
			requestsInput int
			messageType   string
			metadataJSON  string
		)
		if err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt, &message.PayloadDigest, &message.PayloadSize); err != nil {
			return nil, fmt.Errorf("scan conversation fork message: %w", err)
		}
		message.RequestsInput = requestsInput != 0
		message.Type = models.MessageType(messageType)
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, fmt.Errorf("decode conversation fork message metadata: %w", err)
			}
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read conversation fork messages: %w", err)
	}
	return messages, nil
}
