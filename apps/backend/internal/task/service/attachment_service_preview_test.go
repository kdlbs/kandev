package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"

	"github.com/kandev/kandev/internal/task/previewfeedback"
)

func encodePreviewPNG(t *testing.T, bounds image.Rectangle) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(bounds)); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return encoded.Bytes()
}

func previewPNGWithDimensions(t *testing.T, width, height uint32) []byte {
	t.Helper()
	content := encodePreviewPNG(t, image.Rect(0, 0, 1, 1))
	binary.BigEndian.PutUint32(content[16:20], width)
	binary.BigEndian.PutUint32(content[20:24], height)
	binary.BigEndian.PutUint32(content[29:33], crc32.ChecksumIEEE(content[12:29]))
	return content
}

func TestValidatePreviewScreenshotAcceptsBoundedPNG(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	content := encodePreviewPNG(t, image.Rect(0, 0, 320, 180))
	attachment, err := svc.Stage(
		context.Background(), "user-a", "ws-att", "capture.png", "image/png", "image", "prompt",
		bytes.NewReader(content),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if err := svc.ValidatePreviewScreenshot(
		context.Background(), "user-a", "ws-att", "task-1", attachment.ID,
	); err != nil {
		t.Fatalf("ValidatePreviewScreenshot: %v", err)
	}
}

func TestValidatePreviewScreenshotRejectsInvalidPNGBytes(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	attachment, err := svc.Stage(
		context.Background(), "user-a", "ws-att", "capture.png", "image/png", "image", "prompt",
		bytes.NewBufferString("not a PNG"),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	err = svc.ValidatePreviewScreenshot(
		context.Background(), "user-a", "ws-att", "task-1", attachment.ID,
	)
	if !errors.Is(err, previewfeedback.ErrCaptureInvalid) {
		t.Fatalf("ValidatePreviewScreenshot error = %v, want ErrCaptureInvalid", err)
	}
}

func TestValidatePreviewScreenshotRejectsOversizedPixelMetadata(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	content := previewPNGWithDimensions(t, 4001, 4000)
	attachment, err := svc.Stage(
		context.Background(), "user-a", "ws-att", "capture.png", "image/png", "image", "prompt",
		bytes.NewReader(content),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	err = svc.ValidatePreviewScreenshot(
		context.Background(), "user-a", "ws-att", "task-1", attachment.ID,
	)
	if !errors.Is(err, previewfeedback.ErrCaptureTooLarge) {
		t.Fatalf("ValidatePreviewScreenshot error = %v, want ErrCaptureTooLarge", err)
	}
}

func TestValidatePreviewScreenshotAcceptsIdempotentTaskClaim(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	content := encodePreviewPNG(t, image.Rect(0, 0, 20, 20))
	attachment, err := svc.Stage(
		context.Background(), "user-a", "ws-att", "capture.png", "image/png", "image", "prompt",
		bytes.NewReader(content),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := svc.Claim(context.Background(), "user-a", "ws-att", "task-1", "", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	svc.SetTaskAuthorizer(func(context.Context, string) error { return nil })

	if err := svc.ValidatePreviewScreenshot(
		context.Background(), "user-a", "ws-att", "task-1", attachment.ID,
	); err != nil {
		t.Fatalf("ValidatePreviewScreenshot claimed retry: %v", err)
	}
}

func TestValidatePreviewScreenshotRejectsWrongAttachmentMetadata(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	content := encodePreviewPNG(t, image.Rect(0, 0, 20, 20))
	attachment, err := svc.Stage(
		context.Background(), "user-a", "ws-att", "capture.png", "image/png", "image", "path",
		bytes.NewReader(content),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	err = svc.ValidatePreviewScreenshot(
		context.Background(), "user-a", "ws-att", "task-1", attachment.ID,
	)
	if !errors.Is(err, previewfeedback.ErrCaptureInvalid) {
		t.Fatalf("ValidatePreviewScreenshot error = %v, want ErrCaptureInvalid", err)
	}
}
