package acpv2

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type promptTurn struct {
	foregroundTurn
	session      wire.SessionID
	submissionID string
	generation   uint64
	ctx          context.Context
	cancel       context.CancelFunc
	started      chan struct{}
	done         chan struct{}
	result       error
}

func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment, generation uint64) error {
	turn, blocks, err := a.beginPrompt(ctx, message, attachments, generation)
	if err != nil {
		return err
	}
	defer turn.cancel()
	key := turn.submissionID
	if key == "" {
		key = rand.Text()
	}
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.cfg.AgentID, "prompt")
	defer span.End()
	reply, err := a.client.Prompt(ctx, wire.PromptRequest{
		SessionID: turn.session, Prompt: blocks, IdempotencyKey: &key,
	})
	if err != nil {
		return a.rejectPrompt(turn, err)
	}
	a.project(func(batch *eventBatch) {
		turn.acknowledge(string(reply.MessageID))
		a.signalTurnStarted(turn)
		a.settleTurn(turn, batch)
	})
	select {
	case <-turn.done:
		return turn.result
	case <-a.client.NotificationsDone():
		return a.loseAcceptedTurn(turn, "ACP v2 agent disconnected after accepting the prompt")
	case <-ctx.Done():
		return a.loseAcceptedTurn(turn, "ACP v2 stopped waiting for an accepted prompt")
	case <-a.ctx.Done():
		return a.loseAcceptedTurn(turn, "ACP v2 process stopped after accepting the prompt")
	}
}

func (a *Adapter) beginPrompt(
	ctx context.Context, message string, attachments []v1.MessageAttachment, generation uint64,
) (*promptTurn, []wire.ContentBlock, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer a.release()
	a.mu.Lock()
	session := a.session
	if session == "" || a.blocked || a.turn != nil {
		a.mu.Unlock()
		return nil, nil, errors.New("ACP v2 session cannot accept a prompt")
	}
	turnCtx, cancel := context.WithCancel(a.ctx)
	turn := &promptTurn{
		session: session, submissionID: streams.PromptSubmissionID(ctx), generation: generation,
		ctx: turnCtx, cancel: cancel, started: make(chan struct{}), done: make(chan struct{}),
	}
	a.mu.Unlock()
	blocks, err := a.promptContent(session, message, attachments)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	a.mu.Lock()
	a.turn = turn
	a.mu.Unlock()
	return turn, blocks, nil
}

func (a *Adapter) rejectPrompt(turn *promptTurn, err error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.turn = nil
	close(turn.done)
	var rpcErr *acp.RequestError
	if errors.As(err, &rpcErr) && rpcErr.IsRemoteResponse() {
		return safeRPCError("prompt", err)
	}
	a.blocked = true
	return errors.New("ACP v2 prompt acceptance is unknown")
}

func (a *Adapter) signalTurnStarted(turn *promptTurn) {
	select {
	case <-turn.started:
	default:
		close(turn.started)
	}
}

func (a *Adapter) loseAcceptedTurn(turn *promptTurn, message string) error {
	a.project(func(batch *eventBatch) {
		if a.turn == turn {
			a.blocked = true
			finishTurn(turn, string(wire.StopReasonError), message, false, batch)
		}
	})
	return turn.result
}

func (a *Adapter) settleTurn(turn *promptTurn, batch *eventBatch) {
	if turn.userMessage != "" && turn.messageID != "" && turn.userMessage != turn.messageID {
		a.blocked = true
		finishTurn(turn, string(wire.StopReasonError), "ACP v2 acknowledged a different user message", false, batch)
		return
	}
	if !turn.settled() {
		return
	}
	switch wire.StopReason(turn.stopReason) {
	case wire.StopReasonError:
		finishTurn(turn, turn.stopReason, "ACP v2 agent reported a foreground error", true, batch)
	case wire.StopReasonEndTurn, wire.StopReasonCancelled, wire.StopReasonRefusal, wire.StopReasonMaxTokens, wire.StopReasonMaxTurnRequests:
		finishTurn(turn, turn.stopReason, "", false, batch)
	default:
		finishTurn(turn, "unknown", "ACP v2 foreground stopped with an unknown outcome", true, batch)
	}
}

// finishTurn ends callbacks while keeping the turn reserved until its terminal event is enqueued.
func finishTurn(turn *promptTurn, reason, message string, retain bool, batch *eventBatch) {
	event := streams.AgentEvent{
		Type: streams.EventTypeComplete, SessionID: string(turn.session), OperationID: turn.messageID,
		PromptGeneration: turn.generation, DeliverySubmissionID: turn.submissionID,
		Data: map[string]any{"stop_reason": reason},
	}
	if message != "" {
		event.Type, event.Error = streams.EventTypeError, message
		if retain {
			event.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
		}
		turn.result = &streams.ReportedPromptFailure{Message: message}
	}
	batch.add(event)
	batch.settled = turn
	turn.terminal = true
	turn.cancel()
}

func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.Lock()
	turn := a.turn
	a.mu.Unlock()
	if turn == nil {
		return nil
	}
	select {
	case <-turn.started:
	case <-turn.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
	if err := a.acquire(ctx); err != nil {
		return err
	}
	defer a.release()
	a.mu.Lock()
	current := a.turn == turn && !turn.terminal
	a.mu.Unlock()
	if !current {
		return nil
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return safeRPCError("cancel", a.client.Cancel(cancelCtx, wire.CancelSessionNotification{SessionID: turn.session}))
}

func (a *Adapter) promptContent(session wire.SessionID, message string, attachments []v1.MessageAttachment) ([]wire.ContentBlock, error) {
	blocks := []wire.ContentBlock{wire.TextBlock(message)}
	if len(attachments) == 0 {
		return blocks, nil
	}
	manager := shared.NewAttachmentManager(a.cfg.WorkDir, a.log.Zap())
	manager.SetSessionID(string(session))
	for _, attachment := range attachments {
		caps := a.promptCapabilities()
		nativeImage := caps != nil && caps.Image != nil && attachment.Type == "image"
		if nativeImage && attachment.Data != "" && attachment.DeliveryMode != "path" {
			blocks = append(blocks, wire.ContentBlock{Type: "image", Data: &attachment.Data, MIMEType: &attachment.MimeType})
			continue
		}
		saved, err := manager.SaveAttachments([]v1.MessageAttachment{attachment})
		if err != nil || len(saved) == 0 {
			return nil, errors.New("ACP v2 could not deliver a prompt attachment")
		}
		blocks = append(blocks, wire.TextBlock(shared.BuildAttachmentPrompt(saved, attachment.DeliveryMode == "path")))
	}
	return blocks, nil
}

func (a *Adapter) promptCapabilities() *wire.PromptCapabilities {
	if a.info.Capabilities == nil || a.info.Capabilities.Session == nil {
		return nil
	}
	return a.info.Capabilities.Session.Prompt
}
