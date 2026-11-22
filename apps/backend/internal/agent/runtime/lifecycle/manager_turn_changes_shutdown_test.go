package lifecycle

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/turnchanges"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestTurnChangeCapturePreservesShutdownDeliveryOutcome(t *testing.T) {
	for _, test := range []struct {
		name, reason     string
		stopFails, force bool
		want             []string
	}{
		{"backend shutdown", StopReasonBackendShutdown, false, false, []string{"stop", "capture"}},
		{"unconfirmed backend shutdown", StopReasonBackendShutdown, true, false, []string{"stop"}},
		{"forced backend shutdown", StopReasonBackendShutdown, false, true, nil},
		{"explicit stop", "user stop", false, false, []string{"cancel", "capture", "stop"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, backend := newDetachTestManager(t, false)
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			var mu sync.Mutex
			var calls []string
			record := func(call string) {
				mu.Lock()
				calls = append(calls, call)
				mu.Unlock()
			}
			original := mock.server.Config.Handler
			mock.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/stop":
					record("stop")
					if test.stopFails {
						http.Error(w, "stop failed", http.StatusInternalServerError)
						return
					}
				case "/api/v1/git/turn-changes/checkpoint":
					record("capture")
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{}`))
					return
				}
				original.ServeHTTP(w, r)
			})
			mock.handler = func(message ws.Message) *ws.Message {
				if message.Action == "agent.cancel" {
					record("cancel")
					response, _ := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true})
					return response
				}
				return mock.defaultHandler(message)
			}
			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))
			waitForWSConnected(t, mock)
			execution := &AgentExecution{
				ID: "execution", TaskID: "task", SessionID: "session", TaskEnvironmentID: "environment",
				RuntimeName: executor.NameStandalone, Status: v1.AgentStatusRunning, agentctl: client,
				promptGeneration: 1, promptTurnIDs: map[uint64]string{1: "turn"},
			}
			execution.dispatchedPromptPending.Store(true)
			require.NoError(t, manager.executionStore.Add(execution))
			manager.SetTurnChangeCaptureHandler(&shutdownCheckpointHandler{})

			require.NoError(t, manager.StopAgentWithReason(ctx, execution.ID, test.reason, test.force))
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, test.want, calls)
			require.Len(t, backend.stopInstanceCalls, 1)
		})
	}
}

type shutdownCheckpointHandler struct{}

func (*shutdownCheckpointHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (*shutdownCheckpointHandler) FinishTurnChanges(ctx context.Context, _ TurnChangeTerminal, client TurnChangeCheckpointClient) error {
	_, err := client.CaptureTurnCheckpoint(ctx, turnchanges.CheckpointRequest{})
	return err
}
