package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRecordEvidenceUsesOnlyExactPromptOwner(t *testing.T) {
	ctx := context.Background()
	descriptor := journal.RecoveryDescriptor{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 7,
		SubmissionCount: 1, Submissions: []journal.SubmissionSummary{{
			ID: "prompt:one", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7, StreamID: "stream",
		}},
	}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instances":
			_, _ = w.Write([]byte(`{"instances":[]}`))
		case "/api/v1/delivery/retained/evidence":
			_ = json.NewEncoder(w).Encode(journal.RetainedReconstructionEvidence{Descriptor: descriptor})
		default:
			http.NotFound(w, r)
		}
	})
	endpoint, err := url.Parse(client.BaseURL())
	require.NoError(t, err)
	port, err := strconv.Atoi(endpoint.Port())
	require.NoError(t, err)
	process, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Skip(err)
	}
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "boot")
	t.Cleanup(owner.Stop)
	binding, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, binding.SetProcessIdentity(process))
	require.NoError(t, binding.Configure(endpoint.Hostname(), port, "secret", process.PID, nil))
	require.NoError(t, binding.Commit())
	lease, err := owner.Acquire(ctx)
	require.NoError(t, err)
	defer lease.Close()
	manager := newTestManager(t)
	manager.dataDir = t.TempDir()
	manager.SetRuntimeOwner(owner)
	execution := createTestExecution("original", "task", "session")
	execution.agentctl = lease.NewBoundInstanceClient(port, newTestLogger())
	execution.DeliveryMode = DurableDeliveryV1
	execution.DeliveryIncarnationID = "incarnation"
	execution.DeliveryHarnessGeneration = 7
	execution.DeliveryStreamID = "stream"
	execution.promptGeneration = 9
	execution.dispatchedPromptGeneration = 9
	execution.setDeliverySubmissionID("prompt:one")
	require.NoError(t, manager.executionStore.Add(execution))
	request := DeliveryRecordEvidenceRequest{TaskID: "task", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 7}
	evidence, err := manager.InspectDeliveryRecordEvidence(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, evidence.ControlIdentity)
	require.Equal(t, process, evidence.ControlIdentity.OriginalRuntime)
	require.Equal(t, "original", evidence.ControlIdentity.ExecutionID)
	require.EqualValues(t, 9, evidence.ControlIdentity.PromptGeneration)
	execution.promptGeneration = 10
	evidence, err = manager.InspectDeliveryRecordEvidence(ctx, request)
	require.NoError(t, err)
	require.Nil(t, evidence.ControlIdentity, "admitting a successor must not pair its generation with the prior submission")
	execution.promptGeneration = 9
	execution.recoveredPromptGenerationPending.Store(true)
	evidence, err = manager.InspectDeliveryRecordEvidence(ctx, request)
	require.NoError(t, err)
	require.Nil(t, evidence.ControlIdentity, "a recreated generation is not historical prompt proof")
	execution.recoveredPromptGenerationPending.Store(false)
	execution.setDeliverySubmissionID("prompt:newer")
	evidence, err = manager.InspectDeliveryRecordEvidence(ctx, request)
	require.NoError(t, err)
	require.Nil(t, evidence.ControlIdentity, "a successor prompt cannot provide historical identity")
	execution.setDeliverySubmissionID("")
	evidence, err = manager.InspectDeliveryRecordEvidence(ctx, request)
	require.NoError(t, err)
	require.Nil(t, evidence.ControlIdentity, "an idle execution cannot provide historical identity")
}
