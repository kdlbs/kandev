package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const feedbackMarker = "Review\n\n<!-- kandev-comment-files:upload -->"

func TestCommentAttachmentValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*MessageAttachment)
	}{
		{"missing ID", func(file *MessageAttachment) { file.AttachmentID = "" }},
		{"inline bytes", func(file *MessageAttachment) { file.Data = "inline" }},
		{"native delivery", func(file *MessageAttachment) { file.DeliveryMode = attachmentDeliveryPrompt }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := MessageAttachment{AttachmentID: "upload", CommentID: "upload", DeliveryMode: attachmentDeliveryPath}
			tc.change(&file)
			require.Error(t, ValidateCommentAttachments(feedbackMarker, []MessageAttachment{file}))
		})
	}
}

func TestRenderMultipleCommentFiles(t *testing.T) {
	files := []MessageAttachment{
		{AttachmentID: "upload", CommentID: "upload", DeliveryMode: attachmentDeliveryPath, Name: "first.txt", Path: "/one/first.txt"},
		{AttachmentID: "other", CommentID: "upload", DeliveryMode: attachmentDeliveryPath, Name: "second.txt", Path: "/two/second.txt"},
	}
	require.False(t, HasCommentAttachments([]MessageAttachment{{AttachmentID: "ordinary"}}))
	require.True(t, HasCommentAttachments(files))
	rendered, remaining, err := RenderCommentFiles(feedbackMarker, files)
	require.NoError(t, err)
	require.Empty(t, remaining)
	require.Equal(t, "Review\n\n- \"first.txt\": \"/one/first.txt\"\n- \"second.txt\": \"/two/second.txt\"", rendered)
	files[1].Path = ""
	_, _, err = RenderCommentFiles(feedbackMarker, files)
	require.Error(t, err)
}
