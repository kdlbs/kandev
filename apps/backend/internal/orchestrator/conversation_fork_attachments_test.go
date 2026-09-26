package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestAppendConversationForkAttachmentsForInitialPrompt(t *testing.T) {
	base := []v1.MessageAttachment{{AttachmentID: "new-upload", Type: "resource", Name: "new.txt", SizeBytes: 3}}
	forked := []models.ConversationForkAttachment{{
		ID: "fork-copy", Name: "screen.png", MediaType: "image/png", Size: 5, Available: true,
	}}

	got, err := appendConversationForkAttachments(base, forked)

	require.NoError(t, err)
	require.Equal(t, []v1.MessageAttachment{
		base[0],
		{AttachmentID: "fork-copy", Type: "image", MimeType: "image/png", Name: "screen.png", SizeBytes: 5, DeliveryMode: "prompt"},
	}, got)
}

func TestAppendConversationForkAttachmentsChecksCombinedLimits(t *testing.T) {
	base := []v1.MessageAttachment{{AttachmentID: "new-upload", Type: "resource", SizeBytes: models.MaxMessageAttachmentBytes}}
	forked := []models.ConversationForkAttachment{{
		ID: "fork-copy", Name: "screen.png", MediaType: "image/png", Size: 1, Available: true,
	}}

	_, err := appendConversationForkAttachments(base, forked)

	require.ErrorIs(t, err, models.ErrConversationForkLimitExceeded)
}
