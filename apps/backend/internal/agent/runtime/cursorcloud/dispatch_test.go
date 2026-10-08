package cursorcloud

import (
	"context"
	"errors"
	"expvar"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

func TestLaunchReservesBeforeProviderSubmission(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{}
	runtime := newTestRuntime(t, repository, provider, nil)
	input := testLaunchInput()

	ref, err := runtime.Launch(context.Background(), launchSpec(input))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if ref.ID != input.Binding.ExecutionID || ref.SessionID != input.Binding.SessionID {
		t.Fatalf("Launch ref = %#v, want persisted execution identity", ref)
	}
	if provider.createCalls != 0 {
		t.Fatalf("Launch called provider %d times, want no submission before StartExecution", provider.createCalls)
	}
	if repository.binding == nil || repository.operation == nil || repository.operation.State != models.ManagedAgentSubmissionReserved {
		t.Fatalf("reservation not persisted: binding=%#v operation=%#v", repository.binding, repository.operation)
	}
}

func TestCreateDispatchReconcilesAcceptedOperation(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: "bc-00000000-0000-4000-8000-000000000001", LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: "CREATING"},
	}}
	runtime := newTestRuntime(t, repository, provider, nil)
	input := testLaunchInput()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("replayed StartExecution: %v", err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("CreateAgent calls = %d, want 1", provider.createCalls)
	}
	if repository.operation.State != models.ManagedAgentSubmissionAccepted || repository.operation.RemoteRunID != "run-1" {
		t.Fatalf("operation = %#v, want accepted run-1", repository.operation)
	}
}

func TestUnknownCreateReconcilesStableAgentIdentityWithoutResubmitting(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{
		createErr:   cursorcloud.ErrOutcomeUnknown,
		getAgentErr: errors.New("temporary read failure"),
	}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("first StartExecution error = %v, want unknown outcome", err)
	}
	if repository.operation.State != models.ManagedAgentSubmissionUnknown {
		t.Fatalf("first operation state = %s, want unknown", repository.operation.State)
	}
	provider.getAgentErr = nil
	provider.agent = cursorcloud.Agent{ID: input.Binding.RemoteAgentID, LatestRunID: "run-recovered"}
	provider.getRun = cursorcloud.Run{ID: "run-recovered", AgentID: input.Binding.RemoteAgentID, Status: "RUNNING"}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("reconciled StartExecution: %v", err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("CreateAgent calls = %d, want one stable submission", provider.createCalls)
	}
	if repository.operation.State != models.ManagedAgentSubmissionAccepted || repository.operation.RemoteRunID != "run-recovered" {
		t.Fatalf("reconciled operation = %#v, want accepted run-recovered", repository.operation)
	}
}

func TestSerializedFollowupsUseTurnIdentityAndRejectConflictingReplay(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: "bc-00000000-0000-4000-8000-000000000001", LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: "RUNNING"},
	}}
	runtime := newTestRuntime(t, repository, provider, nil)
	input := testLaunchInput()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	provider.createRun = cursorcloud.Run{ID: "run-2", AgentID: input.Binding.RemoteAgentID, Status: "RUNNING"}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "first follow-up"); err != nil {
		t.Fatalf("first follow-up: %v", err)
	}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "first follow-up"); err != nil {
		t.Fatalf("replayed follow-up: %v", err)
	}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "different prompt"); err == nil {
		t.Fatal("same turn identity accepted a different prompt")
	}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-3", "queued follow-up"); err == nil {
		t.Fatal("second follow-up dispatched while the first remote turn was still active")
	}
	if provider.createRunCalls != 1 {
		t.Fatalf("CreateRun calls = %d, want exactly one accepted follow-up", provider.createRunCalls)
	}
	if repository.operation.PromptTurnID != "turn-2" || repository.operation.State != models.ManagedAgentSubmissionAccepted {
		t.Fatalf("journal operation = %#v, want turn-2 accepted", repository.operation)
	}
}

func TestFollowupJournalReadFailureFailsClosed(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: "bc-00000000-0000-4000-8000-000000000001", LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: runStatusFinished},
	}}
	runtime := newTestRuntime(t, repository, provider, nil)
	input := testLaunchInput()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	repository.getOperationErr = errors.New("journal unavailable")
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "follow up"); err == nil || errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("ResumeWithTurnID error = %v, want journal read error", err)
	}
	if provider.createRunCalls != 0 {
		t.Fatalf("CreateRun calls = %d, want no provider dispatch after journal read error", provider.createRunCalls)
	}
}

func TestCancelActiveWaitsForRemoteTerminalConfirmation(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: "bc-00000000-0000-4000-8000-000000000001", LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: "bc-00000000-0000-4000-8000-000000000001", Status: "RUNNING"},
	}}
	runtime := newTestRuntime(t, repository, provider, nil)
	input := testLaunchInput()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	provider.getRunStatus = "RUNNING"
	if err := runtime.CancelActive(context.Background(), input.Binding.ExecutionID); !errors.Is(err, ErrCancellationPending) {
		t.Fatalf("CancelActive error = %v, want pending until remote terminal state", err)
	}
	if repository.operation.State != models.ManagedAgentSubmissionCancelling {
		t.Fatalf("operation state = %s, want cancelling", repository.operation.State)
	}
	provider.getRunStatus = "CANCELLED"
	if err := runtime.CancelActive(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("confirmed CancelActive: %v", err)
	}
	if repository.operation.State != models.ManagedAgentSubmissionCancelled || provider.cancelRunCalls != 2 {
		t.Fatalf("operation=%s cancel calls=%d, want cancelled after readback", repository.operation.State, provider.cancelRunCalls)
	}
}

func TestFollowupUnknownSubmissionIsNotRetried(t *testing.T) {
	repository := newMemoryRepository()
	provider := &fakeProvider{createRunErr: cursorcloud.ErrOutcomeUnknown}
	runtime := newTestRuntime(t, repository, provider, nil)
	unknownMetric := expvar.Get("cursor_cloud_submission_unknown_total").(*expvar.Map)
	unknownKey := cursorCloudMetricLabel("operation", "followup", "reason", "timeout")
	beforeUnknown := int64(0)
	if counter, ok := unknownMetric.Get(unknownKey).(*expvar.Int); ok {
		beforeUnknown = counter.Value()
	}
	input := testLaunchInput()
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	provider.createResponse = cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: input.Binding.RemoteAgentID, LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: input.Binding.RemoteAgentID, Status: runStatusFinished},
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "follow up"); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("Resume error = %v, want unknown submission", err)
	}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "follow up"); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("replayed Resume error = %v, want stored unknown", err)
	}
	if provider.createRunCalls != 1 {
		t.Fatalf("CreateRun calls = %d, want no retry after unknown outcome", provider.createRunCalls)
	}
	if counter := unknownMetric.Get(unknownKey).(*expvar.Int); counter.Value() != beforeUnknown+1 {
		t.Fatalf("unknown metric delta = %d, want one increment for this operation", counter.Value()-beforeUnknown)
	}
}

func TestUnknownFollowupCandidateMustBeBoundFromVerifiedPostSubmitRuns(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: input.Binding.RemoteAgentID, LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: input.Binding.RemoteAgentID, Status: runStatusFinished},
	}, createRunErr: cursorcloud.ErrOutcomeUnknown, getRunStatus: "RUNNING"}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	provider.runs = []cursorcloud.Run{
		{ID: "run-1", AgentID: input.Binding.RemoteAgentID, Status: runStatusFinished, CreatedAt: time.Now().Add(-time.Hour).Format(time.RFC3339Nano)},
		{ID: "run-2", AgentID: input.Binding.RemoteAgentID, Status: "RUNNING", CreatedAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano)},
		{ID: "other-agent-run", AgentID: "bc-00000000-0000-4000-8000-000000000999", Status: "RUNNING", CreatedAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano)},
	}
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "follow up"); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("Resume error = %v, want unknown submission", err)
	}
	operation, candidates, err := runtime.ListSubmissionCandidates(context.Background(), input.Binding.ExecutionID)
	if err != nil {
		t.Fatalf("ListSubmissionCandidates: %v", err)
	}
	if operation.State != models.ManagedAgentSubmissionUnknown || len(candidates) != 1 || candidates[0].RunID != "run-2" {
		t.Fatalf("unknown operation=%+v candidates=%+v, want only verified post-submit run-2", operation, candidates)
	}
	if _, err := runtime.BindSubmissionCandidate(context.Background(), input.Binding.ExecutionID, "run-1"); !errors.Is(err, ErrSubmissionCandidateUnavailable) {
		t.Fatalf("binding the prior run error = %v, want candidate unavailable", err)
	}
	accepted, err := runtime.BindSubmissionCandidate(context.Background(), input.Binding.ExecutionID, "run-2")
	if err != nil {
		t.Fatalf("BindSubmissionCandidate: %v", err)
	}
	if accepted.State != models.ManagedAgentSubmissionAccepted || accepted.RemoteRunID != "run-2" || provider.createRunCalls != 1 {
		t.Fatalf("bound operation=%+v create-run calls=%d, want accepted run-2 without retry", accepted, provider.createRunCalls)
	}
}

func TestUnknownFollowupRetryRequiresExplicitDuplicateWorkAcknowledgment(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: input.Binding.RemoteAgentID, LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: input.Binding.RemoteAgentID, Status: runStatusFinished},
	}, createRunErr: cursorcloud.ErrOutcomeUnknown}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	if err := runtime.ResumeWithTurnID(context.Background(), input.Binding.ExecutionID, "turn-2", "follow up"); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("Resume error = %v, want unknown submission", err)
	}
	firstUnknown := repository.operation
	if _, err := runtime.RetryUnknownSubmission(context.Background(), input.Binding.ExecutionID, "resolution-1", false); !errors.Is(err, ErrRetryAcknowledgmentRequired) {
		t.Fatalf("unacknowledged retry error = %v, want explicit acknowledgment barrier", err)
	}
	if provider.createRunCalls != 1 {
		t.Fatalf("CreateRun calls before acknowledgment = %d, want 1", provider.createRunCalls)
	}
	retried, err := runtime.RetryUnknownSubmission(context.Background(), input.Binding.ExecutionID, "resolution-1", true)
	if !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("acknowledged retry error = %v, want unknown submission from retry", err)
	}
	if firstUnknown.State != models.ManagedAgentSubmissionRetryAcked || retried.ID == firstUnknown.ID ||
		retried.State != models.ManagedAgentSubmissionUnknown || provider.createRunCalls != 2 {
		t.Fatalf("prior=%+v retried=%+v CreateRun calls=%d, want acknowledged old op and one explicit retry", firstUnknown, retried, provider.createRunCalls)
	}
}

func TestUnknownCreateSubmissionCanBindVerifiedRun(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{
		createErr:   cursorcloud.ErrOutcomeUnknown,
		getAgentErr: errors.New("agent read is temporarily unavailable"),
		getRun:      cursorcloud.Run{Status: "RUNNING"},
	}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("StartExecution error = %v, want unknown create outcome", err)
	}
	startedAt := repository.operation.DispatchStartedAt
	if startedAt == nil {
		t.Fatal("initial create has no durable dispatch timestamp")
	}
	provider.runs = []cursorcloud.Run{{
		ID: "run-initial", AgentID: input.Binding.RemoteAgentID, Status: "RUNNING",
		CreatedAt: startedAt.Add(-time.Second).Format(time.RFC3339Nano),
	}}
	operation, candidates, err := runtime.ListSubmissionCandidates(context.Background(), input.Binding.ExecutionID)
	if err != nil {
		t.Fatalf("ListSubmissionCandidates: %v", err)
	}
	if operation.Kind != models.ManagedAgentOperationCreate || len(candidates) != 1 || candidates[0].RunID != "run-initial" {
		t.Fatalf("unknown create operation=%+v candidates=%+v, want verified initial run", operation, candidates)
	}
	accepted, err := runtime.BindSubmissionCandidate(context.Background(), input.Binding.ExecutionID, "run-initial")
	if err != nil {
		t.Fatalf("BindSubmissionCandidate: %v", err)
	}
	if accepted.State != models.ManagedAgentSubmissionAccepted || accepted.RemoteRunID != "run-initial" {
		t.Fatalf("bound initial operation=%+v, want accepted run-initial", accepted)
	}
}

func TestUnknownCreateRetryRequiresAcknowledgmentAndRecordsNewCreate(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{
		createErr:   cursorcloud.ErrOutcomeUnknown,
		getAgentErr: errors.New("agent read is temporarily unavailable"),
	}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("StartExecution error = %v, want unknown create outcome", err)
	}
	original := repository.operation
	if _, err := runtime.RetryUnknownSubmission(context.Background(), input.Binding.ExecutionID, "create-retry-1", false); !errors.Is(err, ErrRetryAcknowledgmentRequired) {
		t.Fatalf("unacknowledged create retry error = %v, want acknowledgment requirement", err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("CreateAgent calls before acknowledgment = %d, want 1", provider.createCalls)
	}
	retried, err := runtime.RetryUnknownSubmission(context.Background(), input.Binding.ExecutionID, "create-retry-1", true)
	if !errors.Is(err, cursorcloud.ErrOutcomeUnknown) {
		t.Fatalf("acknowledged create retry error = %v, want unknown outcome from new attempt", err)
	}
	if original.State != models.ManagedAgentSubmissionRetryAcked || retried.ID == original.ID ||
		retried.Kind != models.ManagedAgentOperationCreate || retried.State != models.ManagedAgentSubmissionUnknown ||
		retried.RequestSnapshot.TurnID != original.RequestSnapshot.TurnID || provider.createCalls != 2 {
		t.Fatalf("original=%+v retried=%+v create calls=%d, want a new create with preserved turn identity", original, retried, provider.createCalls)
	}
}

func TestRestartResumesReservedFollowupAndClassifiesInterruptedSubmit(t *testing.T) {
	repository := newMemoryRepository()
	input := testLaunchInput()
	provider := &fakeProvider{createResponse: cursorcloud.CreateAgentResponse{
		Agent: cursorcloud.Agent{ID: input.Binding.RemoteAgentID, LatestRunID: "run-1"},
		Run:   cursorcloud.Run{ID: "run-1", AgentID: input.Binding.RemoteAgentID, Status: runStatusFinished},
	}, createRun: cursorcloud.Run{ID: "run-2", AgentID: input.Binding.RemoteAgentID, Status: "RUNNING"}}
	runtime := newTestRuntime(t, repository, provider, nil)
	if _, err := runtime.Launch(context.Background(), launchSpec(input)); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := runtime.StartExecution(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	binding := repository.binding
	reserved := &models.ManagedAgentOperation{
		ID: "reserved-followup", BindingID: binding.ID, PromptTurnID: "turn-reserved-followup",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-reserved-followup",
		RequestSnapshot: testRequestSnapshot("durable prompt"),
	}
	_, _, _, err := repository.ReserveManagedAgentOperation(context.Background(), reserved, binding.Revision,
		"reserved-worker", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("reserve follow-up before restart: %v", err)
	}
	if err := runtime.ResumeReservedFollowup(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("ResumeReservedFollowup: %v", err)
	}
	if repository.operation.State != models.ManagedAgentSubmissionAccepted || repository.operation.RemoteRunID != "run-2" ||
		repository.operation.PreSubmitRunID != "run-1" || provider.createRunCalls != 1 {
		t.Fatalf("reserved recovery op=%+v CreateRun calls=%d, want one run-2 dispatch from baseline run-1", repository.operation, provider.createRunCalls)
	}

	repository.operation.State = models.ManagedAgentSubmissionSucceeded
	binding = repository.binding
	interrupted := &models.ManagedAgentOperation{
		ID: "interrupted-followup", BindingID: binding.ID, PromptTurnID: "turn-interrupted-followup",
		Kind: models.ManagedAgentOperationFollowup, RequestDigest: "digest-interrupted-followup",
		RequestSnapshot: testRequestSnapshot("uncertain prompt"),
	}
	reservedBinding, interrupted, _, err := repository.ReserveManagedAgentOperation(context.Background(), interrupted, binding.Revision,
		"interrupted-worker", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("reserve interrupted follow-up: %v", err)
	}
	interrupted.State = models.ManagedAgentSubmissionSubmitting
	interrupted.PreSubmitRunID = "run-2"
	started := time.Now().UTC()
	interrupted.DispatchStartedAt = &started
	repository.operation = interrupted
	repository.operations[interrupted.PromptTurnID] = interrupted
	repository.binding = reservedBinding
	if err := runtime.MarkFollowupSubmissionUnknownAfterRestart(context.Background(), input.Binding.ExecutionID); err != nil {
		t.Fatalf("MarkFollowupSubmissionUnknownAfterRestart: %v", err)
	}
	if repository.operation.State != models.ManagedAgentSubmissionUnknown || provider.createRunCalls != 1 {
		t.Fatalf("interrupted op=%+v CreateRun calls=%d, want classified unknown without resubmission", repository.operation, provider.createRunCalls)
	}
}

func testRequestSnapshot(prompt string) models.ManagedAgentRequestSnapshot {
	return models.ManagedAgentRequestSnapshot{
		Prompt: prompt,
		TurnID: "turn-" + prompt,
	}
}

func launchSpec(input LaunchInput) agentruntime.LaunchSpec {
	return agentruntime.LaunchSpec{
		ExecutorID: "executor-1",
		Metadata:   map[string]any{LaunchMetadataKey: input},
	}
}

func testLaunchInput() LaunchInput {
	return LaunchInput{
		Binding: &models.ManagedAgentBinding{
			ID: "binding-1", SessionID: "session-1", TaskID: "task-1", WorkspaceID: "workspace-1",
			UserID: "user-1", ExecutionID: "execution-1", ProviderKind: "cursor_cloud", ExecutorID: "executor-1",
			ExecutorProfileID: "profile-1", CredentialRef: "secret-1", RemoteAgentID: "bc-00000000-0000-4000-8000-000000000001",
			Lifecycle: models.ManagedAgentBindingCreating,
			Launch: models.ManagedAgentLaunchSnapshot{
				RepositoryID: "repo-1", RepositoryURL: "https://github.com/acme/widget", StartingRef: "main",
				Model: "model-1", CallbackURL: "https://callback.example.test", AutoCreatePR: false,
			},
		},
		Operation: &models.ManagedAgentOperation{
			ID: "operation-1", BindingID: "binding-1", PromptTurnID: InitialPromptTurnID("session-1"),
			Kind: models.ManagedAgentOperationCreate, RequestDigest: "digest-1",
			RequestSnapshot: models.ManagedAgentRequestSnapshot{
				Prompt: "do the task", RepositoryURL: "https://github.com/acme/widget", StartingRef: "main",
				Model: "model-1", CallbackURL: "https://callback.example.test", AutoCreatePR: false,
			},
		},
	}
}

func newTestRuntime(t *testing.T, repository *memoryRepository, provider *fakeProvider, issue func(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation) (MCPGrant, error)) *Runtime {
	t.Helper()
	if issue == nil {
		issue = func(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation) (MCPGrant, error) {
			return MCPGrant{URL: "https://callback.example.test/grant-1", Token: "opaque-token"}, nil
		}
	}
	runtime, err := New(Config{
		Repository:    repository,
		ClientFactory: func(context.Context, *models.ManagedAgentBinding) (Provider, error) { return provider, nil },
		GrantIssuer:   GrantIssuerFunc(issue),
		Now:           func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return runtime
}
