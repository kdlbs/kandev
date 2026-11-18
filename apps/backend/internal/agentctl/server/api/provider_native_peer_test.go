package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// TestProviderReplacementNativePeer is a disposable native-protocol process.
// Its catalog comes from the real helper's authenticated MCP HTTP routes.
func TestProviderReplacementNativePeer(t *testing.T) {
	root := os.Getenv("KANDEV_TEST_PROVIDER_PEER")
	if root == "" {
		return
	}
	_ = appendProviderPeerFile(filepath.Join(root, "starts"))
	configured := map[string]struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"http_headers"`
	}{}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue
		}
		thread := map[string]any{"id": "native-session", "turns": []any{map[string]any{"id": "completed-turn", "status": "completed", "startedAt": time.Now().Add(-time.Minute).Unix()}}}
		result := map[string]any{}
		fault, _ := os.ReadFile(filepath.Join(root, "fault"))
		var callErr error
		switch req.Method {
		case "initialize":
			result["userAgent"] = "codex_cli_rs/0.166.0"
		case "model/list":
			result["data"] = []any{}
		case "thread/resume":
			var params struct {
				ThreadID string `json:"threadId"`
				Config   struct {
					Servers map[string]struct {
						URL     string            `json:"url"`
						Headers map[string]string `json:"http_headers"`
					} `json:"mcp_servers"`
				} `json:"config"`
			}
			callErr = json.Unmarshal(req.Params, &params)
			if params.ThreadID != "native-session" {
				callErr = fmt.Errorf("different native thread")
			}
			configured = params.Config.Servers
			if string(fault) == "wrong-thread" {
				thread["id"] = "other-native"
			}
			result["thread"] = thread
		case "thread/read":
			result["thread"] = thread
		case "thread/backgroundTerminals/list":
			result["data"] = []any{}
		case "mcpServerStatus/list":
			data := []any{}
			for name, server := range configured {
				tools, err := providerPeerCatalog(server.URL, server.Headers)
				if err != nil {
					callErr = err
					break
				}
				data = append(data, map[string]any{"name": name, "runtimeStatus": "connected", "tools": tools, "toolsError": nil})
			}
			if string(fault) == "stale-catalog" {
				data = append(data, map[string]any{"name": "kandev-diagnostics", "runtimeStatus": "connected", "tools": map[string]any{}, "toolsError": nil})
			}
			result["data"] = data
		case "turn/start":
			_ = appendProviderPeerFile(filepath.Join(root, "turns"))
			callErr = fmt.Errorf("replacement must not send a turn")
		default:
			callErr = fmt.Errorf("unsupported native method")
		}
		frame := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		if callErr != nil {
			delete(frame, "result")
			frame["error"] = map[string]any{"code": -32603, "message": callErr.Error()}
		}
		bytes, _ := json.Marshal(frame)
		fmt.Println(string(bytes))
	}
	os.Exit(0)
}

func appendProviderPeerFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.WriteString("1\n")
	return err
}

func providerPeerCatalog(url string, headers map[string]string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.NewStreamableHttpClient(url, transport.WithHTTPHeaders(headers))
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	_, err = c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: "2025-03-26", ClientInfo: mcp.Implementation{Name: "native-peer", Version: "1"}}})
	if err != nil {
		return nil, err
	}
	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(tools.Tools))
	for _, tool := range tools.Tools {
		result[tool.Name] = tool
	}
	return result, nil
}

func newNativeProviderReplacementServer(t *testing.T) (*Server, string) {
	return newNativeProviderFixture(t, nil, func(ctx context.Context, provider adapter.AgentAdapter, servers []types.McpServer) {
		require.NoError(t, provider.LoadSession(ctx, "native-session", servers))
	})
}

func newNativeProviderFixture(t *testing.T, configure func(*config.InstanceConfig), initial func(context.Context, adapter.AgentAdapter, []types.McpServer)) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	httpServer := httptest.NewUnstartedServer(nil)
	_, portString, err := net.SplitHostPort(httpServer.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageDiagnostics}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageDiagnostics: "task"}}
	servers, err := config.BuiltinMCPServersForInstance(profile.BuiltinPackages, "127.0.0.1", port, "execution", true)
	require.NoError(t, err)
	cfg := &config.InstanceConfig{InstanceID: "execution", ScopedMCPRoutes: true, NativeMCPRoutePrefix: "/i/execution", TaskID: "task", SessionID: "session", WorkDir: root, AuthToken: "control", Port: port, MCPHost: "127.0.0.1", McpProfile: &profile, McpServers: servers, Protocol: "codex-app-server", AgentArgs: []string{os.Args[0], "-test.run=^TestProviderReplacementNativePeer$"}, AgentEnv: []string{"KANDEV_TEST_PROVIDER_PEER=" + root}, DurableJournalPath: filepath.Join(root, "delivery.bbolt"), DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}
	if configure != nil {
		configure(cfg)
	}
	log := newTestLogger()
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	host := mcpserver.NewWithProfile(backend, "session", "task", port, log, "", false, profile, mcpserver.WithNativeRoutePrefix(cfg.NativeMCPRoutePrefix))
	t.Cleanup(func() { require.NoError(t, host.Close(context.Background())) })
	s := NewServer(cfg, mgr, host, backend, log)
	httpServer.Config.Handler = s.Router()
	httpServer.Start()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, mgr.StopForTeardown(cleanup))
		httpServer.CloseClientConnections()
		httpServer.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, mgr.Start(ctx))
	provider := mgr.GetAdapter()
	require.NoError(t, provider.Initialize(ctx))
	var attached []types.McpServer
	for _, server := range servers {
		attached = append(attached, types.McpServer{Name: server.Name, URL: server.URL, Type: server.Type})
	}
	initial(ctx, provider, attached)
	if configure == nil {
		settleNativeProviderJournal(t, s)
	}
	return s, root
}

func settleNativeProviderJournal(t *testing.T, s *Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	storage, err := s.procMgr.DeliveryJournal()
	require.NoError(t, err)
	for {
		events, stream, err := storage.Replay(ctx, s.procMgr.DeliveryStreamID(), 0, 100)
		if !errors.Is(err, journal.ErrStreamNotFound) {
			require.NoError(t, err)
		}
		modelObserved := false
		for _, event := range events {
			if event.Type == streams.EventTypeSessionModels {
				modelObserved = true
			}
		}
		if modelObserved {
			require.NoError(t, storage.Acknowledge(ctx, stream.StreamID, stream.HighWater))
			return
		}
		select {
		case <-s.procMgr.DeliveryWakeups():
		case <-ctx.Done():
			t.Fatal("native resume output did not settle")
		}
	}
}
