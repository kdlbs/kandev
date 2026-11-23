package lifecycle

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Covers AC-TASKS-COMMENT-FILES-001.3.
func TestCommentFilesRejectUnresolvedMarkerBeforeDispatch(t *testing.T) {
	sm := &SessionManager{}
	err := sm.callAgentctlPrompt(context.Background(), &AgentExecution{},
		"Feedback\n\n<!-- kandev-comment-files:missing -->\n", []v1.MessageAttachment{{AttachmentID: "id", CommentID: "missing", DeliveryMode: "path"}}, 1, false, "")
	if err == nil || !strings.Contains(err.Error(), "unresolved comment files") {
		t.Fatalf("expected a comment file resolution error, got %v", err)
	}
}

func TestRenderCommentFilesByID(t *testing.T) {
	prompt := "Example\n<!-- kandev-comment-files:example -->\n\nFeedback A\n\n<!-- kandev-comment-files:first -->\n\nFeedback B\n\n<!-- kandev-comment-files:second -->"
	result, remaining, err := v1.RenderCommentFiles(prompt, []v1.MessageAttachment{
		{AttachmentID: "two", CommentID: "second", Name: "notes.txt", Path: "/exec/two/notes.txt", DeliveryMode: "path"},
		{AttachmentID: "one", CommentID: "first", Name: "notes.txt", Path: "/exec/one/notes.txt", DeliveryMode: "path"},
		{AttachmentID: "ordinary", DeliveryMode: "prompt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Example\n<!-- kandev-comment-files:example -->\n\nFeedback A\n\n- \"notes.txt\": \"/exec/one/notes.txt\"\n\nFeedback B\n\n- \"notes.txt\": \"/exec/two/notes.txt\""
	if result != want || len(remaining) != 1 || remaining[0].AttachmentID != "ordinary" {
		t.Fatalf("rendered %q with remaining %+v", result, remaining)
	}
}

func TestCommentFilesRejectDuplicateMappedMarker(t *testing.T) {
	const marker = "<!-- kandev-comment-files:comment -->"
	_, _, err := v1.RenderCommentFiles("Selected example\n"+marker+"\n\nFeedback\n"+marker,
		[]v1.MessageAttachment{{AttachmentID: "file", CommentID: "comment", Name: "notes.txt", Path: "/exec/notes.txt", DeliveryMode: "path"}})
	if err == nil {
		t.Fatal("a duplicate marker could shift the file to untrusted comment content")
	}
}

func TestPassthroughStartupRejectsUnavailableCommentFilesBeforeLaunching(t *testing.T) {
	manager, _, execution := newPassthroughRunnerManager(t)
	execution.setMetadataValue("task_description", "Review\n\n<!-- kandev-comment-files:review -->")
	execution.setMetadataValue("attachments", []MessageAttachment{{
		AttachmentID: "unavailable", CommentID: "review", DeliveryMode: "path",
	}})
	err := manager.startPassthroughSession(context.Background(), execution, nil)
	if err == nil || !strings.Contains(err.Error(), "attachment storage is unavailable") {
		t.Fatalf("expected comment materialization failure before launch, got %v", err)
	}
	if execution.PassthroughProcessID != "" {
		t.Fatalf("launched a passthrough agent without its comment files: %s", execution.PassthroughProcessID)
	}
}

func TestPassthroughStartupRendersLocalCommentFile(t *testing.T) {
	manager, _, execution := newPassthroughRunnerManager(t)
	const prompt = "Review\n\n<!-- kandev-comment-files:review -->"
	execution.setMetadataValue("task_description", prompt)
	execution.setMetadataValue("attachments", []MessageAttachment{{
		AttachmentID: "uploaded", CommentID: "review", DeliveryMode: "path",
	}})
	manager.SetAttachmentReader(testAttachmentReader{})
	_, _, _, command, _, err := manager.passthroughAgentCommandWithPreparation(context.Background(), execution, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(execution.WorkspacePath, ".kandev", "attachments", execution.SessionID, "bundle.zip")
	wantPrompt := "Review\n\n- \"bundle.zip\": " + strconv.Quote(path)
	if command.initialPrompt != wantPrompt {
		t.Fatalf("initial prompt = %q, want %q", command.initialPrompt, wantPrompt)
	}
	if retained := getTaskDescriptionFromMetadata(execution); retained != prompt {
		t.Fatalf("runtime path escaped into retained task description: %q", retained)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "attachment bytes" {
		t.Fatalf("materialized body = %q, err = %v", body, err)
	}
}

func TestSSHPassthroughCommentFilesUseRemoteMaterialization(t *testing.T) {
	manager, _, execution := newPassthroughRunnerManager(t)
	execution.RuntimeName = agentruntime.RuntimeSSH
	manager.SetAttachmentReader(testAttachmentReader{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/attachments/materialize" {
			t.Errorf("unexpected SSH agentctl path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("parse materialization: %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil || string(body) != "attachment bytes" {
			t.Errorf("uploaded body = %q, err = %v", body, err)
		}
		if got := r.FormValue("session_id"); got != execution.SessionID {
			t.Errorf("remote terminal scope = %q, want %q", got, execution.SessionID)
		}
		_, _ = w.Write([]byte(`{"name":"bundle.zip","path":"/remote/project/bundle.zip","size_bytes":16}`))
	}))
	defer server.Close()
	execution.agentctl = createTestClient(t, server.URL)

	const prompt = "Review\n\n<!-- kandev-comment-files:review -->"
	files := []v1.MessageAttachment{{AttachmentID: "uploaded", CommentID: "review", DeliveryMode: "path"}}
	got, handled, err := manager.PrepareRemotePassthroughCommentPrompt(context.Background(), execution.SessionID, prompt, files)
	want := "Review\n\n- \"bundle.zip\": \"/remote/project/bundle.zip\""
	if !handled || err != nil || got != want {
		t.Fatalf("SSH prompt = %q, handled = %t, error = %v", got, handled, err)
	}
}
