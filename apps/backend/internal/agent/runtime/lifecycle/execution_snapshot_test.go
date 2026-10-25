package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotBeforeChatStart(t *testing.T) {
	testExecutionSnapshotChatStart(t, false)
}

func TestExecutionSnapshotBeforeKubernetesRefresh(t *testing.T) {
	testExecutionSnapshotChatStart(t, true)
}

func testExecutionSnapshotChatStart(t *testing.T, refresh bool) {
	t.Helper()
	for _, refuse := range []bool{false, true} {
		t.Run(strconv.FormatBool(refuse), func(t *testing.T) {
			var captured atomic.Bool
			var started atomic.Bool
			var configuredBeforeCapture atomic.Bool
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{launchwrapper.Capability}})
					return
				}
				if !captured.Load() {
					configuredBeforeCapture.Store(true)
				}
				if r.URL.Path == "/api/v1/start" {
					started.Store(true)
				}
				_, _ = w.Write([]byte(`{"success":true,"command":"agent"}`))
			}))
			t.Cleanup(peer.Close)
			host, portText, err := net.SplitHostPort(peer.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			transport := client.NewClient(host, port, newTestRegistryLogger())
			manager := &Manager{}
			execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task", AgentProfileID: "profile", ExecutorType: "local", AgentCommand: "agent", AgentArgs: []string{"/private/agent", "secret-argument"}, LaunchWrapper: launchwrapper.Profile([]string{"/private/wrapper", "secret-wrapper"})}
			persistenceErr := errors.New("snapshot write failed")
			manager.SetExecutionSnapshotRecorder(func(_ context.Context, got *AgentExecution, snapshot models.ExecutionSnapshot, sandbox *contract.Launch) error {
				require.Same(t, execution, got)
				require.Nil(t, sandbox)
				require.Equal(t, execution.SessionID, snapshot.SessionID)
				require.Equal(t, execution.ID, snapshot.ExecutionID)
				require.Equal(t, "chat", snapshot.Transport)
				require.NotEmpty(t, snapshot.AttemptID)
				require.NotNil(t, snapshot.Wrapper)
				raw, marshalErr := json.Marshal(snapshot)
				require.NoError(t, marshalErr)
				require.NotContains(t, string(raw), "/private")
				require.NotContains(t, string(raw), "secret-")
				captured.Store(true)
				if refuse {
					return persistenceErr
				}
				return nil
			})
			if refresh {
				_, err = manager.prepareRestartedKubernetesAgentctl(t.Context(), execution, &RemoteInstanceRefresh{Instance: &ExecutorInstance{Client: transport}})
			} else {
				_, err = manager.configureAndStartProcess(t.Context(), execution, transport, nil)
			}
			if refuse {
				require.ErrorIs(t, err, persistenceErr)
			} else {
				require.NoError(t, err)
			}
			require.True(t, captured.Load())
			require.Equal(t, !refuse, started.Load())
			require.False(t, configuredBeforeCapture.Load())
		})
	}
}
