package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStartupProcessRecoveryCauseSurvivesReadiness(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	exitCode := 1
	lastError := models.LastAgentError{
		Message:          models.AgentErrorCauseProcessExitedDetail,
		OccurredAt:       time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "resume-42",
		ExecutionID:      "exec-1",
		AgentExecutionID: "exec-1",
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodeProcessExited,
			Reason:    models.AgentErrorCauseReasonMiseConfigUntrusted,
			ExitCode:  &exitCode,
		}},
		StampValue: "stamp-resume-42",
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyLastAgentError, lastError); err != nil {
		t.Fatalf("seed last agent error: %v", err)
	}

	attempt := &resumeAttempt{id: 42, taskID: "t1", sessionID: "s1"}
	attempt.setExecutionID("exec-1")
	readiness := fmt.Errorf("session failed: %s", lastError.Message)
	failure := svc.withSessionRecoveryFailureIdentity(ctx, "t1", "s1", attempt, "exec-1", readiness)
	var recoveryFailure *SessionRecoveryFailure
	if !errors.As(failure, &recoveryFailure) {
		t.Fatalf("failure type = %T, want *SessionRecoveryFailure", failure)
	}
	if recoveryFailure.Identity.ErrorStamp != "stamp-resume-42" {
		t.Fatalf("error stamp = %q, want the matching durable stamp", recoveryFailure.Identity.ErrorStamp)
	}
	causes, ok := RecoveryFailureCauses(failure)
	if !ok || len(causes) != 1 || causes[0].ExitCode == nil || *causes[0].ExitCode != 1 {
		t.Fatalf("recovery causes = %#v", causes)
	}

	staleAttempt := &resumeAttempt{id: 43, taskID: "t1", sessionID: "s1"}
	staleAttempt.setExecutionID("exec-1")
	stale := svc.withSessionRecoveryFailureIdentity(ctx, "t1", "s1", staleAttempt, "exec-1", readiness)
	if _, ok := RecoveryFailureCauses(stale); ok {
		t.Fatal("stale attempt acquired another attempt's causes")
	}
	var staleFailure *SessionRecoveryFailure
	if !errors.As(stale, &staleFailure) || staleFailure.Identity.ErrorStamp != "" {
		t.Fatalf("stale failure = %#v, want no matched stamp", staleFailure)
	}
}

func TestStartupProcessDistinctRecoveryHistory(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = newServiceBackedMessageCreator(repo)

	exitCode := 1
	first := models.LastAgentError{
		Message:          models.AgentErrorCauseProcessExitedDetail,
		OccurredAt:       time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC),
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "resume-1",
		ExecutionID:      "exec-1",
		AgentExecutionID: "exec-1",
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodeProcessExited,
			Reason:    models.AgentErrorCauseReasonMiseConfigUntrusted,
			ExitCode:  &exitCode,
		}},
		StampValue: "stamp-first",
	}
	second := models.LastAgentError{
		Message:          "The bootstrap operation could not be completed.",
		OccurredAt:       time.Date(2026, 11, 7, 12, 5, 0, 0, time.UTC),
		Phase:            models.LaunchErrorPhaseBootstrap,
		AttemptID:        "resume-2",
		ExecutionID:      "exec-2",
		AgentExecutionID: "exec-2",
		Code:             models.LaunchErrorCategoryGenericLaunchFailure,
		StampValue:       "stamp-second",
	}
	if err := svc.persistBootstrapFailureMessage(ctx, "t1", "s1", "exec-1", first); err != nil {
		t.Fatalf("persist first failure: %v", err)
	}
	if err := svc.persistBootstrapFailureMessage(ctx, "t1", "s1", "exec-2", second); err != nil {
		t.Fatalf("persist second failure: %v", err)
	}
	messages, err := repo.ListMessages(ctx, "s1")
	if err != nil {
		t.Fatalf("list session messages: %v", err)
	}
	stamps := map[string]bool{}
	var firstCauses int
	for _, message := range messages {
		if message.Metadata == nil || message.Metadata["recovery_actions"] != true {
			continue
		}
		stamp, _ := message.Metadata["error_stamp"].(string)
		if stamp != "" {
			stamps[stamp] = true
		}
		if stamp == "stamp-first" {
			if raw, ok := message.Metadata["causes"]; ok {
				encoded, _ := json.Marshal(raw)
				var causes []models.AgentErrorCause
				if json.Unmarshal(encoded, &causes) == nil {
					firstCauses = len(causes)
				}
			}
		}
	}
	if !stamps["stamp-first"] || !stamps["stamp-second"] {
		t.Fatalf("recovery history stamps = %#v, want both attempts", stamps)
	}
	if firstCauses != 1 {
		t.Fatalf("first history causes = %d, want the original process exit", firstCauses)
	}
}
