package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeLegacyCompatibilityDoesNotHideStorageErrors(t *testing.T) {
	for _, condition := range []string{"legacy", "legacy with v1 history", "unsupported route", "storage unavailable", "unauthorized"} {
		t.Run(condition, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				switch condition {
				case "unsupported route":
					w.WriteHeader(http.StatusNotFound)
				case "unauthorized":
					w.WriteHeader(http.StatusUnauthorized)
				default:
					reason := "storage_not_durable"
					if condition == "storage unavailable" {
						reason = "storage_unavailable"
					}
					_ = json.NewEncoder(w).Encode(journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Reason: reason}})
				}
			}))
			t.Cleanup(server.Close)
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			manager := newTestManager(t)
			if condition == "legacy with v1 history" {
				manager.streamManager.setAgentDeliveryRepository(&adoptionDeliveryRepository{submissions: map[string]*models.AgentDeliverySubmission{
					"interrupted": {ID: "interrupted", SessionID: "session", State: models.DeliverySubmissionInterruptedUnknown},
				}})
			}
			execution := &AgentExecution{ID: "execution", SessionID: "session", ACPSessionID: "native", agentctl: agentctl.NewClient(host, port, newTestLogger())}
			execution.setSessionInitialized(true)
			require.NoError(t, manager.executionStore.Add(execution))
			err = manager.AcknowledgeNativeResumeDelivery(context.Background(), "session")
			if condition == "legacy" || condition == "unsupported route" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
