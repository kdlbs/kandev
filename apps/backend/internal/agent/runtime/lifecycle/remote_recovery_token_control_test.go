package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
)

func spritesBackendManager(t *testing.T) (*Manager, *MockExecutor) {
	t.Helper()
	log := newTestLogger()
	backend := &MockExecutor{name: executor.NameSprites}
	reg := NewExecutorRegistry(log)
	reg.Register(backend)
	return &Manager{logger: log, executorRegistry: reg}, backend
}

// Without a control handle the runtime-level stop cannot honour a token
// precondition, so a conditional delete is refused and nothing is destroyed.
func TestRemoteTokenControlWithoutHandleRefusesConditionalDelete(t *testing.T) {
	m, backend := spritesBackendManager(t)
	ri := &ExecutorInstance{InstanceID: "exec-1", RuntimeName: agentruntime.RuntimeSprites}
	control := remoteTokenControl{m: m, ri: ri}

	if err := control.DeleteInstanceMatching(context.Background(), "exec-1", "observed"); err == nil {
		t.Fatal("conditional delete without a control handle succeeded")
	}
	if len(backend.stopInstanceCalls) != 0 {
		t.Fatalf("runtime stop called %d times for a conditional delete", len(backend.stopInstanceCalls))
	}
	if err := control.DeleteInstanceMatching(context.Background(), "exec-1", ""); err != nil {
		t.Fatalf("unconditional delete: %v", err)
	}
	if len(backend.stopInstanceCalls) != 1 {
		t.Fatalf("runtime stop calls = %d, want 1 for an unconditional delete", len(backend.stopInstanceCalls))
	}
}

type staticEndpoint struct {
	host string
	port int
}

func (e staticEndpoint) Resolve(context.Context, string, int, string) (string, int, error) {
	return e.host, e.port, nil
}
func (staticEndpoint) Close() error { return nil }

func TestRemoteDockerTokenControlReachesTheControlServer(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" fp="+r.Header.Get("X-Expected-Token-Fingerprint")+" auth="+r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	control := remoteDockerTokenControl{
		logger: newTestLogger(), endpoints: staticEndpoint{host, port}, containerID: "c1", authToken: "ctl-token",
	}
	if err := control.DeleteInstanceMatching(context.Background(), "exec-1", "observed"); err != nil {
		t.Fatal(err)
	}
	if err := control.RevokeMCPToken(context.Background(), "exec-1"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 ||
		seen[0] != "DELETE /api/v1/instances/exec-1 fp=observed auth=Bearer ctl-token" ||
		seen[1] != "POST /api/v1/instances/exec-1/mcp-token/revoke fp= auth=Bearer ctl-token" {
		t.Fatalf("control requests = %q", seen)
	}
}
