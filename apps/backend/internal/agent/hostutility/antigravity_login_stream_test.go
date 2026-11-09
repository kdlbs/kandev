package hostutility

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type antigravityLoginStreamFixture struct {
	manager          *Manager
	inst             *instance
	agent            *antigravityLoginCommandAgent
	handshakeStarted chan struct{}
	releaseHandshake chan struct{}
	streamClosed     chan struct{}
	requests         chan ws.Message
	openOnce         sync.Once
	mode             string
}

func newAntigravityLoginStreamFixture(t *testing.T, mode string) *antigravityLoginStreamFixture {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f := &antigravityLoginStreamFixture{
		manager:          NewManager(nil, "", 0, nil, log),
		agent:            &antigravityLoginCommandAgent{AntigravityACP: agents.NewAntigravityACP(), command: agents.NewCommand("/fixtures/agy_acp_server.par")},
		handshakeStarted: make(chan struct{}),
		releaseHandshake: make(chan struct{}),
		streamClosed:     make(chan struct{}),
		requests:         make(chan ws.Message, 4),
		mode:             mode,
	}
	configure := newAntigravityLoginConfigureFixture(t, log)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/start":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true}`)
		case "/api/v1/agent/stream":
			f.serveStream(t, w, r)
		default:
			configure.handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.inst = &instance{client: newAntigravityLoginFixtureClient(t, server, log)}
	t.Cleanup(func() {
		f.inst.client.Close()
		f.open()
		select {
		case <-f.handshakeStarted:
			awaitAntigravityLoginResult(t, f.streamClosed)
		default:
		}
	})
	return f
}

func (f *antigravityLoginStreamFixture) open() {
	f.openOnce.Do(func() { close(f.releaseHandshake) })
}

func (f *antigravityLoginStreamFixture) serveStream(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	close(f.handshakeStarted)
	defer close(f.streamClosed)
	select {
	case <-f.releaseHandshake:
	case <-r.Context().Done():
		return
	}
	if f.mode == "reject" {
		http.Error(w, "stream unavailable", http.StatusServiceUnavailable)
		return
	}
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		t.Errorf("upgrade login stream: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		var request ws.Message
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		f.requests <- request
		if f.mode == "disconnect" && request.Action == "agent.initialize" {
			return
		}
		response, err := ws.NewResponse(request.ID, request.Action, map[string]bool{"success": true})
		if err != nil {
			t.Errorf("encode login response: %v", err)
			return
		}
		if err := conn.WriteJSON(response); err != nil {
			t.Errorf("write login response: %v", err)
			return
		}
	}
}

func (f *antigravityLoginStreamFixture) configure(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- f.manager.configureAntigravityLoginInstance(ctx, f.inst, f.agent, &antigravityLoginAttempt{})
	}()
	return done
}

func awaitAntigravityLoginResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for login fixture")
		var zero T
		return zero
	}
}

// @covers AC-AGENTS-ANTIGRAVITY-ACP-010.4, AC-AGENTS-ANTIGRAVITY-ACP-015.1
func TestAntigravityLoginStreamStartup(t *testing.T) {
	f := newAntigravityLoginStreamFixture(t, "ready")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	configured := f.configure(ctx)
	awaitAntigravityLoginResult(t, f.handshakeStarted)
	f.open()
	if err := awaitAntigravityLoginResult(t, configured); err != nil {
		t.Fatalf("configure healthy login stream: %v", err)
	}
	if err := f.manager.authenticateAntigravityLogin(ctx, f.inst, &antigravityLoginAttempt{}); err != nil {
		t.Fatalf("authenticate over login stream: %v", err)
	}
	initialize := awaitAntigravityLoginResult(t, f.requests)
	authenticate := awaitAntigravityLoginResult(t, f.requests)
	if initialize.Action != "agent.initialize" || authenticate.Action != "agent.session.authenticate" {
		t.Fatalf("login actions = %q, %q", initialize.Action, authenticate.Action)
	}
	var auth struct {
		MethodID string `json:"method_id"`
	}
	if err := authenticate.ParsePayload(&auth); err != nil {
		t.Fatal(err)
	}
	if auth.MethodID != acpcompat.AntigravityPersonalAuthMethod {
		t.Fatalf("authentication method = %q", auth.MethodID)
	}
	if !f.inst.client.HasAgentStream() {
		t.Fatal("healthy login stream was closed before cleanup")
	}
	f.inst.client.Close()
	awaitAntigravityLoginResult(t, f.streamClosed)
}

// @covers AC-AGENTS-ANTIGRAVITY-ACP-010.5
func TestAntigravityLoginStreamFailure(t *testing.T) {
	for _, mode := range []string{"reject", "disconnect", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newAntigravityLoginStreamFixture(t, mode)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			configured := f.configure(ctx)
			awaitAntigravityLoginResult(t, f.handshakeStarted)
			if mode == "cancel" {
				cancel()
			} else {
				f.open()
			}
			err := awaitAntigravityLoginResult(t, configured)
			assertAntigravityLoginStreamFailure(t, ctx, mode, err)
			awaitAntigravityLoginResult(t, f.streamClosed)
			if f.inst.client.HasAgentStream() {
				t.Fatal("failed login retained a connected stream")
			}
			if mode == "disconnect" {
				request := awaitAntigravityLoginResult(t, f.requests)
				if request.Action != "agent.initialize" {
					t.Fatalf("request before disconnect = %q", request.Action)
				}
			}
			select {
			case request := <-f.requests:
				t.Fatalf("unexpected request after failure: %q", request.Action)
			default:
			}
		})
	}
}

func assertAntigravityLoginStreamFailure(t *testing.T, ctx context.Context, mode string, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("failed stream setup reported success")
	}
	switch mode {
	case "reject":
		if !strings.Contains(err.Error(), "open antigravity login stream:") || !strings.Contains(err.Error(), "bad handshake") {
			t.Fatalf("stream rejection error = %v", err)
		}
	case "disconnect":
		if !strings.Contains(err.Error(), "initialize antigravity login instance:") {
			t.Fatalf("initialize disconnect error = %v", err)
		}
	case "cancel":
		attempt := &antigravityLoginAttempt{state: "starting"}
		finishLoginError(attempt, ctx, "setup_failed", err)
		if snapshot := attempt.snapshot(); snapshot.State != "cancelled" || snapshot.ErrorCode != "cancelled" {
			t.Fatalf("stream cancellation snapshot = %+v (setup error: %v)", snapshot, err)
		}
	}
}
