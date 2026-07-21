package share

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// stubReader implements TaskReader for unit tests.
type stubReader struct {
	task     *models.Task
	session  *models.TaskSession
	messages []*models.Message
}

func (s *stubReader) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return s.task, nil
}
func (s *stubReader) GetTaskSession(_ context.Context, _ string) (*models.TaskSession, error) {
	return s.session, nil
}
func (s *stubReader) ListMessages(_ context.Context, _ string) ([]*models.Message, error) {
	return s.messages, nil
}

func TestBuildSnapshot_AllowsRunningAndOtherPostStartStates(t *testing.T) {
	t.Parallel()
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateRunning,
		models.TaskSessionStateIdle,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			r := &stubReader{
				task:    &models.Task{ID: "t-1", Title: "Hi"},
				session: &models.TaskSession{ID: "s-1", TaskID: "t-1", State: state},
				messages: []*models.Message{
					{ID: "m-1", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "hi"},
				},
			}
			snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
			if err != nil {
				t.Fatalf("state %q: unexpected error: %v", state, err)
			}
			if snap == nil || len(snap.Messages) == 0 {
				t.Fatalf("state %q: expected non-empty snapshot", state)
			}
		})
	}
}

func TestBuildSnapshot_RejectsPreHistoryStates(t *testing.T) {
	t.Parallel()
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
	} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			r := &stubReader{
				task:    &models.Task{ID: "t-1", Title: "Hi"},
				session: &models.TaskSession{ID: "s-1", TaskID: "t-1", State: state},
			}
			_, err := BuildSnapshot(context.Background(), r, "s-1", "v")
			if !errors.Is(err, ErrSessionNotShareable) {
				t.Fatalf("state %q: expected ErrSessionNotShareable, got %v", state, err)
			}
		})
	}
}

func TestBuildSnapshot_BuildsBasicConversation(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "Investigate flaky test", WorkflowStepID: "step-debug"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt:   completed.Add(-time.Minute),
			CompletedAt: &completed,
			AgentProfileSnapshot: map[string]interface{}{
				"agent_type": "claude-acp",
				"model":      "claude-opus-4-7",
			},
			ExecutorSnapshot: map[string]interface{}{"type": "local_docker"},
			WorkspacePath:    "/Users/foo/proj",
		},
		messages: []*models.Message{
			{
				ID: "m-1", TaskSessionID: "s-1", AuthorType: models.MessageAuthorUser,
				Type: models.MessageTypeMessage, Content: "Hello",
				CreatedAt: completed.Add(-30 * time.Second),
			},
			{
				ID: "m-2", TaskSessionID: "s-1", AuthorType: models.MessageAuthorAgent,
				Type: models.MessageTypeContent, Content: "Hi there",
				CreatedAt: completed.Add(-25 * time.Second),
			},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v-test")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if snap.Version != SnapshotVersion {
		t.Fatalf("expected version %d, got %d", SnapshotVersion, snap.Version)
	}
	if snap.Task.Title != "Investigate flaky test" {
		t.Fatalf("task.title = %q", snap.Task.Title)
	}
	if snap.Session.AgentType != "claude-acp" || snap.Session.Model != "claude-opus-4-7" || snap.Session.ExecutorType != "local_docker" {
		t.Fatalf("session metadata not populated: %+v", snap.Session)
	}
	if len(snap.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(snap.Messages))
	}
	if snap.Messages[0].Role != roleUser || snap.Messages[1].Role != roleAssistant {
		t.Fatalf("roles wrong: %v", snap.Messages)
	}
	if snap.Messages[0].Blocks[0].Kind != blockKindText {
		t.Fatalf("expected text block, got %q", snap.Messages[0].Blocks[0].Kind)
	}
}

func TestBuildSnapshot_RedactsToolCallEnvAndPaths(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "Shell run"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
			WorkspacePath: "/Users/foo/proj",
		},
		messages: []*models.Message{
			{
				ID: "m-1", TaskSessionID: "s-1", AuthorType: models.MessageAuthorAgent,
				Type:    models.MessageTypeToolExecute,
				Content: "ran ls in /Users/foo/proj/src",
				Metadata: map[string]interface{}{
					"tool_name": "shell",
					"args": map[string]interface{}{
						"cmd": "ls",
						"env": map[string]interface{}{"SECRET": "abc"},
					},
				},
				CreatedAt: completed,
			},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(snap.Messages))
	}
	b := snap.Messages[0].Blocks[0]
	if b.Kind != blockKindToolCall {
		t.Fatalf("expected tool_call block, got %q", b.Kind)
	}
	if strings.Contains(b.Text, "/Users/foo/proj") {
		t.Fatalf("expected workspace root rewritten, got %q", b.Text)
	}
	if strings.Contains(string(b.Args), "SECRET") {
		t.Fatalf("env should be redacted from args, got %s", string(b.Args))
	}
	if !strings.Contains(string(b.Args), `"cmd":"ls"`) {
		t.Fatalf("cmd preserved expected, got %s", string(b.Args))
	}
	got := snap.Redaction.AppliedRules
	wantHas := map[string]bool{RuleEnvVars: false, RuleAbsPath: false}
	for _, r := range got {
		if _, ok := wantHas[r]; ok {
			wantHas[r] = true
		}
	}
	for k, present := range wantHas {
		if !present {
			t.Fatalf("expected applied rule %q, got %v", k, got)
		}
	}
}

// TestBuildSnapshot_ToolCall_ProductionMetadataShape exercises the metadata
// shape that real sessions produce: no "tool_name" key, args under
// metadata["normalized"] as the NormalizedPayload JSON, message Type set to
// "tool_<kind>". The earlier shape ("tool_name"/"args" keys) only exists in
// the debug fixture loader.
func TestBuildSnapshot_ToolCall_ProductionMetadataShape(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "Real session"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
			WorkspacePath: "/workspace/proj",
		},
		messages: []*models.Message{
			{
				ID: "m-1", TaskSessionID: "s-1", AuthorType: models.MessageAuthorAgent,
				Type:    models.MessageTypeToolRead,
				Content: "read /workspace/proj/src/main.go",
				Metadata: map[string]interface{}{
					"normalized": map[string]interface{}{
						"kind": "read_file",
						"read_file": map[string]interface{}{
							"path": "/workspace/proj/src/main.go",
						},
					},
				},
				CreatedAt: completed,
			},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b := snap.Messages[0].Blocks[0]
	if b.Kind != blockKindToolCall {
		t.Fatalf("expected tool_call block, got %q", b.Kind)
	}
	// "tool_" prefix stripped from m.Type to produce a useful label.
	if b.ToolName != "read" {
		t.Fatalf("expected ToolName=read, got %q", b.ToolName)
	}
	// Args populated from metadata["normalized"] and redacted.
	if len(b.Args) == 0 {
		t.Fatalf("expected Args populated from normalized payload, got empty")
	}
	if strings.Contains(string(b.Args), "/workspace/proj") {
		t.Fatalf("expected abs-path redacted in args, got %s", string(b.Args))
	}
	if strings.Contains(b.Text, "/workspace/proj") {
		t.Fatalf("expected abs-path redacted in text, got %q", b.Text)
	}
}

func TestBuildSnapshot_FiltersNoiseTypes(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "T"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
		},
		messages: []*models.Message{
			// Agent noise types — must all be dropped.
			{ID: "n-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeLog, Content: "mock-agent: registered ACP MCP server kandev at http://localhost:41027/sse", CreatedAt: completed},
			{ID: "n-2", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeStatus, Content: "started", CreatedAt: completed},
			{ID: "n-3", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError, Content: "boom", CreatedAt: completed},
			{ID: "n-4", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeThinking, Content: "hmm", CreatedAt: completed},
			{ID: "n-5", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeAgentPlan, Content: "plan", CreatedAt: completed},
			{ID: "n-6", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeTodo, Content: "todo list", CreatedAt: completed},
			{ID: "n-7", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeScriptExecution, Content: "script", CreatedAt: completed},
			{ID: "n-8", AuthorType: models.MessageAuthorAgent, Type: "completely_unknown_type", Content: "future leak", CreatedAt: completed},
			// Legit content — must survive.
			{ID: "ok-1", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "real question", CreatedAt: completed},
			{ID: "ok-2", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "real answer", CreatedAt: completed},
			{ID: "ok-3", AuthorType: models.MessageAuthorAgent, Type: "tool_search", Content: "Search for \"import \"", Metadata: map[string]interface{}{"tool_name": "grep"}, CreatedAt: completed},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.Messages) != 3 {
		t.Fatalf("expected only 3 surviving messages (user prose, agent prose, tool_search pill), got %d:\n%+v", len(snap.Messages), snap.Messages)
	}
	// Make sure none of the noise content leaked in.
	for _, m := range snap.Messages {
		for _, b := range m.Blocks {
			if strings.Contains(b.Text, "mock-agent:") {
				t.Fatal("mock-agent log line leaked into snapshot")
			}
			if strings.Contains(b.Text, "hmm") || strings.Contains(b.Text, "todo list") {
				t.Fatalf("internal type leaked: %q", b.Text)
			}
		}
	}
	// tool_search must produce a tool_call block, not text.
	last := snap.Messages[len(snap.Messages)-1]
	if last.Blocks[0].Kind != blockKindToolCall {
		t.Fatalf("tool_search should render as tool_call block, got kind=%q", last.Blocks[0].Kind)
	}
}

func TestBuildSnapshot_StripsKandevSystemBlocks(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "T"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
		},
		messages: []*models.Message{
			// First-turn prompt with system context wrapper — only the
			// visible bit "real question" should survive.
			{
				ID: "u-1", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
				Content:   "<kandev-system>\nMCP context goes here\nMore secret stuff\n</kandev-system>\nreal question",
				CreatedAt: completed,
			},
			// Pure system payload — should be dropped entirely after stripping.
			{
				ID: "u-2", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
				Content:   "<kandev-system>only system stuff</kandev-system>",
				CreatedAt: completed,
			},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.Messages) != 1 {
		t.Fatalf("expected 1 surviving message after system strip, got %d", len(snap.Messages))
	}
	body := snap.Messages[0].Blocks[0].Text
	if strings.Contains(body, "kandev-system") || strings.Contains(body, "MCP context") || strings.Contains(body, "secret stuff") {
		t.Fatalf("system content leaked into snapshot: %q", body)
	}
	if body != "real question" {
		t.Fatalf("expected only the visible bit to remain, got %q", body)
	}
}

func TestBuildSnapshot_UserMessageWithUnusualTypeStillIncluded(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "T"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
		},
		messages: []*models.Message{
			// A user message tagged with a weird type still represents the
			// user's intent — keep it.
			{ID: "u-1", AuthorType: models.MessageAuthorUser, Type: "some_weird_type", Content: "important question", CreatedAt: completed},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.Messages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(snap.Messages))
	}
	if snap.Messages[0].Role != roleUser {
		t.Fatalf("expected user role, got %q", snap.Messages[0].Role)
	}
}

func TestBuildSnapshot_SkipsStatusAndEmptyMessages(t *testing.T) {
	t.Parallel()
	completed := time.Now().UTC()
	r := &stubReader{
		task: &models.Task{ID: "t-1", Title: "T"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed, CompletedAt: &completed,
		},
		messages: []*models.Message{
			{ID: "m-1", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeStatus, Content: "started", CreatedAt: completed},
			{ID: "m-2", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "  ", CreatedAt: completed},
			{ID: "m-3", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "hi", CreatedAt: completed},
		},
	}
	snap, err := BuildSnapshot(context.Background(), r, "s-1", "v")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Role != roleUser {
		t.Fatalf("expected 1 user message after pruning, got %+v", snap.Messages)
	}
}
