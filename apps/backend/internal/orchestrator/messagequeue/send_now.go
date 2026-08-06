package messagequeue

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/messageconstraints"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	metadataUserMessageRecorded = "user_message_recorded"

	// MetadataSendNowSources records the source identity and provenance that
	// was folded into a replacement prompt. The original rows remain in the
	// SendNowClaim so retry and acknowledgement can address them exactly.
	MetadataSendNowSources = "send_now_sources"
)

// restoreSendNowMetadata keeps the latest persisted metadata while carrying
// forward the monotonic transcript marker set after a replacement prompt was
// recorded. Other claim-time metadata must never overwrite edits made while
// the source was reserved.
func restoreSendNowMetadata(current, claim map[string]interface{}) map[string]interface{} {
	metadata := clearReservedMetadata(current)
	if recorded, _ := claim[metadataUserMessageRecorded].(bool); recorded {
		metadata[metadataUserMessageRecorded] = true
	}
	return metadata
}

var (
	ErrSendNowEmpty               = errors.New("send-now queue selection is empty")
	ErrSendNowClaimChanged        = errors.New("send-now queue selection changed")
	ErrSendNowReservationConflict = errors.New("send-now queue selection is reserved")
	ErrSendNowAttachmentOverflow  = errors.New("send-now attachments exceed the message limits")
	ErrSendNowReferenceOverflow   = errors.New("send-now references exceed the message limit")
)

// SendNowClaim is the durable handoff for an interrupt-and-replace dispatch.
// Sources are retained in FIFO order so every ordinary entry can be restored
// at its original position and every durable lifecycle row can be acknowledged
// only after the replacement prompt is accepted.
type SendNowClaim struct {
	Sources           []QueuedMessage  `json:"sources"`
	Dispatch          QueuedMessage    `json:"dispatch"`
	SourceGenerations map[string]int64 `json:"source_generations,omitempty"`
}

func sendNowSourceGenerationChanged(claim *SendNowClaim, source QueuedMessage, current int64) bool {
	if claim == nil || source.TaskID == "" || claim.SourceGenerations == nil {
		return false
	}
	expected, ok := claim.SourceGenerations[source.TaskID]
	return ok && expected != current
}

// ValidateSendNowEntries checks aggregate admission limits without building a
// dispatch envelope. Callers use it before interrupting an active turn; the
// repository still validates the exact snapshot again inside its claim
// transaction.
func ValidateSendNowEntries(entries []QueuedMessage) error {
	if len(entries) == 0 {
		return ErrSendNowEmpty
	}

	attachmentCount := 0
	attachmentBytes := int64(0)
	seenReferences := make(map[string]struct{})
	referenceCount := 0
	for _, entry := range entries {
		attachmentCount += len(entry.Attachments)
		if attachmentCount > messageconstraints.MaxAttachmentCount {
			return fmt.Errorf("%w: at most %d attachments are allowed", ErrSendNowAttachmentOverflow, messageconstraints.MaxAttachmentCount)
		}
		for _, attachment := range entry.Attachments {
			bytes, err := attachmentPayloadBytes(attachment)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrSendNowAttachmentOverflow, err)
			}
			attachmentBytes += bytes
			if attachmentBytes > messageconstraints.MaxAttachmentBytes {
				return fmt.Errorf("%w: total attachment size exceeds %d bytes", ErrSendNowAttachmentOverflow, messageconstraints.MaxAttachmentBytes)
			}
		}
		for _, reference := range entityrefs.NormalizePersisted(entry.Metadata[MetadataEntityReferences]) {
			if _, duplicate := seenReferences[reference.Ref]; duplicate {
				continue
			}
			seenReferences[reference.Ref] = struct{}{}
			referenceCount++
			if referenceCount > entityrefs.MaxReferencesPerMessage {
				return fmt.Errorf("%w: at most %d references are allowed", ErrSendNowReferenceOverflow, entityrefs.MaxReferencesPerMessage)
			}
		}
	}
	return nil
}

func requestedSendNowIDs(expected []QueuedMessage) (map[string]struct{}, error) {
	requested := make(map[string]struct{}, len(expected))
	for _, entry := range expected {
		if entry.ID == "" {
			return nil, ErrSendNowClaimChanged
		}
		if _, duplicate := requested[entry.ID]; duplicate {
			return nil, ErrSendNowClaimChanged
		}
		requested[entry.ID] = struct{}{}
	}
	return requested, nil
}

func validateSendNowSnapshot(selected []*QueuedMessage, expected []QueuedMessage) error {
	if len(selected) != len(expected) {
		return ErrSendNowClaimChanged
	}
	for index, entry := range selected {
		if !sameQueuedMessageContent(entry, &expected[index]) {
			return ErrSendNowClaimChanged
		}
	}
	return nil
}

// BuildSendNowEnvelope validates and combines an exact, FIFO-ordered source
// snapshot. It performs no repository mutation, which lets callers validate a
// bulk selection before interrupting an active turn.
func BuildSendNowEnvelope(entries []QueuedMessage) (*QueuedMessage, error) {
	if err := ValidateSendNowEntries(entries); err != nil {
		return nil, err
	}

	contentParts := make([]string, 0, len(entries))
	attachments := make([]MessageAttachment, 0)
	seenReferences := make(map[string]struct{})
	references := make([]apiv1.EntityReference, 0)
	sources := make([]map[string]interface{}, 0, len(entries))

	for _, entry := range entries {
		if entry.Content != "" {
			contentParts = append(contentParts, entry.Content)
		}
		attachments = append(attachments, entry.Attachments...)

		for _, reference := range entityrefs.NormalizePersisted(entry.Metadata[MetadataEntityReferences]) {
			if _, duplicate := seenReferences[reference.Ref]; duplicate {
				continue
			}
			seenReferences[reference.Ref] = struct{}{}
			references = append(references, reference)
		}

		sources = append(sources, map[string]interface{}{
			"id":        entry.ID,
			"task_id":   entry.TaskID,
			"position":  entry.Position,
			"queued_by": entry.QueuedBy,
			"queued_at": entry.QueuedAt,
			"metadata":  copyMessageMetadata(entry.Metadata, 0),
		})
	}

	oldest := entries[0]
	metadata := copyMessageMetadata(oldest.Metadata, 2)
	if len(references) == 0 {
		delete(metadata, MetadataEntityReferences)
	} else {
		metadata[MetadataEntityReferences] = references
	}
	metadata[MetadataSendNowSources] = sources

	return &QueuedMessage{
		ID:          "send-now-" + uuid.NewString(),
		SessionID:   oldest.SessionID,
		TaskID:      oldest.TaskID,
		Position:    oldest.Position,
		Content:     strings.Join(contentParts, "\n\n"),
		Model:       oldest.Model,
		PlanMode:    oldest.PlanMode,
		Attachments: attachments,
		Metadata:    metadata,
		QueuedAt:    oldest.QueuedAt,
		QueuedBy:    oldest.QueuedBy,
	}, nil
}

func attachmentPayloadBytes(attachment MessageAttachment) (int64, error) {
	if attachment.AttachmentID != "" {
		if attachment.SizeBytes < 0 {
			return 0, errors.New("attachment size is negative")
		}
		return attachment.SizeBytes, nil
	}
	if attachment.Data == "" {
		return 0, errors.New("attachment data is empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(attachment.Data)
	if err != nil {
		return 0, fmt.Errorf("attachment data is not valid base64: %w", err)
	}
	return int64(len(decoded)), nil
}
