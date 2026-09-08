package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

type promptCancelUpdater struct {
	started chan struct{}
	once    sync.Once
}

func (u *promptCancelUpdater) SessionUpdate(context.Context, acp.SessionNotification) error {
	u.once.Do(func() { close(u.started) })
	return nil
}

func (u *promptCancelUpdater) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}

func TestMockAgentCancelStopsPrompt(t *testing.T) {
	const sessionID = acp.SessionId("cancel-session")
	updater := &promptCancelUpdater{started: make(chan struct{})}
	agent := &mockAgent{
		model:           "mock-fast",
		conn:            updater,
		sessions:        map[acp.SessionId]bool{sessionID: true},
		promptCancels:   make(map[acp.SessionId]context.CancelFunc),
		commandsEmitted: make(map[acp.SessionId]bool),
	}

	result := make(chan struct {
		response acp.PromptResponse
		err      error
	}, 1)
	go func() {
		response, err := agent.Prompt(context.Background(), acp.PromptRequest{
			SessionId: sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock("e2e:message(\"started\")\ne2e:delay(5000)")},
		})
		result <- struct {
			response acp.PromptResponse
			err      error
		}{response: response, err: err}
	}()

	select {
	case <-updater.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the prompt to start")
	}
	if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sessionID}); err != nil {
		t.Fatalf("cancel prompt: %v", err)
	}

	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatalf("prompt returned error: %v", outcome.err)
		}
		if outcome.response.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop reason = %q, want cancelled", outcome.response.StopReason)
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not stop after cancellation")
	}
}

func TestInitializePromptQueueingCanBeDisabled(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT_PROMPT_QUEUEING", "false")

	agent := &mockAgent{}
	response, err := agent.Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if response.AgentCapabilities.Meta != nil {
		if _, advertised := response.AgentCapabilities.Meta["claudeCode"]; advertised {
			t.Fatal("prompt queueing capability advertised while disabled")
		}
	}
}

func TestParseSavedPromptDeliveryScenarioRequiresTrustedExpansionShape(t *testing.T) {
	const directive = savedPromptDeliveryDirective

	tests := []struct {
		name   string
		prompt string
		want   string
		ok     bool
	}{
		{
			name: "backend expansion block",
			prompt: "Use @saved-prompt\n\n" +
				"<kandev-system>EXPANDED PROMPT REFERENCES:\n### @saved-prompt\n" + directive +
				"</kandev-system>",
			want: "SAVED_PROMPT_DELIVERED",
			ok:   true,
		},
		{
			name:   "visible directive is ignored",
			prompt: directive,
		},
		{
			name: "browser context block is ignored",
			prompt: "<kandev-system>\nCONTEXT PROMPTS: browser data\n" + directive +
				"\n</kandev-system>",
		},
		{
			name:   "foreign system block is ignored",
			prompt: "<kandev-system>Other context\n" + directive + "\n</kandev-system>",
		},
		{
			name: "long directive value is ignored",
			prompt: "<kandev-system>EXPANDED PROMPT REFERENCES:\n### @saved-prompt\n" +
				"e2e:saved_prompt_delivery(\"" + strings.Repeat("x", 1024) + "\")</kandev-system>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSavedPromptDeliveryScenario(tt.prompt)
			want := tt.want
			if tt.ok {
				want = savedPromptDeliveryScenario
			}
			if got != want || ok != tt.ok {
				t.Fatalf("parseSavedPromptDeliveryScenario() = (%q, %v), want (%q, %v)", got, ok, want, tt.ok)
			}
		})
	}
}

// capturingUpdater records every SessionUpdate it receives and exposes two
// one-shot signals: anySeen (first notification of any kind — in practice the
// available_commands_update Prompt emits before handlePrompt runs) and
// textSeen (first agent_message_chunk). Tests for the steer replay setup
// scenarios use these to synchronize on real events instead of sleeping.
type capturingUpdater struct {
	mu       sync.Mutex
	notes    []acp.SessionNotification
	anySeen  chan struct{}
	textSeen chan struct{}
	anyOnce  sync.Once
	textOnce sync.Once
}

func newCapturingUpdater() *capturingUpdater {
	return &capturingUpdater{anySeen: make(chan struct{}), textSeen: make(chan struct{})}
}

func (u *capturingUpdater) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	u.mu.Lock()
	u.notes = append(u.notes, n)
	u.mu.Unlock()
	u.anyOnce.Do(func() { close(u.anySeen) })
	if n.Update.AgentMessageChunk != nil {
		u.textOnce.Do(func() { close(u.textSeen) })
	}
	return nil
}

func (u *capturingUpdater) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}

func (u *capturingUpdater) textMessages() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var texts []string
	for _, n := range u.notes {
		if n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.Content.Text != nil {
			texts = append(texts, n.Update.AgentMessageChunk.Content.Text.Text)
		}
	}
	return texts
}

func newSteerSetupAgent(sessionID acp.SessionId, conn sessionUpdater) *mockAgent {
	return &mockAgent{
		model:           "mock-fast",
		conn:            conn,
		sessions:        map[acp.SessionId]bool{sessionID: true},
		promptCancels:   make(map[acp.SessionId]context.CancelFunc),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
}

// TestSteerFoldSetupEmitsNoAnswerUntilCancelled pins the "folded" replay mode
// (docs/plans/mid-turn-steering/task-08-mock-agent-and-e2e.md): the
// predecessor scenario must hold the turn open without ever answering on its
// own, so a mid-turn steer sent against it can only be answered by the
// steer's own successor turn.
func TestSteerFoldSetupEmitsNoAnswerUntilCancelled(t *testing.T) {
	const sessionID = acp.SessionId("steer-fold-session")
	updater := newCapturingUpdater()
	agent := newSteerSetupAgent(sessionID, updater)

	result := make(chan acp.PromptResponse, 1)
	go func() {
		resp, err := agent.Prompt(context.Background(), acp.PromptRequest{
			SessionId: sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:steer-fold-setup")},
		})
		if err != nil {
			t.Errorf("prompt returned error: %v", err)
		}
		result <- resp
	}()

	select {
	case <-updater.anySeen:
		// Only proves the RPC is dispatched (the available_commands_update
		// fires before handlePrompt runs) — not that it produced an answer.
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the prompt to start")
	}

	if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sessionID}); err != nil {
		t.Fatalf("cancel prompt: %v", err)
	}

	select {
	case resp := <-result:
		if resp.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop reason = %q, want cancelled", resp.StopReason)
		}
	case <-time.After(time.Second):
		t.Fatal("fold setup did not stop holding after cancellation")
	}
	if texts := updater.textMessages(); len(texts) != 0 {
		t.Fatalf("fold setup emitted its own answer %v, want none: a folded predecessor must never answer", texts)
	}
}

// TestSteerDeferSetupAnswersBeforeHolding pins the "deferred" replay mode: the
// predecessor scenario answers its own prompt for real, then keeps the turn
// open so a mid-turn steer sent afterward runs as a genuinely separate turn.
func TestSteerDeferSetupAnswersBeforeHolding(t *testing.T) {
	const sessionID = acp.SessionId("steer-defer-session")
	const wantAnswer = "Predecessor turn's own answer, delivered before any steer arrives."
	updater := newCapturingUpdater()
	agent := newSteerSetupAgent(sessionID, updater)

	result := make(chan acp.PromptResponse, 1)
	go func() {
		resp, err := agent.Prompt(context.Background(), acp.PromptRequest{
			SessionId: sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:steer-defer-setup")},
		})
		if err != nil {
			t.Errorf("prompt returned error: %v", err)
		}
		result <- resp
	}()

	select {
	case <-updater.textSeen:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the predecessor's own answer")
	}
	texts := updater.textMessages()
	if len(texts) != 1 || texts[0] != wantAnswer {
		t.Fatalf("predecessor answer = %v, want exactly [%q]", texts, wantAnswer)
	}

	if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: sessionID}); err != nil {
		t.Fatalf("cancel prompt: %v", err)
	}

	select {
	case resp := <-result:
		if resp.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop reason = %q, want cancelled", resp.StopReason)
		}
	case <-time.After(time.Second):
		t.Fatal("defer setup did not stop holding after cancellation")
	}
	// The hold must not itself produce a second answer — only the predecessor's
	// own text, matching "predecessor answers, then the steer runs as its own
	// turn" rather than the predecessor speaking twice.
	if texts := updater.textMessages(); len(texts) != 1 {
		t.Fatalf("defer setup emitted %v after its own answer, want exactly one message total", texts)
	}
}

func TestParseModelFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag returns default",
			args: []string{"mock-agent"},
			want: "mock-default",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--model", "mock-slow"},
			want: "mock-slow",
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", "--model=mock-fast"},
			want: "mock-fast",
		},
		{
			name: "flag with other args before",
			args: []string{"mock-agent", "--verbose", "--model", "mock-slow"},
			want: "mock-slow",
		},
		{
			name: "flag with other args after",
			args: []string{"mock-agent", "--model", "mock-fast", "--verbose"},
			want: "mock-fast",
		},
		{
			name: "dangling flag without value",
			args: []string{"mock-agent", "--model"},
			want: "mock-default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModelFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parseModelFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestStripKandevSystem(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no tags",
			input: "/slow 10s",
			want:  "/slow 10s",
		},
		{
			name:  "tags appended after user text (plan mode)",
			input: "/slow 10s\n\n<kandev-system>\nACTIVE DOCUMENT: editing plan\n</kandev-system>",
			want:  "/slow 10s",
		},
		{
			name:  "multiple tag blocks appended",
			input: "/slow 5s\n\n<kandev-system>\nDOC context\n</kandev-system>\n\n<kandev-system>\nFILE context\n</kandev-system>",
			want:  "/slow 5s",
		},
		{
			name:  "tags prepended before user text (backend system context)",
			input: "<kandev-system>\nKANDEV CONTEXT\n</kandev-system>\n\ne2e:delay(3000)\ne2e:message(\"hello\")",
			want:  "e2e:delay(3000)\ne2e:message(\"hello\")",
		},
		{
			name:  "tags both prepended and appended",
			input: "<kandev-system>\nSYS\n</kandev-system>\n\n/slow 5s\n\n<kandev-system>\nPLAN\n</kandev-system>",
			want:  "/slow 5s",
		},
		{
			name:  "only tags, no user text",
			input: "<kandev-system>\nsome context\n</kandev-system>",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace before tags",
			input: "  hello world  \n\n<kandev-system>ctx</kandev-system>",
			want:  "hello world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripKandevSystem(tt.input)
			if got != tt.want {
				t.Errorf("stripKandevSystem(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsChangesWalkthroughRequest(t *testing.T) {
	legacyPrompt := strings.Join([]string{
		"Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.",
		"",
		"Walkthrough requirements:",
		"- Anchor steps to changed lines or changed line ranges whenever possible.",
		"",
		"Available changed files:",
		"- src/app.ts [uncommitted]",
	}, "\n")
	promptReference := strings.Join([]string{
		"@changes-walkthrough",
		"",
		"<kandev-system>",
		"EXPANDED PROMPT REFERENCES",
		"### @changes-walkthrough",
		"Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.",
		"</kandev-system>",
	}, "\n")

	for _, prompt := range []string{legacyPrompt, promptReference} {
		if !isChangesWalkthroughRequest(prompt) {
			t.Fatalf("expected generated changes walkthrough prompt to be detected:\n%s", prompt)
		}
	}
	if isChangesWalkthroughRequest("show_walkthrough_kandev without the generated prompt shape") {
		t.Fatal("expected unrelated prompt not to be detected")
	}
	if isChangesWalkthroughRequest("what does @changes-walkthrough do?") {
		t.Fatal("expected incidental prompt reference not to be detected")
	}
}

func TestDelayRange(t *testing.T) {
	tests := []struct {
		model     string
		wantMinLo int
		wantMinHi int
		wantMaxLo int
		wantMaxHi int
	}{
		{"mock-fast", 10, 10, 50, 50},
		{"mock-slow", 500, 500, 3000, 3000},
		{"mock-default", 100, 100, 500, 500},
		{"unknown-model", 100, 100, 500, 500},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			lo, hi := delayRange(tt.model)
			if lo != tt.wantMinLo || hi != tt.wantMaxHi {
				t.Errorf("delayRange(%q) = (%d, %d), want (%d, %d)", tt.model, lo, hi, tt.wantMinLo, tt.wantMaxHi)
			}
		})
	}
}

func TestReadFileSnippet(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("reads up to maxLines", func(t *testing.T) {
		result := readFileSnippet(path, 3)
		expected := "line1\nline2\nline3\n"
		if result != expected {
			t.Errorf("readFileSnippet(%q, 3) = %q, want %q", path, result, expected)
		}
	})

	t.Run("reads all lines when maxLines exceeds file", func(t *testing.T) {
		result := readFileSnippet(path, 100)
		expected := "line1\nline2\nline3\nline4\nline5\n"
		if result != expected {
			t.Errorf("readFileSnippet(%q, 100) = %q, want %q", path, result, expected)
		}
	})

	t.Run("returns fallback for missing file", func(t *testing.T) {
		result := readFileSnippet("/nonexistent/file.txt", 10)
		if result != "// (file not readable)\n" {
			t.Errorf("readFileSnippet(missing) = %q, want fallback", result)
		}
	})

	t.Run("handles empty file", func(t *testing.T) {
		emptyPath := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		result := readFileSnippet(emptyPath, 10)
		if result != "\n" {
			t.Errorf("readFileSnippet(empty) = %q, want %q", result, "\n")
		}
	})
}

func TestPickEditableFragment(t *testing.T) {
	dir := t.TempDir()

	t.Run("returns fallback for missing file", func(t *testing.T) {
		old, new_ := pickEditableFragment("/nonexistent/file.go")
		if old != "hello" || new_ != "hello_mock" {
			t.Errorf("pickEditableFragment(missing) = (%q, %q), want (\"hello\", \"hello_mock\")", old, new_)
		}
	})

	t.Run("returns fallback for file with only short lines", func(t *testing.T) {
		path := filepath.Join(dir, "short.txt")
		if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0644); err != nil {
			t.Fatal(err)
		}
		old, new_ := pickEditableFragment(path)
		if old != "original" || new_ != "modified" {
			t.Errorf("pickEditableFragment(short) = (%q, %q), want (\"original\", \"modified\")", old, new_)
		}
	})

	t.Run("produces different old and new strings", func(t *testing.T) {
		path := filepath.Join(dir, "code.go")
		content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		old, new_ := pickEditableFragment(path)
		if old == new_ {
			t.Errorf("pickEditableFragment should produce different old and new, got %q", old)
		}
		if old == "" {
			t.Error("old string should not be empty")
		}
	})
}

func TestDiscoverFiles(t *testing.T) {
	// Reset global state
	workspaceFiles = nil

	// Save and restore working directory
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Create test files
	for _, f := range []struct{ name, content string }{
		{"main.go", "package main"},
		{"util.ts", "export {}"},
		{"image.png", "fake png"}, // should be skipped (non-text extension)
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a skipped directory
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "lib.js"), []byte("//"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reset cache before test
	workspaceFiles = nil
	files := discoverFiles()

	// Should find .go and .ts but not .png or node_modules
	foundGo, foundTs, foundPng, foundNodeModules := false, false, false, false
	for _, f := range files {
		switch filepath.Base(f.absPath) {
		case "main.go":
			foundGo = true
		case "util.ts":
			foundTs = true
		case "image.png":
			foundPng = true
		case "lib.js":
			foundNodeModules = true
		}
	}

	if !foundGo {
		t.Error("expected to find main.go")
	}
	if !foundTs {
		t.Error("expected to find util.ts")
	}
	if foundPng {
		t.Error("should not find image.png (not a text extension)")
	}
	if foundNodeModules {
		t.Error("should not find files in node_modules")
	}

	// Reset global state for other tests
	workspaceFiles = nil
}

func TestParseResumeFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag returns empty",
			args: []string{"mock-agent"},
			want: "",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--resume", "sess-123"},
			want: "sess-123",
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", "--resume=sess-456"},
			want: "sess-456",
		},
		{
			name: "flag with other args before",
			args: []string{"mock-agent", "--model", "fast", "--resume", "sess-789"},
			want: "sess-789",
		},
		{
			name: "flag with other args after",
			args: []string{"mock-agent", "--resume", "sess-abc", "--verbose"},
			want: "sess-abc",
		},
		{
			name: "dangling flag without value",
			args: []string{"mock-agent", "--resume"},
			want: "",
		},
		{
			name: "flag combined with --tui",
			args: []string{"mock-agent", "--tui", "--resume", "sess-xyz"},
			want: "sess-xyz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResumeFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parseResumeFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseSubtaskTitle(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		want   string
		isAuto bool
	}{
		{name: "lowercase no title", cmd: "/subtask", isAuto: true},
		{name: "lowercase with title", cmd: "/subtask My task", want: "My task"},
		{name: "uppercase route, mixed-case title", cmd: "/SUBTASK My Task", want: "My Task"},
		{name: "mixed-case route preserves title casing", cmd: "/SubTask Hello World", want: "Hello World"},
		{name: "extra whitespace trimmed", cmd: "/subtask   trimmed   ", want: "trimmed"},
		{name: "empty mixed-case route", cmd: "/SubTask", isAuto: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubtaskTitle(tt.cmd)
			if tt.isAuto {
				if !strings.HasPrefix(got, "Mock subtask ") {
					t.Errorf("parseSubtaskTitle(%q) = %q, want auto-generated %q-prefixed title", tt.cmd, got, "Mock subtask ")
				}
				return
			}
			if got != tt.want {
				t.Errorf("parseSubtaskTitle(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestParseFailOnResumeFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "absent",
			args: []string{"mock-agent", "--tui"},
			want: false,
		},
		{
			name: "present alone",
			args: []string{"mock-agent", "--tui", "--fail-on-resume"},
			want: true,
		},
		{
			name: "present with -c",
			args: []string{"mock-agent", "--tui", "-c", "--fail-on-resume"},
			want: true,
		},
		{
			name: "present interleaved with other flags",
			args: []string{"mock-agent", "--fail-on-resume", "--tui", "--model", "mock-fast"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFailOnResumeFromArgs(tt.args); got != tt.want {
				t.Errorf("parseFailOnResumeFromArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
