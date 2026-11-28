package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	profile "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type canvasPromptAdapter struct {
	mcpCaptureAdapter
	prompts chan string
}

func (a *canvasPromptAdapter) Prompt(_ context.Context, text string, _ []v1.MessageAttachment, _ uint64) error {
	a.prompts <- text
	return nil
}

func TestCanvasGuidanceOnExistingSession(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s := newTestServer(t)
		p := profile.New(profile.SurfaceKanbanTask, nil, nil)
		if enabled {
			p = p.WithCapability(profile.CapabilityCanvas)
		}
		s.cfg.McpProfile = &p
		s.mcpServer = mcpserver.NewWithProfile(nil, "session", "task", 0, newTestLogger(), "", false, p)
		capture := &canvasPromptAdapter{prompts: make(chan string, 1)}
		s.procMgr.SetAdapterForTest(capture)
		ctx := context.Background()
		load, _ := ws.NewRequest("load", "agent.session.load", LoadSessionRequest{SessionID: "session"})
		if resp := s.handleWSLoadSession(ctx, load); resp.Type != ws.MessageTypeResponse {
			t.Fatalf("load: %v", resp)
		}
		msg, _ := ws.NewRequest("prompt", "agent.prompt", PromptRequest{Text: "Draw a canvas"})
		if resp := s.handleWSPrompt(ctx, msg); resp.Type != ws.MessageTypeResponse {
			t.Fatalf("prompt: %v", resp)
		}
		select {
		case text := <-capture.prompts:
			if strings.Contains(text, "CANVAS AUTHORING:") != enabled || !strings.Contains(text, "Draw a canvas") {
				t.Fatalf("canvas enabled=%t, prompt=%q", enabled, text)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("missing provider prompt")
		}
	}
}

func TestCanvasGuidanceRetryAndReplacement(t *testing.T) {
	s := newTestServer(t)
	first, second := "load", "reset"
	s.pendingCanvasGuidance.Store(&first)
	failed := &promptErrorAdapter{prompted: make(chan uint64, 1), err: errors.New("rejected")}
	req := PromptRequest{Text: "Draw a canvas"}
	if err := s.promptWithCanvasGuidance(context.Background(), failed, req, &first); err == nil || s.pendingCanvasGuidance.Load() != &first {
		t.Fatal("rejected prompt consumed the reminder")
	}
	s.pendingCanvasGuidance.Store(&second)
	capture := &recordingAdapter{}
	for _, pending := range []*string{&first, &second, nil} {
		if err := s.promptWithCanvasGuidance(context.Background(), capture, req, pending); err != nil {
			t.Fatal(err)
		}
		if pending == &first && s.pendingCanvasGuidance.Load() != &second {
			t.Fatal("old completion consumed new reminder")
		}
	}
	if s.pendingCanvasGuidance.Load() != nil || capture.promptCalls[2].text != req.Text {
		t.Fatal("completed delivery repeated the reminder")
	}
}

func TestCanvasGuidanceLeavesSlashCommandsIntact(t *testing.T) {
	s := newTestServer(t)
	attempt := "load"
	s.pendingCanvasGuidance.Store(&attempt)
	capture := &recordingAdapter{}
	for _, text := range []string{"/model example", "  /fast  "} {
		err := s.promptWithCanvasGuidance(context.Background(), capture, PromptRequest{Text: text}, s.pendingCanvasGuidance.Load())
		if err != nil || capture.promptCalls[len(capture.promptCalls)-1].text != text || s.pendingCanvasGuidance.Load() != &attempt {
			t.Fatalf("command %q must reach the provider unchanged and leave guidance pending", text)
		}
	}
}
