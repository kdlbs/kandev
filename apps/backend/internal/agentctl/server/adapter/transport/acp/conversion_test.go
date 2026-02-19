package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

// newTestAdapter creates a minimal Adapter suitable for unit testing conversion functions.
// It uses a nop logger and a buffered updates channel so tests can drain events.
func newTestAdapter() *Adapter {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stderr",
	})
	cfg := &shared.Config{
		AgentID: "test-agent",
		WorkDir: "/tmp/test",
	}
	return NewAdapter(cfg, log)
}

// drainEvents reads all buffered events from the adapter's updates channel.
func drainEvents(a *Adapter) []AgentEvent {
	var events []AgentEvent
	for {
		select {
		case ev := <-a.updatesCh:
			events = append(events, ev)
		default:
			return events
		}
	}
}

// --- derefStr ---

func TestDerefStr_NilPointer(t *testing.T) {
	result := derefStr(nil)
	if result != "" {
		t.Errorf("derefStr(nil) = %q, want empty string", result)
	}
}

func TestDerefStr_NonNilPointer(t *testing.T) {
	s := "hello"
	result := derefStr(&s)
	if result != "hello" {
		t.Errorf("derefStr(&%q) = %q, want %q", s, result, s)
	}
}

func TestDerefStr_EmptyString(t *testing.T) {
	s := ""
	result := derefStr(&s)
	if result != "" {
		t.Errorf("derefStr(&%q) = %q, want empty string", s, result)
	}
}

// --- convertContentBlockToStreams ---

func TestConvertContentBlockToStreams_Text(t *testing.T) {
	a := newTestAdapter()
	cb := acp.TextBlock("hello world")

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for text content block")
	}
	if result.Type != "text" {
		t.Errorf("Type = %q, want %q", result.Type, "text")
	}
	if result.Text != "hello world" {
		t.Errorf("Text = %q, want %q", result.Text, "hello world")
	}
}

func TestConvertContentBlockToStreams_Image(t *testing.T) {
	a := newTestAdapter()
	uri := "https://example.com/img.png"
	cb := acp.ContentBlock{
		Image: &acp.ContentBlockImage{
			Data:     "base64data",
			MimeType: "image/png",
			Type:     "image",
			Uri:      &uri,
		},
	}

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for image content block")
	}
	if result.Type != "image" {
		t.Errorf("Type = %q, want %q", result.Type, "image")
	}
	if result.Data != "base64data" {
		t.Errorf("Data = %q, want %q", result.Data, "base64data")
	}
	if result.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want %q", result.MimeType, "image/png")
	}
	if result.URI != "https://example.com/img.png" {
		t.Errorf("URI = %q, want %q", result.URI, "https://example.com/img.png")
	}
}

func TestConvertContentBlockToStreams_ImageWithoutURI(t *testing.T) {
	a := newTestAdapter()
	cb := acp.ImageBlock("base64data", "image/jpeg")

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for image content block")
	}
	if result.Type != "image" {
		t.Errorf("Type = %q, want %q", result.Type, "image")
	}
	if result.URI != "" {
		t.Errorf("URI = %q, want empty string for image without URI", result.URI)
	}
}

func TestConvertContentBlockToStreams_Audio(t *testing.T) {
	a := newTestAdapter()
	cb := acp.AudioBlock("audiodata", "audio/mp3")

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for audio content block")
	}
	if result.Type != "audio" {
		t.Errorf("Type = %q, want %q", result.Type, "audio")
	}
	if result.Data != "audiodata" {
		t.Errorf("Data = %q, want %q", result.Data, "audiodata")
	}
	if result.MimeType != "audio/mp3" {
		t.Errorf("MimeType = %q, want %q", result.MimeType, "audio/mp3")
	}
}

func TestConvertContentBlockToStreams_ResourceLink(t *testing.T) {
	a := newTestAdapter()
	mime := "text/plain"
	title := "My Resource"
	desc := "A description"
	size := 1024
	cb := acp.ContentBlock{
		ResourceLink: &acp.ContentBlockResourceLink{
			Uri:         "file:///path/to/file.txt",
			Name:        "file.txt",
			MimeType:    &mime,
			Title:       &title,
			Description: &desc,
			Size:        &size,
			Type:        "resource_link",
		},
	}

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for resource_link content block")
	}
	if result.Type != "resource_link" {
		t.Errorf("Type = %q, want %q", result.Type, "resource_link")
	}
	if result.URI != "file:///path/to/file.txt" {
		t.Errorf("URI = %q, want %q", result.URI, "file:///path/to/file.txt")
	}
	if result.Name != "file.txt" {
		t.Errorf("Name = %q, want %q", result.Name, "file.txt")
	}
	if result.MimeType != "text/plain" {
		t.Errorf("MimeType = %q, want %q", result.MimeType, "text/plain")
	}
	if result.Title != "My Resource" {
		t.Errorf("Title = %q, want %q", result.Title, "My Resource")
	}
	if result.Description != "A description" {
		t.Errorf("Description = %q, want %q", result.Description, "A description")
	}
	if result.Size == nil || *result.Size != 1024 {
		t.Errorf("Size = %v, want 1024", result.Size)
	}
}

func TestConvertContentBlockToStreams_ResourceLinkNilOptionals(t *testing.T) {
	a := newTestAdapter()
	cb := acp.ResourceLinkBlock("file.txt", "file:///path")

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for resource_link content block")
	}
	if result.Type != "resource_link" {
		t.Errorf("Type = %q, want %q", result.Type, "resource_link")
	}
	if result.MimeType != "" {
		t.Errorf("MimeType = %q, want empty when nil", result.MimeType)
	}
	if result.Title != "" {
		t.Errorf("Title = %q, want empty when nil", result.Title)
	}
	if result.Description != "" {
		t.Errorf("Description = %q, want empty when nil", result.Description)
	}
	if result.Size != nil {
		t.Errorf("Size = %v, want nil", result.Size)
	}
}

func TestConvertContentBlockToStreams_ResourceWithTextContents(t *testing.T) {
	a := newTestAdapter()
	mime := "text/plain"
	cb := acp.ResourceBlock(acp.EmbeddedResourceResource{
		TextResourceContents: &acp.TextResourceContents{
			Uri:      "file:///readme.md",
			Text:     "# Hello",
			MimeType: &mime,
		},
	})

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for resource content block")
	}
	if result.Type != "resource" {
		t.Errorf("Type = %q, want %q", result.Type, "resource")
	}
	if result.URI != "file:///readme.md" {
		t.Errorf("URI = %q, want %q", result.URI, "file:///readme.md")
	}
	if result.Text != "# Hello" {
		t.Errorf("Text = %q, want %q", result.Text, "# Hello")
	}
	if result.MimeType != "text/plain" {
		t.Errorf("MimeType = %q, want %q", result.MimeType, "text/plain")
	}
}

func TestConvertContentBlockToStreams_ResourceWithBlobContents(t *testing.T) {
	a := newTestAdapter()
	mime := "application/octet-stream"
	cb := acp.ResourceBlock(acp.EmbeddedResourceResource{
		BlobResourceContents: &acp.BlobResourceContents{
			Uri:      "file:///data.bin",
			Blob:     "blobdata",
			MimeType: &mime,
		},
	})

	result := a.convertContentBlockToStreams(cb)

	if result == nil {
		t.Fatal("expected non-nil result for resource content block")
	}
	if result.Type != "resource" {
		t.Errorf("Type = %q, want %q", result.Type, "resource")
	}
	if result.URI != "file:///data.bin" {
		t.Errorf("URI = %q, want %q", result.URI, "file:///data.bin")
	}
	if result.Data != "blobdata" {
		t.Errorf("Data = %q, want %q", result.Data, "blobdata")
	}
	if result.MimeType != "application/octet-stream" {
		t.Errorf("MimeType = %q, want %q", result.MimeType, "application/octet-stream")
	}
}

func TestConvertContentBlockToStreams_UnknownType(t *testing.T) {
	a := newTestAdapter()
	// Empty ContentBlock with no variant set
	cb := acp.ContentBlock{}

	result := a.convertContentBlockToStreams(cb)

	if result != nil {
		t.Errorf("expected nil for unknown content block type, got %+v", result)
	}
}

// --- convertToolCallContents ---

func TestConvertToolCallContents_EmptyInput(t *testing.T) {
	a := newTestAdapter()

	result := a.convertToolCallContents(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}

	result = a.convertToolCallContents([]acp.ToolCallContent{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestConvertToolCallContents_DiffItem(t *testing.T) {
	a := newTestAdapter()
	oldText := "old content"
	contents := []acp.ToolCallContent{
		{
			Diff: &acp.ToolCallContentDiff{
				Path:    "src/main.ts",
				OldText: &oldText,
				NewText: "new content",
				Type:    "diff",
			},
		},
	}

	result := a.convertToolCallContents(contents)

	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Type != "diff" {
		t.Errorf("Type = %q, want %q", result[0].Type, "diff")
	}
	if result[0].Path != "src/main.ts" {
		t.Errorf("Path = %q, want %q", result[0].Path, "src/main.ts")
	}
	if result[0].OldText == nil || *result[0].OldText != "old content" {
		t.Errorf("OldText = %v, want 'old content'", result[0].OldText)
	}
	if result[0].NewText != "new content" {
		t.Errorf("NewText = %q, want %q", result[0].NewText, "new content")
	}
}

func TestConvertToolCallContents_TerminalItem(t *testing.T) {
	a := newTestAdapter()
	contents := []acp.ToolCallContent{
		{
			Terminal: &acp.ToolCallContentTerminal{
				TerminalId: "term-123",
				Type:       "terminal",
			},
		},
	}

	result := a.convertToolCallContents(contents)

	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Type != "terminal" {
		t.Errorf("Type = %q, want %q", result[0].Type, "terminal")
	}
	if result[0].TerminalID != "term-123" {
		t.Errorf("TerminalID = %q, want %q", result[0].TerminalID, "term-123")
	}
}

func TestConvertToolCallContents_ContentItemWithTextBlock(t *testing.T) {
	a := newTestAdapter()
	contents := []acp.ToolCallContent{
		{
			Content: &acp.ToolCallContentContent{
				Content: acp.TextBlock("tool output text"),
				Type:    "content",
			},
		},
	}

	result := a.convertToolCallContents(contents)

	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Type != "content" {
		t.Errorf("Type = %q, want %q", result[0].Type, "content")
	}
	if result[0].Content == nil {
		t.Fatal("expected Content to be set")
	}
	if result[0].Content.Type != "text" {
		t.Errorf("Content.Type = %q, want %q", result[0].Content.Type, "text")
	}
	if result[0].Content.Text != "tool output text" {
		t.Errorf("Content.Text = %q, want %q", result[0].Content.Text, "tool output text")
	}
}

func TestConvertToolCallContents_MixedItems(t *testing.T) {
	a := newTestAdapter()
	oldText := "before"
	contents := []acp.ToolCallContent{
		{
			Diff: &acp.ToolCallContentDiff{
				Path:    "file.go",
				OldText: &oldText,
				NewText: "after",
				Type:    "diff",
			},
		},
		{
			Terminal: &acp.ToolCallContentTerminal{
				TerminalId: "term-456",
				Type:       "terminal",
			},
		},
		{
			Content: &acp.ToolCallContentContent{
				Content: acp.TextBlock("some text"),
				Type:    "content",
			},
		},
	}

	result := a.convertToolCallContents(contents)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0].Type != "diff" {
		t.Errorf("item[0].Type = %q, want %q", result[0].Type, "diff")
	}
	if result[1].Type != "terminal" {
		t.Errorf("item[1].Type = %q, want %q", result[1].Type, "terminal")
	}
	if result[2].Type != "content" {
		t.Errorf("item[2].Type = %q, want %q", result[2].Type, "content")
	}
}

// --- convertMessageChunk ---

func TestConvertMessageChunk_TextAssistant(t *testing.T) {
	a := newTestAdapter()
	cb := acp.TextBlock("Hello from assistant")

	result := a.convertMessageChunk("session-1", cb, "assistant")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != streams.EventTypeMessageChunk {
		t.Errorf("Type = %q, want %q", result.Type, streams.EventTypeMessageChunk)
	}
	if result.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "session-1")
	}
	if result.Text != "Hello from assistant" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello from assistant")
	}
	if result.Role != "" {
		t.Errorf("Role = %q, want empty for assistant messages", result.Role)
	}
	if len(result.ContentBlocks) != 0 {
		t.Errorf("ContentBlocks should be empty for text, got %d", len(result.ContentBlocks))
	}
}

func TestConvertMessageChunk_TextUser(t *testing.T) {
	a := newTestAdapter()
	cb := acp.TextBlock("Hello from user")

	result := a.convertMessageChunk("session-2", cb, "user")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Role != "user" {
		t.Errorf("Role = %q, want %q", result.Role, "user")
	}
	if result.Text != "Hello from user" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello from user")
	}
}

func TestConvertMessageChunk_ImageContent(t *testing.T) {
	a := newTestAdapter()
	cb := acp.ImageBlock("imgdata", "image/png")

	result := a.convertMessageChunk("session-3", cb, "assistant")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != streams.EventTypeMessageChunk {
		t.Errorf("Type = %q, want %q", result.Type, streams.EventTypeMessageChunk)
	}
	if result.Text != "" {
		t.Errorf("Text = %q, want empty for image content", result.Text)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(result.ContentBlocks))
	}
	if result.ContentBlocks[0].Type != "image" {
		t.Errorf("ContentBlocks[0].Type = %q, want %q", result.ContentBlocks[0].Type, "image")
	}
	if result.ContentBlocks[0].Data != "imgdata" {
		t.Errorf("ContentBlocks[0].Data = %q, want %q", result.ContentBlocks[0].Data, "imgdata")
	}
}

func TestConvertMessageChunk_UnknownContent(t *testing.T) {
	a := newTestAdapter()
	// Empty ContentBlock with no variant set
	cb := acp.ContentBlock{}

	result := a.convertMessageChunk("session-4", cb, "assistant")

	if result != nil {
		t.Errorf("expected nil for unknown content type, got %+v", result)
	}
}

func TestConvertMessageChunk_AudioContent(t *testing.T) {
	a := newTestAdapter()
	cb := acp.AudioBlock("audiodata", "audio/wav")

	result := a.convertMessageChunk("session-5", cb, "assistant")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(result.ContentBlocks))
	}
	if result.ContentBlocks[0].Type != "audio" {
		t.Errorf("ContentBlocks[0].Type = %q, want %q", result.ContentBlocks[0].Type, "audio")
	}
}

// --- cancelActiveToolCalls ---

func TestCancelActiveToolCalls_EmitsEventsAndClears(t *testing.T) {
	a := newTestAdapter()
	sessionID := "session-cancel"

	// Set up active tool calls
	a.activeToolCalls["tc-1"] = &streams.NormalizedPayload{}
	a.activeToolCalls["tc-2"] = &streams.NormalizedPayload{}

	a.cancelActiveToolCalls(sessionID)

	// Verify the activeToolCalls map is cleared
	if len(a.activeToolCalls) != 0 {
		t.Errorf("activeToolCalls should be empty after cancel, got %d entries", len(a.activeToolCalls))
	}

	// Drain events from the channel
	events := drainEvents(a)

	if len(events) != 2 {
		t.Fatalf("expected 2 cancel events, got %d", len(events))
	}

	// Collect tool call IDs from events
	seenIDs := map[string]bool{}
	for _, ev := range events {
		if ev.Type != streams.EventTypeToolUpdate {
			t.Errorf("event Type = %q, want %q", ev.Type, streams.EventTypeToolUpdate)
		}
		if ev.SessionID != sessionID {
			t.Errorf("event SessionID = %q, want %q", ev.SessionID, sessionID)
		}
		if ev.ToolStatus != "cancelled" {
			t.Errorf("event ToolStatus = %q, want %q", ev.ToolStatus, "cancelled")
		}
		seenIDs[ev.ToolCallID] = true
	}
	if !seenIDs["tc-1"] {
		t.Error("expected cancel event for tc-1")
	}
	if !seenIDs["tc-2"] {
		t.Error("expected cancel event for tc-2")
	}
}

func TestCancelActiveToolCalls_EmptyMap(t *testing.T) {
	a := newTestAdapter()

	// Should not panic or emit events when map is empty
	a.cancelActiveToolCalls("session-empty")

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty activeToolCalls, got %d", len(events))
	}
}

// --- convertAvailableCommands ---

func TestConvertAvailableCommands_Basic(t *testing.T) {
	a := newTestAdapter()
	update := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{
			{
				Name:        "commit",
				Description: "Commit changes",
			},
			{
				Name:        "review",
				Description: "Review code",
			},
		},
	}

	result := a.convertAvailableCommands("session-cmd", update)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != streams.EventTypeAvailableCommands {
		t.Errorf("Type = %q, want %q", result.Type, streams.EventTypeAvailableCommands)
	}
	if result.SessionID != "session-cmd" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "session-cmd")
	}
	if len(result.AvailableCommands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(result.AvailableCommands))
	}
	if result.AvailableCommands[0].Name != "commit" {
		t.Errorf("command[0].Name = %q, want %q", result.AvailableCommands[0].Name, "commit")
	}
	if result.AvailableCommands[0].Description != "Commit changes" {
		t.Errorf("command[0].Description = %q, want %q", result.AvailableCommands[0].Description, "Commit changes")
	}
	if result.AvailableCommands[1].Name != "review" {
		t.Errorf("command[1].Name = %q, want %q", result.AvailableCommands[1].Name, "review")
	}
}

func TestConvertAvailableCommands_WithInputHint(t *testing.T) {
	a := newTestAdapter()
	update := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{
			{
				Name:        "search",
				Description: "Search codebase",
				Input: &acp.AvailableCommandInput{
					UnstructuredCommandInput: &acp.AvailableCommandUnstructuredCommandInput{
						Hint: "Enter search query",
					},
				},
			},
		},
	}

	result := a.convertAvailableCommands("session-hint", update)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.AvailableCommands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result.AvailableCommands))
	}
	if result.AvailableCommands[0].InputHint != "Enter search query" {
		t.Errorf("InputHint = %q, want %q", result.AvailableCommands[0].InputHint, "Enter search query")
	}
}

func TestConvertAvailableCommands_NoInputHint(t *testing.T) {
	a := newTestAdapter()
	update := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{
			{
				Name:        "help",
				Description: "Show help",
				Input:       nil,
			},
		},
	}

	result := a.convertAvailableCommands("session-nohint", update)

	if result.AvailableCommands[0].InputHint != "" {
		t.Errorf("InputHint = %q, want empty for nil Input", result.AvailableCommands[0].InputHint)
	}
}

func TestConvertAvailableCommands_Empty(t *testing.T) {
	a := newTestAdapter()
	update := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{},
	}

	result := a.convertAvailableCommands("session-empty", update)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.AvailableCommands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(result.AvailableCommands))
	}
}

// --- emitInitialModeState ---

func TestEmitInitialModeState(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "session-mode"

	modes := &acp.SessionModeState{
		CurrentModeId: "architect",
	}

	a.emitInitialModeState(modes)

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != streams.EventTypeSessionMode {
		t.Errorf("Type = %q, want %q", events[0].Type, streams.EventTypeSessionMode)
	}
	if events[0].SessionID != "session-mode" {
		t.Errorf("SessionID = %q, want %q", events[0].SessionID, "session-mode")
	}
	if events[0].CurrentModeID != "architect" {
		t.Errorf("CurrentModeID = %q, want %q", events[0].CurrentModeID, "architect")
	}
}

// --- sendUpdate ---

func TestSendUpdate_NormalOperation(t *testing.T) {
	a := newTestAdapter()
	event := AgentEvent{
		Type:      streams.EventTypeComplete,
		SessionID: "session-send",
	}

	a.sendUpdate(event)

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != streams.EventTypeComplete {
		t.Errorf("Type = %q, want %q", events[0].Type, streams.EventTypeComplete)
	}
}

func TestSendUpdate_ClosedAdapter(t *testing.T) {
	a := newTestAdapter()
	// Close the adapter
	_ = a.Close()

	// Should not panic when sending to closed adapter
	// (sendUpdate checks the closed flag)
	// We need a fresh adapter since Close() closes the channel
	a2 := newTestAdapter()
	a2.mu.Lock()
	a2.closed = true
	a2.mu.Unlock()

	// This should not panic
	a2.sendUpdate(AgentEvent{Type: streams.EventTypeComplete})
}
