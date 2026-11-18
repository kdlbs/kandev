package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementRequiresExplicitControlAuthentication(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
	mgr := process.NewManager(cfg, logger.Default())
	defer func() { _ = mgr.StopForTeardown(context.Background()) }()
	s := NewServer(cfg, mgr, nil, nil, logger.Default())
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		response := httptest.NewRecorder()
		s.Router().ServeHTTP(response, httptest.NewRequest(method, "/api/v1/agent/provider-replacement", strings.NewReader(`{"action":"prepare","operation_id":"operation"}`)))
		require.Equal(t, http.StatusForbidden, response.Code, "disabled control auth must not expose replacement to a provider")
	}
}

func TestProviderReplacementPrepareAndReleaseRetainsSource(t *testing.T) {
	s := newProviderReplacementServer(t)
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	request := types.ProviderReplacementRequest{Action: "prepare", OperationID: "operation", Source: source}
	prepared := replacementControlRequest(t, s, request, http.StatusOK)
	require.Equal(t, "prepared", prepared.Phase)
	require.Equal(t, source, prepared.Source)
	replay := replacementControlRequest(t, s, request, http.StatusOK)
	require.Equal(t, prepared, replay)
	_, err = s.procMgr.BeginProviderOperation("")
	require.Error(t, err)
	request.Action = "release"
	request.ExpectedVersion = prepared.Version
	released := replacementControlRequest(t, s, request, http.StatusOK)
	require.Equal(t, "released", released.Phase)
	require.Equal(t, process.StatusRunning, s.procMgr.Status())
	require.Equal(t, source.ProcessGeneration, s.procMgr.ProcessGeneration())
	free, err := s.procMgr.BeginProviderOperation("")
	require.NoError(t, err)
	free()
}

type replacementReadyAdapter struct {
	adapter.AgentAdapter
	catalogError error
}

func (a *replacementReadyAdapter) GetSessionID() string                              { return "native-session" }
func (a *replacementReadyAdapter) ProviderMCPReplacementReady(context.Context) error { return nil }
func (a *replacementReadyAdapter) ProviderMCPCatalog(context.Context, string) (map[string][]string, error) {
	return map[string][]string{"kandev": {"step_complete_kandev"}}, a.catalogError
}
func (a *replacementReadyAdapter) RecordedTurnStart(string) (time.Time, bool) {
	return time.Now(), true
}

func newProviderReplacementServer(t *testing.T) *Server {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("complete provider workload evidence requires Linux")
	}
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	root := t.TempDir()
	cfg := &config.InstanceConfig{InstanceID: "execution", TaskID: "task", SessionID: "session", WorkDir: root, AuthToken: "control", Port: 12345, McpProfile: &profile, Protocol: "codex-app-server", AgentArgs: []string{os.Args[0], "-test.run=^TestAttachmentRescanPeerProcess$"}, AgentEnv: []string{"KANDEV_TEST_ATTACHMENT_CAPTURE=" + filepath.Join(root, "capture")}, DurableJournalPath: filepath.Join(root, "delivery.bbolt"), DeliveryStreamID: "stream", DeliveryHarnessGeneration: 1}
	log := newTestLogger()
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	require.NoError(t, mgr.Start(context.Background()))
	mgr.SetAdapterForTest(&replacementReadyAdapter{AgentAdapter: mgr.GetAdapter()})
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	host := mcpserver.NewWithProfile(backend, "session", "task", 12345, log, "", false, profile)
	t.Cleanup(func() { require.NoError(t, host.Close(context.Background())) })
	s := NewServer(cfg, mgr, host, backend, log)

	return s
}

func replacementControlRequest(t *testing.T, s *Server, request types.ProviderReplacementRequest, code int) types.ProviderReplacementStatus {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/provider-replacement", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer control")
	req.Header.Set(types.ProviderMCPReplacementHeader, "1")
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.Router().ServeHTTP(response, req)
	require.Equal(t, code, response.Code, response.Body.String())
	if request.ScopeCredential != "" {
		require.NotContains(t, response.Body.String(), request.ScopeCredential)
	}
	var status types.ProviderReplacementStatus
	if code == http.StatusOK {
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	}
	return status
}

func TestProviderReplacementRetirementIsDurableAndCannotRelease(t *testing.T) {
	s := newProviderReplacementServer(t)
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	req := types.ProviderReplacementRequest{Action: "prepare", OperationID: "retire-operation", Source: source}
	prepared := replacementControlRequest(t, s, req, http.StatusOK)
	req.Action = "retire"
	req.ExpectedVersion = prepared.Version
	retired := replacementControlRequest(t, s, req, http.StatusOK)
	require.Equal(t, "retired", retired.Phase)
	require.Equal(t, process.StatusStopped, s.procMgr.Status())
	require.Equal(t, source.ProcessGeneration, s.procMgr.ProcessGeneration())
	replay := replacementControlRequest(t, s, req, http.StatusOK)
	require.Equal(t, retired, replay)
	req.Action = "release"
	req.ExpectedVersion = retired.Version
	replacementControlRequest(t, s, req, http.StatusConflict)
	_, err = s.procMgr.BeginProviderOperation("")
	require.Error(t, err, "retired authority must never reopen on timeout or release")
}

func TestProviderReplacementScopedCredentialCannotUseControlRoutes(t *testing.T) {
	s, _ := newBuiltinSessionServer(t, "control")
	profile := s.mcpServer.Profile()
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	scope := p.ProviderMCPScope{BindingID: "scoped", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}
	secret := strings.Repeat("s", 64)
	require.NoError(t, s.mcpServer.PrepareBuiltinGeneration(scope, secret, profile))
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/generations/scoped", strings.NewReader(initialize))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	s.Router().ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agent/provider-replacement", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	response = httptest.NewRecorder()
	s.Router().ServeHTTP(response, req)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestProviderReplacementNativeResumeAndCatalogAcceptance(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("complete provider workload evidence requires Linux")
	}
	s, root := newNativeProviderReplacementServer(t)
	oldStream, newStream := providerReplacementControlStreams(t, s)
	oldClosed := make(chan struct{})
	go func() {
		defer close(oldClosed)
		for {
			if _, _, err := oldStream.ReadMessage(); err != nil {
				return
			}
		}
	}()
	pong := make(chan struct{}, 1)
	newStream.SetPongHandler(func(string) error {
		select {
		case pong <- struct{}{}:
		default:
		}
		return nil
	})
	targetSeen := make(chan struct{}, 1)
	go func() {
		for {
			_, body, err := newStream.ReadMessage()
			if err != nil {
				return
			}
			if bytes.Contains(body, []byte(`"session_status":"resumed"`)) {
				select {
				case targetSeen <- struct{}{}:
				default:
				}
			}
		}
	}()
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	req := types.ProviderReplacementRequest{OperationID: "native-operation", Action: "prepare", Source: source}
	prepared := replacementControlRequest(t, s, req, http.StatusOK)
	req.Action = "retire"
	req.ExpectedVersion = prepared.Version
	retired := replacementControlRequest(t, s, req, http.StatusOK)
	target := s.mcpServer.Profile()
	target.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: target.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	digest, err := p.BuiltinBindingDigest(target)
	require.NoError(t, err)
	req.Action = "activate"
	req.ExpectedVersion = retired.Version
	req.Profile = target
	req.Scope = p.ProviderMCPScope{BindingID: "native-target", ExecutionID: "execution", HelperID: source.HelperID, ProcessGeneration: source.ProcessGeneration + 1, HarnessGeneration: source.HarnessGeneration, ProfileDigest: digest}
	req.TargetStreamID = source.StreamID
	req.ScopeCredential = ""
	started := replacementControlRequest(t, s, req, http.StatusOK)
	require.Equal(t, "started", started.Phase)
	require.True(t, started.NativeResumeConfirmed)
	require.Equal(t, "native-session", s.procMgr.GetSessionID())
	require.Equal(t, "execution", s.cfg.InstanceID)
	require.Equal(t, source.StreamID, s.procMgr.DeliveryStreamID())
	require.Equal(t, source.HarnessGeneration, s.procMgr.DeliveryHarnessGeneration())
	select {
	case <-oldClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("stale backend stream retained access after scoped activation")
	}
	require.NoError(t, newStream.WriteControl(websocket.PingMessage, []byte("alive"), time.Now().Add(5*time.Second)))
	select {
	case <-pong:
	case <-time.After(5 * time.Second):
		t.Fatal("compatible backend stream did not survive provider refresh")
	}
	select {
	case <-targetSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("retained control stream did not receive resumed output on the same journal")
	}
	replay := replacementControlRequest(t, s, req, http.StatusOK)
	require.Equal(t, started, replay)
	starts, err := os.ReadFile(filepath.Join(root, "starts"))
	require.NoError(t, err)
	require.Equal(t, "1\n1\n", string(starts))
	_, err = os.Stat(filepath.Join(root, "turns"))
	require.True(t, os.IsNotExist(err), "provider replacement must not synthesize a turn")
	_, err = s.procMgr.BeginProviderOperation("")
	require.Error(t, err, "SQL acceptance must precede reopening prompt admission")
	req.Action = "accept"
	req.ExpectedVersion = started.Version
	accepted := replacementControlRequest(t, s, req, http.StatusOK)
	require.Equal(t, "accepted", accepted.Phase)
	servers, err := s.builtinSessionMCPServers(nil)
	require.NoError(t, err)
	require.Len(t, servers, 2)
	require.True(t, strings.HasPrefix(servers[0].Headers["Authorization"], "Bearer "))
	require.Greater(t, len(servers[0].Headers["Authorization"]), 32)
	require.NotEqual(t, "Bearer control", servers[0].Headers["Authorization"])
	require.Contains(t, servers[0].URL, "/mcp/generations/native-target")
	modeRequest := httptest.NewRequest(http.MethodPut, "/api/v1/mcp/mode", strings.NewReader(`{"mode":"task-title-pending"}`))
	modeRequest.Header.Set("Authorization", "Bearer control")
	modeRequest.Header.Set(types.ProviderMCPReplacementHeader, "1")
	modeRequest.Header.Set("Content-Type", "application/json")
	modeResponse := httptest.NewRecorder()
	s.Router().ServeHTTP(modeResponse, modeRequest)
	require.Equal(t, http.StatusOK, modeResponse.Code)
	liveCatalog := s.mcpServer.BuiltinGenerationToolCatalog("native-target")
	require.Contains(t, liveCatalog[p.PackageCore], "set_task_title_kandev", "title context must update the current scope")
	afterMode, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	require.Equal(t, req.Scope.ProfileDigest, afterMode.ProfileDigest, "runtime context must not rewrite the immutable package binding")
	require.NotContains(t, s.mcpServer.BuiltinCatalog()[p.PackageCore], "set_task_title_kandev", "initial registry remains historical")
	_, err = s.builtinSessionMCPServers(nil)
	require.NoError(t, err)

	free, err := s.procMgr.BeginProviderOperation("")
	require.NoError(t, err)
	free()
}

func providerReplacementControlStreams(t *testing.T, s *Server) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	header := http.Header{"Authorization": []string{"Bearer control"}, journal.DeliveryProtocolHeader: []string{"2"}}
	storage, err := s.procMgr.DeliveryJournal()
	require.NoError(t, err)
	source, err := storage.GetStream(context.Background(), s.procMgr.DeliveryStreamID())
	require.NoError(t, err)
	url := fmt.Sprintf("ws://127.0.0.1:%d/api/v1/agent/stream?after=%d", s.cfg.Port, source.Acknowledged)
	old, _, err := websocket.DefaultDialer.Dial(url, header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = old.Close() })
	header.Set(types.ProviderMCPReplacementHeader, "1")
	current, _, err := websocket.DefaultDialer.Dial(url, header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = current.Close() })
	return old, current
}

func TestProviderReplacementFailureDoesNotReplayOrOpenAdmission(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("complete provider workload evidence requires Linux")
	}
	for _, fault := range []string{"wrong-thread", "stale-catalog", "spawn"} {
		t.Run(fault, func(t *testing.T) {
			s, root := newNativeProviderReplacementServer(t)
			source, err := s.procMgr.ProviderReplacementSource()
			require.NoError(t, err)
			req := types.ProviderReplacementRequest{Action: "prepare", OperationID: "failed-operation", Source: source}
			prepared := replacementControlRequest(t, s, req, http.StatusOK)
			req.Action = "retire"
			req.ExpectedVersion = prepared.Version
			retired := replacementControlRequest(t, s, req, http.StatusOK)
			profile := s.mcpServer.Profile()
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			digest, err := p.BuiltinBindingDigest(profile)
			require.NoError(t, err)
			req.Action = "activate"
			req.ExpectedVersion = retired.Version
			req.Profile = profile
			req.Scope = p.ProviderMCPScope{BindingID: "failed-binding", ExecutionID: source.ExecutionID, HelperID: source.HelperID, ProcessGeneration: source.ProcessGeneration + 1, HarnessGeneration: source.HarnessGeneration, ProfileDigest: digest}
			req.TargetStreamID = source.StreamID
			req.ScopeCredential = strings.Repeat("s", 64)
			if fault == "spawn" {
				req.Args = []string{filepath.Join(root, "missing-provider")}
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(root, "fault"), []byte(fault), 0600))
			}
			failed := replacementControlRequest(t, s, req, http.StatusOK)
			require.Equal(t, "failed", failed.Phase)
			require.False(t, failed.NativeResumeConfirmed)
			require.True(t, failed.SourceTerminated)
			replay := replacementControlRequest(t, s, req, http.StatusOK)
			require.Equal(t, failed, replay)
			starts, err := os.ReadFile(filepath.Join(root, "starts"))
			require.NoError(t, err)
			expected := "1\n1\n"
			if fault == "spawn" {
				expected = "1\n"
			}
			require.Equal(t, expected, string(starts))
			_, err = s.procMgr.BeginProviderOperation("")
			require.Error(t, err, "failed candidates must not dispatch queued input")
			current, err := s.procMgr.ProviderReplacementSource()
			require.NoError(t, err)
			current.NativeSessionID = source.NativeSessionID
			next := types.ProviderReplacementRequest{Action: "prepare", OperationID: "retry-operation", Source: current}
			raw, err := json.Marshal(next)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(raw, &payload))
			payload["previous_operation_id"] = req.OperationID
			raw, err = json.Marshal(payload)
			require.NoError(t, err)
			httpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/agent/provider-replacement", bytes.NewReader(raw))
			httpRequest.Header.Set("Authorization", "Bearer control")
			httpRequest.Header.Set("Content-Type", "application/json")
			httpRequest.Header.Set(types.ProviderMCPReplacementHeader, "1")
			response := httptest.NewRecorder()
			s.Router().ServeHTTP(response, httpRequest)
			require.Equal(t, http.StatusOK, response.Code, "an explicit new operation must recover the failed candidate without opening prompt admission: %s", response.Body.String())
			var retryPrepared types.ProviderReplacementStatus
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &retryPrepared))
			require.Equal(t, "prepared", retryPrepared.Phase)
			next.Action = "retire"
			next.ExpectedVersion = retryPrepared.Version
			retryRetired := replacementControlRequest(t, s, next, http.StatusOK)
			if fault != "spawn" {
				require.NoError(t, os.Remove(filepath.Join(root, "fault")))
			}
			next.Action = "activate"
			next.ExpectedVersion = retryRetired.Version
			next.Profile = profile
			next.Scope = p.ProviderMCPScope{BindingID: "retry-binding", ExecutionID: current.ExecutionID, HelperID: current.HelperID, ProcessGeneration: current.ProcessGeneration + 1, HarnessGeneration: current.HarnessGeneration, ProfileDigest: digest}
			next.TargetStreamID = current.StreamID
			next.Args = []string{os.Args[0], "-test.run=^TestProviderReplacementNativePeer$"}
			retryStarted := replacementControlRequest(t, s, next, http.StatusOK)
			require.Equal(t, "started", retryStarted.Phase)
			require.True(t, retryStarted.NativeResumeConfirmed)
			require.Equal(t, source.NativeSessionID, s.procMgr.GetSessionID())
			require.Equal(t, source.StreamID, s.procMgr.DeliveryStreamID())
			next.Action = "accept"
			next.ExpectedVersion = retryStarted.Version
			replacementControlRequest(t, s, next, http.StatusOK)
			_, err = os.Stat(filepath.Join(root, "turns"))
			require.True(t, os.IsNotExist(err), "retry must not create a turn")
			if fault == "spawn" {
				current, err := s.procMgr.ProviderReplacementSource()
				require.NoError(t, err)
				third := types.ProviderReplacementRequest{Action: "prepare", OperationID: "accepted-recovery", PreviousOperationID: next.OperationID, Source: current}
				held := replacementControlRequest(t, s, third, http.StatusOK)
				third.Action = "retire"
				third.ExpectedVersion = held.Version
				stopped := replacementControlRequest(t, s, third, http.StatusOK)
				third.Action = "activate"
				third.ExpectedVersion = stopped.Version
				third.Profile = profile
				third.Scope = p.ProviderMCPScope{BindingID: "accepted-recovery-binding", ExecutionID: current.ExecutionID, HelperID: current.HelperID, ProcessGeneration: current.ProcessGeneration + 1, HarnessGeneration: current.HarnessGeneration, ProfileDigest: digest}
				third.TargetStreamID = current.StreamID
				resumed := replacementControlRequest(t, s, third, http.StatusOK)
				require.Equal(t, "started", resumed.Phase)
				require.Equal(t, source.NativeSessionID, s.procMgr.GetSessionID())
			}

		})
	}
}

func TestProviderReplacementReleasedReceiptFinishesAdmissionOnReplay(t *testing.T) {
	s := newProviderReplacementServer(t)
	ctx := context.Background()
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	req := types.ProviderReplacementRequest{Action: "prepare", OperationID: "release-recovery", Source: source}
	prepared := replacementControlRequest(t, s, req, http.StatusOK)
	storage, err := s.procMgr.DeliveryJournal()
	require.NoError(t, err)
	receipt, err := storage.ReadProviderReplacement(ctx, req.OperationID)
	require.NoError(t, err)
	receipt.Phase = journal.ProviderReplacementReleased
	_, err = storage.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.NoError(t, err)
	req.Action = "release"
	req.ExpectedVersion = prepared.Version
	replacementControlRequest(t, s, req, http.StatusOK)
	release, err := s.procMgr.BeginProviderOperation("")
	require.NoError(t, err, "a recorded release must finish reopening source admission after a lost response")
	release()
	next := types.ProviderReplacementRequest{Action: "prepare", OperationID: "after-release", PreviousOperationID: req.OperationID, Source: source}
	preparedAgain := replacementControlRequest(t, s, next, http.StatusOK)
	next.Action = "release"
	next.ExpectedVersion = preparedAgain.Version
	replacementControlRequest(t, s, next, http.StatusOK)

}

func TestProviderReplacementUncertainSourceCatalogKeepsProviderLive(t *testing.T) {
	s := newProviderReplacementServer(t)
	source, err := s.procMgr.ProviderReplacementSource()
	require.NoError(t, err)
	s.procMgr.SetAdapterForTest(&replacementReadyAdapter{AgentAdapter: s.procMgr.GetAdapter(), catalogError: fmt.Errorf("source MCP status unavailable")})
	replacementControlRequest(t, s, types.ProviderReplacementRequest{Action: "prepare", OperationID: "catalog-uncertain", Source: source}, http.StatusConflict)
	require.Equal(t, process.StatusRunning, s.procMgr.Status())
	require.Equal(t, source.ProcessGeneration, s.procMgr.ProcessGeneration())
	release, err := s.procMgr.BeginProviderOperation("")
	require.NoError(t, err)
	release()
}
