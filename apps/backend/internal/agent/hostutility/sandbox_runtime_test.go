//go:build linux

package hostutility

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

type utilityAttemptFixture struct {
	launch                *sandbox.Launch
	marked, bound, closed atomic.Bool
	needsReceipt          bool
	receiptApplied        atomic.Bool
}

func (a *utilityAttemptFixture) Launch() *sandbox.Launch                           { return a.launch.Clone() }
func (a *utilityAttemptFixture) ValidateCommand([]string, map[string]string) error { return nil }
func (a *utilityAttemptFixture) MarkStart(context.Context) error                   { a.marked.Store(true); return nil }
func (a *utilityAttemptFixture) BindOwner(_ context.Context, _ string, owner *sandbox.ProcessOwner) error {
	a.bound.Store(owner.AttemptID == a.launch.AttemptID)
	return nil
}
func (a *utilityAttemptFixture) Close() error {
	if a.needsReceipt && !a.receiptApplied.Load() {
		return errors.New("start result is unknown")
	}
	a.closed.Store(true)
	return nil
}
func (a *utilityAttemptFixture) ApplyStartReceipt(_ context.Context, _ string, receipt *sandbox.StartReceipt) error {
	if err := receipt.ValidateStopped(a.launch); err != nil {
		return err
	}
	a.receiptApplied.Store(true)
	return nil
}

func TestHostUtilitySelectedSandboxRequiresOwnershipAndCleanup(t *testing.T) {
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, files.Close()) }()
	attempt := &utilityAttemptFixture{launch: &sandbox.Launch{Version: 1, AttemptID: "utility-attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}}
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, process.Start())
	defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
	identity, err := processidentity.Capture(process.Process.Pid)
	require.NoError(t, err)
	owner := sandbox.ProcessOwner{AttemptID: attempt.launch.AttemptID, Generation: 1, PolicyDigest: attempt.launch.PolicyDigest, Supervisor: identity}
	var acknowledged atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{sandbox.LaunchCapability}}))
			return
		}
		if r.Method == http.MethodGet {
			require.NoError(t, json.NewEncoder(w).Encode(owner))
			return
		}
		require.True(t, attempt.bound.Load())
		acknowledged.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	inst := &instance{instanceID: "instance", client: client.NewClient(host, port, log)}
	profile := &ProfileProbeContext{SandboxSelection: &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}}
	cfg := &utility.InferenceConfigDTO{Command: []string{"agent"}, Env: map[string]string{"KEY": "value"}}
	manager := &Manager{}
	called := false
	err = manager.runSandboxUtility(t.Context(), inst, profile, cfg, func(context.Context) error { called = true; return nil })
	require.Error(t, err)
	require.False(t, called)
	manager.SetSandboxRuntime(func(_ context.Context, received *ProfileProbeContext, config *utility.InferenceConfigDTO, _ *client.Client) (SandboxAttempt, error) {
		require.Same(t, profile, received)
		require.Same(t, cfg, config)
		return attempt, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	err = manager.runSandboxUtility(ctx, inst, profile, cfg, func(ctx context.Context) error {
		require.True(t, attempt.marked.Load())
		require.Equal(t, attempt.launch, cfg.Sandbox)
		for !acknowledged.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, attempt.closed.Load())
}

func TestHostUtilityFailedSpawnConsumesReceiptBeforeCleanup(t *testing.T) {
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, files.Close()) }()
	launch := &sandbox.Launch{Version: 1, AttemptID: "failed-start", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
	attempt := &utilityAttemptFixture{launch: launch, needsReceipt: true}
	var receiptRead atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{sandbox.LaunchCapability}}))
		case "/api/v1/inference/sandbox/failed-start/receipt":
			receiptRead.Store(true)
			require.True(t, attempt.marked.Load())
			require.NoError(t, json.NewEncoder(w).Encode(sandbox.StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Outcome: sandbox.StartNotStarted}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	inst := &instance{instanceID: "instance", client: client.NewClient(host, port, log)}
	manager := &Manager{}
	manager.SetSandboxRuntime(func(context.Context, *ProfileProbeContext, *utility.InferenceConfigDTO, *client.Client) (SandboxAttempt, error) {
		return attempt, nil
	})
	profile := &ProfileProbeContext{SandboxSelection: &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}}
	cfg := &utility.InferenceConfigDTO{Command: []string{"agent"}, Env: map[string]string{}}
	err = manager.runSandboxUtility(t.Context(), inst, profile, cfg, func(context.Context) error { return errors.New("failed to start") })
	require.Error(t, err, "cleanup must not turn a failed probe into success")
	require.True(t, receiptRead.Load(), "an unknown startup result needs an authenticated receipt")
	require.True(t, attempt.receiptApplied.Load())
	require.True(t, attempt.closed.Load())
	require.False(t, attempt.bound.Load())
}
