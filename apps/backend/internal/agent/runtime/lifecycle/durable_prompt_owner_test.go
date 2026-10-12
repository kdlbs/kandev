package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDurablePromptBindingRetainsHistoricalProcessProof(t *testing.T) {
	process, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Skip(err)
	}
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "proof")
	t.Cleanup(owner.Stop)
	binding, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, binding.SetProcessIdentity(process))
	require.NoError(t, binding.Configure("127.0.0.1", 1234, "secret", process.PID, nil))
	require.NoError(t, binding.Commit())
	lease, err := owner.Acquire(context.Background())
	require.NoError(t, err)
	defer lease.Close()
	sm, execution, _ := initialDeliveryPersistenceFixture(t)
	execution.agentctl = lease.NewBoundInstanceClient(1234, newTestLogger())
	execution.DeliveryStreamID = "stream"
	execution.promptGeneration = 3
	execution.setPromptTurnID("turn")
	require.NoError(t, sm.persistDurablePromptTurnBinding(context.Background(), execution, "source"))
	raw, err := json.Marshal(buildRunningFromExecution(execution, nil).Metadata)
	require.NoError(t, err)
	var metadata map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &metadata))
	var saved struct {
		OriginalRuntime  processidentity.Identity `json:"original_runtime"`
		PromptGeneration uint64                   `json:"prompt_generation"`
	}
	require.NoError(t, json.Unmarshal(metadata[deliveryPromptTurnBindingMetadataKey], &saved))
	require.Equal(t, process, saved.OriginalRuntime)
	require.EqualValues(t, 3, saved.PromptGeneration)
}

func TestDeliveryRecordUsesPersistedHistoricalBinding(t *testing.T) {
	process, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Skip(err)
	}
	for _, condition := range []string{"historical", "missing birth", "foreign prompt", "unknown generation"} {
		t.Run(condition, func(t *testing.T) {
			manager := newTestManager(t)
			binding := deliveryPromptTurnBinding{TaskID: "task", SessionID: "session", ExecutionID: "original",
				IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt:old",
				TurnID: "turn", OriginalRuntime: process, PromptGeneration: 3}
			switch condition {
			case "missing birth":
				binding.OriginalRuntime = processidentity.Identity{}
			case "foreign prompt":
				binding.SubmissionID = "prompt:other"
			case "unknown generation":
				binding.PromptGeneration = 0
			}
			manager.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: &models.ExecutorRunning{
				TaskID: "task", SessionID: "session", AgentExecutionID: "replacement",
				Metadata: map[string]interface{}{deliveryPromptTurnBindingMetadataKey: binding}}})
			replacement := createTestExecution("replacement", "task", "session")
			require.NoError(t, manager.executionStore.Add(replacement))
			descriptor := journal.RecoveryDescriptor{SubmissionCount: 1, Submissions: []journal.SubmissionSummary{{
				ID: "prompt:old", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}}}
			identity := manager.deliveryRecordControlIdentity(context.Background(), DeliveryRecordEvidenceRequest{
				TaskID: "task", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1}, descriptor)
			if condition != "historical" {
				require.Nil(t, identity)
				return
			}
			require.NotNil(t, identity, "an idle replacement cannot erase the original prompt's persisted process proof")
			require.Equal(t, "original", identity.ExecutionID)
			require.Equal(t, process, identity.OriginalRuntime)
			require.EqualValues(t, 3, identity.PromptGeneration)
		})
	}
}
