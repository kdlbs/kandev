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
	ws "github.com/kandev/kandev/pkg/websocket"
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

// TestDispatchInitialPromptAcceptanceKeepsTurnInFlight proves agentctl's
// acknowledgement is not an idle transition. The mock accepts the first prompt
// but deliberately never completes it.
func TestDispatchInitialPromptAcceptanceKeepsTurnInFlight(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	if !ok {
		t.Fatal("claude-acp test agent is not registered")
	}
	mock := newMockAgentServer(t)
	defer mock.Close()
	promptAccepted := make(chan struct{})
	mock.handler = func(message ws.Message) *ws.Message {
		if message.Action == "agent.prompt" {
			close(promptAccepted)
		}
		return mock.defaultHandler(message)
	}
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
	dispatched := make(chan struct{})
	releaseDispatch := make(chan struct{})
	execution.setInitialPromptDispatchCallbacks(func() {
		close(dispatched)
		<-releaseDispatch
	}, nil)
	ready := make(chan string, 1)
	sm.dispatchInitialPrompt(context.Background(), execution, agentConfig, "deliver this prompt", nil, func(id string) error {
		ready <- id
		return nil
	})
	select {
	case <-promptAccepted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("initial prompt was not accepted by agentctl")
	}
	select {
	case <-dispatched:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("initial prompt dispatch callback was not reached")
	}
	close(releaseDispatch)
	select {
	case id := <-ready:
		t.Fatalf("accepted in-flight prompt marked execution ready: %q", id)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDispatchInitialPromptDeliveryFailureDoesNotMarkReady(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	if !ok {
		t.Fatal("claude-acp test agent is not registered")
	}
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	failures := make(chan InitialPromptFailure, 1)
	sm.SetInitialPromptFailureHandler(func(failure InitialPromptFailure) { failures <- failure })
	ready := make(chan string, 1)
	sm.dispatchInitialPrompt(context.Background(), &AgentExecution{
		ID: "execution-delivery-failure", SessionID: "session-delivery-failure",
	}, agentConfig, "deliver this prompt", nil, func(id string) error {
		ready <- id
		return nil
	})
	select {
	case <-failures:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("initial prompt delivery failure was not reported")
	}
	select {
	case id := <-ready:
		t.Fatalf("delivery failure marked execution ready: %q", id)
	default:
	}
}

func TestDispatchInitialPromptWithoutWorkMarksReadyOnce(t *testing.T) {
	agentConfig, ok := newTestRegistry().Get("claude-acp")
	if !ok {
		t.Fatal("claude-acp test agent is not registered")
	}
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	ready := make(chan string, 2)
	execution := &AgentExecution{ID: "execution-no-prompt", SessionID: "session-no-prompt"}
	sm.dispatchInitialPrompt(context.Background(), execution, agentConfig, "", nil, func(id string) error {
		ready <- id
		return nil
	})
	select {
	case id := <-ready:
		if id != execution.ID {
			t.Fatalf("ready execution ID = %q, want %q", id, execution.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no-prompt startup did not mark ready")
	}
	select {
	case id := <-ready:
		t.Fatalf("no-prompt startup marked ready twice: %q", id)
	default:
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
