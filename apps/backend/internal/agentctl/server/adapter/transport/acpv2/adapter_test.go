package acpv2

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

const fixtureSession = "native-session"

type peerFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params map[string]any  `json:"params"`
	Result json.RawMessage `json:"result"`
}

type wireFixture struct {
	t       *testing.T
	ctx     context.Context
	peer    net.Conn
	decoder *json.Decoder
	encoder *json.Encoder
	adapter *Adapter
}

func newWireFixture(t *testing.T) *wireFixture {
	t.Helper()
	adapter := NewAdapter(&shared.Config{AgentID: "claude-acp", WorkDir: t.TempDir()}, logger.Default())
	local, remote := net.Pipe()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(func() {
		cancel()
		_ = local.Close()
		_ = remote.Close()
		_ = adapter.Close()
	})
	deadline, _ := ctx.Deadline()
	require.NoError(t, remote.SetDeadline(deadline))
	require.NoError(t, adapter.Connect(local, local))
	return &wireFixture{t: t, ctx: ctx, peer: remote, decoder: json.NewDecoder(remote), encoder: json.NewEncoder(remote), adapter: adapter}
}

func (f *wireFixture) initialize() {
	f.t.Helper()
	done := asyncCall(func() error { return f.adapter.Initialize(f.ctx) })
	request := f.read("initialize")
	require.EqualValues(f.t, 2, request.Params["protocolVersion"])
	info := request.Params["info"].(map[string]any)
	require.Equal(f.t, "kandev", info["name"])
	f.reply(request, map[string]any{
		"protocolVersion": 2, "info": map[string]any{"name": "fixture", "version": "1"},
		"capabilities": map[string]any{"session": map[string]any{"mcp": map[string]any{"http": map[string]any{}}}},
	})
	require.NoError(f.t, f.wait(done))
	f.event(streams.EventTypeAgentCapabilities)
}

func (f *wireFixture) newSession(id string) {
	f.t.Helper()
	done := asyncCall(func() error {
		selected, err := f.adapter.NewSession(f.ctx, nil)
		if err == nil && selected != id {
			f.t.Errorf("selected session = %q, want %q", selected, id)
		}
		return err
	})
	f.reply(f.read("session/new"), map[string]any{"sessionId": id})
	require.NoError(f.t, f.wait(done))
	f.event(streams.EventTypeSessionStatus)
	f.event(streams.EventTypeSessionModels)
}

func (f *wireFixture) read(method string) peerFrame {
	f.t.Helper()
	var request peerFrame
	require.NoError(f.t, f.decoder.Decode(&request))
	require.Equal(f.t, method, request.Method, "unexpected request: %v", request)
	return request
}

func (f *wireFixture) send(fields map[string]any) {
	f.t.Helper()
	fields["jsonrpc"] = "2.0"
	require.NoError(f.t, f.encoder.Encode(fields))
}

func (f *wireFixture) reply(request peerFrame, result map[string]any) {
	f.t.Helper()
	require.NotEmpty(f.t, request.ID, "request ID must be present")
	f.send(map[string]any{"id": request.ID, "result": result})
}

func (f *wireFixture) update(session string, update map[string]any) {
	f.t.Helper()
	f.send(map[string]any{"method": "session/update", "params": map[string]any{"sessionId": session, "update": update}})
}

func (f *wireFixture) state(state, reason string) {
	f.t.Helper()
	update := map[string]any{"sessionUpdate": "state_update", "state": state}
	if reason != "" {
		update["stopReason"] = reason
	}
	if reason == "error" {
		update["error"] = map[string]any{"code": 10, "message": "failure fixture"}
	}
	f.update(fixtureSession, update)
}

func (f *wireFixture) chunk(session, id, text string) {
	f.t.Helper()
	f.update(session, map[string]any{
		"sessionUpdate": "agent_message_chunk", "messageId": id,
		"content": map[string]any{"type": "text", "text": text},
	})
}

func (f *wireFixture) event(kind string) streams.AgentEvent {
	f.t.Helper()
	select {
	case event := <-f.adapter.Updates():
		require.Equal(f.t, kind, event.Type)
		return event
	case <-f.ctx.Done():
		f.t.Fatalf("waiting for %s: %v", kind, f.ctx.Err())
		return streams.AgentEvent{}
	}
}

func (f *wireFixture) wait(done <-chan error) error {
	f.t.Helper()
	select {
	case err := <-done:
		return err
	case <-f.ctx.Done():
		f.t.Fatalf("operation did not finish: %v", f.ctx.Err())
		return f.ctx.Err()
	}
}

func (f *wireFixture) prompt() (<-chan error, peerFrame) {
	f.t.Helper()
	ctx := streams.WithPromptSubmissionID(f.ctx, "prompt:stable")
	done := asyncCall(func() error { return f.adapter.Prompt(ctx, "prompt text", nil, 27) })
	request := f.read("session/prompt")
	require.Equal(f.t, "prompt:stable", request.Params["idempotencyKey"])
	require.Equal(f.t, fixtureSession, request.Params["sessionId"])
	return done, request
}

func asyncCall(operation func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- operation() }()
	return done
}

func TestWirePromptWaitsForForegroundCompletion(t *testing.T) {
	for _, order := range []string{"ack-first", "running-first"} {
		t.Run(order, func(t *testing.T) {
			fixture := newWireFixture(t)
			fixture.initialize()
			fixture.newSession(fixtureSession)
			done, request := fixture.prompt()
			if order == "running-first" {
				fixture.state("running", "")
			}
			fixture.reply(request, map[string]any{"messageId": "user-message"})
			if order == "ack-first" {
				fixture.state("running", "")
			}
			fixture.update(fixtureSession, map[string]any{"sessionUpdate": "future_update", "unexpected": true})
			fixture.chunk("another-session", "reused-id", "unrelated")
			fixture.chunk(fixtureSession, "answer", "before idle")
			chunk := fixture.event(streams.EventTypeMessageChunk)
			require.Equal(t, "before idle", chunk.Text)
			require.Equal(t, "prompt:stable", chunk.DeliverySubmissionID)
			select {
			case err := <-done:
				t.Fatalf("prompt returned before idle: %v", err)
			default:
			}
			fixture.state("idle", "end_turn")
			outcome := fixture.event(streams.EventTypeComplete)
			require.Equal(t, uint64(27), outcome.PromptGeneration)
			require.Equal(t, "user-message", outcome.OperationID)
			require.NoError(t, fixture.wait(done))
			fixture.state("idle", "end_turn")
			fixture.chunk(fixtureSession, "later", "after turn")
			require.Equal(t, "after turn", fixture.event(streams.EventTypeMessageChunk).Text)
		})
	}
}

func TestWireCancellationAndAcceptedFailure(t *testing.T) {
	for _, reason := range []string{"cancelled", "error"} {
		t.Run(reason, func(t *testing.T) {
			fixture := newWireFixture(t)
			fixture.initialize()
			fixture.newSession(fixtureSession)
			done, request := fixture.prompt()
			fixture.reply(request, map[string]any{"messageId": "accepted"})
			fixture.state("running", "")
			if reason == "cancelled" {
				cancel := asyncCall(func() error { return fixture.adapter.Cancel(fixture.ctx) })
				note := fixture.read("session/cancel")
				require.Empty(t, note.ID)
				require.Equal(t, fixtureSession, note.Params["sessionId"])
				require.NoError(t, fixture.wait(cancel))
			}
			fixture.state("idle", reason)
			if reason == "error" {
				failure := fixture.event(streams.EventTypeError)
				require.Equal(t, streams.PromptFailureDispositionRetainRuntime, failure.PromptFailureDisposition)
				require.ErrorAs(t, fixture.wait(done), new(*streams.ReportedPromptFailure))
			} else {
				outcome := fixture.event(streams.EventTypeComplete)
				require.Equal(t, "cancelled", outcome.Data["stop_reason"])
				require.NoError(t, fixture.wait(done))
			}
		})
	}
}

func TestWireDisconnectBeforeAndAfterAcceptance(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "accepted"}[accepted], func(t *testing.T) {
			fixture := newWireFixture(t)
			fixture.initialize()
			fixture.newSession(fixtureSession)
			done, request := fixture.prompt()
			if accepted {
				fixture.reply(request, map[string]any{"messageId": "accepted"})
				fixture.chunk(fixtureSession, "answer", "partial")
				fixture.event(streams.EventTypeMessageChunk)
			}
			require.NoError(t, fixture.peer.Close())
			err := fixture.wait(done)
			if accepted {
				require.ErrorAs(t, err, new(*streams.ReportedPromptFailure))
				fixture.event(streams.EventTypeError)
			} else {
				require.ErrorContains(t, err, "acceptance is unknown")
			}
			require.ErrorContains(t, fixture.adapter.Prompt(fixture.ctx, "retry", nil, 28), "cannot accept")
		})
	}
}

func TestWireDisconnectSettlesPreviouslyReceivedTerminalState(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	fixture.newSession(fixtureSession)
	done, request := fixture.prompt()
	fixture.adapter.mu.Lock()
	started := fixture.adapter.turn.started
	fixture.adapter.mu.Unlock()
	fixture.reply(request, map[string]any{"messageId": "accepted"})
	select {
	case <-started:
	case <-fixture.ctx.Done():
		t.Fatal("prompt acknowledgement was not applied")
	}
	fixture.state("running", "")
	fixture.chunk(fixtureSession, "answer", "first delta")
	fixture.event(streams.EventTypeMessageChunk)
	func() {
		fixture.adapter.mu.Lock()
		defer fixture.adapter.mu.Unlock()
		fixture.chunk(fixtureSession, "answer", "last delta")
		fixture.state("idle", "end_turn")
		require.NoError(t, fixture.peer.Close())
		select {
		case <-fixture.adapter.client.Done():
		case <-fixture.ctx.Done():
			t.Fatal("transport did not disconnect")
		}
	}()
	require.Equal(t, "last delta", fixture.event(streams.EventTypeMessageChunk).Text)
	require.Equal(t, "end_turn", fixture.event(streams.EventTypeComplete).Data["stop_reason"])
	require.NoError(t, fixture.wait(done))
}

func TestWireRejectsMismatchedUserMessageInEitherOrder(t *testing.T) {
	for _, ackFirst := range []bool{false, true} {
		name := map[bool]string{false: "update-first", true: "ack-first"}[ackFirst]
		t.Run(name, func(t *testing.T) {
			fixture := newWireFixture(t)
			fixture.initialize()
			fixture.newSession(fixtureSession)
			done, request := fixture.prompt()
			fixture.adapter.mu.Lock()
			started := fixture.adapter.turn.started
			fixture.adapter.mu.Unlock()
			acknowledge := func() { fixture.reply(request, map[string]any{"messageId": "accepted"}) }
			if ackFirst {
				acknowledge()
				select {
				case <-started:
				case <-fixture.ctx.Done():
					t.Fatal("prompt acknowledgement was not applied")
				}
			}
			fixture.update(fixtureSession, map[string]any{
				"sessionUpdate": "user_message_chunk", "messageId": "unrelated",
				"content": map[string]any{"type": "text", "text": "different message"},
			})
			if !ackFirst {
				acknowledge()
			}
			require.Contains(t, fixture.event(streams.EventTypeError).Error, "different user message")
			require.ErrorContains(t, fixture.wait(done), "different user message")
		})
	}
}

func TestWireResumeFencesReplayAndPreservesIdentity(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	load := func() error { return fixture.adapter.LoadSession(fixture.ctx, fixtureSession, nil) }
	done := asyncCall(load)
	request := fixture.read("session/resume")
	require.Equal(t, fixtureSession, request.Params["sessionId"])
	require.Equal(t, map[string]any{"type": "start"}, request.Params["replayFrom"])
	fixture.chunk(fixtureSession, "old", "persisted text")
	fixture.reply(request, map[string]any{})
	select {
	case err := <-done:
		t.Fatalf("resume finished before replay marker: %v", err)
	default:
	}
	fixture.update(fixtureSession, map[string]any{"sessionUpdate": "replay_complete"})
	require.NoError(t, fixture.wait(done))
	require.Equal(t, streams.SessionStatusResumed, fixture.event(streams.EventTypeSessionStatus).SessionStatus)
	fixture.event(streams.EventTypeSessionModels)
	fixture.chunk(fixtureSession, "new", "live text")
	require.Equal(t, "live text", fixture.event(streams.EventTypeMessageChunk).Text)
	done = asyncCall(load)
	request = fixture.read("session/resume")
	fixture.send(map[string]any{"id": request.ID, "error": map[string]any{"code": -32602, "message": "not found"}})
	require.Error(t, fixture.wait(done))
	require.Equal(t, fixtureSession, fixture.adapter.GetSessionID())
}

func TestWireAdvertisedMCPAndTypedConfig(t *testing.T) {
	fixture := newWireFixture(t)
	fixture.initialize()
	ctx := streams.WithMCPAttachmentContext(fixture.ctx, streams.MCPAttachmentAttempt{AttemptID: "attachment"})
	done := asyncCall(func() error {
		_, err := fixture.adapter.NewSession(ctx, []types.McpServer{
			{Name: "sentinel", Type: "sse", URL: "http://localhost/sse"},
			{Name: "sentinel", Type: "streamable_http", URL: "http://localhost/mcp"},
		})
		return err
	})
	request := fixture.read("session/new")
	require.Equal(t, []any{map[string]any{"name": "sentinel", "type": "http", "url": "http://localhost/mcp"}}, request.Params["mcpServers"])
	fixture.reply(request, map[string]any{"sessionId": fixtureSession, "configOptions": fixtureOptions("ask")})
	require.NoError(t, fixture.wait(done))
	require.Equal(t, "attachment", fixture.event(streams.EventTypeMCPAttachment).MCPAttachmentAttempt.AttemptID)
	require.Equal(t, streams.MCPAttachmentEvidenceDelivered, fixture.event(streams.EventTypeMCPAttachment).MCPAttachment.Kind)
	require.Equal(t, streams.MCPAttachmentEvidenceSessionAccepted, fixture.event(streams.EventTypeMCPAttachment).MCPAttachment.Kind)
	fixture.event(streams.EventTypeSessionStatus)
	setup := fixture.event(streams.EventTypeSessionModels)
	require.Equal(t, streams.ConfigOptionsSourceSessionSetup, setup.Data[streams.ConfigOptionsSourceKey])
	require.Equal(t, "ask", fixture.event(streams.EventTypeSessionMode).CurrentModeID)
	done = asyncCall(func() error {
		_, err := fixture.adapter.SetMode(fixture.ctx, "plan")
		return err
	})
	request = fixture.read("session/set_config_option")
	require.Equal(t, "provider-mode", request.Params["configId"])
	require.Equal(t, "id", request.Params["type"])
	require.Equal(t, "plan", request.Params["value"])
	fixture.reply(request, map[string]any{"configOptions": fixtureOptions("plan")})
	require.NoError(t, fixture.wait(done))
	confirmed := fixture.event(streams.EventTypeSessionModels)
	require.Equal(t, "provider_response", confirmed.Data[streams.ConfigOptionsSourceKey])
	require.Equal(t, "provider-mode", confirmed.Data["config_options_config_id"])
	require.Equal(t, "plan", fixture.event(streams.EventTypeSessionMode).CurrentModeID)
}

func fixtureOptions(current string) []map[string]any {
	return []map[string]any{{
		"configId": "provider-mode", "name": "Mode", "category": "mode", "type": "select", "currentValue": current,
		"options": []map[string]any{{"value": "ask", "name": "Ask"}, {"value": "plan", "name": "Plan"}},
	}}
}
