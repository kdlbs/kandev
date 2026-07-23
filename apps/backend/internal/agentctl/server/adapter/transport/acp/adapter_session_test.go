package acp

import (
	"context"
	"io"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
)

type sessionRequestCaptureAgent struct {
	newRequest  acpsdk.NewSessionRequest
	loadRequest acpsdk.LoadSessionRequest
}

var (
	_ acpsdk.Agent       = (*sessionRequestCaptureAgent)(nil)
	_ acpsdk.AgentLoader = (*sessionRequestCaptureAgent)(nil)
)

func (*sessionRequestCaptureAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*sessionRequestCaptureAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{}, nil
}

func (*sessionRequestCaptureAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}

func (*sessionRequestCaptureAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (*sessionRequestCaptureAgent) CloseSession(context.Context, acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	return acpsdk.CloseSessionResponse{}, nil
}

func (*sessionRequestCaptureAgent) DeleteSession(context.Context, acpsdk.DeleteSessionRequest) (acpsdk.DeleteSessionResponse, error) {
	return acpsdk.DeleteSessionResponse{}, nil
}

func (*sessionRequestCaptureAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}

func (a *sessionRequestCaptureAgent) NewSession(_ context.Context, request acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	a.newRequest = request
	return acpsdk.NewSessionResponse{SessionId: "session-1"}, nil
}

func (*sessionRequestCaptureAgent) Prompt(context.Context, acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*sessionRequestCaptureAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}

func (*sessionRequestCaptureAgent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}

func (*sessionRequestCaptureAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func (a *sessionRequestCaptureAgent) LoadSession(_ context.Context, request acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	a.loadRequest = request
	return acpsdk.LoadSessionResponse{}, nil
}

func TestMCPSessionNewAndLoadUseHTTPWithSSEFallback(t *testing.T) {
	tests := []struct {
		name         string
		capabilities acpsdk.McpCapabilities
		wantType     string
	}{
		{name: "HTTP preferred", capabilities: acpsdk.McpCapabilities{Http: true, Sse: true}, wantType: "http"},
		{name: "SSE fallback", capabilities: acpsdk.McpCapabilities{Sse: true}, wantType: "sse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, capture := newSessionRequestCaptureAdapter(t, tt.capabilities)
			servers := []types.McpServer{
				{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
				{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
			}

			if _, err := adapter.NewSession(context.Background(), servers); err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			assertCapturedKandevTransport(t, capture.newRequest.McpServers, tt.wantType)

			if err := adapter.LoadSession(context.Background(), "session-1", servers); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			assertCapturedKandevTransport(t, capture.loadRequest.McpServers, tt.wantType)
		})
	}
}

func newSessionRequestCaptureAdapter(t *testing.T, capabilities acpsdk.McpCapabilities) (*Adapter, *sessionRequestCaptureAgent) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	capture := &sessionRequestCaptureAgent{}
	clientConnection := acpsdk.NewClientSideConnection(acpclient.NewClient(), clientToAgentWriter, agentToClientReader)
	_ = acpsdk.NewAgentSideConnection(capture, agentToClientWriter, clientToAgentReader)

	t.Cleanup(func() {
		_ = clientToAgentWriter.Close()
		_ = clientToAgentReader.Close()
		_ = agentToClientWriter.Close()
		_ = agentToClientReader.Close()
	})

	adapter := newTestAdapter()
	t.Cleanup(func() { _ = adapter.Close() })
	adapter.acpConn = clientConnection
	adapter.capabilities = acpsdk.AgentCapabilities{
		LoadSession:     true,
		McpCapabilities: capabilities,
	}
	return adapter, capture
}

func assertCapturedKandevTransport(t *testing.T, servers []acpsdk.McpServer, wantType string) {
	t.Helper()
	if len(servers) != 1 {
		t.Fatalf("captured MCP servers = %+v, want one deduplicated kandev server", servers)
	}
	switch wantType {
	case "http":
		if servers[0].Http == nil || servers[0].Http.Name != "kandev" || servers[0].Http.Url != "http://localhost:10005/mcp" {
			t.Fatalf("captured MCP server = %+v, want kandev HTTP", servers[0])
		}
	case "sse":
		if servers[0].Sse == nil || servers[0].Sse.Name != "kandev" || servers[0].Sse.Url != "http://localhost:10005/sse" {
			t.Fatalf("captured MCP server = %+v, want kandev SSE", servers[0])
		}
	default:
		t.Fatalf("unknown transport %q", wantType)
	}
}
