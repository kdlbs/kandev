package websocket

import (
	"fmt"
	"net"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

func TestResolveAndBindReportsOccupiedPort(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	lifecycleMgr := lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
	t.Cleanup(func() { _ = lifecycleMgr.Stop() })

	execution := &lifecycle.AgentExecution{ID: "execution", SessionID: "session"}
	execution.SetAgentCtlClientForTesting(agentctl.NewClient("127.0.0.1", 1, log))
	if err := lifecycleMgr.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	occupied, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen for occupied tunnel port: %v", err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	port := occupied.Addr().(*net.TCPAddr).Port

	mgr := NewTunnelManager(lifecycleMgr, log)
	_, _, _, err = mgr.resolveAndBind("session", port)
	if err == nil {
		t.Fatal("resolveAndBind() = nil, want occupied-port error")
	}
	want := fmt.Sprintf("port %d is already in use", port)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}
