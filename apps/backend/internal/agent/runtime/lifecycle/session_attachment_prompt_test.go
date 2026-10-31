package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type filesystemAttachmentReader struct {
	bodies map[string][]byte
	errs   map[string]error
}

func (r filesystemAttachmentReader) OpenClaimed(
	_ context.Context, id, _ string, _ string,
) (io.ReadCloser, string, string, int64, error) {
	if err := r.errs[id]; err != nil {
		return nil, "", "", 0, err
	}
	body, ok := r.bodies[id]
	if !ok {
		return nil, "", "", 0, errors.New("attachment bytes unavailable")
	}
	return io.NopCloser(bytes.NewReader(body)), id + ".bin", "application/octet-stream", int64(len(body)), nil
}

func TestSessionPromptMaterializesClaimedAttachments(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	reader := filesystemAttachmentReader{bodies: map[string][]byte{
		"attachment-image":    []byte("image bytes"),
		"attachment-resource": []byte("resource bytes"),
	}}
	sm := NewSessionManager(logger.Default(), nil)
	sm.SetAttachmentReader(reader)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	if err := client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect agent stream: %v", err)
	}
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server did not see the agent stream connection")
	}
	execution := &AgentExecution{
		ID: "execution-attachments", TaskID: "task-attachments", SessionID: "session-attachments",
		ACPSessionID: "test-session-123", Status: v1.AgentStatusReady,
		agentctl:     client,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	attachments := []v1.MessageAttachment{
		{AttachmentID: "attachment-image", Type: "image", Name: "screenshot.png", DeliveryMode: "prompt"},
		{AttachmentID: "attachment-resource", Type: "resource", Name: "logs.zip", DeliveryMode: "path"},
	}

	if _, err := sm.sendPrompt(
		context.Background(), execution, "deliver creation files", false, attachments, true,
		sendPromptCallbacks{}, false,
	); err != nil {
		t.Fatalf("send prompt with attachments: %v", err)
	}

	uploads := mock.materializedUploads()
	if len(uploads) != 2 {
		t.Fatalf("materialized uploads = %+v, want two", uploads)
	}
	if uploads[0].attachmentID != "attachment-image" || uploads[0].body != "image bytes" ||
		uploads[1].attachmentID != "attachment-resource" || uploads[1].body != "resource bytes" {
		t.Fatalf("materialized uploads = %+v", uploads)
	}
	promptSets := mock.promptAttachmentSets()
	if len(promptSets) != 1 || len(promptSets[0]) != 2 {
		t.Fatalf("prompt attachment sets = %+v", promptSets)
	}
	for _, attachment := range promptSets[0] {
		if attachment.Data != "" || attachment.AttachmentID == "" {
			t.Fatalf("prompt attachment = %+v, want descriptor-only", attachment)
		}
	}

	before := len(mock.promptAttachmentSets())
	missing := filesystemAttachmentReader{errs: map[string]error{
		"missing-attachment": errors.New("attachment bytes missing"),
	}}
	sm.SetAttachmentReader(missing)
	if _, err := sm.sendPrompt(
		context.Background(), execution, "must not dispatch", false,
		[]v1.MessageAttachment{{AttachmentID: "missing-attachment", Type: "resource", Name: "missing.bin"}},
		true, sendPromptCallbacks{}, false,
	); err == nil {
		t.Fatal("missing attachment bytes did not fail the prompt")
	}
	if got := len(mock.promptAttachmentSets()); got != before {
		t.Fatalf("prompt count after missing bytes = %d, want %d", got, before)
	}
}
