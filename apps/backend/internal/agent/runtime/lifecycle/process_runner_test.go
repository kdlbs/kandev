package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func processTestClient(t *testing.T, handler http.Handler) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return agentctl.NewClient(host, port, log)
}

func TestProcessRunnerStartErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}

	if _, err := manager.StartProcess(context.Background(), StartProcessRequest{
		SessionID: "missing",
		Kind:      "dev",
		Command:   "echo hi",
	}); err == nil {
		t.Fatal("expected error when no execution exists for session")
	}

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	manager.executionStore.Add(exec)
	if _, err := manager.StartProcess(context.Background(), StartProcessRequest{
		SessionID: "session-1",
		Kind:      "dev",
		Command:   "echo hi",
	}); err == nil {
		t.Fatal("expected error when agentctl client is missing")
	}
}

func TestProcessRunnerListErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if _, err := manager.ListProcesses(context.Background(), "missing"); err == nil {
		t.Fatal("expected error when no execution exists for session")
	}

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	manager.executionStore.Add(exec)
	if _, err := manager.ListProcesses(context.Background(), "session-1"); err == nil {
		t.Fatal("expected error when agentctl client is missing")
	}
}

func TestProcessRunnerStopErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if err := manager.StopProcess(context.Background(), "process-1"); err == nil {
		t.Fatal("expected error when process not found")
	}
}

func TestProcessRunnerGetErrors(t *testing.T) {
	manager := &Manager{executionStore: NewExecutionStore()}
	if _, err := manager.GetProcess(context.Background(), "process-1", false); err == nil {
		t.Fatal("expected error when process not found")
	}
}

func TestStartProcessMergesManagedGoCacheAndReleasesTerminalLease(t *testing.T) {
	coordinator := activity.NewCoordinator(activity.Options{})
	var received agentctl.StartProcessRequest
	client := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode start request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"process":{"id":"process-1","session_id":"session-1","status":"exited"}}`))
	}))
	manager := &Manager{executionStore: NewExecutionStore()}
	manager.SetActivityCoordinator(coordinator)
	execution := &AgentExecution{
		ID: "exec-1", SessionID: "session-1", agentctl: client,
		Metadata: map[string]interface{}{managedGoCacheMetadataKey: "/managed/go-cache"},
	}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	process, err := manager.StartProcess(context.Background(), StartProcessRequest{
		SessionID: "session-1", Kind: "test", Command: "go test ./...",
		Env: map[string]string{"GOCACHE": "/user/go-cache"},
	})
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	if received.Env["GOCACHE"] != "/managed/go-cache" {
		t.Fatalf("received GOCACHE = %q, want managed path", received.Env["GOCACHE"])
	}
	if process.Status != agentctltypes.ProcessStatusExited {
		t.Fatalf("process status = %q, want exited", process.Status)
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("terminal process retained activity lease: %v", err)
	}
	maintenance.Release()
}

func TestStopProcessVerifiesExecutionOwnershipBeforeReleasingLease(t *testing.T) {
	nonOwnerStopCalled := false
	nonOwner := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			nonOwnerStopCalled = true
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	ownerStopped := false
	owner := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			ownerStopped = true
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"process-owned","session_id":"session-owner","status":"running"}`))
	}))
	coordinator := activity.NewCoordinator(activity.Options{})
	manager := &Manager{executionStore: NewExecutionStore()}
	manager.SetActivityCoordinator(coordinator)
	for _, execution := range []*AgentExecution{
		{ID: "exec-non-owner", SessionID: "session-other", agentctl: nonOwner},
		{ID: "exec-owner", SessionID: "session-owner", agentctl: owner},
	} {
		if err := manager.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindShellCommand)
	if err != nil {
		t.Fatal(err)
	}
	manager.trackActivity(processActivityKey("process-owned"), lease)

	if err := manager.StopProcess(context.Background(), "process-owned"); err != nil {
		t.Fatalf("StopProcess: %v", err)
	}
	if nonOwnerStopCalled || !ownerStopped {
		t.Fatalf("stop calls: non-owner=%v owner=%v", nonOwnerStopCalled, ownerStopped)
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("owned stopped process retained activity lease: %v", err)
	}
	maintenance.Release()
}

func TestStopProcessReturnsOwnerStopErrorAndRetainsLease(t *testing.T) {
	client := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			http.Error(w, "stop exploded", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"id":"process-owned","session_id":"session-owner","status":"running"}`))
	}))
	coordinator := activity.NewCoordinator(activity.Options{})
	manager := &Manager{executionStore: NewExecutionStore()}
	manager.SetActivityCoordinator(coordinator)
	if err := manager.executionStore.Add(&AgentExecution{
		ID: "exec-owner", SessionID: "session-owner", agentctl: client,
	}); err != nil {
		t.Fatal(err)
	}
	lease, err := coordinator.AcquireTask(context.Background(), activity.KindShellCommand)
	if err != nil {
		t.Fatal(err)
	}
	manager.trackActivity(processActivityKey("process-owned"), lease)

	err = manager.StopProcess(context.Background(), "process-owned")
	if err == nil || !strings.Contains(err.Error(), "stop exploded") {
		t.Fatalf("StopProcess error = %v, want owner stop failure", err)
	}
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0); !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy while process may still run", err)
	}
	manager.releaseActivity(processActivityKey("process-owned"))
}

func TestStartProcessClientCancellationDoesNotDropIndeterminateLease(t *testing.T) {
	accepted := make(chan struct{})
	releaseResponse := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseResponse)
		}
	}()
	client := processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(accepted)
		<-releaseResponse
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"process":{"id":"process-late","session_id":"session-1","status":"running"}}`))
	}))
	coordinator := activity.NewCoordinator(activity.Options{})
	manager := &Manager{executionStore: NewExecutionStore()}
	manager.SetActivityCoordinator(coordinator)
	if err := manager.executionStore.Add(&AgentExecution{
		ID: "exec-1", SessionID: "session-1", agentctl: client,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := manager.StartProcess(ctx, StartProcessRequest{
			SessionID: "session-1", Kind: "dev", Command: "pnpm dev",
		})
		result <- err
	}()
	<-accepted
	cancel()
	select {
	case err := <-result:
		t.Fatalf("StartProcess returned before accepted start was reconciled: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0); !errors.Is(err, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy during indeterminate start", err)
	}
	close(releaseResponse)
	released = true
	if err := <-result; err != nil {
		t.Fatalf("StartProcess after accepted response: %v", err)
	}
	manager.releaseActivity(processActivityKey("process-late"))
}
