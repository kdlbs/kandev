package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestExistingInstanceErrorDoesNotProveProcessTermination(t *testing.T) {
	var port int
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var result any
		switch r.URL.Path {
		case "/api/v1/instances/execution":
			result = agentctl.InstanceInfo{ID: "execution", TaskID: "task", SessionID: "session", Port: port}
		case "/api/v1/status":
			result = agentctl.StatusResponse{AgentStatus: "error"}
		case "/api/v1/agent/delivery":
			result = journal.RecoveryDescriptor{StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentVersion}, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
		case "/api/v1/agent/submissions/prompt":
			result = journal.Submission{ID: "prompt", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", State: journal.SubmissionInterruptedUnknown}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(result))
	})
	endpoint, err := url.Parse(client.BaseURL())
	require.NoError(t, err)
	port, err = strconv.Atoi(endpoint.Port())
	require.NoError(t, err)
	manager := newTestManager(t)
	manager.SetRuntimeOwner(runtimeOwnerForTestClient(t, client))
	result := manager.RecoverAgentPromptStreamWithIdentity(context.Background(), AgentDeliveryRecoveryIdentity{TaskID: "task", SessionID: "session", ExecutionID: "execution", SubmissionID: "prompt", StreamID: "stream", IncarnationID: "incarnation", HarnessGeneration: 1, PromptGeneration: 1})
	require.Equal(t, DeliveryReconciliationUncertain, result.Outcome)
	require.False(t, result.ProcessTerminated, "a status string cannot attest process and descendant termination")
}
