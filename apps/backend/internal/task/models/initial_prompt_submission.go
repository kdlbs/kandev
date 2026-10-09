package models

import (
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	SessionMetaKeyInitialPromptSubmission = "initial_prompt_submission"
	InitialPromptSubmissionVersion        = 1
	MaxInitialPromptSubmissionBytes       = 1 << 20

	InitialPromptSubmissionPending       = "pending"
	InitialPromptSubmissionDispatching   = "dispatching"
	InitialPromptSubmissionAccepted      = "accepted"
	InitialPromptSubmissionReplayBlocked = "replay_blocked"
)

var ErrInvalidInitialPromptSubmission = errors.New("invalid initial prompt submission")

// InitialPromptSubmission is the bounded, session-owned source for replaying a
// task's first prompt after a startup failure. Attachment bytes and inline data
// are intentionally excluded.
type InitialPromptSubmission struct {
	Version                  int                    `json:"version"`
	Content                  string                 `json:"content"`
	PlanMode                 bool                   `json:"plan_mode"`
	Attachments              []v1.MessageAttachment `json:"attachments"`
	InlineAttachmentCount    int                    `json:"inline_attachment_count,omitempty"`
	State                    string                 `json:"state"`
	ExecutionID              string                 `json:"execution_id,omitempty"`
	AttemptID                string                 `json:"attempt_id,omitempty"`
	DispatchStartedAt        string                 `json:"dispatch_started_at,omitempty"`
	AcceptedAt               string                 `json:"accepted_at,omitempty"`
	ReplayBlockedAt          string                 `json:"replay_blocked_at,omitempty"`
	ReplayBlockedReason      string                 `json:"replay_blocked_reason,omitempty"`
	ReplayBlockedExecutionID string                 `json:"replay_blocked_execution_id,omitempty"`
	ReplayBlockedTurnID      string                 `json:"replay_blocked_turn_id,omitempty"`
}

// NewInitialPromptSubmission copies the bounded submitted content and only
// file-backed attachment descriptors. Inline bytes are counted so recovery can
// refuse an incomplete replay without retaining them in session metadata.
func NewInitialPromptSubmission(content string, planMode bool, attachments []v1.MessageAttachment) (*InitialPromptSubmission, error) {
	if len(content) > MaxInitialPromptSubmissionBytes || len(attachments) > MaxMessageAttachmentCount {
		return nil, fmt.Errorf("%w: submission exceeds its size limit", ErrInvalidInitialPromptSubmission)
	}
	submission := &InitialPromptSubmission{
		Version:     InitialPromptSubmissionVersion,
		Content:     content,
		PlanMode:    planMode,
		Attachments: make([]v1.MessageAttachment, 0, len(attachments)),
		State:       InitialPromptSubmissionPending,
	}
	seen := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		if attachment.AttachmentID == "" {
			if attachment.Data != "" {
				submission.InlineAttachmentCount++
			}
			continue
		}
		if attachment.Data != "" || !attachment.HasValidDeliveryMode() {
			return nil, fmt.Errorf("%w: file-backed descriptor is malformed", ErrInvalidInitialPromptSubmission)
		}
		if _, duplicate := seen[attachment.AttachmentID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate attachment", ErrInvalidInitialPromptSubmission)
		}
		seen[attachment.AttachmentID] = struct{}{}
		deliveryMode := attachment.DeliveryMode
		if deliveryMode == "" {
			deliveryMode = "prompt"
		}
		submission.Attachments = append(submission.Attachments, v1.MessageAttachment{
			AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Name:         attachment.Name,
			MimeType:     attachment.MimeType,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: deliveryMode,
		})
	}
	return submission, nil
}

func LoadInitialPromptSubmission(metadata map[string]interface{}) (*InitialPromptSubmission, bool, error) {
	if metadata == nil {
		return nil, false, nil
	}
	raw, exists := metadata[SessionMetaKeyInitialPromptSubmission]
	if !exists || raw == nil {
		return nil, false, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, true, fmt.Errorf("%w: cannot encode stored value", ErrInvalidInitialPromptSubmission)
	}
	var submission InitialPromptSubmission
	if err := json.Unmarshal(payload, &submission); err != nil {
		return nil, true, fmt.Errorf("%w: cannot decode stored value", ErrInvalidInitialPromptSubmission)
	}
	if err := submission.Validate(); err != nil {
		return nil, true, err
	}
	return &submission, true, nil
}

func (submission *InitialPromptSubmission) Validate() error {
	if err := validateInitialPromptSubmissionBounds(submission); err != nil {
		return err
	}
	if err := validateInitialPromptSubmissionState(submission); err != nil {
		return err
	}
	return validateInitialPromptSubmissionAttachments(submission)
}

func validateInitialPromptSubmissionBounds(submission *InitialPromptSubmission) error {
	if submission == nil || submission.Version != InitialPromptSubmissionVersion ||
		len(submission.Content) > MaxInitialPromptSubmissionBytes ||
		submission.InlineAttachmentCount < 0 || len(submission.Attachments) > MaxMessageAttachmentCount {
		return fmt.Errorf("%w: unsupported or out-of-bounds record", ErrInvalidInitialPromptSubmission)
	}
	return nil
}

func validateInitialPromptSubmissionState(submission *InitialPromptSubmission) error {
	switch submission.State {
	case InitialPromptSubmissionPending, InitialPromptSubmissionDispatching, InitialPromptSubmissionAccepted,
		InitialPromptSubmissionReplayBlocked:
	default:
		return fmt.Errorf("%w: unknown delivery state", ErrInvalidInitialPromptSubmission)
	}
	if submission.State == InitialPromptSubmissionDispatching && submission.DispatchStartedAt == "" {
		return fmt.Errorf("%w: dispatch receipt is incomplete", ErrInvalidInitialPromptSubmission)
	}
	if submission.State == InitialPromptSubmissionAccepted && submission.AcceptedAt == "" {
		return fmt.Errorf("%w: acceptance receipt is incomplete", ErrInvalidInitialPromptSubmission)
	}
	if submission.State == InitialPromptSubmissionReplayBlocked &&
		(submission.ReplayBlockedAt == "" || submission.ReplayBlockedReason == "") {
		return fmt.Errorf("%w: replay-blocked receipt is incomplete", ErrInvalidInitialPromptSubmission)
	}
	return nil
}

func validateInitialPromptSubmissionAttachments(submission *InitialPromptSubmission) error {
	seen := make(map[string]struct{}, len(submission.Attachments))
	for _, attachment := range submission.Attachments {
		if attachment.AttachmentID == "" || attachment.Data != "" || !attachment.HasValidDeliveryMode() {
			return fmt.Errorf("%w: invalid file-backed descriptor", ErrInvalidInitialPromptSubmission)
		}
		if _, duplicate := seen[attachment.AttachmentID]; duplicate {
			return fmt.Errorf("%w: duplicate attachment", ErrInvalidInitialPromptSubmission)
		}
		seen[attachment.AttachmentID] = struct{}{}
	}
	return nil
}

func (submission *InitialPromptSubmission) HasReplayableContent() bool {
	return submission != nil && (submission.Content != "" || len(submission.Attachments) > 0)
}
