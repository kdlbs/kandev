package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type startSandboxFixture struct {
	launch                *contract.Launch
	marked, bound, closed bool
	receiptApplied        bool
}

func (f *startSandboxFixture) Launch() *contract.Launch        { return f.launch.Clone() }
func (f *startSandboxFixture) MarkStart(context.Context) error { f.marked = true; return nil }
func (f *startSandboxFixture) BindOwner(_ context.Context, _ string, owner *contract.ProcessOwner) error {
	f.bound = owner.AttemptID == f.launch.AttemptID
	return nil
}
func (f *startSandboxFixture) Close() error { f.closed = true; return nil }

func TestSandboxLifecycleConfiguresBeforeStartAndBindsOwner(t *testing.T) {
	bytes := []byte("fixture")
	digest := sha256.Sum256(bytes)
	files, err := contract.MaterializeRuntime(t.Context(), t.TempDir(), bytes, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	attempt := &startSandboxFixture{launch: &contract.Launch{Version: 1, AttemptID: "attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}}
	configured := false
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{contract.LaunchCapability}}))
		case "/api/v1/agent/configure":
			var body struct {
				Sandbox *contract.Launch `json:"sandbox"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, attempt.launch, body.Sandbox)
			configured = true
			require.False(t, attempt.marked)
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			require.True(t, configured)
			require.True(t, attempt.marked)
			_, _ = w.Write([]byte(`{"success":true,"command":"agent"}`))
		case "/api/v1/agent/sandbox/owner":
			require.NoError(t, json.NewEncoder(w).Encode(contract.ProcessOwner{AttemptID: "attempt", Generation: 1, PolicyDigest: attempt.launch.PolicyDigest, Supervisor: processidentity.Identity{PID: 100, GroupID: 100, SessionID: 100, BirthToken: "birth"}}))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer peer.Close()
	host, portText, err := net.SplitHostPort(peer.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	transport := client.NewClient(host, port, log)
	execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task", AgentArgs: []string{"agent"}, SandboxSelection: &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}}
	manager := &Manager{}
	captured := false
	manager.SetExecutionSnapshotRecorder(func(_ context.Context, _ *AgentExecution, snapshot models.ExecutionSnapshot, launch *contract.Launch) error {
		require.False(t, configured)
		require.Equal(t, attempt.launch.AttemptID, snapshot.AttemptID)
		require.Equal(t, attempt.launch, launch)
		captured = true
		return nil
	})
	manager.SetSandboxRuntime(func(_ context.Context, _ *AgentExecution, generation uint64, restartOf string, _ map[string]string, _ *client.Client) (SandboxAttempt, error) {
		require.EqualValues(t, 1, generation)
		require.Empty(t, restartOf)
		return attempt, nil
	})
	_, err = manager.configureAndStartProcess(t.Context(), execution, transport, nil)
	require.NoError(t, err)
	require.True(t, attempt.bound)
	require.True(t, captured)
}

func TestSandboxLifecycleMissingRuntimeRefusesSelectedLaunch(t *testing.T) {
	manager := &Manager{}
	execution := &AgentExecution{SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}}
	_, err := manager.configureAndStartProcess(t.Context(), execution, nil, nil)
	require.Error(t, err)
}

func (f *startSandboxFixture) ValidateCommand([]string, map[string]string) error { return nil }

func TestSandboxLifecycleRestartPassesPreviousAttemptAfterCleanup(t *testing.T) {
	previous := &startSandboxFixture{launch: &contract.Launch{AttemptID: "previous", Generation: 4}}
	execution := &AgentExecution{SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}, sandboxAttempt: previous, sandboxGeneration: 4, sandboxDispatched: true}
	next := &startSandboxFixture{launch: &contract.Launch{AttemptID: "next", Generation: 5}}
	manager := &Manager{}
	manager.SetSandboxRuntime(func(_ context.Context, _ *AgentExecution, generation uint64, restartOf string, _ map[string]string, _ *client.Client) (SandboxAttempt, error) {
		require.EqualValues(t, 5, generation)
		require.Equal(t, "previous", restartOf)
		return next, nil
	})
	prepared, err := manager.prepareExecutionSandbox(t.Context(), execution, nil, nil)
	require.NoError(t, err)
	require.Same(t, next, prepared)
}

func (f *startSandboxFixture) ApplyStartReceipt(_ context.Context, _ string, receipt *contract.StartReceipt) error {
	if err := receipt.ValidateStopped(f.launch); err != nil {
		return err
	}
	f.receiptApplied = true
	return nil
}

func TestSandboxLifecycleFailedStartUsesCompletedReceipt(t *testing.T) {
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	files, err := contract.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	attempt := &startSandboxFixture{launch: &contract.Launch{Version: 1, AttemptID: "failed-attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{contract.LaunchCapability}}))
		case "/api/v1/agent/configure":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"start failed"}`))
		case "/api/v1/agent/sandbox/failed-attempt/receipt":
			require.True(t, attempt.marked)
			require.NoError(t, json.NewEncoder(w).Encode(contract.StartReceipt{Version: 1, AttemptID: attempt.launch.AttemptID, Generation: 1, PolicyDigest: attempt.launch.PolicyDigest, Outcome: contract.StartNotStarted}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer peer.Close()
	host, portText, err := net.SplitHostPort(peer.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	transport := client.NewClient(host, port, log)
	execution := &AgentExecution{ID: "execution", AgentArgs: []string{"agent"}, SandboxSelection: &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}, sandboxAttempt: attempt}
	manager := &Manager{}
	_, err = manager.configureAndStartProcess(t.Context(), execution, transport, nil)
	require.Error(t, err, "a cleanup receipt does not turn failed startup into success")
	require.True(t, attempt.receiptApplied, "cleanup must consume the exact completed attempt")
	require.False(t, attempt.bound)
	require.NoError(t, execution.closeSandbox())
	require.True(t, attempt.closed)
}
