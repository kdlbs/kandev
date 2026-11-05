//go:build linux

package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processcontainment"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxLifecycleExitBeforeBindingConsumesReceipt(t *testing.T) {
	group, err := processcontainment.Create()
	if err != nil && os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated cgroup v2")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, group.Kill(context.Background())); require.NoError(t, group.Remove()) })
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	files, err := contract.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	attempt := &startSandboxFixture{launch: &contract.Launch{Containment: group, Version: 1, AttemptID: "early-exit", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}}
	var owner atomic.Pointer[contract.ProcessOwner]
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{contract.LaunchCapability}}))
		case "/api/v1/agent/configure":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			require.True(t, attempt.marked)
			command := exec.Command("sleep", "30")
			command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			lease, e := group.Configure(command)
			require.NoError(t, e)
			require.NoError(t, command.Start())
			require.NoError(t, lease.Close())
			captured, e := contract.CaptureProcessOwner(attempt.launch, command.Process.Pid)
			require.NoError(t, e)
			owner.Store(captured)
			require.NoError(t, command.Process.Kill())
			require.Error(t, command.Wait())
			_, _ = w.Write([]byte(`{"success":true,"command":"agent"}`))
		case "/api/v1/agent/sandbox/early-exit/receipt":
			require.NoError(t, json.NewEncoder(w).Encode(contract.StartReceipt{Version: 1, AttemptID: attempt.launch.AttemptID, Generation: 1, PolicyDigest: attempt.launch.PolicyDigest, Outcome: contract.StartTerminated, Owner: owner.Load()}))
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
	execution := &AgentExecution{ID: "execution", AgentArgs: []string{"agent"}, SandboxSelection: &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}, sandboxAttempt: attempt}
	manager := &Manager{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = manager.configureAndStartProcess(ctx, execution, client.NewClient(host, port, log), nil)
	require.Error(t, err)
	require.True(t, attempt.receiptApplied, "an exited supervisor needs completion evidence after binding fails")
	require.False(t, attempt.bound)
	require.NoError(t, execution.closeSandbox())
}
