package acp

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func antigravityIntPtr(value int) *int { return &value }

func newAntigravityTestAdapter() *Adapter {
	return newTestAdapterForAgent(acpcompat.AntigravityAgentID)
}

// TestAntigravityUsage covers REQ-AGENTS-ANTIGRAVITY-ACP-014 for the native
// typed prompt usage and the separate context-occupancy channel. Fixtures are
// synthetic and carry no provider-billed dollar amount, because the
// investigated server does not supply one. Native prompt usage covers the main
// trajectory only: child-agent tokens are not part of this frame, and their
// absence is not a zero measurement.
func TestAntigravityUsage(t *testing.T) {
	t.Run("native typed usage keeps every supplied field", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		resp := &acpsdk.PromptResponse{Usage: &acpsdk.Usage{
			InputTokens:       1200,
			OutputTokens:      340,
			CachedReadTokens:  antigravityIntPtr(800),
			CachedWriteTokens: antigravityIntPtr(120),
			ThoughtTokens:     antigravityIntPtr(90),
			TotalTokens:       2550,
		}}

		usage := a.dialect.promptUsage(extractUsage(resp), resp.Meta)
		if usage == nil {
			t.Fatal("expected non-nil usage")
		}
		if usage.InputTokens != 1200 || usage.OutputTokens != 340 {
			t.Errorf("input/output = (%d, %d), want (1200, 340)", usage.InputTokens, usage.OutputTokens)
		}
		if usage.CachedReadTokens != 800 || usage.CachedWriteTokens != 120 {
			t.Errorf("cache read/write = (%d, %d), want (800, 120)",
				usage.CachedReadTokens, usage.CachedWriteTokens)
		}
		if usage.ThoughtTokens != 90 || usage.TotalTokens != 2550 {
			t.Errorf("thought/total = (%d, %d), want (90, 2550)", usage.ThoughtTokens, usage.TotalTokens)
		}
		if !usage.OutputTokensPresent {
			t.Error("OutputTokensPresent = false, want true for a native output sample")
		}
		if usage.Estimated {
			t.Error("Estimated = true, want false for native typed usage")
		}
		if usage.ProviderReportedCostPresent || usage.ProviderReportedCostSubcents != 0 {
			t.Errorf("provider cost = (%d, %t), want absent",
				usage.ProviderReportedCostSubcents, usage.ProviderReportedCostPresent)
		}
	})

	t.Run("omitted optional fields stay zero", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		resp := &acpsdk.PromptResponse{Usage: &acpsdk.Usage{
			InputTokens:  40,
			OutputTokens: 2,
			TotalTokens:  42,
		}}

		usage := a.dialect.promptUsage(extractUsage(resp), resp.Meta)
		if usage == nil {
			t.Fatal("expected non-nil usage")
		}
		if usage.CachedReadTokens != 0 || usage.CachedWriteTokens != 0 || usage.ThoughtTokens != 0 {
			t.Errorf("omitted fields = (%d, %d, %d), want zeroes",
				usage.CachedReadTokens, usage.CachedWriteTokens, usage.ThoughtTokens)
		}
		if usage.TotalTokens != 42 {
			t.Errorf("TotalTokens = %d, want 42", usage.TotalTokens)
		}
		if usage.ProviderReportedCostPresent {
			t.Error("ProviderReportedCostPresent = true, want false without a native cost sample")
		}
	})

	t.Run("context occupancy is separate from prompt tokens", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		const sessionID = "antigravity-context"
		event := a.convertUsageUpdate(sessionID, usageUpdate(200_000, 48_000))
		if event == nil {
			t.Fatal("expected a context window event")
		}
		if event.Type != streams.EventTypeContextWindow {
			t.Errorf("event type = %q, want context window", event.Type)
		}
		if event.ContextWindowSize != 200_000 || event.ContextWindowUsed != 48_000 {
			t.Errorf("context = %d/%d, want 48000/200000",
				event.ContextWindowUsed, event.ContextWindowSize)
		}
		if event.ContextWindowRemaining != 152_000 {
			t.Errorf("remaining = %d, want 152000", event.ContextWindowRemaining)
		}

		resp := &acpsdk.PromptResponse{Usage: &acpsdk.Usage{
			InputTokens: 1200, OutputTokens: 40, TotalTokens: 1240,
		}}
		usage := a.dialect.promptUsage(extractUsage(resp), resp.Meta)
		if usage == nil || usage.InputTokens != 1200 {
			t.Fatalf("prompt usage = %#v, want native input 1200", usage)
		}
		delta, cost, present := a.consumeUsageDeltaWithPresence(sessionID)
		if delta != 48_000 || cost != 0 || present {
			t.Errorf("occupancy delta = (%d, %d, %t), want (48000, 0, false)",
				delta, cost, present)
		}
		if usage.InputTokens == delta {
			t.Error("prompt input tokens were replaced by context occupancy")
		}
	})

	t.Run("repeated context updates produce one nonnegative delta", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		const sessionID = "antigravity-repeated"
		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 40_000))
		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 40_000))
		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 46_000))

		delta, cost, present := a.consumeUsageDeltaWithPresence(sessionID)
		if delta != 46_000 || cost != 0 || present {
			t.Fatalf("delta = (%d, %d, %t), want (46000, 0, false)", delta, cost, present)
		}
		delta, _, _ = a.consumeUsageDeltaWithPresence(sessionID)
		if delta != 0 {
			t.Errorf("second consume delta = %d, want 0 (no duplicate delta)", delta)
		}

		stale := a.convertUsageUpdate(sessionID, usageUpdate(200_000, 46_500))
		if stale == nil || stale.ContextWindowSize != 200_000 {
			t.Fatalf("sticky context size = %#v, want 200000", stale)
		}
	})

	t.Run("resumed turn replay establishes a baseline", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		const sessionID = "antigravity-resumed"
		// session/load replays the restored conversation's context sample.
		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 50_000))
		a.mu.Lock()
		a.consumeUsageBaselineLocked(sessionID)
		a.mu.Unlock()

		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 52_000))
		delta, cost, present := a.consumeUsageDeltaWithPresence(sessionID)
		if delta != 2_000 || cost != 0 || present {
			t.Errorf("resumed delta = (%d, %d, %t), want (2000, 0, false)",
				delta, cost, present)
		}
	})

	t.Run("session creation sample is not billed to the first prompt", func(t *testing.T) {
		a := newAntigravityTestAdapter()
		const sessionID = "antigravity-created"
		a.convertUsageUpdate(sessionID, usageUpdate(200_000, 30_000))
		a.mu.Lock()
		a.retainConsumedUsageBaselineLocked(sessionID)
		a.mu.Unlock()

		delta, _, _ := a.consumeUsageDeltaWithPresence(sessionID)
		if delta != 0 {
			t.Errorf("first-prompt delta = %d, want 0 after the creation baseline", delta)
		}
	})
}

// antigravityNativeUsageAgent is a synthetic ACP peer for the native /usage
// command. It returns a Markdown allowance report and reports unchanged
// context occupancy, matching a command that performs no model inference.
type antigravityNativeUsageAgent struct {
	burstAgent
	conn     *acpsdk.AgentSideConnection
	mu       sync.Mutex
	prompts  []acpsdk.PromptRequest
	markdown string
	size     int
	used     int
}

func (a *antigravityNativeUsageAgent) Initialize(
	_ context.Context,
	req acpsdk.InitializeRequest,
) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (a *antigravityNativeUsageAgent) NewSession(
	context.Context,
	acpsdk.NewSessionRequest,
) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "antigravity-native-usage"}, nil
}

func (a *antigravityNativeUsageAgent) Prompt(
	ctx context.Context,
	req acpsdk.PromptRequest,
) (acpsdk.PromptResponse, error) {
	a.mu.Lock()
	a.prompts = append(a.prompts, req)
	markdown, size, used := a.markdown, a.size, a.used
	a.mu.Unlock()

	if markdown != "" {
		err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
			SessionId: req.SessionId,
			Update: acpsdk.SessionUpdate{
				AgentMessageChunk: &acpsdk.SessionUpdateAgentMessageChunk{
					Content: acpsdk.TextBlock(markdown),
				},
			},
		})
		if err != nil {
			return acpsdk.PromptResponse{}, err
		}
	}
	if size > 0 {
		err := a.conn.SessionUpdate(ctx, acpsdk.SessionNotification{
			SessionId: req.SessionId,
			Update:    acpsdk.SessionUpdate{UsageUpdate: usageUpdate(int64(size), int64(used))},
		})
		if err != nil {
			return acpsdk.PromptResponse{}, err
		}
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *antigravityNativeUsageAgent) capturedPrompts() []acpsdk.PromptRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]acpsdk.PromptRequest(nil), a.prompts...)
}

func promptTextBlocks(req acpsdk.PromptRequest) []string {
	texts := make([]string, 0, len(req.Prompt))
	for _, block := range req.Prompt {
		if block.Text != nil {
			texts = append(texts, block.Text.Text)
		}
	}
	return texts
}

func setupAntigravityNativeUsageAgent(
	t *testing.T,
	fake *antigravityNativeUsageAgent,
) (*Adapter, string) {
	t.Helper()
	a := newTestAdapterForAgent(acpcompat.AntigravityAgentID)
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake.conn = acpsdk.NewAgentSideConnection(fake, a2cW, c2aR)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sessionID, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	drainEvents(a)
	return a, sessionID
}

// TestAntigravityNativeUsageCommand proves /usage is forwarded as an ordinary
// prompt, renders the native Markdown verbatim, and does not synthesize a
// token delta or a model turn when the command reports no model usage.
func TestAntigravityNativeUsageCommand(t *testing.T) {
	const markdown = "## Antigravity usage\n\n" +
		"| Window | Used | Reset |\n| --- | --- | --- |\n" +
		"| 5-hour | 42% | in 3 hours |\n"
	fake := &antigravityNativeUsageAgent{markdown: markdown, size: 200_000, used: 40_000}
	a, sessionID := setupAntigravityNativeUsageAgent(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Establish the session's occupancy baseline before the command runs, so
	// the unchanged frame it reports cannot be billed as new prompt growth.
	a.convertUsageUpdate(sessionID, usageUpdate(200_000, 40_000))
	if delta, _, _ := a.consumeUsageDeltaWithPresence(sessionID); delta != 40_000 {
		t.Fatalf("baseline delta = %d, want 40000", delta)
	}
	drainEvents(a)

	if err := a.Prompt(ctx, "/usage", nil, 9); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	prompts := fake.capturedPrompts()
	if len(prompts) != 1 {
		t.Fatalf("prompt count = %d, want 1", len(prompts))
	}
	texts := promptTextBlocks(prompts[0])
	if len(texts) != 1 || texts[0] != "/usage" {
		t.Fatalf("prompt blocks = %#v, want the native /usage command", texts)
	}

	events := drainEvents(a)
	var complete *AgentEvent
	var contextEvent *AgentEvent
	var markdownSeen bool
	for i := range events {
		event := &events[i]
		switch event.Type {
		case streams.EventTypeMessageChunk:
			if event.Text == markdown {
				markdownSeen = true
			}
		case streams.EventTypeContextWindow:
			contextEvent = event
		case streams.EventTypeComplete:
			complete = event
		case streams.EventTypeError:
			t.Fatalf("unexpected error event: %s", event.Error)
		}
	}
	if !markdownSeen {
		t.Fatalf("native Markdown report was not rendered as a message chunk: %#v", events)
	}
	if complete == nil {
		t.Fatal("missing complete event")
	}
	if complete.PromptGeneration != 9 {
		t.Errorf("complete generation = %d, want 9", complete.PromptGeneration)
	}
	if complete.Usage != nil {
		t.Errorf("complete usage = %#v, want nil for a native command with no model turn", complete.Usage)
	}
	if complete.Data["stop_reason"] != string(acpsdk.StopReasonEndTurn) {
		t.Errorf("stop_reason = %#v, want end_turn", complete.Data["stop_reason"])
	}
	if contextEvent == nil || contextEvent.ContextWindowUsed != 40_000 {
		t.Errorf("context event = %#v, want occupancy 40000 kept separate from billing", contextEvent)
	}

	delta, cost, present := a.consumeUsageDeltaWithPresence(sessionID)
	if delta != 0 || cost != 0 || present {
		t.Errorf("post-command delta = (%d, %d, %t), want (0, 0, false)", delta, cost, present)
	}
}
