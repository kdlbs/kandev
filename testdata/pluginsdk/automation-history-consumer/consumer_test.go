package consumer

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const workspaceID = "workspace-consumer"

type consumerHostServer struct {
	pluginv1.UnimplementedHostServer
	mu        sync.Mutex
	runStatus string
	revoked   bool
	oversized bool
}

func (s *consumerHostServer) setRunStatus(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runStatus = value
}

func (s *consumerHostServer) setFailure(revoked, oversized bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked = revoked
	s.oversized = oversized
}

func (s *consumerHostServer) failure() error {
	if s.revoked {
		return status.Error(codes.PermissionDenied, "capability approval is no longer active")
	}
	if s.oversized {
		return status.Error(codes.ResourceExhausted, "automation history snapshot exceeds its bounded size")
	}
	return nil
}

func (s *consumerHostServer) GetState(context.Context, *pluginv1.GetStateRequest) (*pluginv1.GetStateResponse, error) {
	return &pluginv1.GetStateResponse{Found: false}, nil
}

func (s *consumerHostServer) GetCapabilityContext(context.Context, *pluginv1.GetCapabilityContextRequest) (*pluginv1.GetCapabilityContextResponse, error) {
	operations := make([]*pluginv1.HostOperationSupport, 0, 3)
	for _, method := range []string{
		"ListAutomationDefinitionsExact", "ListAutomationRunsExact", "GetAutomationRunExact",
	} {
		operations = append(operations, &pluginv1.HostOperationSupport{
			Method: method, CapabilityId: "host.v2.read:automations", Supported: true, Authorized: true,
		})
	}
	return &pluginv1.GetCapabilityContextResponse{Context: &pluginv1.HostCapabilityContext{
		InstallationId: "installation-consumer", WorkspaceId: workspaceID, ApprovalRevision: 4,
		ApprovalState: "active", Operations: operations,
	}}, nil
}

func (s *consumerHostServer) ListAutomationDefinitionsExact(
	_ context.Context, request *pluginv1.ListAutomationDefinitionsExactRequest,
) (*pluginv1.ListAutomationDefinitionsExactResponse, error) {
	if err := s.failure(); err != nil {
		return nil, err
	}
	response := &pluginv1.ListAutomationDefinitionsExactResponse{
		Items: []*pluginv1.AutomationDefinitionObservation{{
			Id: "automation-1", WorkspaceId: request.GetWorkspaceId(), Name: "nightly", Enabled: true,
			CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
			ResourceVersion: "definition-version", ObservedAt: "2026-10-03T00:00:00Z",
			Triggers: []*pluginv1.AutomationTriggerObservation{{
				Id: "trigger-1", Kind: "scheduled", Enabled: true,
				CronExpression: "0 9 * * *", ScheduleAvailable: true,
			}},
		}},
		PageInfo: &pluginv1.ExactReadPageInfo{SnapshotVersion: "snapshot-definitions", ObservedAt: "2026-10-03T00:00:00Z"},
	}
	return response, nil
}

func (s *consumerHostServer) ListAutomationRunsExact(
	_ context.Context, request *pluginv1.ListAutomationRunsExactRequest,
) (*pluginv1.ListAutomationRunsExactResponse, error) {
	if err := s.failure(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	runStatus := s.runStatus
	s.mu.Unlock()
	return &pluginv1.ListAutomationRunsExactResponse{
		Items: []*pluginv1.AutomationRunObservation{{
			Id: "run-1", WorkspaceId: request.GetWorkspaceId(), AutomationId: "automation-1",
			AutomationName: "nightly", TriggerId: "trigger-1", TriggerKind: "scheduled",
			TaskId: "task-1", SourceStatus: "task_created", Status: runStatus,
			RetryState: "none", CreatedAt: "2026-10-01T00:00:00Z",
			Summary: "safe summary", ResourceVersion: "run-version-" + runStatus,
			ObservedAt: "2026-10-03T00:00:00Z",
		}},
		PageInfo: &pluginv1.ExactReadPageInfo{
			SnapshotVersion: "snapshot-runs", ObservedAt: "2026-10-03T00:00:00Z",
			Receipt: &pluginv1.HostReadReceipt{
				InstallationId: "installation-consumer", WorkspaceId: workspaceID,
				CapabilityRevision: 4, RequestId: request.GetRequestId(),
				SnapshotVersion: "snapshot-runs", ObservedAt: "2026-10-03T00:00:00Z",
			},
		},
	}, nil
}

func (s *consumerHostServer) GetAutomationRunExact(
	_ context.Context, request *pluginv1.GetAutomationRunExactRequest,
) (*pluginv1.GetAutomationRunExactResponse, error) {
	if err := s.failure(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	runStatus := s.runStatus
	s.mu.Unlock()
	return &pluginv1.GetAutomationRunExactResponse{
		Item: &pluginv1.AutomationRunObservation{
			Id: request.GetRunId(), WorkspaceId: request.GetWorkspaceId(), AutomationId: "automation-1",
			SourceStatus: "task_created", Status: runStatus, CreatedAt: "2026-10-01T00:00:00Z",
			ResourceVersion: "run-version-" + runStatus, ObservedAt: "2026-10-04T00:00:00Z",
		},
		Receipt: &pluginv1.HostReadReceipt{
			InstallationId: "installation-consumer", WorkspaceId: request.GetWorkspaceId(),
			CapabilityRevision: 4, RequestId: request.GetRequestId(),
			SnapshotVersion: "receipt-version", ObservedAt: "2026-10-04T00:00:00Z",
		},
	}, nil
}

type legacyHostServer struct {
	pluginv1.UnimplementedHostServer
}

func (legacyHostServer) GetState(context.Context, *pluginv1.GetStateRequest) (*pluginv1.GetStateResponse, error) {
	return &pluginv1.GetStateResponse{Found: false}, nil
}

func dial(t *testing.T, server pluginv1.HostServer) pluginsdk.Host {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	pluginv1.RegisterHostServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	connection, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return pluginsdk.NewHostClient(connection)
}

func TestAutomationHistoryIndependentConsumer(t *testing.T) {
	ctx := context.Background()
	server := &consumerHostServer{runStatus: "task_created"}
	host := dial(t, server)

	exact, ok := pluginsdk.HostV2(host)
	if !ok {
		t.Fatal("capability discovery is unavailable")
	}
	capability, err := exact.GetCapabilityContext(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	supported := map[string]bool{}
	for _, operation := range capability.Operations {
		supported[operation.Method] = operation.Supported && operation.Authorized
	}
	for _, method := range []string{"ListAutomationDefinitionsExact", "ListAutomationRunsExact", "GetAutomationRunExact"} {
		if !supported[method] {
			t.Fatalf("%s is not supported and authorized: %+v", method, capability.Operations)
		}
	}

	reader, ok := pluginsdk.HostAutomationHistory(host)
	if !ok {
		t.Fatal("automation history reader is unavailable")
	}
	definitions, err := reader.ListAutomations(ctx, pluginsdk.AutomationDefinitionQuery{
		RequestID: "consumer-definitions", WorkspaceID: workspaceID,
		Filter: pluginsdk.AutomationDefinitionFilter{AutomationID: "automation-1"},
		Page:   pluginsdk.ExactReadPage{Limit: 50},
	})
	if err != nil || len(definitions.Items) != 1 || definitions.Items[0].Name != "nightly" {
		t.Fatalf("definitions = %+v, err=%v", definitions, err)
	}
	if !definitions.Items[0].Triggers[0].ScheduleAvailable {
		t.Fatalf("definition schedule is not available: %+v", definitions.Items[0].Triggers)
	}

	// A missed event is reconciled by listing retained runs and refreshing the
	// known run identity instead of waiting for another wake hint.
	runs, err := reader.ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "consumer-runs", WorkspaceID: workspaceID, Page: pluginsdk.ExactReadPage{Limit: 50},
	})
	if err != nil || len(runs.Items) != 1 || runs.Items[0].Status != "task_created" {
		t.Fatalf("runs = %+v, err=%v", runs, err)
	}
	server.setRunStatus("archived")
	refreshed, receipt, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "consumer-refresh", WorkspaceID: workspaceID, RunID: runs.Items[0].ID,
	})
	if err != nil || refreshed.Status != "archived" || refreshed.SourceStatus != "task_created" {
		t.Fatalf("refreshed run = %+v, err=%v", refreshed, err)
	}
	if receipt.RequestID != "consumer-refresh" || receipt.CapabilityRevision != 4 {
		t.Fatalf("receipt = %+v", receipt)
	}

	server.setFailure(true, false)
	if _, err := reader.ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "consumer-revoked", WorkspaceID: workspaceID, Page: pluginsdk.ExactReadPage{Limit: 50},
	}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("revoked list = %v, want PermissionDenied", err)
	}
	server.setFailure(false, true)
	if _, err := reader.ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "consumer-oversized", WorkspaceID: workspaceID, Page: pluginsdk.ExactReadPage{Limit: 50},
	}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized list = %v, want ResourceExhausted", err)
	}
}

func TestAutomationHistoryOldHost(t *testing.T) {
	ctx := context.Background()
	host := dial(t, legacyHostServer{})
	reader, ok := pluginsdk.HostAutomationHistory(host)
	if !ok {
		t.Fatal("the remote SDK wrapper exists even when the server lacks the RPCs")
	}
	_, err := reader.ListRuns(ctx, pluginsdk.AutomationRunQuery{
		RequestID: "old-host", WorkspaceID: workspaceID, Page: pluginsdk.ExactReadPage{Limit: 50},
	})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("old host list = %v, want Unimplemented", err)
	}
	if _, _, err := reader.GetRun(ctx, pluginsdk.AutomationRunGetQuery{
		RequestID: "old-host-refresh", WorkspaceID: workspaceID, RunID: "run-1",
	}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("old host refresh = %v, want Unimplemented", err)
	}
	if _, found, err := host.GetState(ctx, "scope", "scope-id", "key"); err != nil || found {
		t.Fatalf("legacy Host call = found=%v, err=%v; want an unchanged supported call", found, err)
	}
}
