package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// TestListInstancesReturnsEnvelopeWithSessionAndTaskID drives GET
// /api/v1/instances through the real handler and the real agentctl client
// (agentctl.ControlClient), rather than a hand-written response fixture.
// agentctl.ControlClient.ListInstances has always decoded a
// {"instances": [...]} envelope, but handleListInstances wrote the bare
// []*InstanceInfo array straight from instance.Manager.ListInstances — a
// mismatch a fixture-based client test can never see, since it fabricates
// the "correct" response by hand instead of asking the server for one. It
// also pins that CreateInstance's SessionID/TaskID survive into the listed
// InstanceInfo, needed to correlate a recovered instance back to its owning
// session on a backend restart (discoveries G, J).
func TestListInstancesReturnsEnvelopeWithSessionAndTaskID(t *testing.T) {
	log := logger.Default()
	mgr := instance.NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})

	created, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{
		WorkspacePath: t.TempDir(),
		SessionID:     "session-123",
		TaskID:        "task-456",
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), created.ID) })

	cs := NewControlServer(&config.Config{}, mgr, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)

	instances, err := client.ListInstances(t.Context())
	if err != nil {
		t.Fatalf("ListInstances: %v (handler response did not decode as the {\"instances\":[...]} envelope the client expects)", err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(instances))
	}
	got := instances[0]
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "session-123")
	}
	if got.TaskID != "task-456" {
		t.Errorf("TaskID = %q, want %q", got.TaskID, "task-456")
	}
}

// TestListInstancesReturnsLiveWorkspaceSourceRoots drives GET
// /api/v1/instances through the real handler and the real agentctl client to
// pin AC-EXECUTORS-SURVIVAL-002.14's "workspace source roots" reconstruction
// row: the backend must read this back from the adopted instance rather than
// push its own, so the wire contract has to expose whatever the instance is
// currently enforcing, not just what it was created with.
func TestListInstancesReturnsLiveWorkspaceSourceRoots(t *testing.T) {
	log := logger.Default()
	mgr := instance.NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})

	workspacePath, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	created, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{
		WorkspacePath:        workspacePath,
		SessionID:            "session-123",
		WorkspaceSourceRoots: []string{workspacePath},
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), created.ID) })

	cs := NewControlServer(&config.Config{}, mgr, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)

	instances, err := client.ListInstances(t.Context())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(instances))
	}
	got := instances[0].WorkspaceSourceRoots
	if len(got) != 1 || got[0] != workspacePath {
		t.Errorf("WorkspaceSourceRoots = %v, want [%q]", got, workspacePath)
	}
}
