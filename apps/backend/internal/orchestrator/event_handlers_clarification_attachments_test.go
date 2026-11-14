package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestClarificationAnswerAttachmentsV1UsesPathDelivery(t *testing.T) {
	attachments := clarificationAnswerAttachmentsV1([]models.ClarificationAnswerAttachment{{
		AttachmentID: "att-1",
		Type:         "image",
		Name:         "screen.png",
		MimeType:     "image/png",
		SizeBytes:    123,
		DeliveryMode: "path",
	}})
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v", attachments)
	}
	got := attachments[0]
	if got.AttachmentID != "att-1" || got.Type != "image" || got.Name != "screen.png" ||
		got.MimeType != "image/png" || got.SizeBytes != 123 || got.DeliveryMode != "path" {
		t.Fatalf("attachment = %+v", got)
	}
}

func TestClarificationAnswerAttachmentsQueueUsesPathDelivery(t *testing.T) {
	attachments := clarificationAnswerAttachmentsQueue([]models.ClarificationAnswerAttachment{{
		AttachmentID: "att-1",
		Type:         "resource",
		Name:         "log.txt",
		MimeType:     "text/plain",
		SizeBytes:    10,
		DeliveryMode: "path",
	}})
	if len(attachments) != 1 || attachments[0].AttachmentID != "att-1" ||
		attachments[0].Type != "resource" || attachments[0].Name != "log.txt" ||
		attachments[0].MimeType != "text/plain" || attachments[0].SizeBytes != 10 ||
		attachments[0].DeliveryMode != "path" {
		t.Fatalf("attachments = %#v", attachments)
	}
}
