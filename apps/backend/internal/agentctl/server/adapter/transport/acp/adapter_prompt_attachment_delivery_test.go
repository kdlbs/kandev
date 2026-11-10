package acp

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type attachmentCaptureAgent struct {
	burstAgent
	requests chan sdk.PromptRequest
}

func (f *attachmentCaptureAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "attachment-session"}, nil
}

func (f *attachmentCaptureAgent) Prompt(_ context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	f.requests <- req
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func attachmentDeliveryAdapter(t *testing.T) (*Adapter, *attachmentCaptureAgent, context.Context) {
	t.Helper()
	a := newTestAdapterForAgent(codexAgentID)
	a.cfg.WorkDir = t.TempDir()
	a.attachMgr = shared.NewAttachmentManager(a.cfg.WorkDir, a.logger.Zap())
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	require.NoError(t, a.Connect(c2aW, a2cR))
	fake := &attachmentCaptureAgent{requests: make(chan sdk.PromptRequest, 4)}
	_ = sdk.NewAgentSideConnection(fake, a2cW, c2aR)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, a.Initialize(ctx))
	_, err := a.NewSession(ctx, nil)
	require.NoError(t, err)
	return a, fake, ctx
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-001.1, AC-TASKS-PROMPT-ATTACHMENTS-001.6, AC-TASKS-PROMPT-ATTACHMENTS-001.7
func TestAttachmentDelivery_RejectsMissingDescriptorBeforeDispatch(t *testing.T) {
	for _, kind := range []string{"image", "audio", "resource"} {
		for _, mode := range []string{"prompt", "path"} {
			for _, steer := range []bool{false, true} {
				t.Run(kind+"/"+mode+"/steer="+strconv.FormatBool(steer), func(t *testing.T) {
					a, fake, ctx := attachmentDeliveryAdapter(t)
					a.capabilities.PromptCapabilities.EmbeddedContext = true
					att := v1.MessageAttachment{Type: kind, AttachmentID: "missing", Name: "missing.bin", MimeType: "application/octet-stream", DeliveryMode: mode, SizeBytes: 3}
					valid := v1.MessageAttachment{Type: "image", Data: base64.StdEncoding.EncodeToString([]byte("valid sibling")), MimeType: "image/png"}
					send := a.Prompt
					if steer {
						send = a.PromptSteer
					}
					require.Error(t, send(ctx, "with missing file", []v1.MessageAttachment{valid, att}, 1))
					select {
					case <-fake.requests:
						t.Fatal("provider received a submission with a missing file")
					default:
					}
					require.NoError(t, send(ctx, "valid retry", []v1.MessageAttachment{valid}, 2))
					req := <-fake.requests
					require.Equal(t, valid.Data, req.Prompt[1].Image.Data)
				})
			}
		}
	}
}

func TestAttachmentDelivery_MaterializedImageBytes(t *testing.T) {
	a, fake, ctx := attachmentDeliveryAdapter(t)
	dir := filepath.Join(a.cfg.WorkDir, ".kandev", "attachments", a.sessionID)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	data := []byte("materialized bytes")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"), data, 0o600))
	att := v1.MessageAttachment{Type: "image", AttachmentID: "image-id", Name: "image.png", MimeType: "image/png", SizeBytes: int64(len(data))}
	require.NoError(t, a.Prompt(ctx, "", []v1.MessageAttachment{att}, 1))
	req := <-fake.requests
	require.Equal(t, base64.StdEncoding.EncodeToString(data), req.Prompt[1].Image.Data)
	att.Type, att.DeliveryMode = "resource", "path"
	require.NoError(t, a.Prompt(ctx, "", []v1.MessageAttachment{att}, 2))
	pathRequest := <-fake.requests
	require.NotNil(t, pathRequest.Prompt[1].Text)
	require.Contains(t, pathRequest.Prompt[1].Text.Text, filepath.Join(".kandev", "attachments", a.sessionID, att.Name))
	stored, err := os.ReadFile(filepath.Join(a.cfg.WorkDir, ".kandev", "attachments", a.sessionID, att.Name))
	require.NoError(t, err)
	require.Equal(t, data, stored)
}

func TestAttachmentDelivery_UnavailableContentPreservesPendingContext(t *testing.T) {
	for _, unavailableManager := range []bool{false, true} {
		t.Run(strconv.FormatBool(unavailableManager), func(t *testing.T) {
			a, fake, ctx := attachmentDeliveryAdapter(t)
			dir := filepath.Join(a.cfg.WorkDir, ".kandev", "attachments", a.sessionID)
			require.NoError(t, os.MkdirAll(dir, 0o700))
			path := filepath.Join(dir, "unreadable.png")
			require.NoError(t, os.WriteFile(path, []byte("image"), 0o600))
			if unavailableManager {
				manager := a.attachMgr
				defer func() { a.attachMgr = manager }()
				a.attachMgr = nil
			} else {
				require.NoError(t, os.Chmod(path, 0))
				t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0o600)) })
				if file, err := os.Open(path); err == nil {
					require.NoError(t, file.Close())
					t.Skip("filesystem does not enforce unreadable permissions")
				}
			}
			a.SetPendingContext("retained context")
			err := a.Prompt(ctx, "first", []v1.MessageAttachment{{Type: "image", AttachmentID: "unreadable", Name: "unreadable.png"}}, 1)
			require.Error(t, err)
			require.NotContains(t, err.Error(), a.cfg.WorkDir)
			require.Empty(t, fake.requests)
			require.NoError(t, a.Prompt(ctx, "retry", nil, 2))
			require.Equal(t, "retained context", (<-fake.requests).Prompt[0].Text.Text)
		})
	}
}

func TestAttachmentDelivery_FailedSteerPreservesGeneratingTurn(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	a.attachMgr = shared.NewAttachmentManager(t.TempDir(), a.logger.Zap())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, a.Initialize(ctx))
	_, err := a.NewSession(ctx, nil)
	require.NoError(t, err)
	firstDone := make(chan error, 1)
	go func() { firstDone <- a.Prompt(ctx, "generating", nil, 71) }()
	<-fake.entered
	missing := v1.MessageAttachment{Type: "image", AttachmentID: "missing", Name: "missing.png"}
	require.Error(t, a.PromptSteer(ctx, "invalid steer", []v1.MessageAttachment{missing}, 72))
	fake.releasePrompts()
	require.NoError(t, <-firstDone)
	require.Equal(t, uint64(71), waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration)
	require.Equal(t, int32(1), fake.promptCount.Load())
}
