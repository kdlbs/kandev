package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// TestProviderReplacementManagedCodexConformance uses a pinned native binary,
// an isolated provider home and a local model response. It never uses a live
// user's conversation, credentials, model endpoint, or workspace.
func TestProviderReplacementManagedCodexConformance(t *testing.T) {
	binary := os.Getenv("KANDEV_CODEX_MCP_CONFORMANCE_BIN")
	if binary == "" {
		t.Skip("set KANDEV_CODEX_MCP_CONFORMANCE_BIN to a proven managed Codex binary")
	}
	if runtime.GOOS != "linux" {
		t.Skip("complete provider workload evidence requires Linux")
	}
	var modelCalls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			http.Error(w, "unknown fixture endpoint", http.StatusNotFound)
			return
		}
		modelCalls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		message := map[string]any{"id": "fixture-message", "type": "message", "status": "completed", "role": "assistant", "phase": "final_answer", "content": []any{map[string]any{"type": "output_text", "text": "MCP_CONFORMANCE"}}}
		events := []map[string]any{{"type": "response.created", "response": map[string]any{"id": "fixture-response", "status": "in_progress", "model": "conformance"}}, {"type": "response.output_item.done", "output_index": 0, "item": message}, {"type": "response.completed", "response": map[string]any{"id": "fixture-response", "status": "completed", "model": "conformance", "output": []any{message}, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}}}
		for _, event := range events {
			raw, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], raw)
		}
	}))
	defer model.Close()
	home := t.TempDir()
	configuration := fmt.Sprintf("model = \"conformance\"\nmodel_provider = \"conformance\"\n[model_providers.conformance]\nname = \"Conformance fixture\"\nbase_url = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\n", model.URL)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(configuration), 0600))
	var nativeID string
	s, _ := newNativeProviderFixture(t, func(cfg *config.InstanceConfig) {
		cfg.AgentArgs = []string{binary, "app-server"}
		cfg.AgentEnv = []string{"CODEX_HOME=" + home, "OPENAI_API_KEY=", "CODEX_API_KEY=", "CODEX_AUTH_TOKEN="}
		cfg.ShellEnabled = true
	}, func(ctx context.Context, provider adapter.AgentAdapter, servers []types.McpServer) {
		var err error
		nativeID, err = provider.NewSession(ctx, servers)
		require.NoError(t, err)
		require.NotEmpty(t, nativeID)
		require.NoError(t, provider.Prompt(ctx, "Reply MCP_CONFORMANCE. Do not use tools.", nil, 1))
	})
	awaitConformanceTurn(t, s)
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	require.Equal(t, nativeID, source.NativeSessionID)
	terminal, err := s.procMgr.StartTerminalShell("conformance", shell.DefaultConfig(s.cfg.WorkDir))
	require.NoError(t, err)
	terminalPID := terminal.Status().Pid
	preview, err := s.procMgr.PublishWorkspacePreview(process.WorkspacePreviewRequest{Path: "index.html", Content: "kept preview"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.procMgr.CloseWorkspacePreview(context.Background()) })
	child, err := s.procMgr.StartPipedProcess(process.PipedStartRequest{SessionID: "workspace-editor", Kind: types.ProcessKindCustom, Command: "cat"})
	require.NoError(t, err)
	defer func() { _ = child.Stdin.Close(); _ = child.Stdout.Close() }()
	settledCtx, settledCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer settledCancel()
	// Model completion can precede provider background settlement.
	require.Eventually(t, func() bool { return s.verifyProviderReplacementSource(settledCtx, source) == nil }, 15*time.Second, 50*time.Millisecond, "native provider safe-boundary evidence")
	request := types.ProviderReplacementRequest{Action: "prepare", OperationID: "native-conformance", Source: source}
	prepared := replacementControlRequest(t, s, request, http.StatusOK)
	request.Action = "retire"
	request.ExpectedVersion = prepared.Version
	retired := replacementControlRequest(t, s, request, http.StatusOK)
	checkConformanceWorkspace(t, s, terminalPID, terminal.Status().Pid, preview.Port, preview.Path)
	profile := s.mcpServer.Profile()
	profile.BuiltinPackages = profile.BuiltinPackages.Clone()
	profile.BuiltinPackages.Packages = []p.PackageID{p.PackageCore, p.PackageConfiguration}
	profile.BuiltinPackages.Origins = map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	request.Action = "activate"
	request.ExpectedVersion = retired.Version
	request.Profile = profile
	request.Scope = p.ProviderMCPScope{BindingID: "conforming-native", ExecutionID: source.ExecutionID, HelperID: source.HelperID, ProcessGeneration: source.ProcessGeneration + 1, HarnessGeneration: source.HarnessGeneration, ProfileDigest: digest}
	request.TargetStreamID = source.StreamID
	started := replacementControlRequest(t, s, request, http.StatusOK)
	require.Equal(t, "started", started.Phase)
	require.True(t, started.NativeResumeConfirmed)
	require.Contains(t, started.Catalog, "kandev-configuration")
	require.NotContains(t, started.Catalog, "kandev-diagnostics")
	request.Action = "accept"
	request.ExpectedVersion = started.Version
	replacementControlRequest(t, s, request, http.StatusOK)
	require.Equal(t, nativeID, s.procMgr.GetSessionID())
	require.Equal(t, source.StreamID, s.procMgr.DeliveryStreamID())
	require.Equal(t, source.HarnessGeneration, s.procMgr.DeliveryHarnessGeneration())
	require.EqualValues(t, 1, modelCalls.Load(), "provider refresh must not send a model turn")
	checkConformanceWorkspace(t, s, terminalPID, terminal.Status().Pid, preview.Port, preview.Path)
	checkConformanceTerminal(t, terminal)
	echo := []byte("editor connection survived\n")
	_, err = child.Stdin.Write(echo)
	require.NoError(t, err)
	got := make([]byte, len(echo))
	read := make(chan error, 1)
	go func() { _, err := io.ReadFull(child.Stdout, got); read <- err }()
	select {
	case err := <-read:
		require.NoError(t, err)
		require.Equal(t, echo, got)
	case <-time.After(5 * time.Second):
		t.Fatal("workspace protocol connection did not survive")
	}
}

func awaitConformanceTurn(t *testing.T, s *Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	storage, err := s.procMgr.DeliveryJournal()
	require.NoError(t, err)
	for {
		events, stream, err := storage.Replay(ctx, s.procMgr.DeliveryStreamID(), 0, 1000)
		if !errors.Is(err, journal.ErrStreamNotFound) {
			require.NoError(t, err)
		}
		complete := false
		for _, event := range events {
			if event.Type == streams.EventTypeError {
				var diagnostic adapter.AgentEvent
				require.NoError(t, json.Unmarshal(event.Payload, &diagnostic))
				t.Fatalf("isolated model turn failed: %s", diagnostic.Error)
			}
			if event.Type == streams.EventTypeComplete {
				complete = true
			}
		}
		if complete {
			require.NoError(t, storage.Acknowledge(ctx, stream.StreamID, stream.HighWater))
			return
		}
		select {
		case <-s.procMgr.DeliveryWakeups():
		case <-ctx.Done():
			t.Fatal("isolated model turn did not complete")
		}
	}
}

func checkConformanceWorkspace(t *testing.T, s *Server, before, after, port int, path string) {
	t.Helper()
	require.Equal(t, before, after)
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "kept preview", string(body))
	require.True(t, s.procMgr.Shell().Status().Running)
}

func checkConformanceTerminal(t *testing.T, terminal *shell.Session) {
	t.Helper()
	output := make(chan []byte, 32)
	terminal.Subscribe(output)
	defer terminal.Unsubscribe(output)
	_, err := terminal.Write([]byte("printf 'terminal-%s\\n' alive\n"))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var received strings.Builder
	for !strings.Contains(received.String(), "terminal-alive") {
		select {
		case data := <-output:
			received.Write(data)
		case <-ctx.Done():
			t.Fatal("retained terminal did not execute a command")
		}
	}
}
