package models

import (
	"encoding/json"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestInitialPromptPreviewOmitsInlineBytes(t *testing.T) {
	attachments := []v1.MessageAttachment{
		{Type: "image", Data: "legacy-bytes"},
		{AttachmentID: "image-1", Type: "image", Name: "screen.png", MimeType: "image/png", Data: "private-bytes", SizeBytes: 32},
	}
	preview := NewInitialPromptPreview("prompt", attachments)
	require.Len(t, preview.Attachments, 1)
	require.Empty(t, preview.Attachments[0].Data)
	encoded, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "legacy-bytes")
	require.NotContains(t, string(encoded), "private-bytes")
	attachments[1].Name = "changed"
	require.Equal(t, "screen.png", preview.Attachments[0].Name)
}
