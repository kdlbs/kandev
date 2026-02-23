package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type handlersTestBroadcaster struct {
	mu    sync.Mutex
	calls int
}

func (b *handlersTestBroadcaster) Broadcast(msg *ws.Message) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
}

func (b *handlersTestBroadcaster) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

type handlersTestAgent struct {
	id string

	blockListModels bool
	ctxDone         chan struct{}
}

func (a *handlersTestAgent) ID() string                             { return a.id }
func (a *handlersTestAgent) Name() string                           { return a.id }
func (a *handlersTestAgent) DisplayName() string                    { return a.id }
func (a *handlersTestAgent) Description() string                    { return "" }
func (a *handlersTestAgent) Enabled() bool                          { return true }
func (a *handlersTestAgent) DisplayOrder() int                      { return 0 }
func (a *handlersTestAgent) Logo(variant agents.LogoVariant) []byte { return nil }
func (a *handlersTestAgent) DefaultModel() string                   { return "" }
func (a *handlersTestAgent) BuildCommand(opts agents.CommandOptions) agents.Command {
	return agents.NewCommand()
}
func (a *handlersTestAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *handlersTestAgent) Runtime() *agents.RuntimeConfig { return nil }
func (a *handlersTestAgent) RemoteAuth() *agents.RemoteAuth { return nil }

func (a *handlersTestAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: true}, nil
}

func (a *handlersTestAgent) ListModels(ctx context.Context) (*agents.ModelList, error) {
	if !a.blockListModels {
		return &agents.ModelList{SupportsDynamic: false}, nil
	}
	<-ctx.Done()
	if a.ctxDone != nil {
		select {
		case <-a.ctxDone:
		default:
			close(a.ctxDone)
		}
	}
	return nil, ctx.Err()
}

func newHandlersTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func TestBroadcastAvailableAgentsAsync_UsesTimeoutAndDoesNotBroadcastOnError(t *testing.T) {
	log := newHandlersTestLogger(t)
	ag := &handlersTestAgent{id: "codex"}

	agentRegistry := registry.NewRegistry(log)
	if err := agentRegistry.Register(ag); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		t.Fatalf("load discovery registry: %v", err)
	}

	ctrl := controller.NewController(nil, discoveryRegistry, agentRegistry, nil, log)
	hub := &handlersTestBroadcaster{}
	h := NewHandlers(ctrl, hub, log)

	prevTimeout := availableAgentsBroadcastTimeout
	availableAgentsBroadcastTimeout = 20 * time.Millisecond
	t.Cleanup(func() { availableAgentsBroadcastTimeout = prevTimeout })

	ag.blockListModels = true
	ag.ctxDone = make(chan struct{})
	start := time.Now()
	h.broadcastAvailableAgentsAsync()
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Fatalf("broadcast call blocked for %v", elapsed)
	}

	select {
	case <-ag.ctxDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ListModels did not observe context timeout")
	}

	time.Sleep(20 * time.Millisecond)
	if got := hub.callCount(); got != 1 {
		t.Fatalf("broadcast calls = %d, want 1", got)
	}
}
