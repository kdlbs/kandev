package acp

import (
	"context"
	"io"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type setModeHandler func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error)

type setModeTestAgent struct {
	*sessionRequestCaptureAgent
	connection *acpsdk.AgentSideConnection
	handler    setModeHandler
}

func (a *setModeTestAgent) SetSessionMode(ctx context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return a.handler(ctx, request)
}

func newSetModeTestAdapter(t *testing.T, handler setModeHandler) (*Adapter, *setModeTestAgent, chan string) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	adapter := newTestAdapter()
	processed := make(chan string, 8)
	client := acpclient.NewClient(acpclient.WithUpdateHandler(func(notification acpsdk.SessionNotification) {
		if notification.Update.CurrentModeUpdate == nil {
			return
		}
		if event := adapter.convertNotification(notification); event != nil {
			adapter.sendUpdate(*event)
		}
		processed <- string(notification.Update.CurrentModeUpdate.CurrentModeId)
	}))
	connection := acpsdk.NewClientSideConnection(client, clientToAgentWriter, agentToClientReader)
	agent := &setModeTestAgent{
		sessionRequestCaptureAgent: &sessionRequestCaptureAgent{},
		handler:                    handler,
	}
	agent.connection = acpsdk.NewAgentSideConnection(agent, agentToClientWriter, clientToAgentReader)
	adapter.acpConn = connection
	adapter.acpClient = client
	adapter.sessionID = "session-1"
	adapter.noteCurrentMode("session-1", "default")
	t.Cleanup(func() {
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})
	return adapter, agent, processed
}

func reportModeFromAgent(agent *setModeTestAgent, mode string) error {
	return agent.connection.SessionUpdate(context.Background(), acpsdk.SessionNotification{
		SessionId: "session-1",
		Update: acpsdk.SessionUpdate{
			CurrentModeUpdate: &acpsdk.SessionCurrentModeUpdate{
				SessionUpdate: "current_mode_update",
				CurrentModeId: acpsdk.SessionModeId(mode),
			},
		},
	})
}

func TestSetModeUsesReportArrivingBeforeRPCResponse(t *testing.T) {
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		if err := reportModeFromAgent(agent, "default"); err != nil {
			return acpsdk.SetSessionModeResponse{}, err
		}
		select {
		case <-processed:
		case <-ctx.Done():
			return acpsdk.SetSessionModeResponse{}, ctx.Err()
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})

	type result struct {
		modeResult streams.ModeResult
		err        error
	}
	resultCh := make(chan result, 1)
	go func() {
		modeResult, err := adapter.SetMode(context.Background(), "bypassPermissions")
		resultCh <- result{modeResult: modeResult, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("SetMode: %v", result.err)
		}
		if !result.modeResult.Confirmed || result.modeResult.Effective != "default" {
			t.Fatalf("SetMode result = %+v, want the early clamp report", result.modeResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetMode did not finish after the early report")
	}

	events := drainEvents(adapter)
	if len(events) < 2 {
		t.Fatalf("mode update events = %+v, want provider and SetMode events", events)
	}
	last := events[len(events)-1]
	if last.CurrentModeID != "default" || last.RequestedModeID != "bypassPermissions" {
		t.Fatalf("SetMode event = %+v, want the reported clamp and request", last)
	}
}

func TestConcurrentSetModeRequestsCannotShareAReport(t *testing.T) {
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{}, 1)
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, request acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		switch string(request.ModeId) {
		case "plan":
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
			if err := reportModeFromAgent(agent, "plan"); err != nil {
				return acpsdk.SetSessionModeResponse{}, err
			}
			select {
			case <-processed:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
		case "bypassPermissions":
			secondEntered <- struct{}{}
		default:
			return acpsdk.SetSessionModeResponse{}, context.Canceled
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})

	type result struct {
		mode streams.ModeResult
		err  error
	}
	firstResult := make(chan result, 1)
	go func() {
		mode, err := adapter.SetMode(context.Background(), "plan")
		firstResult <- result{mode: mode, err: err}
	}()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first SetMode request did not reach the agent")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.SetMode(canceled, "bypassPermissions"); err == nil {
		t.Fatal("concurrent canceled SetMode request unexpectedly proceeded")
	}
	select {
	case <-secondEntered:
		t.Fatal("concurrent request reached the agent while the first mode change was active")
	default:
	}

	close(releaseFirst)
	select {
	case got := <-firstResult:
		if got.err != nil || !got.mode.Confirmed || got.mode.Effective != "plan" {
			t.Fatalf("first SetMode result = %+v, error = %v", got.mode, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first SetMode request did not settle")
	}

	second, err := adapter.SetMode(context.Background(), "bypassPermissions")
	if err != nil {
		t.Fatalf("second SetMode: %v", err)
	}
	select {
	case <-secondEntered:
	default:
		t.Fatal("second SetMode request did not reach the agent")
	}
	if second.Confirmed || second.Effective != "" {
		t.Fatalf("second SetMode result = %+v, want no confirmation from the first request's report", second)
	}
}
