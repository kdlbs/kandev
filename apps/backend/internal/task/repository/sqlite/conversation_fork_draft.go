package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const conversationForkDraftColumns = `id, owner_id, workspace_id, source_task_id, source_session_id,
	source_message_id, start_message_id, source_revision, source_task_title, compiler_version,
	compiled_text, content_hash, selection_json, omissions_json, attachments_json, estimate_json,
	message_count, text_bytes, created_at, expires_at, state, draft_request_id, draft_request_fingerprint,
	COALESCE(destination_request_id, '') AS destination_request_id, destination_request_fingerprint,
	destination_kind, destination_complete, destination_task_id, destination_session_id`

func (r *Repository) CreateConversationForkDraft(ctx context.Context, draft *models.ConversationForkDraft) (models.ConversationForkDraft, error) {
	if draft == nil || draft.OwnerID == "" || draft.WorkspaceID == "" || draft.DraftRequestID == "" || draft.RequestFingerprint == "" {
		return models.ConversationForkDraft{}, models.ErrConversationForkConflict
	}
	now := time.Now().UTC()
	encoded, err := prepareConversationForkDraft(draft, now)
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return models.ConversationForkDraft{}, fmt.Errorf("begin conversation fork draft: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockConversationForkOwner(ctx, tx, draft.OwnerID); err != nil {
		return models.ConversationForkDraft{}, err
	}
	if existing, found, err := getExistingConversationForkDraft(ctx, tx, draft.OwnerID, draft.DraftRequestID, draft.RequestFingerprint); err != nil {
		return models.ConversationForkDraft{}, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return models.ConversationForkDraft{}, fmt.Errorf("commit conversation fork idempotency read: %w", err)
		}
		return existing, nil
	}
	if err := checkConversationForkDraftQuota(ctx, tx, draft.OwnerID, now); err != nil {
		return models.ConversationForkDraft{}, err
	}
	if err := insertConversationForkDraft(ctx, tx, draft, encoded, now); err != nil {
		return models.ConversationForkDraft{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationForkDraft{}, fmt.Errorf("commit conversation fork draft: %w", err)
	}
	draft.Descriptor.CreatedAt = now
	return *draft, nil
}

type encodedConversationForkDraft struct {
	omissions        string
	attachments      string
	estimate         string
	selection        string
	destinationReqID any
}

func prepareConversationForkDraft(draft *models.ConversationForkDraft, now time.Time) (encodedConversationForkDraft, error) {
	if draft.Descriptor.ID == "" {
		draft.Descriptor.ID = uuid.NewString()
	}
	if draft.Descriptor.ExpiresAt.IsZero() {
		draft.Descriptor.ExpiresAt = now.Add(24 * time.Hour)
	}
	if draft.Descriptor.State == "" {
		draft.Descriptor.State = conversationForkStateDraft
	}
	if draft.Descriptor.Omissions == nil {
		draft.Descriptor.Omissions = map[string]int{}
	}
	if draft.Descriptor.AttachmentDescriptors == nil {
		draft.Descriptor.AttachmentDescriptors = []models.ConversationForkAttachment{}
	}
	omissions, err := json.Marshal(draft.Descriptor.Omissions)
	if err != nil {
		return encodedConversationForkDraft{}, fmt.Errorf("encode conversation fork omissions: %w", err)
	}
	attachments, err := json.Marshal(draft.Descriptor.AttachmentDescriptors)
	if err != nil {
		return encodedConversationForkDraft{}, fmt.Errorf("encode conversation fork attachments: %w", err)
	}
	estimate, err := json.Marshal(draft.Descriptor.Estimate)
	if err != nil {
		return encodedConversationForkDraft{}, fmt.Errorf("encode conversation fork estimate: %w", err)
	}
	selection := draft.SelectionJSON
	if selection == "" {
		selection = "{}"
	}
	var destinationRequestID any
	if draft.DestinationRequestID != "" {
		destinationRequestID = draft.DestinationRequestID
	}
	return encodedConversationForkDraft{
		omissions: string(omissions), attachments: string(attachments), estimate: string(estimate),
		selection: selection, destinationReqID: destinationRequestID,
	}, nil
}

func (r *Repository) lockConversationForkOwner(ctx context.Context, tx *sqlx.Tx, ownerID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO task_conversation_fork_owners(owner_id) VALUES (?)
		ON CONFLICT (owner_id) DO NOTHING
	`), ownerID); err != nil {
		return fmt.Errorf("lock conversation fork owner: %w", err)
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		var lockedOwner string
		if err := tx.QueryRowxContext(ctx, `SELECT owner_id FROM task_conversation_fork_owners WHERE owner_id = $1 FOR UPDATE`, ownerID).Scan(&lockedOwner); err != nil {
			return fmt.Errorf("lock conversation fork quota: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_conversation_fork_owners SET owner_id = owner_id WHERE owner_id = ?`), ownerID); err != nil {
		return fmt.Errorf("lock conversation fork quota: %w", err)
	}
	return nil
}

func getExistingConversationForkDraft(ctx context.Context, tx *sqlx.Tx, ownerID, requestID, fingerprint string) (models.ConversationForkDraft, bool, error) {
	existing, err := getConversationForkDraftByRequest(ctx, tx, ownerID, requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ConversationForkDraft{}, false, nil
	}
	if err != nil {
		return models.ConversationForkDraft{}, false, err
	}
	if existing.RequestFingerprint != fingerprint {
		return models.ConversationForkDraft{}, false, models.ErrConversationForkConflict
	}
	return existing, true, nil
}

func checkConversationForkDraftQuota(ctx context.Context, tx *sqlx.Tx, ownerID string, now time.Time) error {
	var activeDrafts int
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT COUNT(*) FROM task_conversation_forks
		WHERE owner_id = ? AND state = 'draft' AND expires_at > ?
	`), ownerID, now).Scan(&activeDrafts); err != nil {
		return fmt.Errorf("count conversation fork drafts: %w", err)
	}
	if activeDrafts >= 20 {
		return models.ErrConversationForkQuotaExceeded
	}
	return nil
}

func insertConversationForkDraft(ctx context.Context, tx *sqlx.Tx, draft *models.ConversationForkDraft, encoded encodedConversationForkDraft, now time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO task_conversation_forks (
			id, owner_id, workspace_id, source_task_id, source_session_id, source_message_id,
			start_message_id, source_revision, source_task_title, compiler_version, compiled_text,
			content_hash, selection_json, omissions_json, attachments_json, estimate_json,
			message_count, text_bytes, created_at, expires_at, state, draft_request_id,
			draft_request_fingerprint, destination_request_id, destination_request_fingerprint,
			destination_kind, destination_task_id, destination_session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), draft.Descriptor.ID, draft.OwnerID, draft.WorkspaceID, draft.Descriptor.SourceTaskID,
		draft.Descriptor.SourceSessionID, draft.Descriptor.SourceMessageID, draft.Descriptor.StartMessageID,
		draft.Descriptor.SourceRevision, draft.Descriptor.SourceTaskTitle, draft.Descriptor.CompilerVersion,
		draft.CompiledText, draft.Descriptor.ContentHash, encoded.selection, encoded.omissions, encoded.attachments,
		encoded.estimate, draft.Descriptor.MessageCount, draft.Descriptor.TextBytes, now,
		draft.Descriptor.ExpiresAt, draft.Descriptor.State, draft.DraftRequestID, draft.RequestFingerprint,
		encoded.destinationReqID, draft.DestinationFingerprint, draft.Descriptor.DestinationKind, draft.Descriptor.DestinationTaskID, draft.Descriptor.DestinationSessionID)
	if err != nil {
		return fmt.Errorf("insert conversation fork draft: %w", err)
	}
	return nil
}

func (r *Repository) GetConversationForkDraft(ctx context.Context, ownerID, id string, now time.Time) (models.ConversationForkDraft, error) {
	if ownerID == "" || id == "" {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	draft, err := scanConversationForkDraft(r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT `+conversationForkDraftColumns+` FROM task_conversation_forks WHERE owner_id = ? AND id = ?
	`), ownerID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ConversationForkDraft{}, models.ErrConversationForkNotFound
	}
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	if draft.Descriptor.State == conversationForkStateDraft && !now.Before(draft.Descriptor.ExpiresAt) {
		return draft, models.ErrConversationForkExpired
	}
	return draft, nil
}

func (r *Repository) DiscardConversationForkDraft(ctx context.Context, ownerID, id string) error {
	if ownerID == "" || id == "" {
		return models.ErrConversationForkNotFound
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_conversation_forks
		SET state = 'discarded', compiled_text = '', content_hash = '', selection_json = '{}',
		    omissions_json = '{}', estimate_json = '{}', message_count = 0, text_bytes = 0, expires_at = ?
		WHERE owner_id = ? AND id = ? AND state = 'draft'
	`), now, ownerID, id)
	if err != nil {
		return fmt.Errorf("discard conversation fork draft: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count discarded conversation fork rows: %w", err)
	}
	if count > 0 {
		return nil
	}
	var state string
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT state FROM task_conversation_forks WHERE owner_id = ? AND id = ?
	`), ownerID, id).Scan(&state); errors.Is(err, sql.ErrNoRows) {
		return models.ErrConversationForkNotFound
	} else if err != nil {
		return fmt.Errorf("check conversation fork draft: %w", err)
	}
	if state == conversationForkStateDiscarded {
		return nil
	}
	return models.ErrConversationForkConflict
}

func (r *Repository) UpdateConversationForkEstimate(ctx context.Context, ownerID, id string, estimate models.ConversationForkEstimate) error {
	encoded, err := json.Marshal(estimate)
	if err != nil {
		return fmt.Errorf("encode conversation fork estimate: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_conversation_forks SET estimate_json = ?
		WHERE owner_id = ? AND id = ? AND state = 'draft'
	`), string(encoded), ownerID, id)
	if err != nil {
		return fmt.Errorf("update conversation fork estimate: %w", err)
	}
	if count, err := result.RowsAffected(); err == nil && count > 0 {
		return nil
	}
	var exists int
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_conversation_forks WHERE owner_id = ? AND id = ?
	`), ownerID, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return models.ErrConversationForkNotFound
	} else if err != nil {
		return fmt.Errorf("check conversation fork before estimate update: %w", err)
	}
	return models.ErrConversationForkConflict
}

func (r *Repository) DeleteExpiredConversationForkDrafts(ctx context.Context, now time.Time) ([]models.ConversationForkExpiredDraftAttachments, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		DELETE FROM task_conversation_forks
		WHERE state IN ('draft', 'discarded') AND expires_at <= ?
		RETURNING owner_id, attachments_json
	`), now)
	if err != nil {
		return nil, fmt.Errorf("delete expired conversation fork drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var expired []models.ConversationForkExpiredDraftAttachments
	for rows.Next() {
		var item models.ConversationForkExpiredDraftAttachments
		var attachments string
		if err := rows.Scan(&item.OwnerID, &attachments); err != nil {
			return nil, fmt.Errorf("scan expired conversation fork attachment copies: %w", err)
		}
		if err := json.Unmarshal([]byte(attachments), &item.Attachments); err != nil {
			return nil, fmt.Errorf("decode expired conversation fork attachment copies: %w", err)
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read expired conversation fork attachment copies: %w", err)
	}
	return expired, nil
}

type conversationForkDraftScanner interface {
	Scan(dest ...any) error
}

func scanConversationForkDraft(row conversationForkDraftScanner) (models.ConversationForkDraft, error) {
	var (
		draft                                                                   models.ConversationForkDraft
		selection, omissions, attachments, estimate, requestID, destinationKind string
	)
	d := &draft.Descriptor
	if err := row.Scan(
		&d.ID, &draft.OwnerID, &draft.WorkspaceID, &d.SourceTaskID, &d.SourceSessionID,
		&d.SourceMessageID, &d.StartMessageID, &d.SourceRevision, &d.SourceTaskTitle,
		&d.CompilerVersion, &draft.CompiledText, &d.ContentHash, &selection, &omissions,
		&attachments, &estimate, &d.MessageCount, &d.TextBytes, &d.CreatedAt, &d.ExpiresAt,
		&d.State, &requestID, &draft.RequestFingerprint, &draft.DestinationRequestID,
		&draft.DestinationFingerprint, &destinationKind, &d.DestinationComplete, &d.DestinationTaskID, &d.DestinationSessionID,
	); err != nil {
		return models.ConversationForkDraft{}, err
	}
	draft.DraftRequestID = requestID
	d.DestinationKind = destinationKind
	if err := json.Unmarshal([]byte(omissions), &d.Omissions); err != nil {
		return models.ConversationForkDraft{}, fmt.Errorf("decode conversation fork omissions: %w", err)
	}
	if err := json.Unmarshal([]byte(attachments), &d.AttachmentDescriptors); err != nil {
		return models.ConversationForkDraft{}, fmt.Errorf("decode conversation fork attachments: %w", err)
	}
	if err := json.Unmarshal([]byte(estimate), &d.Estimate); err != nil {
		return models.ConversationForkDraft{}, fmt.Errorf("decode conversation fork estimate: %w", err)
	}
	draft.SelectionJSON = selection
	return draft, nil
}

func getConversationForkDraftByRequest(ctx context.Context, tx *sqlx.Tx, ownerID, requestID string) (models.ConversationForkDraft, error) {
	return scanConversationForkDraft(tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT `+conversationForkDraftColumns+` FROM task_conversation_forks
		WHERE owner_id = ? AND draft_request_id = ?
	`), ownerID, requestID))
}
