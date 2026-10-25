package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

// tokenCreateExecutor records the create request and serves an /identity that
// reports the fingerprint of the token the request carried.
type tokenCreateExecutor struct {
	MockExecutor
	req    atomic.Pointer[ExecutorCreateRequest]
	client *agentctl.Client
}

func newTokenCreateExecutor(t *testing.T) *tokenCreateExecutor {
	t.Helper()
	e := &tokenCreateExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/identity":
			req := e.req.Load()
			fp := ""
			if req != nil {
				fp = sessionmcp.Fingerprint(req.SessionMCPToken)
			}
			_, _ = w.Write([]byte(`{"capabilities":["session_mcp_token"],"mcp_token_fingerprint":"` + fp + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	e.client = agentctl.NewClient(host, port, newTestLogger())
	return e
}

func (e *tokenCreateExecutor) CreateInstance(_ context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	e.req.Store(req)
	return &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: e.Name(), Client: e.client, WorkspacePath: req.WorkspacePath,
	}, nil
}

// A workspace-only instance can later be promoted to an agent instance, so it
// must be created holding a minted token and persisted nonce.
func TestWorkspaceOnlyExecutionIsCreatedWithSessionToken(t *testing.T) {
	log := newTestLogger()
	backend := newTokenCreateExecutor(t)
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log,
	)
	mgr.secretStore = &keyedStore{key: launchKey()}
	cleanupManagerStopCh(t, mgr)

	execution, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID: "session-1", AgentID: "auggie", WorkspacePath: "/workspace/task-1",
	})
	if err != nil {
		t.Fatalf("createExecution: %v", err)
	}
	req := backend.req.Load()
	if req == nil || req.SessionMCPToken == "" || req.SessionMCPNonce == "" {
		t.Fatalf("workspace-only create carried no token/nonce: %+v", req)
	}
	if got := execution.MetadataSnapshot()[MetadataKeyMCPTokenNonce]; got != req.SessionMCPNonce {
		t.Fatalf("execution nonce metadata = %v, want %q", got, req.SessionMCPNonce)
	}
}
