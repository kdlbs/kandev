package messagequeue

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/entityrefs"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	// MaxMessageAttachmentBytes and MaxMessageAttachmentCount are shared by
	// ordinary queue admission and send-now aggregation.
	MaxMessageAttachmentBytes int64 = 100 * 1024 * 1024
	MaxMessageAttachmentCount       = 10

	// MetadataSendNowSources records the source identity and provenance that
	// was folded into a replacement prompt. The original rows remain in the
	// SendNowClaim so retry and acknowledgement can address them exactly.
	MetadataSendNowSources = "send_now_sources"
)

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
	Sources  []QueuedMessage `json:"sources"`
	Dispatch QueuedMessage   `json:"dispatch"`
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
	if len(entries) == 0 {
		return nil, ErrSendNowEmpty
	}

	contentParts := make([]string, 0, len(entries))
	attachments := make([]MessageAttachment, 0)
	seenReferences := make(map[string]struct{})
	references := make([]apiv1.EntityReference, 0)
	sources := make([]map[string]interface{}, 0, len(entries))
	attachmentBytes := int64(0)

	for _, entry := range entries {
		if entry.Content != "" {
			contentParts = append(contentParts, entry.Content)
		}
		if len(attachments)+len(entry.Attachments) > MaxMessageAttachmentCount {
			return nil, fmt.Errorf("%w: at most %d attachments are allowed", ErrSendNowAttachmentOverflow, MaxMessageAttachmentCount)
		}
		for _, attachment := range entry.Attachments {
			bytes, err := attachmentPayloadBytes(attachment)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrSendNowAttachmentOverflow, err)
			}
			attachmentBytes += bytes
			if attachmentBytes > MaxMessageAttachmentBytes {
				return nil, fmt.Errorf("%w: total attachment size exceeds %d bytes", ErrSendNowAttachmentOverflow, MaxMessageAttachmentBytes)
			}
			attachments = append(attachments, attachment)
		}

		for _, reference := range entityrefs.NormalizePersisted(entry.Metadata[MetadataEntityReferences]) {
			if _, duplicate := seenReferences[reference.Ref]; duplicate {
				continue
			}
			seenReferences[reference.Ref] = struct{}{}
			references = append(references, reference)
			if len(references) > entityrefs.MaxReferencesPerMessage {
				return nil, fmt.Errorf("%w: at most %d references are allowed", ErrSendNowReferenceOverflow, entityrefs.MaxReferencesPerMessage)
			}
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
