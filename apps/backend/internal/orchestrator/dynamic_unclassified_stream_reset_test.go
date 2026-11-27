package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRawOutputChunksDeferUnclassifiedStreakResetAndKeepTranscriptIdentity(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
	); handled {
		t.Fatal("first matching failure unexpectedly launched a successor")
	}
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
	fixture.agentManager.currentPromptExecutionID = "execution-two"
	fixture.agentManager.currentPromptGeneration.Store(2)
	// Prime foreground ownership before probing repository access. The first
	// genuine output can otherwise publish the foreground transition, whose
	// operator-facing payload independently reads the session row.
	fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")

	repository := &blockingStreamResetRepository{
		sessionExecutorStore: fixture.svc.repo,
		readStarted:          make(chan struct{}, 1),
		releaseRead:          make(chan struct{}),
	}
	var handlers sync.WaitGroup
	var releaseOnce sync.Once
	releaseRead := func() { releaseOnce.Do(func() { close(repository.releaseRead) }) }
	t.Cleanup(func() {
		releaseRead()
		handlers.Wait()
	})
	fixture.svc.repo = repository
	fixture.barrier.snapshotCalls = 0
	fixture.barrier.snapshotError = errors.New("stream callback must not reach route-state persistence")

	baseMessages := &mockMessageCreator{}
	messages := &streamingIdentityRecorder{MessageCreator: baseMessages}
	fixture.svc.messageCreator = messages

	chunks := []struct {
		text      string
		candidate bool
	}{
		{text: "ordinary ", candidate: false},
		{text: "provider ", candidate: true},
		{text: "response", candidate: false},
		{text: " text", candidate: true},
	}
	const messageID = "assistant-message-1"
	for index, chunk := range chunks {
		finished := make(chan struct{})
		handlers.Add(1)
		go func(chunk struct {
			text      string
			candidate bool
		}) {
			defer handlers.Done()
			fixture.svc.handleAgentStreamEvent(fixture.ctx, &lifecycle.AgentStreamEventPayload{
				TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: "execution-two",
				OwnerKind: lifecycle.ExecutionOwnerTask,
				Data: &lifecycle.AgentStreamEventData{
					Type:                        "message_chunk",
					Text:                        chunk.text,
					PromptGeneration:            2,
					ProviderDiagnosticCandidate: chunk.candidate,
				},
			})
			close(finished)
		}(chunk)
		select {
		case <-repository.readStarted:
			releaseRead()
			<-finished
			t.Fatalf("raw chunk %d entered a blocked route-state reset read", index)
		case <-finished:
		case <-time.After(5 * time.Second):
			releaseRead()
			t.Fatalf("raw chunk %d did not return while reset reads were blocked", index)
		}

		fixture.svc.handleAgentStreamEvent(fixture.ctx, &lifecycle.AgentStreamEventPayload{
			TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: "execution-two",
			OwnerKind: lifecycle.ExecutionOwnerTask,
			Data: &lifecycle.AgentStreamEventData{
				Type: "message_streaming", MessageID: messageID, Text: chunk.text,
				IsAppend: index > 0, PromptGeneration: 2,
			},
		})
	}

	if repository.sessionReads != 0 || repository.taskReads != 0 || fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("raw stream reset DB operations = sessions:%d tasks:%d route_writes:%d, want zero",
			repository.sessionReads, repository.taskReads, fixture.barrier.snapshotCalls)
	}
	attempt, ok := fixture.svc.promptAttemptForSession(fixture.sessionID)
	if !ok {
		t.Fatal("raw output removed the active prompt attempt")
	}
	attempt.mu.Lock()
	outputObserved := attempt.output
	attempt.mu.Unlock()
	if !outputObserved {
		t.Fatal("raw output did not update prompt recovery evidence")
	}
	if len(messages.createdIDs) != 1 || messages.createdIDs[0] != messageID || len(messages.appendedIDs) != len(chunks)-1 {
		t.Fatalf("transcript IDs = created %v, appended %v, want one create and %d stable appends",
			messages.createdIDs, messages.appendedIDs, len(chunks)-1)
	}
	for _, id := range messages.appendedIDs {
		if id != messageID {
			t.Fatalf("transcript append ID = %q, want stable ID %q", id, messageID)
		}
	}
	if got := baseMessages.agentStreamTexts; len(got) != len(chunks) {
		t.Fatalf("transcript writes = %d, want %d chunks", len(got), len(chunks))
	} else if joined := strings.Join(got, ""); joined != "ordinary provider response text" {
		t.Fatalf("transcript text = %q, want exact joined output", joined)
	}
}

func TestPendingStreakResetSurvivesPromptReplacementBeforeNoOutputFailure(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
	); handled {
		t.Fatal("first matching failure unexpectedly launched a successor")
	}
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	fixture.agentManager.currentPromptExecutionID = "execution-two"
	fixture.agentManager.currentPromptGeneration.Store(2)
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
	fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")

	fixture.barrier.snapshotCalls = 0
	fixture.barrier.snapshotError = errors.New("temporary reset write failure")
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, "message_chunk", "visible output", 2,
	))
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, agentEventToolCall, "", 2,
	))
	if fixture.barrier.snapshotCalls != 1 {
		t.Fatalf("first semantic-boundary reset writes = %d, want one failed reset", fixture.barrier.snapshotCalls)
	}
	value, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID)
	if !ok {
		t.Fatal("failed reset lost its pending prompt identity")
	}
	pending := value.(*pendingDynamicStreakReset)
	pending.mu.Lock()
	capturedRoute := pending.capturedRouteIdentity
	oldGeneration := pending.routeGeneration
	pending.mu.Unlock()
	if !capturedRoute || oldGeneration == 0 {
		t.Fatalf("failed reset did not retain route identity: captured=%v generation=%d", capturedRoute, oldGeneration)
	}

	fixture.svc.beginInteractivePromptAttempt(fixture.ctx, fixture.sessionID, "execution-two", true)
	fixture.agentManager.currentPromptGeneration.Store(3)
	value, ok = fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID)
	if !ok {
		t.Fatal("failed new-prompt retry dropped the old output reset")
	}
	pending = value.(*pendingDynamicStreakReset)
	pending.mu.Lock()
	stillOldPrompt := pending.event.PromptGeneration == 2 && pending.event.AgentExecutionID == "execution-two"
	pending.mu.Unlock()
	if !stillOldPrompt {
		t.Fatal("new-prompt replacement changed the pending reset's source identity")
	}
	if fixture.barrier.snapshotCalls != 2 {
		t.Fatalf("reset writes through failed prompt replacement = %d, want two failed attempts",
			fixture.barrier.snapshotCalls)
	}

	fixture.barrier.snapshotError = nil
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-two", 3), fixture.failure(),
	); handled {
		t.Fatal("prompt-three no-output failure inherited prompt-two's unclassified streak")
	}
	if fixture.barrier.snapshotCalls != 4 {
		t.Fatalf("route-state writes through terminal failure = %d, want two failed resets, one reset retry, and the failure decision",
			fixture.barrier.snapshotCalls)
	}
	state, err := fixture.repo.LoadRouteState(fixture.ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after prompt-three failure: %v", err)
	}
	var policy dynamic.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
		t.Fatalf("decode policy state after prompt-three failure: %v", err)
	}
	if policy.Unclassified == nil || policy.Unclassified.Count != 1 {
		t.Fatalf("prompt-three unclassified streak = %+v, want a fresh count of one", policy.Unclassified)
	}
}

func TestStalePendingStreakResetDoesNotWriteSuccessorRoute(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*models.TaskSession)
	}{
		{name: "route generation", edit: func(session *models.TaskSession) { session.RouteGeneration++ }},
		{name: "logical profile", edit: func(session *models.TaskSession) { session.AgentProfileID = "replacement-logical" }},
		{name: "execution profile", edit: func(session *models.TaskSession) { session.ExecutionProfileID = "candidate-b" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			if handled := fixture.svc.routeDynamicAgentFailure(
				fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
			); handled {
				t.Fatal("first matching failure unexpectedly launched a successor")
			}
			fixture.resume(t, "execution-two")
			fixture.persistRunningExecution(t, "execution-two")
			fixture.agentManager.currentPromptExecutionID = "execution-two"
			fixture.agentManager.currentPromptGeneration.Store(2)
			fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
			fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
			fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
				fixture, "message_chunk", "visible output", 2,
			))
			fixture.barrier.snapshotCalls = 0
			fixture.barrier.snapshotError = errors.New("temporary reset write failure")
			fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
				fixture, agentEventToolCall, "", 2,
			))
			if fixture.barrier.snapshotCalls != 1 {
				t.Fatalf("initial failed reset writes = %d, want one", fixture.barrier.snapshotCalls)
			}
			fixture.barrier.snapshotError = nil
			session := mustTaskSession(t, fixture.repo, fixture.ctx, fixture.sessionID)
			change.edit(session)
			if err := fixture.repo.UpdateTaskSession(fixture.ctx, session); err != nil {
				t.Fatalf("UpdateTaskSession successor identity: %v", err)
			}
			if !fixture.svc.flushPendingDynamicStreakReset(fixture.ctx, fixture.sessionID, nil) {
				t.Fatal("stale route-bound reset was reported as a persistence failure")
			}
			if fixture.barrier.snapshotCalls != 1 {
				t.Fatalf("stale reset writes = %d, want no write after the original failed attempt", fixture.barrier.snapshotCalls)
			}
			if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); ok {
				t.Fatal("stale reset remained pending after route identity changed")
			}
		})
	}
}

func TestUncapturedResetCannotCrossReusedExecutionPromptOrRoute(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
	); handled {
		t.Fatal("first matching failure unexpectedly launched a successor")
	}
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	fixture.agentManager.currentPromptExecutionID = "execution-two"
	fixture.agentManager.currentPromptGeneration.Store(2)
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
	fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, "message_chunk", "visible output", 2,
	))

	old, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID)
	if !ok || old.(*pendingDynamicStreakReset).capturedRouteIdentity {
		t.Fatal("test setup expected an uncaptured reset identity")
	}
	// Reuse the same runtime execution while the prompt generation and durable
	// candidate identity advance. The old output has no captured route identity,
	// so execution equality alone cannot authorize a write to the successor.
	session := mustTaskSession(t, fixture.repo, fixture.ctx, fixture.sessionID)
	session.RouteGeneration++
	session.ExecutionProfileID = "candidate-b"
	if err := fixture.repo.UpdateTaskSession(fixture.ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession successor route: %v", err)
	}
	fixture.agentManager.currentPromptGeneration.Store(3)
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 3, true)
	fixture.barrier.snapshotCalls = 0
	if fixture.svc.flushPendingDynamicStreakReset(fixture.ctx, fixture.sessionID, nil) {
		t.Fatal("uncaptured stale reset unexpectedly completed after prompt/route replacement")
	}
	if fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("uncaptured reset wrote successor route state %d times, want zero", fixture.barrier.snapshotCalls)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("uncaptured reset intent was dropped without proving a safe route identity")
	}
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-two", 3), fixture.failure(),
	); handled {
		t.Fatal("replacement prompt failure automatically routed with an unproven prior reset")
	}
	if fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("fail-closed replacement failure wrote route state %d times, want zero", fixture.barrier.snapshotCalls)
	}
	state, err := fixture.repo.LoadRouteState(fixture.ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after fail-closed failure: %v", err)
	}
	var policy dynamic.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
		t.Fatalf("decode policy state after fail-closed failure: %v", err)
	}
	if policy.Unclassified == nil || policy.Unclassified.Count != 1 {
		t.Fatalf("fail-closed failure advanced unclassified streak to %+v, want prior count one", policy.Unclassified)
	}
}

func TestUncapturedResetReadFailureRetainsManualRecoveryFence(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
	); handled {
		t.Fatal("first matching failure unexpectedly launched a successor")
	}
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	fixture.agentManager.currentPromptExecutionID = "execution-two"
	fixture.agentManager.currentPromptGeneration.Store(2)
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
	fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, "message_chunk", "visible output", 2,
	))

	repository := &failingResetSessionRepository{sessionExecutorStore: fixture.svc.repo, fail: true}
	fixture.svc.repo = repository
	fixture.barrier.snapshotCalls = 0
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, agentEventToolCall, "", 2,
	))
	if repository.sessionReads != 1 || fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("initial failed reset reads/writes = %d/%d, want one session read and zero route writes",
			repository.sessionReads, fixture.barrier.snapshotCalls)
	}
	value, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID)
	if !ok || value.(*pendingDynamicStreakReset).capturedRouteIdentity {
		t.Fatal("test setup expected a pending reset whose route identity was never captured")
	}

	fixture.svc.beginInteractivePromptAttempt(fixture.ctx, fixture.sessionID, "execution-two", true)
	if repository.sessionReads != 2 {
		t.Fatalf("new-prompt retry session reads = %d, want a second failed read", repository.sessionReads)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("failed new-prompt retry discarded the uncaptured reset intent")
	}

	repository.fail = false
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-two", 3), fixture.failure(),
	); handled {
		t.Fatal("no-output replacement failure automatically routed after uncaptured reset read failures")
	}
	if fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("replacement terminal failure wrote route state %d times, want zero", fixture.barrier.snapshotCalls)
	}
	state, err := fixture.repo.LoadRouteState(fixture.ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after failed reset reads: %v", err)
	}
	var policy dynamic.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
		t.Fatalf("decode policy state after failed reset reads: %v", err)
	}
	if policy.Unclassified == nil || policy.Unclassified.Count != 1 {
		t.Fatalf("failed reset read advanced unclassified streak to %+v, want prior count one", policy.Unclassified)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("uncaptured reset intent was dropped rather than kept behind the manual recovery fence")
	}
}

func TestOwnedSuccessfulCompletionClearsUncapturedPriorReset(t *testing.T) {
	for _, ownsPrompt := range []bool{true, false} {
		name := "stale completion stays fenced"
		if ownsPrompt {
			name = "current completion clears prior uncertainty"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			if handled := fixture.svc.routeDynamicAgentFailure(
				fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
			); handled {
				t.Fatal("first matching failure unexpectedly launched a successor")
			}
			fixture.resume(t, "execution-two")
			fixture.persistRunningExecution(t, "execution-two")
			fixture.agentManager.currentPromptExecutionID = "execution-two"
			fixture.agentManager.currentPromptGeneration.Store(2)
			fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
			fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
			fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
				fixture, "message_chunk", "visible output", 2,
			))

			failingRepo := &failingResetSessionRepository{sessionExecutorStore: fixture.repo, fail: true}
			fixture.svc.repo = failingRepo
			fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
				fixture, agentEventToolCall, "", 2,
			))
			fixture.svc.beginInteractivePromptAttempt(fixture.ctx, fixture.sessionID, "execution-two", true)
			pending, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID)
			if !ok || pending.(*pendingDynamicStreakReset).capturedRouteIdentity {
				t.Fatal("test setup expected an uncaptured reset carried across prompt replacement")
			}

			completionGeneration := uint64(3)
			if !ownsPrompt {
				completionGeneration = 4
			}
			fixture.agentManager.currentPromptGeneration.Store(completionGeneration)
			fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-two", 3)
			repository := &countingStreamResetRepository{sessionExecutorStore: fixture.repo}
			fixture.svc.repo = repository
			fixture.barrier.snapshotCalls = 0
			completed := fixture.event("execution-two", 3)
			cleared := fixture.svc.clearDynamicUnclassifiedStreakForCompletion(fixture.ctx, completed)
			if ownsPrompt {
				assertCurrentCompletionClearedReset(t, fixture, repository, cleared)
			} else {
				assertStaleCompletionRetainedReset(t, fixture, repository, cleared)
			}
		})
	}
}

func assertCurrentCompletionClearedReset(
	t *testing.T,
	fixture unclassifiedWorkflowFenceFixture,
	repository *countingStreamResetRepository,
	cleared bool,
) {
	t.Helper()
	if !cleared {
		t.Fatal("authoritative successful completion did not clear the current route streak")
	}
	if repository.sessionReads != 1 || repository.taskReads != 1 || fixture.barrier.snapshotCalls != 1 {
		t.Fatalf("current completion reads/writes = sessions:%d tasks:%d route_writes:%d, want one current-route reset (1/1/1)",
			repository.sessionReads, repository.taskReads, fixture.barrier.snapshotCalls)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); ok {
		t.Fatal("current successful completion retained obsolete uncaptured reset intent")
	}
	state, err := fixture.repo.LoadRouteState(fixture.ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState after current completion: %v", err)
	}
	var policy dynamic.PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policy); err != nil {
		t.Fatalf("decode policy after current completion: %v", err)
	}
	if policy.Unclassified != nil {
		t.Fatalf("successful completion left unclassified streak %+v, want cleared", policy.Unclassified)
	}
}

func assertStaleCompletionRetainedReset(
	t *testing.T,
	fixture unclassifiedWorkflowFenceFixture,
	repository *countingStreamResetRepository,
	cleared bool,
) {
	t.Helper()
	if cleared {
		t.Fatal("stale completion cleared an unclassified streak it did not own")
	}
	if repository.sessionReads != 0 || repository.taskReads != 0 || fixture.barrier.snapshotCalls != 0 {
		t.Fatalf("stale completion reads/writes = sessions:%d tasks:%d route_writes:%d, want zero",
			repository.sessionReads, repository.taskReads, fixture.barrier.snapshotCalls)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("stale completion discarded unresolved reset intent")
	}
}

func TestCompletedGenerationOwnsStreakResetAfterStreamEvidenceRetires(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		streamFirst       bool
		managerOwnsPrompt bool
		wantReads         int
		wantWrites        int
	}{
		{name: "stream complete before lifecycle completion", streamFirst: true, managerOwnsPrompt: true, wantReads: 1, wantWrites: 1},
		{name: "lifecycle completion before stream complete", managerOwnsPrompt: true, wantReads: 1, wantWrites: 1},
		{name: "stale generation", streamFirst: true, managerOwnsPrompt: false, wantReads: 0, wantWrites: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
			defer fixture.svc.stopDynamicSuccessorWorkers()
			if handled := fixture.svc.routeDynamicAgentFailure(
				fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
			); handled {
				t.Fatal("first matching failure unexpectedly launched a successor")
			}
			fixture.resume(t, "execution-two")
			fixture.persistRunningExecution(t, "execution-two")
			fixture.agentManager.currentPromptExecutionID = "execution-two"
			fixture.agentManager.currentPromptGeneration.Store(2)
			fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
			fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
			fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
				fixture, "message_chunk", "visible output", 2,
			))

			repository := &countingStreamResetRepository{sessionExecutorStore: fixture.svc.repo}
			fixture.svc.repo = repository
			fixture.barrier.snapshotCalls = 0
			fixture.agentManager.currentPromptGeneration.Store(2)
			fixture.agentManager.currentPromptExecutionID = "execution-two"
			if testCase.streamFirst {
				fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-two", 2)
			}
			if !testCase.managerOwnsPrompt {
				fixture.agentManager.currentPromptGeneration.Store(3)
			}
			completed := watcher.AgentEventData{
				TaskID: fixture.taskID, SessionID: fixture.sessionID, OwnerKind: "task",
				AgentExecutionID: "execution-two", PromptGeneration: 2,
			}
			cleared := fixture.svc.clearDynamicUnclassifiedStreakForCompletion(fixture.ctx, completed)
			if !testCase.streamFirst {
				fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-two", 2)
			}
			if cleared != testCase.managerOwnsPrompt {
				t.Fatalf("completion clear = %v, want %v from generation ownership", cleared, testCase.managerOwnsPrompt)
			}
			if repository.sessionReads != testCase.wantReads || repository.taskReads != testCase.wantReads {
				t.Fatalf("completion reset reads = sessions:%d tasks:%d, want sessions:%d tasks:%d",
					repository.sessionReads, repository.taskReads, testCase.wantReads, testCase.wantReads)
			}
			if fixture.barrier.snapshotCalls != testCase.wantWrites {
				t.Fatalf("completion reset writes = %d, want %d", fixture.barrier.snapshotCalls, testCase.wantWrites)
			}
		})
	}
}

func TestCompletionResetPersistenceFailureRetainsPendingIntent(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	if handled := fixture.svc.routeDynamicAgentFailure(
		fixture.ctx, fixture.event("execution-one", 1), fixture.failure(),
	); handled {
		t.Fatal("first matching failure unexpectedly launched a successor")
	}
	fixture.resume(t, "execution-two")
	fixture.persistRunningExecution(t, "execution-two")
	fixture.agentManager.currentPromptExecutionID = "execution-two"
	fixture.agentManager.currentPromptGeneration.Store(2)
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-two", 2, true)
	fixture.svc.markForegroundGenerating(fixture.sessionID, "execution-two")
	fixture.svc.handleAgentStreamEvent(fixture.ctx, rawStreamPayload(
		fixture, "message_chunk", "visible output", 2,
	))
	fixture.svc.clearPromptAttemptEvidence(fixture.sessionID, "execution-two", 2)

	repository := &countingStreamResetRepository{sessionExecutorStore: fixture.repo}
	fixture.svc.repo = repository
	fixture.barrier.snapshotCalls = 0
	fixture.barrier.snapshotError = errors.New("completion reset persistence failed")
	if fixture.svc.clearDynamicUnclassifiedStreakForCompletion(fixture.ctx, fixture.event("execution-two", 2)) {
		t.Fatal("completion reported success after its route reset write failed")
	}
	if repository.sessionReads != 1 || repository.taskReads != 1 || fixture.barrier.snapshotCalls != 1 {
		t.Fatalf("failed completion reset reads/writes = sessions:%d tasks:%d route_writes:%d, want exactly 1/1/1",
			repository.sessionReads, repository.taskReads, fixture.barrier.snapshotCalls)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("failed successful-completion reset dropped the pending intent instead of preserving it for a later boundary")
	}
}

func TestSessionDeletionRetiresPendingStreakResetOnlyAfterDeleteSucceeds(t *testing.T) {
	fixture := newUnclassifiedWorkflowFenceFixture(t, false, false)
	defer fixture.svc.stopDynamicSuccessorWorkers()
	fixture.svc.beginPromptAttempt(fixture.sessionID, "execution-one", 1, true)
	fixture.svc.observePromptAttempt(fixture.sessionID, "execution-one", 1, true, false)
	fixture.svc.markDynamicStreakResetPending(watcher.AgentEventData{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, OwnerKind: "task",
		AgentExecutionID: "execution-one", PromptGeneration: 1,
	})
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("test setup did not create a pending streak reset")
	}
	session := mustTaskSession(t, fixture.repo, fixture.ctx, fixture.sessionID)
	repository := &pendingResetDeleteRepository{
		sessionExecutorStore: fixture.repo,
		deleteError:          errors.New("temporary session deletion failure"),
	}
	fixture.svc.repo = repository
	if err := fixture.svc.deleteSessionAndCleanAttachments(fixture.ctx, session); err == nil {
		t.Fatal("session deletion unexpectedly succeeded")
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); !ok {
		t.Fatal("failed session deletion discarded the pending streak reset")
	}
	repository.deleteError = nil
	if err := fixture.svc.deleteSessionAndCleanAttachments(fixture.ctx, session); err != nil {
		t.Fatalf("successful session deletion: %v", err)
	}
	if _, ok := fixture.svc.pendingDynamicStreakResets.Load(fixture.sessionID); ok {
		t.Fatal("successful session deletion retained its pending streak reset")
	}
	if _, err := fixture.repo.GetTaskSession(fixture.ctx, fixture.sessionID); err == nil {
		t.Fatal("successful session deletion left the session row")
	}
}

func rawStreamPayload(
	fixture unclassifiedWorkflowFenceFixture,
	eventType, text string,
	promptGeneration uint64,
) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, ExecutionID: "execution-two",
		OwnerKind: lifecycle.ExecutionOwnerTask,
		Data: &lifecycle.AgentStreamEventData{
			Type: eventType, Text: text, ToolCallID: "tool-boundary",
			ToolName: "read_file", PromptGeneration: promptGeneration,
		},
	}
}

type blockingStreamResetRepository struct {
	sessionExecutorStore
	readStarted  chan struct{}
	releaseRead  chan struct{}
	sessionReads int
	taskReads    int
	readOnce     sync.Once
}

type countingStreamResetRepository struct {
	sessionExecutorStore
	sessionReads int
	taskReads    int
}

type failingResetSessionRepository struct {
	sessionExecutorStore
	sessionReads int
	fail         bool
}

type pendingResetDeleteRepository struct {
	sessionExecutorStore
	deleteError error
}

func (r *pendingResetDeleteRepository) DeleteTaskSession(ctx context.Context, session *models.TaskSession) error {
	if r.deleteError != nil {
		return r.deleteError
	}
	return r.sessionExecutorStore.DeleteTaskSession(ctx, session)
}

func (r *failingResetSessionRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.sessionReads++
	if r.fail {
		return nil, errors.New("temporary route identity read failure")
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, id)
}

func (r *countingStreamResetRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.sessionReads++
	return r.sessionExecutorStore.GetTaskSession(ctx, id)
}

func (r *countingStreamResetRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.taskReads++
	return r.sessionExecutorStore.GetTask(ctx, id)
}

func (r *blockingStreamResetRepository) GetTaskSession(
	ctx context.Context,
	id string,
) (*models.TaskSession, error) {
	r.sessionReads++
	r.readOnce.Do(func() {
		close(r.readStarted)
		<-r.releaseRead
	})
	return nil, errors.New("blocked stream reset read released")
}

func (r *blockingStreamResetRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.taskReads++
	return nil, errors.New("blocked stream reset task read")
}

type streamingIdentityRecorder struct {
	MessageCreator
	createdIDs  []string
	appendedIDs []string
}

func (r *streamingIdentityRecorder) CreateAgentMessageStreaming(
	ctx context.Context,
	messageID, taskID, content, sessionID, turnID string,
) error {
	r.createdIDs = append(r.createdIDs, messageID)
	return r.MessageCreator.CreateAgentMessageStreaming(ctx, messageID, taskID, content, sessionID, turnID)
}

func (r *streamingIdentityRecorder) AppendAgentMessage(
	ctx context.Context,
	messageID, additionalContent string,
) error {
	r.appendedIDs = append(r.appendedIDs, messageID)
	return r.MessageCreator.AppendAgentMessage(ctx, messageID, additionalContent)
}
