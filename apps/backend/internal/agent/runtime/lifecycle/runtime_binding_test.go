package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestRuntimeConsumerRebindUsesFreshCredential(t *testing.T) {
	var oldRequests, newRequests atomic.Int32
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer old-secret" {
			t.Errorf("old runtime Authorization = %q", got)
		}
		oldRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer new-secret" {
			t.Errorf("new runtime Authorization = %q", got)
		}
		newRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer newServer.Close()
	oldHost, oldPort := splitTestServerHostPort(t, oldServer)
	newHost, newPort := splitTestServerHostPort(t, newServer)

	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	t.Cleanup(owner.Stop)
	oldBinding, _ := owner.PrepareBinding()
	if err := oldBinding.Configure(oldHost, oldPort, "old-secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Commit(); err != nil {
		t.Fatal(err)
	}

	standalone := NewStandaloneExecutor(nil, "", 0, newTestLogger())
	standalone.SetRuntimeOwner(owner)
	if err := standalone.HealthCheck(context.Background()); err != nil {
		t.Fatalf("initial health: %v", err)
	}
	if !owner.MarkUnavailableEpoch(oldBinding.Epoch(), agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire old runtime")
	}
	newBinding, _ := owner.PrepareBinding()
	if err := newBinding.Configure(newHost, newPort, "new-secret", 22, nil); err != nil {
		t.Fatal(err)
	}
	if err := newBinding.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := standalone.HealthCheck(context.Background()); err != nil {
		t.Fatalf("successor health: %v", err)
	}
	if oldRequests.Load() != 1 || newRequests.Load() != 1 {
		t.Fatalf("old/new control requests = %d/%d, want one each", oldRequests.Load(), newRequests.Load())
	}
}

func TestRuntimeConsumerPrepareFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	host, port := splitTestServerHostPort(t, server)
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	t.Cleanup(owner.Stop)
	candidate, _ := owner.PrepareBinding()
	if err := candidate.Configure(host, port, "unpublished-secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	standalone := NewStandaloneExecutor(nil, "", 0, newTestLogger())
	standalone.SetRuntimeOwner(owner)
	if err := standalone.HealthCheck(context.Background()); !errors.Is(err, agentctl.ErrRuntimeUnavailable) {
		t.Fatalf("health while successor is only prepared = %v, want unavailable", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unpublished binding received %d requests", requests.Load())
	}
	if err := candidate.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredRuntimeEventsAndDisconnectsAreIgnored(t *testing.T) {
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	oldBinding, _ := owner.PrepareBinding()
	if err := oldBinding.Configure("127.0.0.1", 1, "old-secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := oldBinding.Commit(); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{runtimeOwner: owner, logger: newTestLogger()}
	execution := &AgentExecution{ID: "local-exec", SessionID: "session", runtimeEpoch: oldBinding.Epoch(), Status: "running"}
	if !owner.MarkUnavailableEpoch(oldBinding.Epoch(), agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire original runtime")
	}
	newBinding, _ := owner.PrepareBinding()
	if err := newBinding.Configure("127.0.0.1", 2, "new-secret", 22, nil); err != nil {
		t.Fatal(err)
	}
	if err := newBinding.Commit(); err != nil {
		t.Fatal(err)
	}

	manager.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "late output"})
	manager.handleStreamDisconnect(execution, errors.New("old runtime stream closed"), 1)
	if execution.Status != "running" || execution.messageBuffer.Len() != 0 {
		t.Fatalf("retired runtime changed execution status=%q buffered=%d", execution.Status, execution.messageBuffer.Len())
	}
	owner.Stop()
}

func TestRuntimeRebindPreservesRemoteExecution(t *testing.T) {
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot-one")
	candidate, _ := owner.PrepareBinding()
	if err := candidate.Configure("127.0.0.1", 1, "secret", 11, nil); err != nil {
		t.Fatal(err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{runtimeOwner: owner}
	local := &AgentExecution{runtimeEpoch: candidate.Epoch()}
	remote := &AgentExecution{runtimeEpoch: 0}
	if !manager.runtimeExecutionCurrent(local) || !manager.runtimeExecutionCurrent(remote) {
		t.Fatal("active local and remote executions should both be current")
	}
	if !owner.MarkUnavailableEpoch(candidate.Epoch(), agentctl.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire local runtime")
	}
	if manager.runtimeExecutionCurrent(local) {
		t.Fatal("retired local execution remained current")
	}
	if !manager.runtimeExecutionCurrent(remote) {
		t.Fatal("remote execution was fenced by local runtime retirement")
	}
	owner.Stop()
}

func splitTestServerHostPort(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split test server address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return host, port
}
