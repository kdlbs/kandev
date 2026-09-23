package lifecycle

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestDispatchInitialPromptReportsDeliveryFailure(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	if !ok {
		t.Fatal("claude-acp test agent is not registered")
	}
	sm := NewSessionManager(logger.Default(), nil)
	wantErr := "has no agentctl client"
	failures := make(chan InitialPromptFailure, 1)
	sm.SetInitialPromptFailureHandler(func(failure InitialPromptFailure) {
		if failure.ExecutionID != "execution-initial-prompt" {
			t.Errorf("execution ID = %q", failure.ExecutionID)
		}
		failures <- failure
	})
	execution := &AgentExecution{
		ID:        "execution-initial-prompt",
		TaskID:    "task-initial-prompt",
		SessionID: "session-initial-prompt",
	}

	sm.dispatchInitialPrompt(
		context.Background(),
		execution,
		agentConfig,
		"deliver this prompt",
		nil,
		func(string) error { return errors.New("mark ready must not run") },
	)

	select {
	case failure := <-failures:
		if failure.Err == nil || !strings.Contains(failure.Err.Error(), wantErr) {
			t.Fatalf("initial prompt error = %v, want %q", failure.Err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial prompt failure callback")
	}
}

// TestDispatchInitialPromptSuccessSignalsBootReadiness exercises the actual
// prompted startup transport. A successfully accepted first prompt must cross
// the readiness boundary; no-prompt startup is the only path that may mark
// ready synchronously.
func TestDispatchInitialPromptSuccessSignalsBootReadiness(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	if !ok {
		t.Fatal("claude-acp test agent is not registered")
	}
	mock := newMockAgentServer(t)
	defer mock.Close()
	client := createTestClient(t, mock.server.URL)
	defer client.Close()
	connectAgentStream(t, mock, client)
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	execution := &AgentExecution{ID: "execution-prompt-success", TaskID: "task-prompt-success", SessionID: "session-prompt-success", agentctl: client}
	connectedClient, releaseClient := execution.AcquireAgentCtlClient()
	if connectedClient != client {
		releaseClient()
		t.Fatal("fixture did not retain the connected agentctl client")
	}
	releaseClient()
	ready := make(chan string, 1)
	sm.dispatchInitialPrompt(context.Background(), execution, agentConfig, "deliver this prompt", nil, func(id string) error {
		ready <- id
		return nil
	})
	select {
	case id := <-ready:
		if id != execution.ID {
			t.Fatalf("ready execution ID = %q, want %q", id, execution.ID)
		}
		for _, action := range mock.getActionLog() {
			if action == "agent.prompt" {
				return
			}
		}
		t.Fatal("readiness crossed without agentctl accepting the initial prompt")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("successful prompted startup never crossed the boot-readiness boundary")
	}
}

type testAttachmentReader struct{}

func (testAttachmentReader) OpenClaimed(context.Context, string, string, string) (io.ReadCloser, string, string, int64, error) {
	return io.NopCloser(strings.NewReader("attachment bytes")), "bundle.zip", "application/zip", 16, nil
}

func TestMaterializeAttachmentsStreamsClaimedDescriptor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/attachments/materialize" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("session_id"); got != "acp-session" {
			t.Errorf("session_id = %q", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(body) != "attachment bytes" {
			t.Errorf("body = %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"bundle.zip","size_bytes":16}`))
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	sm := NewSessionManager(logger.Default(), nil)
	sm.SetAttachmentReader(testAttachmentReader{})
	execution := &AgentExecution{
		TaskID:       "task-1",
		SessionID:    "session-1",
		ACPSessionID: "acp-session",
		agentctl:     agentctl.NewClient(parsed.Hostname(), port, logger.Default()),
	}

	attachments, err := sm.materializeAttachments(context.Background(), execution, []v1.MessageAttachment{
		{AttachmentID: "attachment-1", Type: "resource", Name: "bundle.zip", SizeBytes: 16},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %+v", attachments)
	}
	if attachments[0].DeliveryMode != "prompt" || attachments[0].Data != "" || attachments[0].Name != "bundle.zip" {
		t.Fatalf("materialized attachment = %+v", attachments[0])
	}
}

func TestMaterializeAttachmentsPreservesPromptDeliveryMode(t *testing.T) {
	sm := NewSessionManager(logger.Default(), nil)
	sm.SetAttachmentReader(testAttachmentReader{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"photo.png","size_bytes":16}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{
		TaskID: "task-1", SessionID: "session-1", ACPSessionID: "acp-session",
		agentctl: agentctl.NewClient(parsed.Hostname(), port, logger.Default()),
	}
	attachments, err := sm.materializeAttachments(context.Background(), execution, []v1.MessageAttachment{
		{AttachmentID: "attachment-1", Type: "image", Name: "photo.png", DeliveryMode: "prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].DeliveryMode != "prompt" {
		t.Fatalf("materialized attachment = %+v, want prompt delivery", attachments)
	}
}
