package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPreparedSessionSeedAppliesAndValidates(t *testing.T) {
	preview := NewInitialPromptPreview("hello", []v1.MessageAttachment{{
		AttachmentID: "attachment-1", Type: "image", Name: "screen.png", MimeType: "image/png", SizeBytes: 12,
	}})
	submission, err := NewInitialPromptSubmission("hello", true, nil)
	require.NoError(t, err)
	seed := &PreparedSessionSeed{InitialPromptPreview: preview, InitialPromptSubmission: submission}
	require.False(t, seed.Empty())
	require.NoError(t, seed.Validate())

	metadata := seed.ApplyToMetadata(nil)
	require.Equal(t, preview, metadata[SessionMetaKeyInitialPromptPreview])
	require.Equal(t, submission, metadata[SessionMetaKeyInitialPromptSubmission])

	submission.State = "not-a-state"
	require.ErrorIs(t, seed.Validate(), ErrInvalidInitialPromptSubmission)
}

func TestPreparedSessionSeedRejectsOversizedPreview(t *testing.T) {
	seed := &PreparedSessionSeed{InitialPromptPreview: NewInitialPromptPreview(
		strings.Repeat("x", MaxInitialPromptSubmissionBytes+1), nil,
	)}
	require.ErrorIs(t, seed.Validate(), ErrInvalidInitialPromptSubmission)
}

func TestPreparedSessionSeedEmptyIsSafe(t *testing.T) {
	var seed *PreparedSessionSeed
	require.True(t, seed.Empty())
	require.NoError(t, seed.Validate())
	require.Nil(t, seed.ApplyToMetadata(nil))

	require.True(t, (&PreparedSessionSeed{}).Empty())
}
