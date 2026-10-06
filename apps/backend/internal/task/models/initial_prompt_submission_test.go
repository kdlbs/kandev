package models

import (
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestNewInitialPromptSubmissionBoundsAndCopiesFileDescriptors(t *testing.T) {
	input := []v1.MessageAttachment{
		{AttachmentID: "image-id", Type: "image", Name: "screen.png", MimeType: "image/png", SizeBytes: 12, DeliveryMode: "prompt"},
		{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"},
		{AttachmentID: "resource-id", Type: "resource", Name: "report.zip", MimeType: "application/zip", SizeBytes: 34, DeliveryMode: "path"},
	}
	submission, err := NewInitialPromptSubmission("original content", true, input)
	require.NoError(t, err)
	require.Equal(t, InitialPromptSubmissionPending, submission.State)
	require.True(t, submission.PlanMode)
	require.Equal(t, "original content", submission.Content)
	require.Equal(t, 1, submission.InlineAttachmentCount)
	require.Equal(t, []string{"image-id", "resource-id"}, []string{
		submission.Attachments[0].AttachmentID,
		submission.Attachments[1].AttachmentID,
	})
	input[0].Name = "changed.png"
	require.Equal(t, "screen.png", submission.Attachments[0].Name)
}

func TestLoadInitialPromptSubmissionRejectsAmbiguousAndMalformedRecords(t *testing.T) {
	for name, raw := range map[string]interface{}{
		"dispatching without receipt": map[string]interface{}{
			"version": 1, "content": "original", "attachments": []interface{}{}, "state": "dispatching",
		},
		"inline bytes stored": map[string]interface{}{
			"version": 1, "content": "original", "state": "pending", "attachments": []interface{}{
				map[string]interface{}{"attachment_id": "id", "type": "image", "data": "aGVsbG8="},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, exists, err := LoadInitialPromptSubmission(map[string]interface{}{
				SessionMetaKeyInitialPromptSubmission: raw,
			})
			require.True(t, exists)
			require.ErrorIs(t, err, ErrInvalidInitialPromptSubmission)
		})
	}
}

func TestNewInitialPromptSubmissionRejectsUnboundedContent(t *testing.T) {
	_, err := NewInitialPromptSubmission(string(make([]byte, MaxInitialPromptSubmissionBytes+1)), false, nil)
	require.ErrorIs(t, err, ErrInvalidInitialPromptSubmission)
}

func TestInitialPromptSubmissionReplayBlockedRequiresProvenance(t *testing.T) {
	submission, err := NewInitialPromptSubmission("original", false, nil)
	require.NoError(t, err)
	submission.State = InitialPromptSubmissionReplayBlocked
	require.ErrorIs(t, submission.Validate(), ErrInvalidInitialPromptSubmission)

	submission.ReplayBlockedAt = "2026-10-06T00:00:00Z"
	submission.ReplayBlockedReason = "later_prompt_provider_admission"
	require.NoError(t, submission.Validate())
}
