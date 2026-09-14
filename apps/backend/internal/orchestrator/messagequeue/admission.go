package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/task/plancomments"
)

// MaxQueueAdmissionIDLength bounds the caller-owned identity retained by a
// queue admission receipt.
const MaxQueueAdmissionIDLength = 128

// ErrQueueAdmissionUnavailable means the repository cannot provide durable
// replay protection for an identified queue admission.
var ErrQueueAdmissionUnavailable = errors.New("durable queue admission is unavailable")

type queueAdmissionRepository interface {
	LookupQueueAdmission(
		context.Context,
		QueueSessionIdentity,
		string,
		*QueuedMessage,
	) (*QueuedMessage, bool, error)
	AdmitQueueMessage(
		context.Context,
		QueueSessionIdentity,
		string,
		*QueuedMessage,
		*QueueAttachmentClaim,
		int,
		*AutoMergePolicy,
	) (*QueuedMessage, bool, error)
}

type queueAdmissionKey struct {
	TaskID               string
	SessionID            string
	SessionIncarnationID string
	ClientQueueID        string
}

type queueAdmissionReceipt struct {
	Fingerprint string
	Message     *QueuedMessage
}

type queueAdmissionFingerprintInput struct {
	TaskID               string                 `json:"task_id"`
	SessionID            string                 `json:"session_id"`
	SessionIncarnationID string                 `json:"session_incarnation_id"`
	Content              string                 `json:"content"`
	Model                string                 `json:"model"`
	PlanMode             bool                   `json:"plan_mode"`
	Attachments          []MessageAttachment    `json:"attachments"`
	Metadata             map[string]interface{} `json:"metadata"`
	QueuedBy             string                 `json:"queued_by"`
}

func queueAdmissionFingerprint(identity QueueSessionIdentity, message *QueuedMessage) (string, error) {
	if message == nil {
		return "", errors.New("queue admission message is nil")
	}
	attachments := append([]MessageAttachment(nil), message.Attachments...)
	if attachments == nil {
		attachments = []MessageAttachment{}
	}
	metadata := copyMessageMetadata(message.Metadata, 0)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	delete(metadata, plancomments.MetadataClientQueueID)
	delete(metadata, plancomments.MetadataRequestFingerprint)
	delete(metadata, plancomments.MetadataClientMessageFingerprint)
	input := queueAdmissionFingerprintInput{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		Content:              message.Content, Model: message.Model, PlanMode: message.PlanMode,
		Attachments: attachments, Metadata: metadata, QueuedBy: message.QueuedBy,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func queueAdmissionResponseSnapshot(message *QueuedMessage) *QueuedMessage {
	snapshot := cloneQueuedMessage(message)
	if snapshot == nil {
		return nil
	}
	for index := range snapshot.Attachments {
		snapshot.Attachments[index].Data = ""
	}
	return snapshot
}
