package cursorcloud

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

func TestReplayCheckpointPublishesEachEventOnce(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamEvents = []provider.StreamEvent{
		{ID: "1", Type: "assistant", Data: json.RawMessage(`{"text":"hello"}`)},
		{ID: "2", Type: "tool_call", Data: json.RawMessage(`{"callId":"call-1","name":"read_file","status":"completed","truncated":{"args":true}}`)},
	}
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("first ObserveOnce: %v", err)
	}
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "disconnect"); err != nil {
		t.Fatalf("replayed ObserveOnce: %v", err)
	}
	if len(providerClient.streamLastIDs) != 2 || providerClient.streamLastIDs[0] != "" || providerClient.streamLastIDs[1] != "2" {
		t.Fatalf("stream cursors = %v, want initial and committed cursor", providerClient.streamLastIDs)
	}
	if len(repo.streamMessages) != 2 || repo.streamMessages[0].Content != "hello" || repo.streamMessages[1].Type != models.MessageTypeToolCall {
		t.Fatalf("persisted messages = %#v, want one assistant and one tool message", repo.streamMessages)
	}
	if len(published) != 2 {
		t.Fatalf("published events = %d, want each replayed event once", len(published))
	}
	if published[0].Data == nil || published[0].Data.Type != "message_streaming" || published[0].Data.IsAppend {
		t.Fatalf("first assistant projection = %#v, want initial message chunk", published[0].Data)
	}
}

func TestTerminalResultSettlesAndPublishesOnce(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamEvents = []provider.StreamEvent{
		{ID: "1", Type: "assistant", Data: json.RawMessage(`{"text":"partial"}`)},
		{ID: "2", Type: "result", Data: json.RawMessage(`{"status":"FINISHED","text":"complete result"}`)},
	}
	providerClient.getRunStatus = runStatusFinished
	providerClient.getRun.Git = &provider.GitResult{Branches: []provider.GitBranch{
		{RepositoryURL: "github.com/acme/widget", Branch: "cursor/fix", PRURL: "https://github.com/acme/widget/pull/9"},
	}}
	providerClient.agent = provider.Agent{URL: "https://cursor.com/agents/bc-123"}
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("ObserveOnce: %v", err)
	}
	if repo.operation.State != models.ManagedAgentSubmissionSucceeded {
		t.Fatalf("operation state = %s, want succeeded after provider readback", repo.operation.State)
	}
	wantResult := models.ManagedAgentResultSnapshot{
		RepositoryID: input.Binding.Launch.RepositoryID, Branch: "cursor/fix",
		PullRequestURL: "https://github.com/acme/widget/pull/9", AgentURL: "https://cursor.com/agents/bc-123",
	}
	if repo.operation.ResultSnapshot != wantResult {
		t.Fatalf("result snapshot = %+v, want %+v", repo.operation.ResultSnapshot, wantResult)
	}
	if len(repo.streamMessages) != 1 || repo.streamMessages[0].Content != "complete result" {
		t.Fatalf("assistant result = %#v, want final result reconciled into the existing message", repo.streamMessages)
	}
	if len(published) != 2 || published[len(published)-1].Data == nil || published[len(published)-1].Data.Type != "complete" {
		t.Fatalf("published terminal events = %#v, want one completion after provider confirmation", published)
	}
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "disconnect"); err != nil {
		t.Fatalf("terminal replay ObserveOnce: %v", err)
	}
	if len(published) != 2 {
		t.Fatalf("published events after terminal replay = %d, want exactly 2", len(published))
	}
}

func TestTerminalRunReadbackReplacesAssistantResultAfterHistoryGap(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamErr = &provider.APIError{StatusCode: 410, Code: "stream_expired"}
	providerClient.getRunStatus = runStatusFinished
	providerClient.getRun.Result = "final answer from run readback"
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("ObserveOnce after history expiry: %v", err)
	}
	if len(repo.streamMessages) != 1 || repo.streamMessages[0].Content != providerClient.getRun.Result {
		t.Fatalf("assistant result after history expiry = %#v, want provider readback", repo.streamMessages)
	}
	if repo.operation.ResultSnapshot.AssistantResult != providerClient.getRun.Result {
		t.Fatalf("durable assistant result = %q, want provider readback", repo.operation.ResultSnapshot.AssistantResult)
	}
	if len(published) != 2 || published[0].Data.Type != "message_streaming" || published[0].Data.Text != providerClient.getRun.Result ||
		published[0].Data.IsAppend || published[0].Data.MessageUpdated ||
		published[0].Data.MessageID != "cursor-cloud-assistant-"+repo.operation.ID {
		t.Fatalf("readback publications = %#v, want a stable replacement before completion", published)
	}
}

func TestTerminalRunReadbackReplacesPartialAssistantResult(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamEvents = []provider.StreamEvent{
		{ID: "1", Type: "assistant", Data: json.RawMessage(`{"text":"partial answer"}`)},
	}
	providerClient.streamErr = &provider.APIError{StatusCode: 410, Code: "stream_expired"}
	providerClient.getRunStatus = runStatusFinished
	providerClient.getRun.Result = "complete answer from run readback"
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("ObserveOnce after partial stream expiry: %v", err)
	}
	if len(repo.streamMessages) != 1 || repo.streamMessages[0].Content != providerClient.getRun.Result {
		t.Fatalf("assistant result after partial stream = %#v, want final replacement without duplication", repo.streamMessages)
	}
	if repo.operation.ResultSnapshot.AssistantResult != providerClient.getRun.Result {
		t.Fatalf("durable assistant result = %q, want provider readback", repo.operation.ResultSnapshot.AssistantResult)
	}
	checkpoint, err := repo.GetManagedAgentStreamCheckpoint(context.Background(), input.Binding.ID, "run-1")
	if err != nil || checkpoint.LastEventID != "1" {
		t.Fatalf("provider stream cursor after result readback = %+v, %v; want the last provider event ID", checkpoint, err)
	}
	if len(published) != 3 || published[0].Data.Text != "partial answer" || published[1].Data.Text != providerClient.getRun.Result ||
		published[1].Data.IsAppend || !published[1].Data.MessageUpdated || published[1].Data.MessageID != published[0].Data.MessageID {
		t.Fatalf("partial and readback publications = %#v, want same-message replacement before completion", published)
	}
}

func TestTerminalCompletionReplaysAfterPersistenceBeforeDeliveryAndAcknowledgesOnce(t *testing.T) {
	providerClient := observerProvider()
	providerClient.getRunStatus = runStatusFinished
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)
	runtime.publishStream = nil

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("terminal settlement before simulated crash: %v", err)
	}
	if !repo.operation.CompletionPending {
		t.Fatal("terminal settlement did not persist the pending completion before publication")
	}

	runtime.publishStream = func(_ context.Context, event *lifecycle.AgentStreamEventPayload) {
		published = append(published, event)
	}
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("replay pending terminal completion: %v", err)
	}
	if len(published) != 1 || published[0].Data.Type != "complete" {
		t.Fatalf("replayed completion = %#v, want one completion delivery", published)
	}
	if providerClient.getRunCalls != 1 || providerClient.streamCalls != 1 {
		t.Fatalf("provider calls after completion replay = stream %d run %d, want no remote re-read", providerClient.streamCalls, providerClient.getRunCalls)
	}
	if err := repo.AcknowledgeManagedAgentCompletion(context.Background(), repo.operation.ID); err != nil {
		t.Fatalf("acknowledge orchestrator completion: %v", err)
	}
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "disconnect"); err != nil {
		t.Fatalf("observe acknowledged completion: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("completion deliveries after acknowledgement = %d, want one", len(published))
	}
}

func TestStatusFallbackIsRateLimited(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamErr = errors.New("stream disconnected")
	var published []*lifecycle.AgentStreamEventPayload
	_, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("status fallback after first disconnect: %v", err)
	}
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "disconnect"); err == nil {
		t.Fatal("second disconnected observation unexpectedly succeeded")
	}
	if providerClient.getRunCalls != 1 {
		t.Fatalf("status fallback calls = %d, want one call in the rate window", providerClient.getRunCalls)
	}
}

func TestCloudResultIdentity(t *testing.T) {
	binding := testLaunchInput().Binding
	run := provider.Run{Git: &provider.GitResult{Branches: []provider.GitBranch{
		{RepositoryURL: "https://github.com/other/repo", Branch: "wrong/repo", PRURL: "https://github.com/other/repo/pull/1"},
		{RepositoryURL: "github.com/acme/widget.git", Branch: "cursor/fix", PRURL: "https://github.com/acme/widget/pull/12"},
		{RepositoryURL: "https://github.com/acme/widget", Branch: "untrusted", PRURL: "https://github.com/other/repo/pull/3"},
	}}}
	got := resultSnapshotForRun(binding, run)
	if got.RepositoryID != binding.Launch.RepositoryID || got.Branch != "cursor/fix" ||
		got.PullRequestURL != "https://github.com/acme/widget/pull/12" {
		t.Fatalf("result snapshot = %+v, want only the bound repository's branch and PR", got)
	}
}

func TestRemoteLivenessProbeSettlesOnlyProviderConfirmedTerminalState(t *testing.T) {
	providerClient := observerProvider()
	providerClient.getRunStatus = runStatusFinished
	var published []*lifecycle.AgentStreamEventPayload
	repo, managedRuntime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, managedRuntime, input)

	state, err := managedRuntime.ProbeRemoteLiveness(context.Background(), input.Binding.ExecutionID)
	if err != nil || state != RemoteLivenessTerminal {
		t.Fatalf("terminal liveness = %q, %v; want terminal", state, err)
	}
	if repo.operation.State != models.ManagedAgentSubmissionSucceeded {
		t.Fatalf("operation state = %s, want succeeded", repo.operation.State)
	}
	if len(published) != 1 || published[0].Data.Type != "complete" {
		t.Fatalf("terminal events = %#v, want one completion", published)
	}
	if _, err := managedRuntime.ProbeRemoteLiveness(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 {
		t.Fatalf("terminal event replay count = %d, want one", len(published))
	}
}

func TestRemoteLivenessProbeFailsClosedAndHonorsRetryAfter(t *testing.T) {
	providerClient := observerProvider()
	providerClient.getRunErr = &provider.APIError{StatusCode: 429, RetryAfter: time.Minute}
	_, managedRuntime, input := newObserverRuntime(t, providerClient, nil)
	startObserverRuntime(t, managedRuntime, input)
	state, err := managedRuntime.ProbeRemoteLiveness(context.Background(), input.Binding.ExecutionID)
	if state != RemoteLivenessUnknown || err == nil {
		t.Fatalf("failed provider liveness = %q, %v; want unknown with error", state, err)
	}
	if delay := observationRetryDelay(0, &provider.APIError{RetryAfter: time.Minute}); delay < time.Minute {
		t.Fatalf("retry delay = %s, want at least Retry-After", delay)
	}
}

func TestRetentionGapPreservesMessagesAndDoesNotTrustUnknownStatus(t *testing.T) {
	providerClient := observerProvider()
	providerClient.streamErr = &provider.APIError{StatusCode: 410, Code: "stream_expired"}
	providerClient.getRunStatus = "WAITING"
	var published []*lifecycle.AgentStreamEventPayload
	repo, runtime, input := newObserverRuntime(t, providerClient, &published)
	startObserverRuntime(t, runtime, input)

	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "backend_restart"); err != nil {
		t.Fatalf("ObserveOnce after retention expiry: %v", err)
	}
	checkpoint, err := repo.GetManagedAgentStreamCheckpoint(context.Background(), input.Binding.ID, "run-1")
	if err != nil || !checkpoint.HistoryGap {
		t.Fatalf("history checkpoint = %+v err=%v, want a durable gap marker", checkpoint, err)
	}
	if repo.operation.State != models.ManagedAgentSubmissionAccepted {
		t.Fatalf("unknown provider status settled operation as %s", repo.operation.State)
	}
	providerClient.streamErr = nil
	if err := runtime.ObserveOnce(context.Background(), input.Binding.ExecutionID, "disconnect"); err != nil {
		t.Fatalf("ObserveOnce after gap: %v", err)
	}
	if providerClient.streamLastIDs[len(providerClient.streamLastIDs)-1] != "" {
		t.Fatalf("stream cursor after retention gap = %q, want replay from the retained window", providerClient.streamLastIDs[len(providerClient.streamLastIDs)-1])
	}
}

func observerProvider() *fakeProvider {
	return &fakeProvider{createResponse: provider.CreateAgentResponse{
		Agent: provider.Agent{ID: "bc-00000000-0000-4000-8000-000000000001", LatestRunID: "run-1"},
		Run:   provider.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: "RUNNING"},
	}, getRun: provider.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: "RUNNING"}}
}

func newObserverRuntime(t *testing.T, providerClient *fakeProvider, published *[]*lifecycle.AgentStreamEventPayload) (*memoryRepository, *Runtime, LaunchInput) {
	t.Helper()
	repo := newMemoryRepository()
	input := testLaunchInput()
	input.Operation.RequestSnapshot.TurnID = "turn-1"
	managedRuntime, err := New(Config{
		Repository: repo,
		ClientFactory: func(context.Context, *models.ManagedAgentBinding) (Provider, error) {
			return providerClient, nil
		},
		GrantIssuer: GrantIssuerFunc(func(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation) (MCPGrant, error) {
			return MCPGrant{URL: "https://callback.example.test/grant", Token: "token"}, nil
		}),
		ProjectStream: projectRecoveryEvent,
		PublishStream: func(ctx context.Context, event *lifecycle.AgentStreamEventPayload) {
			*published = append(*published, event)
			if event.ManagedAgentOperationID != "" && event.Data != nil && event.Data.Type == "complete" {
				if err := repo.AcknowledgeManagedAgentCompletion(ctx, event.ManagedAgentOperationID); err != nil {
					t.Errorf("acknowledge delivered completion: %v", err)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return repo, managedRuntime, input
}

func startObserverRuntime(t *testing.T, runtime *Runtime, input LaunchInput) {
	t.Helper()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
}

func projectRecoveryEvent(_ context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, event provider.StreamEvent) (*StreamProjection, error) {
	payload := &lifecycle.AgentStreamEventPayload{
		Type: "agent/event", AgentID: binding.ExecutionID, ExecutionID: binding.ExecutionID,
		TaskID: binding.TaskID, SessionID: binding.SessionID,
	}
	switch event.Type {
	case "assistant":
		var data struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, err
		}
		payload.Data = &lifecycle.AgentStreamEventData{Type: "message_streaming", Text: data.Text}
		return &StreamProjection{
			Message: &models.Message{
				ID: "cursor-cloud-assistant-" + operation.ID, TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
				TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
				AuthorID: binding.ExecutionID, Type: models.MessageTypeMessage, Content: data.Text,
			},
			Payload: payload, AppendMessage: true,
		}, nil
	case "tool_call":
		var data struct {
			CallID    string          `json:"callId"`
			Name      string          `json:"name"`
			Status    string          `json:"status"`
			Truncated map[string]bool `json:"truncated"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, err
		}
		payload.Data = &lifecycle.AgentStreamEventData{Type: "tool_call", ToolCallID: data.CallID, ToolName: data.Name, ToolStatus: data.Status}
		return &StreamProjection{
			Message: &models.Message{
				ID: "tool-" + data.CallID, TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
				TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
				AuthorID: binding.ExecutionID, Type: models.MessageTypeToolCall, Content: data.Name,
				Metadata: map[string]interface{}{"call_id": data.CallID, "name": data.Name, "status": data.Status, "truncated": data.Truncated},
			},
			Payload: payload,
		}, nil
	case "result":
		var data struct {
			Status string `json:"status"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return nil, err
		}
		var message *models.Message
		if data.Text != "" {
			message = &models.Message{
				ID: "cursor-cloud-assistant-" + operation.ID, TaskSessionID: binding.SessionID, TaskID: binding.TaskID,
				TurnID: operation.RequestSnapshot.TurnID, AuthorType: models.MessageAuthorAgent,
				AuthorID: binding.ExecutionID, Type: models.MessageTypeMessage, Content: data.Text,
			}
		}
		return &StreamProjection{Message: message, Payload: payload, TerminalStatus: data.Status}, nil
	default:
		return nil, nil
	}
}
