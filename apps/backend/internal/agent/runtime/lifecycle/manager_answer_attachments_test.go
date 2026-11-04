package lifecycle

import (
	"context"
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

func TestMaterializeAnswerAttachmentsRejectsExecutionReplacement(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
			return
		}
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"bundle.zip","path":"/workspace/.kandev/attachments/acp-session/bundle.zip","size_bytes":16}`))
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

	store := NewExecutionStore()
	oldExecution := &AgentExecution{
		ID: "execution-old", TaskID: "task-1", SessionID: "session-1",
		ACPSessionID: "acp-session",
		agentctl:     agentctl.NewClient(parsed.Hostname(), port, logger.Default()),
	}
	if err := store.Add(oldExecution); err != nil {
		t.Fatalf("add old execution: %v", err)
	}
	sm := NewSessionManager(logger.Default(), nil)
	sm.SetAttachmentReader(testAttachmentReader{})
	manager := &Manager{executionStore: store, sessionManager: sm}

	result := make(chan error, 1)
	go func() {
		_, materializeErr := manager.MaterializeAnswerAttachments(
			context.Background(),
			"session-1",
			[]v1.MessageAttachment{{AttachmentID: "attachment-1", Type: "resource", Name: "bundle.zip", SizeBytes: 16}},
		)
		result <- materializeErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("materialization did not start")
	}
	store.Remove(oldExecution.ID)
	replacement := &AgentExecution{
		ID: "execution-new", TaskID: "task-1", SessionID: "session-1",
		ACPSessionID: "acp-session-2",
	}
	if err := store.Add(replacement); err != nil {
		t.Fatalf("add replacement: %v", err)
	}
	close(release)

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "changed during transfer") {
			t.Fatalf("materialize error = %v, want execution replacement rejection", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("materialization did not finish")
	}
}
