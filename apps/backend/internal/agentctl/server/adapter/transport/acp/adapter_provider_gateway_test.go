package acp

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/acpprovider"
	"github.com/kandev/kandev/internal/common/logger"
)

// gatewayCaptureAgent records the initialize and authenticate requests it was
// handed, and can be told to reject authenticate.
type gatewayCaptureAgent struct {
	burstAgent
	mu       sync.Mutex
	initReq  acp.InitializeRequest
	authReq  acp.AuthenticateRequest
	authSeen bool
	authErr  error
}

func (a *gatewayCaptureAgent) Initialize(_ context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	a.mu.Lock()
	a.initReq = req
	a.mu.Unlock()
	return acp.InitializeResponse{ProtocolVersion: req.ProtocolVersion}, nil
}

func (a *gatewayCaptureAgent) Authenticate(_ context.Context, req acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	a.mu.Lock()
	a.authReq = req
	a.authSeen = true
	err := a.authErr
	a.mu.Unlock()
	return acp.AuthenticateResponse{}, err
}

func (a *gatewayCaptureAgent) snapshot() (acp.InitializeRequest, acp.AuthenticateRequest, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initReq, a.authReq, a.authSeen
}

func runGatewayHandshake(t *testing.T, gw *acpprovider.GatewayAuth, authErr error) (*gatewayCaptureAgent, error) {
	t.Helper()

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	a := NewAdapter(&shared.Config{AgentID: codexAgentID, WorkDir: "/tmp/test", ProviderGatewayAuth: gw}, log)

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
	fake := &gatewayCaptureAgent{authErr: authErr}
	acp.NewAgentSideConnection(fake, a2cW, c2aR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.Initialize(ctx)
	if err == nil {
		drainEvents(a)
	}
	return fake, err
}

func TestInitialize_SendsGatewayAuthenticateAfterInitialize(t *testing.T) {
	gw := acpprovider.BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", "sk-router")
	fake, err := runGatewayHandshake(t, &gw, nil)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	initReq, authReq, seen := fake.snapshot()
	if !seen {
		t.Fatal("agent never received authenticate")
	}
	if initReq.ClientCapabilities.Auth.Meta["gateway"] != true {
		t.Errorf("initialize auth meta = %v, want gateway=true", initReq.ClientCapabilities.Auth.Meta)
	}
	if string(authReq.MethodId) != "gateway" {
		t.Errorf("authenticate methodId = %q, want gateway", authReq.MethodId)
	}
	gwMeta, ok := authReq.Meta["gateway"].(map[string]any)
	if !ok {
		t.Fatalf("authenticate meta.gateway = %v", authReq.Meta["gateway"])
	}
	if gwMeta["baseUrl"] != "http://localhost:20128/v1" {
		t.Errorf("baseUrl = %v", gwMeta["baseUrl"])
	}
	if hdr, _ := gwMeta["headers"].(map[string]any); hdr["Authorization"] != "Bearer sk-router" {
		t.Errorf("headers = %v", gwMeta["headers"])
	}
}

func TestInitialize_GatewayAuthFailureAbortsConnection(t *testing.T) {
	gw := acpprovider.BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", "sk-router")
	_, err := runGatewayHandshake(t, &gw, errors.New("bad key"))
	if err == nil {
		t.Fatal("Initialize succeeded despite gateway auth failure")
	}
}

func TestInitialize_NoGatewayAuthLeavesHandshakePlain(t *testing.T) {
	fake, err := runGatewayHandshake(t, nil, nil)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	initReq, _, seen := fake.snapshot()
	if seen {
		t.Error("authenticate sent without provider gateway auth")
	}
	if initReq.ClientCapabilities.Auth.Meta != nil {
		t.Errorf("auth meta = %v, want nil", initReq.ClientCapabilities.Auth.Meta)
	}
}
