//go:build linux

package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/kandev/kandev/internal/common/processcontainment"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxNativeLaunchRetainsPTYOwnerBeforeReturning(t *testing.T) {
	testSandboxNativeLaunch(t, false, false)
}
func TestSandboxNativeLaunchConsumesFailedSpawnReceipt(t *testing.T) {
	testSandboxNativeLaunch(t, true, false)
}
func testSandboxNativeLaunch(t *testing.T, failed, exited bool) {
	t.Helper()
	manager, runner, execution := newPassthroughRunnerManager(t)
	runtime := []byte("#!/bin/sh\nexec \"$@\"\n")
	digest := sha256.Sum256(runtime)
	files, err := contract.MaterializeRuntime(t.Context(), t.TempDir(), runtime, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	group, err := processcontainment.Create()
	if err != nil && os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated cgroup v2")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, group.Kill(context.Background())); require.NoError(t, group.Remove()) })
	attempt := &startSandboxFixture{launch: &contract.Launch{Containment: group, Version: 1, AttemptID: "native", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}}
	execution.SandboxSelection = &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}
	execution.sandboxAttempt = attempt
	captured := false
	manager.SetExecutionSnapshotRecorder(func(_ context.Context, _ *AgentExecution, snapshot models.ExecutionSnapshot, launch *contract.Launch) error {
		require.False(t, attempt.marked)
		require.Equal(t, attempt.launch.AttemptID, snapshot.AttemptID)
		require.Equal(t, "native_terminal", snapshot.Transport)
		require.Equal(t, attempt.launch, launch)
		captured = true
		return nil
	})
	ownerStored := false
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{contract.LaunchCapability}}))
		case "/api/v1/agent/configure":
			require.False(t, attempt.marked)
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/agent/sandbox/owner":
			require.Equal(t, http.MethodPut, r.Method)
			require.True(t, attempt.marked)
			var owner contract.ProcessOwner
			require.NoError(t, json.NewDecoder(r.Body).Decode(&owner))
			require.Equal(t, attempt.launch.AttemptID, owner.AttemptID)
			_, err := owner.LiveSnapshot()
			require.NoError(t, err)
			ownerStored = true
			w.WriteHeader(http.StatusNoContent)
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
	execution.replaceAgentctlClient(client.NewClient(host, port, log))
	command := []string{"/bin/sh", "-c", "read line"}
	if failed {
		command[0] = filepath.Join(t.TempDir(), "missing-agent")
	}
	launchRunner := executionPassthroughRunner(runner)
	if exited {
		command[2] = "exit 0"
		launchRunner = &exitedSandboxRunner{InteractiveRunner: runner, t: t}
	}
	launchCtx, cancelLaunch := context.WithTimeout(t.Context(), time.Second)
	defer cancelLaunch()
	info, err := manager.startSandboxInteractive(launchCtx, execution, launchRunner, process.InteractiveStartRequest{SessionID: execution.SessionID, WorkingDir: execution.WorkspacePath, Command: command, ImmediateStart: true})
	require.True(t, captured)
	if failed || exited {
		require.Error(t, err)
		require.Nil(t, info)
		require.True(t, attempt.receiptApplied, "native failed spawn must resolve the retained attempt")
		require.False(t, ownerStored)
		require.False(t, attempt.bound)
		require.NoError(t, execution.closeSandbox())
		return
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, runner.Stop(ctx, info.ID))
	})
	require.True(t, ownerStored)
	require.True(t, attempt.bound)
	require.True(t, execution.sandboxDispatched)
}

func TestSandboxMCPBuildersUseCapturedRoute(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		manager, execution, profile := newPassthroughMCPTestManager(t, "pi-acp")
		execution.SandboxSelection = &policy.SelectionContext{ExecutorID: "local"}
		route := ""
		if allowed {
			route = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
		}
		execution.sandboxAttempt = &startSandboxFixture{launch: &contract.Launch{MCPURL: route}}
		agent, err := manager.getAgentConfigForExecution(execution)
		require.NoError(t, err)
		servers, err := manager.passthroughMCPServers(t.Context(), execution, agent, profile, execution.ExecutorType, nil)
		require.NoError(t, err)
		found := false
		for _, server := range servers {
			if server.Name == "kandev" {
				found = true
				require.Equal(t, route, server.URL)
			}
		}
		require.Equal(t, allowed, found)
	}
}

func TestSandboxNativeExitBeforeBindingConsumesReceipt(t *testing.T) {
	testSandboxNativeLaunch(t, false, true)
}

type exitedSandboxRunner struct {
	*process.InteractiveRunner
	t *testing.T
}

func (r *exitedSandboxRunner) Start(ctx context.Context, request process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error) {
	info, err := r.InteractiveRunner.Start(ctx, request)
	if err != nil {
		return nil, err
	}
	require.Eventually(r.t, func() bool { _, err := r.SandboxOwner(info.ID); return err != nil }, time.Second, 10*time.Millisecond)
	return info, nil
}
