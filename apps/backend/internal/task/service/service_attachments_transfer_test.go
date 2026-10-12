package service

import (
	"context"
	"testing"
)

func TestTransferSessionMessageAttachmentsRejectsMissingAttachmentService(t *testing.T) {
	service := &Service{}
	err := service.TransferSessionMessageAttachments(
		context.Background(), "task", "session-old", "session-new",
		[]string{"attachment"},
	)
	if err == nil {
		t.Fatal("transfer unexpectedly succeeded without attachment service")
	}
}
