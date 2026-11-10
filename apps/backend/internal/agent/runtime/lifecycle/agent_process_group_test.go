package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
)

var (
	agentGroupRuntime = processidentity.Identity{PID: 100, GroupID: 100, SessionID: 100, BirthToken: "linux:boot:1000"}
	agentGroupChild   = processidentity.Identity{PID: 200, GroupID: 200, SessionID: 100, BirthToken: "linux:boot:2000"}
)

func agentGroupStatusClient(t *testing.T, agent *processidentity.Identity) *agentctl.Client {
	t.Helper()
	return ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/delivery" {
			t.Errorf("unexpected delivery request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
			StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
			SessionID:         "session", StreamID: "stream", AgentProcess: agent,
		}); err != nil {
			t.Errorf("encode delivery status: %v", err)
		}
	})
}

func TestWithAgentProcessGroupRecordsReportedAgentGroup(t *testing.T) {
	ctx := context.Background()
	owner := withAgentProcessGroup(ctx, agentGroupStatusClient(t, &agentGroupChild), "stream", agentGroupRuntime)
	require.Equal(t, agentGroupRuntime.PID, owner.PID, "the runtime stays the primary identity")
	require.Equal(t, agentGroupChild.GroupID, owner.AgentGroupID)
	require.Equal(t, agentGroupChild.BirthToken, owner.AgentBirthToken)

	legacy := withAgentProcessGroup(ctx, agentGroupStatusClient(t, nil), "stream", agentGroupRuntime)
	require.Equal(t, agentGroupRuntime, legacy, "a peer without agent evidence keeps the runtime-only identity")

	nonLeader := agentGroupChild
	nonLeader.GroupID = 300
	unverifiable := withAgentProcessGroup(ctx, agentGroupStatusClient(t, &nonLeader), "stream", agentGroupRuntime)
	require.Zero(t, unverifiable.AgentGroupID, "an agent that does not lead its group is not recorded")
}

func TestBoundAgentProcessGroupReusesPromptBinding(t *testing.T) {
	execution := &AgentExecution{ID: "execution"}
	owner := agentGroupRuntime.WithAgentGroup(agentGroupChild)
	execution.setMetadataValue(deliveryPromptTurnBindingMetadataKey, deliveryPromptTurnBinding{
		OriginalRuntime: owner, ExecutionID: "execution", SubmissionID: "prompt:one",
	})

	require.Equal(t, owner, boundAgentProcessGroup(execution, agentGroupRuntime, "one"),
		"reconciliation reuses the agent group bound with the same runtime and submission")
	require.Equal(t, agentGroupRuntime, boundAgentProcessGroup(execution, agentGroupRuntime, "prompt:other"))

	replacement := agentGroupRuntime
	replacement.BirthToken = "linux:boot:9999"
	require.Equal(t, replacement, boundAgentProcessGroup(execution, replacement, "prompt:one"),
		"a different runtime never inherits another runtime's agent group")
}
