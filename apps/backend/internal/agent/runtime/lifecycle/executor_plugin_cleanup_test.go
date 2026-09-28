package lifecycle

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestPluginExecutorStopMatrix(t *testing.T) {
	stopRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/stop" {
			http.NotFound(w, req)
			return
		}
		stopRequests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{record: pluginExecutorRecoveryRecord(t, "ready", &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}), session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", QueueIncarnationID: "incarnation-plugin-recovery", State: models.TaskSessionStateCompleted}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)

	ordinaryClient := newPluginExecutorTestClient(t, server)
	if err := runtime.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
		Client: ordinaryClient,
	}, false); err != nil {
		t.Fatalf("ordinary stop: %v", err)
	}
	if operations.destroyRequest != nil || stopRequests != 1 {
		t.Fatalf("ordinary stop destroyed compute or skipped agentctl stop: destroy=%#v stop_requests=%d", operations.destroyRequest, stopRequests)
	}

	shutdownClient := newPluginExecutorTestClient(t, server)
	if err := runtime.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "execution-plugin-recovery", StopReason: StopReasonBackendShutdown, Client: shutdownClient,
	}, false); err != nil {
		t.Fatalf("backend shutdown: %v", err)
	}
	if operations.destroyRequest != nil || stopRequests != 1 {
		t.Fatalf("backend shutdown stopped or destroyed remote compute: destroy=%#v stop_requests=%d", operations.destroyRequest, stopRequests)
	}

	archiveClient := newPluginExecutorTestClient(t, server)
	instance := &ExecutorInstance{
		InstanceID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
		Client: archiveClient, StopReason: StopReasonTaskArchived, Metadata: store.record.Metadata,
	}
	if err := runtime.StopInstance(context.Background(), instance, true); err != nil {
		t.Fatalf("archive cleanup: %v", err)
	}
	if operations.destroyRequest == nil || operations.destroyRequest.GetResource().GetResourceHandle() != "resource-recovery" ||
		operations.destroyRequest.GetCleanupClaim() != "operation-plugin-recovery:cleanup" || operations.destroyRequest.GetCleanupReason() != "task_cleanup" {
		t.Fatalf("destroy request = %#v", operations.destroyRequest)
	}
	if store.claimRequest.OwnerTaskID != "task-plugin-recovery" || store.claimRequest.OwnershipGeneration != 7 ||
		!store.claimRequest.AllowCurrentSessionRuntime || store.releaseCount != 1 {
		t.Fatalf("cleanup claim = %+v releases=%d", store.claimRequest, store.releaseCount)
	}
	updatedInventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || updatedInventory.Phase != "absent" || updatedInventory.Resource != nil {
		t.Fatalf("post-cleanup inventory = %+v, err=%v", updatedInventory, err)
	}
	if store.record.ResumeToken != "resume-preserved" || store.record.LastMessageUUID != "message-preserved" {
		t.Fatalf("cleanup changed conversation resume data: %+v", store.record)
	}
}

func TestPluginExecutorOwnershipFence(t *testing.T) {
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record: pluginExecutorRecoveryRecord(t, "ready", &pluginsdk.ExecutorResourceDescriptor{
			ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
		}),
		session:  &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", QueueIncarnationID: "incarnation-plugin-recovery", State: models.TaskSessionStateCompleted},
		claimErr: models.ErrExecutionRotated,
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	err := runtime.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
		StopReason: StopReasonTaskDeleted, Metadata: store.record.Metadata,
	}, true)
	if err == nil || !strings.Contains(err.Error(), "fenced") {
		t.Fatalf("stale ownership cleanup error = %v, want fail-closed fence", err)
	}
	if operations.destroyRequest != nil || store.deleted {
		t.Fatalf("stale owner destroyed or deleted successor inventory: destroy=%#v deleted=%v", operations.destroyRequest, store.deleted)
	}
}

func newPluginExecutorTestClient(t *testing.T, server *httptest.Server) *agentctl.Client {
	t.Helper()
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("parse test agentctl address: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse test agentctl port: %v", err)
	}
	return agentctl.NewClient(host, port, newTestLogger())
}
