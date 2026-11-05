package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// clarificationAttachmentPlan carries the authenticated scope and the ordered
// answer-file IDs supplied by the caller. Claims are keyed by question ID.
type clarificationAttachmentPlan struct {
	scope  models.ClarificationAttachmentScope
	claims []models.ClarificationAttachmentClaim
}

type clarificationAttachmentClaimUpdate struct {
	attachmentID string
	messageID    string
}

type clarificationAttachmentSelection struct {
	canonical map[string][]models.ClarificationAnswerAttachment
	released  []string
	updates   []clarificationAttachmentClaimUpdate
	scope     models.ClarificationAttachmentScope
}

// adoptClarificationAnswerAttachments validates the complete submitted file
// set and adopts its staged rows in the response transaction. Any invalid,
// expired, foreign, duplicate, or oversized file rolls the whole answer back.
func (r *Repository) adoptClarificationAnswerAttachments(
	ctx context.Context,
	tx *sqlx.Tx,
	messages []*models.Message,
	status string,
	plan *clarificationAttachmentPlan,
) (map[string][]models.ClarificationAnswerAttachment, []string, error) {
	selection, err := r.selectClarificationAnswerAttachments(ctx, tx, messages, status, plan)
	if err != nil {
		return nil, nil, err
	}
	if err := r.markClarificationAnswerAttachmentsClaimed(ctx, tx, selection); err != nil {
		return nil, nil, err
	}
	return selection.canonical, selection.released, nil
}

func (r *Repository) selectClarificationAnswerAttachments(
	ctx context.Context,
	tx *sqlx.Tx,
	messages []*models.Message,
	status string,
	plan *clarificationAttachmentPlan,
) (clarificationAttachmentSelection, error) {
	selection := clarificationAttachmentSelection{
		canonical: make(map[string][]models.ClarificationAnswerAttachment, len(messages)),
		scope:     plan.scope,
	}
	messagesByQuestion, err := clarificationMessagesByQuestion(messages)
	if err != nil {
		return selection, err
	}
	desiredByQuestion, err := desiredClarificationAnswerAttachmentIDs(messagesByQuestion, status, plan)
	if err != nil {
		return selection, err
	}
	now := time.Now().UTC()
	seen := make(map[string]struct{})
	var totalCount int
	var totalBytes int64
	for questionID, message := range messagesByQuestion {
		existing, err := clarificationManifest(message.Metadata)
		if err != nil {
			return selection, err
		}
		existingIDs := clarificationAttachmentIDSet(existing)
		desiredIDs := desiredByQuestion[questionID]
		canonical := make([]models.ClarificationAnswerAttachment, 0, len(desiredIDs))
		for _, id := range desiredIDs {
			if _, duplicate := seen[id]; duplicate {
				return selection, fmt.Errorf("%w: duplicate answer attachment %s", models.ErrAttachmentClaimConflict, id)
			}
			seen[id] = struct{}{}
			row, err := r.loadClarificationAnswerAttachment(ctx, tx, id)
			if err != nil {
				return selection, err
			}
			descriptor, staged, err := validateClarificationAnswerAttachment(row, message, existingIDs, plan.scope, now)
			if err != nil {
				return selection, err
			}
			if staged {
				selection.updates = append(selection.updates, clarificationAttachmentClaimUpdate{
					attachmentID: id,
					messageID:    message.ID,
				})
			}
			canonical = append(canonical, descriptor)
			totalCount++
			totalBytes += row.SizeBytes
		}
		if totalCount > models.MaxMessageAttachmentCount {
			return selection, models.ErrTooManyAttachments
		}
		if totalBytes > models.MaxMessageAttachmentBytes {
			return selection, models.ErrAttachmentTotalTooLarge
		}
		if len(canonical) == 0 {
			selection.canonical[questionID] = nil
		} else {
			selection.canonical[questionID] = canonical
		}
		selection.released = append(selection.released, removedClarificationAttachmentIDs(existing, desiredIDs)...)
	}
	return selection, nil
}

func (r *Repository) clarificationAttachmentQueueTablePresent(
	ctx context.Context,
	plan *clarificationAttachmentPlan,
) (bool, error) {
	if plan == nil {
		return false, nil
	}
	present, err := r.tableExistsContext(ctx, "queued_messages")
	if err != nil {
		return false, fmt.Errorf("probe queue attachment references: %w", err)
	}
	return present, nil
}

// completeClarificationClaim adopts the submitted file set, writes the terminal
// manifests, and expires removed files before the response transaction commits.
func (r *Repository) completeClarificationClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	status string,
	responses map[string]interface{},
	plan *clarificationAttachmentPlan,
	queueTablePresent bool,
) ([]string, error) {
	var manifests map[string][]models.ClarificationAnswerAttachment
	var released []string
	var err error
	if plan != nil {
		manifests, released, err = r.adoptClarificationAnswerAttachments(ctx, tx, messages, status, plan)
		if err != nil {
			return nil, err
		}
	}
	if err := r.completeClaimedClarificationMessagesWithManifests(
		ctx, tx, drv, messages, status, responses, manifests,
	); err != nil {
		return nil, err
	}
	if plan != nil {
		if err := r.expireRemovedClarificationAnswerAttachments(
			ctx, tx, plan.scope, released, queueTablePresent,
		); err != nil {
			return nil, err
		}
	}
	return released, nil
}

// expireRemovedClarificationAnswerAttachments marks removed retry files expired
// in the same transaction that drops their manifests. The existing expired
// sweep can then reclaim their bytes even if the process stops before the
// best-effort post-commit release runs. Queue and surviving transcript
// references keep a file claimed.
func (r *Repository) expireRemovedClarificationAnswerAttachments(
	ctx context.Context,
	tx *sqlx.Tx,
	scope models.ClarificationAttachmentScope,
	ids []string,
	queueTablePresent bool,
) error {
	if len(ids) == 0 || scope.TaskID == "" || scope.SessionID == "" {
		return nil
	}
	referenced, err := referencedAttachmentIDsTx(
		ctx, r, tx, scope.TaskID, scope.SessionID, queueTablePresent,
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		if _, keep := referenced[id]; keep {
			continue
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET state = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND session_id = ? AND state = ?
		`), models.AttachmentStateExpired, now, id, scope.TaskID, scope.SessionID,
			models.AttachmentStateClaimed); err != nil {
			return fmt.Errorf("expire removed clarification answer attachment %q: %w", id, err)
		}
	}
	return nil
}

func clarificationMessagesByQuestion(messages []*models.Message) (map[string]*models.Message, error) {
	out := make(map[string]*models.Message, len(messages))
	for _, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("clarification bundle contains an empty message")
		}
		questionID, _ := message.Metadata["question_id"].(string)
		if questionID == "" {
			return nil, fmt.Errorf("clarification message %s is missing question_id", message.ID)
		}
		if _, duplicate := out[questionID]; duplicate {
			return nil, fmt.Errorf("clarification bundle repeats question_id %q", questionID)
		}
		out[questionID] = message
	}
	return out, nil
}

// desiredClarificationAnswerAttachmentIDs resolves the final ordered set for
// every question. A rejection clears every owned retry manifest. An answered
// question without an explicit claim keeps its retained retry manifest.
func desiredClarificationAnswerAttachmentIDs(
	messagesByQuestion map[string]*models.Message,
	status string,
	plan *clarificationAttachmentPlan,
) (map[string][]string, error) {
	desired := make(map[string][]string, len(messagesByQuestion))
	for questionID, message := range messagesByQuestion {
		existing, err := clarificationManifest(message.Metadata)
		if err != nil {
			return nil, err
		}
		if status == clarificationStatusRejected {
			desired[questionID] = nil
			continue
		}
		desired[questionID] = clarificationAttachmentIDs(existing)
	}
	if status == clarificationStatusRejected {
		return desired, nil
	}
	claimedQuestions := make(map[string]struct{}, len(plan.claims))
	for _, claim := range plan.claims {
		if claim.QuestionID == "" {
			return nil, fmt.Errorf("%w: answer attachment claim is missing question_id", models.ErrAttachmentClaimConflict)
		}
		if _, owned := messagesByQuestion[claim.QuestionID]; !owned {
			// Terminal siblings already have a canonical response. Their
			// incoming descriptors are ignored, and their files are never
			// adopted or released by this claim.
			continue
		}
		if _, duplicate := claimedQuestions[claim.QuestionID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate answer attachment claim for %q", models.ErrAttachmentClaimConflict, claim.QuestionID)
		}
		claimedQuestions[claim.QuestionID] = struct{}{}
		desired[claim.QuestionID] = append([]string(nil), claim.AttachmentIDs...)
	}
	return desired, nil
}

func (r *Repository) loadClarificationAnswerAttachment(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
) (*models.TaskMessageAttachment, error) {
	var attachment models.TaskMessageAttachment
	err := tx.GetContext(ctx, &attachment, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE id = ?
	`), id)
	if errorsIsNoRows(err) {
		return nil, models.ErrAttachmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load answer attachment %q: %w", id, err)
	}
	return &attachment, nil
}

func validateClarificationAnswerAttachment(
	row *models.TaskMessageAttachment,
	message *models.Message,
	existingIDs map[string]struct{},
	scope models.ClarificationAttachmentScope,
	now time.Time,
) (models.ClarificationAnswerAttachment, bool, error) {
	if row == nil || row.OwnerID != scope.OwnerID || row.WorkspaceID != scope.WorkspaceID {
		return models.ClarificationAnswerAttachment{}, false, models.ErrAttachmentClaimConflict
	}
	staged, err := validateClarificationAttachmentState(row, message, existingIDs, scope, now)
	if err != nil {
		return models.ClarificationAnswerAttachment{}, false, err
	}
	if err := validateClarificationAnswerAttachmentMetadata(row); err != nil {
		return models.ClarificationAnswerAttachment{}, false, err
	}
	return models.ClarificationAnswerAttachment{
		AttachmentID: row.ID,
		Type:         row.Kind,
		Name:         row.Name,
		MimeType:     row.MimeType,
		SizeBytes:    row.SizeBytes,
		DeliveryMode: "path",
	}, staged, nil
}

func validateClarificationAttachmentState(
	row *models.TaskMessageAttachment,
	message *models.Message,
	existingIDs map[string]struct{},
	scope models.ClarificationAttachmentScope,
	now time.Time,
) (bool, error) {
	switch row.State {
	case models.AttachmentStateStaged:
		if row.TaskID != "" || row.SessionID != "" || row.MessageID != "" || row.QueueID != "" {
			return false, models.ErrAttachmentClaimConflict
		}
		if !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
			return false, models.ErrAttachmentClaimConflict
		}
		return true, nil
	case models.AttachmentStateClaimed:
		if row.TaskID != scope.TaskID || row.MessageID != message.ID ||
			(row.SessionID != "" && row.SessionID != scope.SessionID) {
			return false, models.ErrAttachmentClaimConflict
		}
		if _, retained := existingIDs[row.ID]; !retained {
			return false, models.ErrAttachmentClaimConflict
		}
		return false, nil
	default:
		return false, models.ErrAttachmentClaimConflict
	}
}

func validateClarificationAnswerAttachmentMetadata(row *models.TaskMessageAttachment) error {
	if strings.TrimSpace(row.Name) == "" || strings.ContainsAny(row.Name, "/\\\x00") {
		return fmt.Errorf("%w: invalid attachment name", models.ErrAttachmentInvalid)
	}
	if strings.TrimSpace(row.MimeType) == "" {
		return fmt.Errorf("%w: mime type is required", models.ErrAttachmentInvalid)
	}
	switch row.Kind {
	case "image", "audio", "resource":
	default:
		return fmt.Errorf("%w: unsupported attachment kind %q", models.ErrAttachmentInvalid, row.Kind)
	}
	if row.DeliveryMode != "path" {
		return fmt.Errorf("%w: answer attachments require path delivery", models.ErrAttachmentInvalid)
	}
	if row.SizeBytes < 0 || row.SizeBytes > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTooLarge
	}
	if row.StorageKey == "" || filepath.Base(row.StorageKey) != row.StorageKey {
		return fmt.Errorf("%w: invalid storage key", models.ErrAttachmentInvalid)
	}
	return nil
}

func (r *Repository) markClarificationAnswerAttachmentsClaimed(
	ctx context.Context,
	tx *sqlx.Tx,
	selection clarificationAttachmentSelection,
) error {
	now := time.Now().UTC()
	for _, update := range selection.updates {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = ?, session_id = ?, message_id = ?, state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
			  AND task_id = '' AND session_id = '' AND message_id = '' AND queue_id = ''
		`), selection.scope.TaskID, selection.scope.SessionID, update.messageID,
			models.AttachmentStateClaimed, now, update.attachmentID,
			selection.scope.OwnerID, selection.scope.WorkspaceID, models.AttachmentStateStaged)
		if err != nil {
			return fmt.Errorf("claim answer attachment %q: %w", update.attachmentID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count claimed answer attachment %q: %w", update.attachmentID, err)
		}
		if affected != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

func clarificationManifest(metadata map[string]interface{}) ([]models.ClarificationAnswerAttachment, error) {
	raw, ok := metadata["attachments"]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.([]models.ClarificationAnswerAttachment); ok {
		return typed, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal clarification attachment manifest: %w", err)
	}
	var manifest []models.ClarificationAnswerAttachment
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return nil, fmt.Errorf("decode clarification attachment manifest: %w", err)
	}
	return manifest, nil
}

func clarificationAttachmentIDs(manifest []models.ClarificationAnswerAttachment) []string {
	if len(manifest) == 0 {
		return nil
	}
	ids := make([]string, 0, len(manifest))
	for _, attachment := range manifest {
		if attachment.AttachmentID != "" {
			ids = append(ids, attachment.AttachmentID)
		}
	}
	return ids
}

func clarificationAttachmentIDSet(manifest []models.ClarificationAnswerAttachment) map[string]struct{} {
	out := make(map[string]struct{}, len(manifest))
	for _, attachment := range manifest {
		if attachment.AttachmentID != "" {
			out[attachment.AttachmentID] = struct{}{}
		}
	}
	return out
}

func removedClarificationAttachmentIDs(
	existing []models.ClarificationAnswerAttachment,
	desired []string,
) []string {
	if len(existing) == 0 {
		return nil
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, id := range desired {
		desiredSet[id] = struct{}{}
	}
	var removed []string
	for _, attachment := range existing {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, keep := desiredSet[attachment.AttachmentID]; !keep {
			removed = append(removed, attachment.AttachmentID)
		}
	}
	return removed
}
