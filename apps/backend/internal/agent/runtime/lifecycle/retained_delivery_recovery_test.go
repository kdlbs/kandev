package lifecycle

import (
	"context"
	"encoding/json"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRetrySessionDeliveryWithoutExecutionReplaysRetainedTerminal(t *testing.T) {
	root := t.TempDir()
	capability := journal.CheckStorage(root, "session")
	j, err := journal.Open(journal.Config{Path: capability.Path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = j.PutSubmission(ctx, journal.Submission{ID: "prompt", Hash: "hash", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", State: journal.SubmissionDispatching}); err != nil {
		t.Fatal(err)
	}
	_, err = j.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt", Type: "message_chunk", Payload: []byte(`{"type":"message_chunk","text":"retained recovery output"}`)})
	require.NoError(t, err)
	if _, err = j.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "prompt", Type: "complete", Terminal: true, Payload: []byte(`{"type":"complete"}`)}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	// The boundary uses a real exclusive journal on each authenticated request.
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/delivery/retained" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var request journal.RetainedRecoveryRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		result, err := journal.ReadRetainedRecovery(r.Context(), request)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	manager := newTestManager(t)
	manager.dataDir = root
	owner := runtimeOwnerForTestClient(t, client)
	manager.SetRuntimeOwner(owner)
	repository, database := runOwnerTestRepository(t)
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", Title: "Retained recovery"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "session"}))
	require.NoError(t, repository.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: "session", IncarnationID: "session", Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	_, err = repository.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "prompt", SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("original"), State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	manager.SetAgentDeliveryRepository(repository)
	result := manager.RecoverAgentPromptStreamWithIdentity(ctx, AgentDeliveryRecoveryIdentity{TaskID: "task", SessionID: "session", ExecutionID: "execution", SubmissionID: "prompt", StreamID: "stream", IncarnationID: "session", HarnessGeneration: 1, PromptGeneration: 1})
	if result.Outcome != DeliveryReconciliationTerminalSettled {
		t.Fatalf("retained recovery=%+v", result)
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor.ProjectedSequence)
	messages, err := repository.ListMessages(ctx, "session")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "retained recovery output", messages[0].Content)
	submission, err := repository.GetAgentDeliverySubmission(ctx, "prompt")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, submission.State)
	result = manager.RecoverAgentPromptStreamWithIdentity(ctx, AgentDeliveryRecoveryIdentity{TaskID: "task", SessionID: "session", ExecutionID: "execution", SubmissionID: "prompt", StreamID: "stream", IncarnationID: "session", HarnessGeneration: 1, PromptGeneration: 1})
	require.Equal(t, DeliveryReconciliationTerminalSettled, result.Outcome)
	var count int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ?`, "stream").Scan(&count))
	require.Equal(t, 2, count)
	messages, err = repository.ListMessages(ctx, "session")
	require.NoError(t, err)
	require.Len(t, messages, 1, "repeated retained replay must not duplicate canonical output")
	if _, exists := manager.GetExecutionBySessionID("session"); exists {
		t.Fatal("retry created a harness execution")
	}
}

func runtimeOwnerForTestClient(t *testing.T, client *agentctl.Client) *agentctl.RuntimeOwner {
	t.Helper()
	endpoint, err := url.Parse(client.BaseURL())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil {
		t.Fatal(err)
	}
	owner := agentctl.NewRuntimeOwner(nil, newTestLogger(), "test")
	t.Cleanup(owner.Stop)
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.Configure(endpoint.Hostname(), port, "secret", 0, nil); err != nil {
		t.Fatal(err)
	}
	if err = candidate.Commit(); err != nil {
		t.Fatal(err)
	}
	return owner
}
