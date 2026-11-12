package orchestrator

import (
	"context"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	previousCandidateProfileID = "candidate-previous"
	nextCandidateProfileID     = "candidate-next"
)

// seedDynamicCandidateRuntimeSettings records what the previous concrete
// provider reported (model and permission mode) plus an explicit user choice.
func seedDynamicCandidateRuntimeSettings(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.ExecutionProfileID = previousCandidateProfileID
	session.RouteGeneration = 1
	session.RouteState = "running"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	for key, value := range map[string]interface{}{
		models.SessionMetaKeyRuntimeConfig:          models.SessionRuntimeConfig{Model: "previous-model", Mode: "previous-mode"},
		models.SessionMetaKeySessionMode:            "previous-mode",
		models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{ConfigOptions: map[string]string{"effort": "high"}},
	} {
		if err := repo.SetSessionMetadataKey(ctx, sessionID, key, value); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
}

func TestPersistDynamicLaunchDecisionDropsPreviousCandidateRuntimeSettings(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-candidate-switch", "session-candidate-switch", models.TaskSessionStateFailed)
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-switch")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.persistDynamicLaunchDecision(ctx, "session-candidate-switch", dynamicruntime.RouteDecision{
		ExecutionProfileID: nextCandidateProfileID,
		Generation:         2,
	}); err != nil {
		t.Fatalf("persist candidate switch: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session-candidate-switch")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.ExecutionProfileID != nextCandidateProfileID {
		t.Fatalf("execution profile = %q, want %q", session.ExecutionProfileID, nextCandidateProfileID)
	}
	effective, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if effective.Model == "previous-model" || effective.Mode == "previous-mode" {
		t.Fatalf("next candidate inherits the previous provider's runtime settings: model=%q mode=%q",
			effective.Model, effective.Mode)
	}
	if _, ok := session.Metadata[models.SessionMetaKeyRuntimeConfig]; ok {
		t.Fatalf("previous provider runtime_config survived the candidate switch: %#v",
			session.Metadata[models.SessionMetaKeyRuntimeConfig])
	}
	if mode, ok := session.Metadata[models.SessionMetaKeySessionMode]; ok {
		t.Fatalf("previous provider session_mode survived the candidate switch: %#v", mode)
	}
	if effective.ConfigOptions["effort"] != "high" {
		t.Fatalf("explicit user override lost on candidate switch: %#v", effective.ConfigOptions)
	}
	if session.State != models.TaskSessionStateCreated {
		t.Fatalf("session state = %q, want reopened CREATED for the successor launch", session.State)
	}
}

func TestPersistDynamicLaunchDecisionKeepsWorkflowDeclaredModeOnCandidateSwitch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "task-candidate-mode", "session-candidate-mode", "step-plan")
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-mode")
	steps := newMockStepGetter()
	steps.steps["step-plan"] = &wfmodels.WorkflowStep{
		ID: "step-plan",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type:   wfmodels.OnEnterSetSessionMode,
			Config: map[string]interface{}{"mode": "plan"},
		}}},
	}
	svc := createTestService(repo, steps, newMockTaskRepo())

	if err := svc.persistDynamicLaunchDecision(ctx, "session-candidate-mode", dynamicruntime.RouteDecision{
		ExecutionProfileID: nextCandidateProfileID,
		Generation:         2,
	}); err != nil {
		t.Fatalf("persist candidate switch: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session-candidate-mode")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if mode := session.Metadata[models.SessionMetaKeySessionMode]; mode != "plan" {
		t.Fatalf("session_mode = %#v, want the workflow-declared mode %q", mode, "plan")
	}
}

func TestPersistDynamicLaunchDecisionKeepsRuntimeSettingsForSameCandidate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-candidate-retry", "session-candidate-retry", models.TaskSessionStateFailed)
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-retry")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if err := svc.persistDynamicLaunchDecision(ctx, "session-candidate-retry", dynamicruntime.RouteDecision{
		ExecutionProfileID: previousCandidateProfileID,
		Generation:         2,
	}); err != nil {
		t.Fatalf("persist same-candidate retry: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session-candidate-retry")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	effective, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if effective.Model != "previous-model" || effective.Mode != "previous-mode" {
		t.Fatalf("same-candidate retry lost runtime settings: model=%q mode=%q", effective.Model, effective.Mode)
	}
}

func TestSessionModeEventFromPreviousCandidateIsIgnoredAfterSwitch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-candidate-late", "session-candidate-late", models.TaskSessionStateFailed)
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-late")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = &recordingEventBus{}

	if err := svc.persistDynamicLaunchDecision(ctx, "session-candidate-late", dynamicruntime.RouteDecision{
		ExecutionProfileID: nextCandidateProfileID,
		Generation:         2,
	}); err != nil {
		t.Fatalf("persist candidate switch: %v", err)
	}
	svc.handleSessionModeEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:             "task-candidate-late",
		SessionID:          "session-candidate-late",
		ExecutionID:        "previous-execution",
		ExecutionProfileID: previousCandidateProfileID,
		Data:               &lifecycle.AgentStreamEventData{CurrentModeID: "previous-mode"},
	})

	session, err := repo.GetTaskSession(ctx, "session-candidate-late")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	effective, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if effective.Mode == "previous-mode" {
		t.Fatalf("late mode event from the previous candidate restored its mode: %#v", session.Metadata)
	}
}

// The models handler persists the current model after its staleness check, so
// a late models event from the previous candidate must be dropped there too.
func TestSessionModelsEventFromPreviousCandidateIsIgnoredAfterSwitch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-candidate-late-models", "session-candidate-late-models", models.TaskSessionStateFailed)
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-late-models")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := &recordingEventBus{}
	svc.eventBus = eventBus

	if err := svc.persistDynamicLaunchDecision(ctx, "session-candidate-late-models", dynamicruntime.RouteDecision{
		ExecutionProfileID: nextCandidateProfileID,
		Generation:         2,
	}); err != nil {
		t.Fatalf("persist candidate switch: %v", err)
	}
	svc.handleSessionModelsEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:             "task-candidate-late-models",
		SessionID:          "session-candidate-late-models",
		ExecutionID:        "previous-execution",
		ExecutionProfileID: previousCandidateProfileID,
		Data:               &lifecycle.AgentStreamEventData{CurrentModelID: "previous-model"},
	})

	session, err := repo.GetTaskSession(ctx, "session-candidate-late-models")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	effective, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if effective.Model == "previous-model" {
		t.Fatalf("late models event from the previous candidate restored its model: %#v", session.Metadata)
	}
	if len(eventBus.events) != 0 {
		t.Fatalf("late models event from the previous candidate was published: %d events", len(eventBus.events))
	}
}

// Manual route actions persist a candidate switch through the exported entry
// point; a session revision that moved since the caller read it must not be
// overwritten.
func TestPersistDynamicCandidateSwitchRejectsChangedSessionRevision(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-candidate-manual", "session-candidate-manual", models.TaskSessionStateFailed)
	seedDynamicCandidateRuntimeSettings(t, repo, "session-candidate-manual")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	session, err := repo.GetTaskSession(ctx, "session-candidate-manual")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	staleRevision := session.UpdatedAt.Add(-time.Minute)
	session.ExecutionProfileID = nextCandidateProfileID
	changed, err := svc.PersistDynamicCandidateSwitch(ctx, session, session.State, staleRevision)
	if err != nil || changed {
		t.Fatalf("stale switch = (%v, %v), want (false, nil)", changed, err)
	}

	session, err = repo.GetTaskSession(ctx, "session-candidate-manual")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	expectedRevision := session.UpdatedAt
	session.ExecutionProfileID = nextCandidateProfileID
	changed, err = svc.PersistDynamicCandidateSwitch(ctx, session, session.State, expectedRevision)
	if err != nil || !changed {
		t.Fatalf("current switch = (%v, %v), want (true, nil)", changed, err)
	}
	session, err = repo.GetTaskSession(ctx, "session-candidate-manual")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	effective, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	if effective.Model == "previous-model" || effective.Mode == "previous-mode" {
		t.Fatalf("manual candidate switch kept previous provider settings: model=%q mode=%q", effective.Model, effective.Mode)
	}
}
