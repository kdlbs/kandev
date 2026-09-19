package utility

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/common/acpprovider"
	"go.uber.org/zap"
	"strings"
)

// gatewayProbeAgent is a probeCaptureAgent that also records the authenticate
// request and can be told to reject it.
type gatewayProbeAgent struct {
	probeCaptureAgent
	authMu   sync.Mutex
	authReq  acp.AuthenticateRequest
	authSeen bool
	authErr  error
}

func (a *gatewayProbeAgent) Authenticate(_ context.Context, req acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	a.authMu.Lock()
	a.authReq = req
	a.authSeen = true
	err := a.authErr
	a.authMu.Unlock()
	return acp.AuthenticateResponse{}, err
}

func (a *gatewayProbeAgent) authSnapshot() (acp.AuthenticateRequest, bool) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.authReq, a.authSeen
}

func gatewayPipes(t *testing.T, fake acp.Agent) (io.Writer, io.Reader) {
	t.Helper()
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	acp.NewAgentSideConnection(fake, a2cW, c2aR)
	return c2aW, a2cR
}

func TestProbeACPSession_AppliesProviderGatewayAuth(t *testing.T) {
	fake := &gatewayProbeAgent{}
	stdin, stdout := gatewayPipes(t, fake)
	gw := acpprovider.BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", "sk-router")

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := e.probeACPSessionWithContext(ctx, stdin, stdout, t.TempDir(), "codex-acp", "", "", nil, &gw); err != nil {
		t.Fatalf("probe: %v", err)
	}

	initReq := fake.captured()
	if initReq.ClientCapabilities.Auth.Meta["gateway"] != true {
		t.Errorf("init auth meta = %v, want gateway=true", initReq.ClientCapabilities.Auth.Meta)
	}
	authReq, seen := fake.authSnapshot()
	if !seen {
		t.Fatal("probe did not authenticate against the gateway")
	}
	gwMeta, _ := authReq.Meta["gateway"].(map[string]any)
	if hdr, _ := gwMeta["headers"].(map[string]any); hdr["Authorization"] != "Bearer sk-router" {
		t.Errorf("authenticate headers = %v", gwMeta["headers"])
	}
}

func TestExecuteACPSession_GatewayAuthFailureAborts(t *testing.T) {
	fake := &gatewayProbeAgent{authErr: errors.New("401 Unauthorized")}
	stdin, stdout := gatewayPipes(t, fake)
	gw := acpprovider.BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", "sk-bad")

	e := NewACPInferenceExecutor(zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := e.executeACPSession(ctx, stdin, stdout, t.TempDir(), "codex-acp", "hi", "", nil, "", nil, nil, &gw)
	if err == nil {
		t.Fatal("executeACPSession succeeded despite gateway auth failure")
	}
}

func TestWithUpstreamHint(t *testing.T) {
	gw := acpprovider.BuildGatewayAuth("gateway", "Kandev", "http://h/v1", "sk")
	base := errors.New("ACP session/new failed: peer disconnected before response")

	got := withUpstreamHint(base, &gw, "openai call failed: HTTP 401 Unauthorized\n  at /tmp/xyz/codex")
	if !strings.Contains(got, "401") || strings.Contains(got, "/tmp/xyz") {
		t.Errorf("hint = %q, want a sanitized 401 indication", got)
	}

	// No gateway: message is untouched.
	if got := withUpstreamHint(base, nil, "HTTP 500"); got != base.Error() {
		t.Errorf("non-gateway hint = %q, want unchanged", got)
	}

	// Gateway but nothing recognizable: message is untouched.
	if got := withUpstreamHint(base, &gw, "some noise without a status"); got != base.Error() {
		t.Errorf("unrecognized hint = %q, want unchanged", got)
	}
}
