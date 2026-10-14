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
		*WorkflowEntryIdentity,
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
	TaskID                     string                 `json:"task_id"`
	SessionID                  string                 `json:"session_id"`
	SessionIncarnationID       string                 `json:"session_incarnation_id"`
	Content                    string                 `json:"content"`
	Model                      string                 `json:"model"`
	PlanMode                   bool                   `json:"plan_mode"`
	Attachments                []MessageAttachment    `json:"attachments"`
	Metadata                   map[string]interface{} `json:"metadata"`
	QueuedBy                   string                 `json:"queued_by"`
	WorkflowID                 string                 `json:"workflow_id,omitempty"`
	WorkflowStepID             string                 `json:"workflow_step_id,omitempty"`
	TransitionID               int64                  `json:"transition_id,omitempty"`
	LifecycleGeneration        int64                  `json:"lifecycle_generation,omitempty"`
	TaskResourceVersion        string                 `json:"task_resource_version,omitempty"`
	SessionResourceVersion     string                 `json:"session_resource_version,omitempty"`
	RejectPendingMove          bool                   `json:"reject_pending_move,omitempty"`
	EnforceTaskManagementClaim bool                   `json:"enforce_task_management_claim,omitempty"`
	ManagementInstallationID   string                 `json:"management_installation_id,omitempty"`
	ManagementInstanceKey      string                 `json:"management_instance_key,omitempty"`
	ExpectedClaimGeneration    int64                  `json:"expected_claim_generation,omitempty"`
}

func queueAdmissionFingerprint(identity QueueSessionIdentity, message *QueuedMessage, workflowEntries ...*WorkflowEntryIdentity) (string, error) {
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
	delete(metadata, MetadataQueueAdmissionIDs)
	input := queueAdmissionFingerprintInput{
		TaskID: identity.TaskID, SessionID: identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		Content:              message.Content, Model: message.Model, PlanMode: message.PlanMode,
		Attachments: attachments, Metadata: metadata, QueuedBy: message.QueuedBy,
	}
	var entry *WorkflowEntryIdentity
	if len(workflowEntries) > 0 {
		entry = workflowEntries[0]
	}
	if entry != nil {
		input.WorkflowID = entry.WorkflowID
		input.WorkflowStepID = entry.WorkflowStepID
		input.TransitionID = entry.TransitionID
		input.LifecycleGeneration = entry.LifecycleGeneration
		input.TaskResourceVersion = entry.ExpectedTaskResourceVersion
		input.SessionResourceVersion = entry.ExpectedSessionResourceVersion
		input.RejectPendingMove = entry.RejectPendingMove
		input.EnforceTaskManagementClaim = entry.EnforceTaskManagementClaim
		input.ManagementInstallationID = entry.ManagementInstallationID
		input.ManagementInstanceKey = entry.ManagementInstanceKey
		input.ExpectedClaimGeneration = entry.ExpectedClaimGeneration
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
