package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/pkg/agent"
)

const projectDeliveryInstanceID = "instance-project-delivery"

type projectDeliveryTestEnv struct {
	cfg      *config.InstanceConfig
	manifest *p.BuiltinManifest
	delivery *types.ProjectMCPDelivery
}

func newProjectDeliveryTestEnv(t *testing.T, packages ...p.PackageID) projectDeliveryTestEnv {
	t.Helper()
	manifest := &p.BuiltinManifest{
		Version: 1, CatalogVersion: 1, WorkspaceID: "workspace-1", TaskID: "task-1",
		Surface: p.SurfaceKanbanTask, Packages: packages,
		Origins: map[p.PackageID]string{p.PackageCore: "required"},
	}
	for _, id := range packages {
		if id != p.PackageCore {
			manifest.Origins[id] = "task"
		}
	}
	profile := p.Normalize(p.Context{Surface: p.SurfaceKanbanTask, BuiltinPackages: manifest})
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	workspace := t.TempDir()
	cfg := &config.InstanceConfig{
		InstanceID: projectDeliveryInstanceID,
		Port:       43210,
		MCPHost:    "localhost",
		WorkDir:    workspace,
		TaskID:     "task-1",
		McpProfile: &profile,
		Protocol:   agent.ProtocolACP,
		AgentType:  "pi-acp",
	}
	names := make([]string, 0, len(packages))
	for _, id := range packages {
		names = append(names, string(id))
	}
	return projectDeliveryTestEnv{
		cfg:      cfg,
		manifest: manifest,
		delivery: &types.ProjectMCPDelivery{
			Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
			Workspace: filepath.Clean(workspace), TaskID: "task-1", InstanceID: projectDeliveryInstanceID,
			BindingDigest: digest, Packages: names,
		},
	}
}

func (e projectDeliveryTestEnv) cloneConfig() *config.InstanceConfig {
	clone := *e.cfg
	return &clone
}

// @covers AC-AGENTS-BUILTIN-MCP-003.2, AC-AGENTS-BUILTIN-MCP-003.5, AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPDeliveryRejectsMismatchedConfiguration(t *testing.T) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore, p.PackageDiagnostics)
	otherWorkspace := t.TempDir()
	for _, tc := range []struct {
		name   string
		mutate func(*types.ProjectMCPDelivery)
		absent bool
	}{
		{name: "absent record clears accepted evidence", absent: true},
		{name: "invalid version", mutate: func(d *types.ProjectMCPDelivery) { d.Version = 2 }},
		{name: "unsupported strategy", mutate: func(d *types.ProjectMCPDelivery) { d.Strategy = "cursor" }},
		{name: "replaced helper", mutate: func(d *types.ProjectMCPDelivery) { d.InstanceID = "instance-2" }},
		{name: "stale task", mutate: func(d *types.ProjectMCPDelivery) { d.TaskID = "task-2" }},
		{name: "other workspace", mutate: func(d *types.ProjectMCPDelivery) { d.Workspace = filepath.Clean(otherWorkspace) }},
		{name: "other binding", mutate: func(d *types.ProjectMCPDelivery) { d.BindingDigest = "b" + d.BindingDigest[1:] }},
		{name: "unbound package", mutate: func(d *types.ProjectMCPDelivery) { d.Packages = append(d.Packages, string(p.PackageConfiguration)) }},
		{name: "no package", mutate: func(d *types.ProjectMCPDelivery) { d.Packages = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewManager(env.cloneConfig(), newTestLogger(t))
			require.NoError(t, manager.ConfigureWithProjectDelivery("pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery))
			require.Equal(t, env.delivery, manager.cfg.ProjectMCPDelivery)

			var evidence *types.ProjectMCPDelivery
			if !tc.absent {
				evidence = env.delivery.Clone()
				tc.mutate(evidence)
			}
			err := manager.ConfigureWithProjectDelivery("pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, evidence)
			if tc.absent {
				require.NoError(t, err)
				require.Nil(t, manager.cfg.ProjectMCPDelivery, "accepted configuration must be cleared when the record is absent")
				return
			}
			require.Error(t, err)
			require.Equal(t, env.delivery, manager.cfg.ProjectMCPDelivery, "invalid explicit evidence must not replace accepted state")
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5
func TestProjectMCPDeliveryRequiresMatchingAgent(t *testing.T) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore)
	cfg := env.cloneConfig()
	cfg.AgentType = "claude-acp"
	manager := NewManager(cfg, newTestLogger(t))
	require.ErrorContains(t, manager.ConfigureWithProjectDelivery(
		"claude-acp", []string{"claude-acp"}, true, nil, "", nil, false, env.delivery,
	), "does not match this agent")
	require.Nil(t, manager.cfg.ProjectMCPDelivery)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestPiProjectMCPDeliveryACPFlow(t *testing.T) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore, p.PackageDiagnostics)
	manager, peer, cleanup := newProjectDeliveryACPFlow(t, env)
	defer cleanup()

	servers := projectDeliverySessionServers(t, env.manifest)
	ctx := context.Background()
	sessionAdapter := manager.adapter.(projectDeliverySessionAdapter)
	_, err := sessionAdapter.NewSession(ctx, servers)
	require.NoError(t, err)
	require.NoError(t, sessionAdapter.LoadSession(ctx, "saved-1", servers))
	require.NoError(t, sessionAdapter.ResumeSession(ctx, "saved-1", servers))
	_, err = sessionAdapter.ResetSession(ctx, servers)
	require.NoError(t, err)

	require.Equal(t, 2, peer.newCalls(), "new and reset must create a provider session")
	require.Equal(t, 2, peer.resumeCalls(), "load must prefer resume, then the explicit resume")
	for _, wire := range peer.wireServers() {
		require.Empty(t, wire, "project delivery must never add ACP wire entries")
	}

	partial := newProjectDeliveryTestEnv(t, p.PackageCore, p.PackageDiagnostics)
	partial.delivery.Packages = []string{string(p.PackageCore)}
	partialManager, partialPeer, partialCleanup := newProjectDeliveryACPFlow(t, partial)
	defer partialCleanup()
	_, err = partialManager.adapter.NewSession(ctx, servers)
	require.ErrorContains(t, err, "built-in MCP")
	require.Zero(t, partialPeer.newCalls(), "partial delivery must fail before the provider receives the request")
}

func newProjectDeliveryACPFlow(t *testing.T, env projectDeliveryTestEnv) (*Manager, *projectDeliveryACP, func()) {
	t.Helper()
	manager := NewManager(env.cfg, newTestLogger(t))
	t.Cleanup(manager.lifetimeCancel)
	if err := manager.ConfigureWithProjectDelivery("pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery); err != nil {
		t.Fatalf("ConfigureWithProjectDelivery: %v", err)
	}
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig: %v", err)
	}
	peer, cleanup := connectProjectDeliveryAdapter(t, manager)
	return manager, peer, func() {
		cleanup()
		manager.lifetimeCancel()
	}
}

func connectProjectDeliveryAdapter(t *testing.T, manager *Manager) (*projectDeliveryACP, func()) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	if err := manager.adapter.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("adapter Connect: %v", err)
	}
	peer := &projectDeliveryACP{}
	peer.conn = acpsdk.NewAgentSideConnection(peer, agentToClientWriter, clientToAgentReader)
	adapter := manager.adapter
	cleanup := func() {
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	}
	t.Cleanup(cleanup)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.adapter.Initialize(ctx); err != nil {
		cleanup()
		t.Fatalf("adapter Initialize: %v", err)
	}
	return peer, cleanup
}

// projectDeliverySessionAdapter exposes the full ACP session operation family
// on the process manager's adapter wrapper.
type projectDeliverySessionAdapter interface {
	NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error)
	LoadSession(ctx context.Context, sessionID string, mcpServers []types.McpServer) error
	ResumeSession(ctx context.Context, sessionID string, mcpServers []types.McpServer) error
	ResetSession(ctx context.Context, mcpServers []types.McpServer) (string, error)
}

// projectDeliveryACP advertises no ACP MCP transports, like pi-acp.
type projectDeliveryACP struct {
	burstPermissionAgent
	conn  *acpsdk.AgentSideConnection
	mu    sync.Mutex
	next  int
	wires [][]acpsdk.McpServer
	newN  int
	resN  int
}

func (a *projectDeliveryACP) Prompt(context.Context, acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *projectDeliveryACP) Initialize(_ context.Context, req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: req.ProtocolVersion,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession:         true,
			SessionCapabilities: acpsdk.SessionCapabilities{Resume: &acpsdk.SessionResumeCapabilities{}},
		},
	}, nil
}

func (a *projectDeliveryACP) NewSession(_ context.Context, req acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	a.newN++
	a.wires = append(a.wires, req.McpServers)
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(fmt.Sprintf("session-%d", a.next))}, nil
}

func (a *projectDeliveryACP) ResumeSession(_ context.Context, req acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resN++
	a.wires = append(a.wires, req.McpServers)
	return acpsdk.ResumeSessionResponse{}, nil
}

func (a *projectDeliveryACP) newCalls() int { a.mu.Lock(); defer a.mu.Unlock(); return a.newN }

func (a *projectDeliveryACP) resumeCalls() int { a.mu.Lock(); defer a.mu.Unlock(); return a.resN }

func (a *projectDeliveryACP) wireServers() [][]acpsdk.McpServer {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([][]acpsdk.McpServer(nil), a.wires...)
}

func projectDeliverySessionServers(t *testing.T, manifest *p.BuiltinManifest) []types.McpServer {
	t.Helper()
	configured, err := config.BuiltinMCPServers(manifest, "localhost", 43210, "")
	require.NoError(t, err)
	servers := make([]types.McpServer, 0, len(configured))
	for _, server := range configured {
		servers = append(servers, types.McpServer{
			Name: server.Name, Type: server.Type, URL: server.URL,
			BuiltinPackage: p.BuiltinPackageForRuntimeName(server.Name),
		})
	}
	require.NotEmpty(t, servers)
	return servers
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPDeliveryConsumedByProcessGeneration(t *testing.T) {
	for _, scenario := range []string{"stop then start", "exit then start"} {
		t.Run(scenario, func(t *testing.T) {
			runProjectMCPGenerationScenario(t, scenario)
		})
	}
}

func runProjectMCPGenerationScenario(t *testing.T, scenario string) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore, p.PackageDiagnostics)
	manager := NewManager(env.cfg, newTestLogger(t))
	t.Cleanup(manager.lifetimeCancel)
	require.NoError(t, manager.ConfigureWithProjectDelivery(
		"pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery,
	))
	require.NoError(t, manager.buildAdapterConfig())
	require.Nil(t, manager.cfg.ProjectMCPDelivery, "one process generation consumes the pending evidence")

	servers := projectDeliverySessionServers(t, env.manifest)
	peer, cleanup := connectProjectDeliveryAdapter(t, manager)
	_, err := manager.adapter.NewSession(context.Background(), servers)
	require.NoError(t, err, "the current adapter retains the immutable delivery copy")
	require.Equal(t, 1, peer.newCalls())
	if scenario == "stop then start" {
		require.NoError(t, manager.Stop(context.Background()))
		cleanup()
	} else {
		// Simulate the child exiting on its own: the manager observes a stopped
		// status with no terminal Stop call, then a replacement Start arrives.
		manager.status.Store(StatusStopped)
		manager.exitCode.Store(0)
		cleanup()
	}

	// A replacement process must not inherit the consumed evidence without a
	// fresh authenticated configuration, whether the predecessor stopped or
	// exited on its own.
	require.NoError(t, manager.buildAdapterConfig())
	replacement, replacementCleanup := connectProjectDeliveryAdapter(t, manager)
	_, err = manager.adapter.NewSession(context.Background(), servers)
	require.ErrorContains(t, err, "built-in MCP")
	require.Zero(t, replacement.newCalls(), "missing evidence must fail before the provider receives the request")
	replacementCleanup()

	require.NoError(t, manager.ConfigureWithProjectDelivery(
		"pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery,
	))
	require.NoError(t, manager.buildAdapterConfig())
	rearmed, rearmedCleanup := connectProjectDeliveryAdapter(t, manager)
	defer rearmedCleanup()
	_, err = manager.adapter.NewSession(context.Background(), servers)
	require.NoError(t, err, "a fresh configuration re-arms the next process generation")
	require.Equal(t, 1, rearmed.newCalls())
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPDeliveryAcceptsNestedWorkspacePath(t *testing.T) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore)
	nested := filepath.Join(t.TempDir(), strings.Repeat("nested-segment/", 20), "workspace")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	require.Greater(t, len(nested), 256)
	cfg := env.cloneConfig()
	cfg.WorkDir = nested
	env.delivery.Workspace = types.CanonicalProjectMCPDeliveryWorkspace(nested)
	manager := NewManager(cfg, newTestLogger(t))
	require.NoError(t, manager.ConfigureWithProjectDelivery(
		"pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery,
	))
	require.Equal(t, env.delivery, manager.cfg.ProjectMCPDelivery)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestRebindWorkspaceClearsPendingProjectDelivery(t *testing.T) {
	env := newProjectDeliveryTestEnv(t, p.PackageCore)
	manager := NewManager(env.cfg, newTestLogger(t))
	t.Cleanup(func() {
		manager.repoTrackersMu.Lock()
		trackers := append([]*WorkspaceTracker{manager.workspaceTracker}, manager.repoTrackers...)
		manager.repoTrackersMu.Unlock()
		for _, tracker := range trackers {
			if tracker != nil {
				tracker.Stop()
			}
		}
		manager.lifetimeCancel()
	})
	require.NoError(t, manager.ConfigureWithProjectDelivery(
		"pi-acp", []string{"pi-acp"}, true, nil, "", nil, false, env.delivery,
	))
	require.NotNil(t, manager.cfg.ProjectMCPDelivery)

	require.NoError(t, manager.RebindWorkspace(context.Background(), t.TempDir()))
	require.Nil(t, manager.cfg.ProjectMCPDelivery, "workspace rebind invalidates old delivery evidence")
}
