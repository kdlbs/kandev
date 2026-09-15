package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBuildWorkflowMovePreview_SelectsTheSameReusableSessionAsMove(t *testing.T) {
	now := time.Now().UTC()
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		UpdatedAt:      now,
	}
	older := &models.TaskSession{
		ID:             "session-older",
		TaskID:         "task-1",
		AgentProfileID: "profile-implement",
		State:          models.TaskSessionStateIdle,
		UpdatedAt:      now.Add(-time.Minute),
	}
	newer := &models.TaskSession{
		ID:             "session-newer",
		TaskID:         "task-1",
		AgentProfileID: "profile-implement",
		State:          models.TaskSessionStateWaitingForInput,
		UpdatedAt:      now.Add(time.Minute),
	}
	terminal := &models.TaskSession{
		ID:             "session-terminal",
		TaskID:         "task-1",
		AgentProfileID: "profile-implement",
		State:          models.TaskSessionStateCompleted,
		UpdatedAt:      now.Add(2 * time.Minute),
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current, older, newer, terminal},
		Destination:     &wfmodels.WorkflowStep{ID: "step-implement", Name: "Implement"},
		Source:          &wfmodels.WorkflowStep{ID: "step-analysis", Name: "Analysis"},
		TargetProfileID: "profile-implement",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
		SourceEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
		ProfileName:     "Implementation",
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeReuseOther {
		t.Fatalf("outcome = %q, want reuse_other", preview.Outcome)
	}
	if preview.Recipient == nil || preview.Recipient.SessionID != newer.ID {
		t.Fatalf("recipient = %#v, want the newest nonterminal target session", preview.Recipient)
	}
	if preview.SourceDisposition != WorkflowMovePreviewSourceDispositionPark {
		t.Fatalf("source disposition = %q, want park", preview.SourceDisposition)
	}
}

func TestBuildWorkflowMovePreview_NewPolicyDoesNotReuseAnExistingSession(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
	}
	existing := &models.TaskSession{
		ID:             "session-existing",
		TaskID:         "task-1",
		AgentProfileID: "profile-implement",
		State:          models.TaskSessionStateIdle,
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:                   "task-1",
		SourceSession:            current,
		Sessions:                 []*models.TaskSession{current, existing},
		Destination:              &wfmodels.WorkflowStep{ID: "step-implement", Name: "Implement"},
		TargetProfileID:          "profile-implement",
		StartPolicy:              models.WorkflowProfileSessionStartPolicyNew,
		SourceEndPolicy:          models.WorkflowProfileSessionEndPolicyComplete,
		SessionlessLaunchAllowed: true,
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeCreateNew {
		t.Fatalf("outcome = %q, want create_new", preview.Outcome)
	}
	if preview.Recipient == nil || preview.Recipient.SessionID != "" {
		t.Fatalf("recipient = %#v, want a sessionless new recipient", preview.Recipient)
	}
}

func TestBuildWorkflowMovePreview_ReportsDeferredDispatchForActiveSessions(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateRunning,
		IsPrimary:      true,
	}
	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:        "task-1",
		SourceSession: current,
		Sessions:      []*models.TaskSession{current},
		Destination: &wfmodels.WorkflowStep{
			ID:     "step-review",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
		},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
	})
	if preview.Dispatch != WorkflowMovePreviewDispatchDeferred {
		t.Fatalf("dispatch = %q, want deferred", preview.Dispatch)
	}
}

func TestBuildWorkflowMovePreview_ReportsDeferredDispatchForReusableTarget(t *testing.T) {
	now := time.Now().UTC()
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		UpdatedAt:      now,
	}
	target := &models.TaskSession{
		ID:             "session-target",
		TaskID:         "task-1",
		AgentProfileID: "profile-review",
		State:          models.TaskSessionStateRunning,
		UpdatedAt:      now.Add(time.Minute),
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:                   "task-1",
		SourceSession:            current,
		Sessions:                 []*models.TaskSession{current, target},
		Destination:              &wfmodels.WorkflowStep{ID: "step-review", Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}},
		TargetProfileID:          target.AgentProfileID,
		StartPolicy:              models.WorkflowProfileSessionStartPolicyReuse,
		SessionlessLaunchAllowed: true,
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeReuseOther {
		t.Fatalf("outcome = %q, want reuse_other", preview.Outcome)
	}
	if preview.Dispatch != WorkflowMovePreviewDispatchDeferred {
		t.Fatalf("dispatch = %q, want deferred for the active reusable target", preview.Dispatch)
	}
}

func TestBuildWorkflowMovePreview_ProfileSwitchWithSourceCreatesSessionWithoutAutoStart(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:                   "task-1",
		SourceSession:            current,
		Sessions:                 []*models.TaskSession{current},
		Destination:              &wfmodels.WorkflowStep{ID: "step-review"},
		TargetProfileID:          "profile-review",
		StartPolicy:              models.WorkflowProfileSessionStartPolicyReuse,
		SessionlessLaunchAllowed: false,
	})

	if preview.Outcome != WorkflowMovePreviewOutcomeCreateNew {
		t.Fatalf("outcome = %q, want create_new for an existing-source profile switch", preview.Outcome)
	}
}

func TestBuildWorkflowMovePreview_SkipsFreshSessionContextReset(t *testing.T) {
	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:                   "task-1",
		Destination:              &wfmodels.WorkflowStep{ID: "step-review"},
		TargetProfileID:          "profile-review",
		SessionlessLaunchAllowed: true,
		EntryOptions:             &workflowmove.EntryOptions{ResetContext: true},
	})

	if preview.ContextResetState != WorkflowMovePreviewSkipped {
		t.Fatalf("context reset state = %q, want skipped for a fresh session", preview.ContextResetState)
	}
}

func TestBuildWorkflowMovePreview_SkipsModeForFreshPassthroughProfile(t *testing.T) {
	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:                   "task-1",
		Destination:              &wfmodels.WorkflowStep{ID: "step-review", Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{previewSettingMode: "plan"}}}}},
		TargetProfileID:          "profile-review",
		TargetPassthrough:        true,
		ProfileInfo:              &executor.AgentProfileInfo{Model: "gpt-5.6-luna"},
		SessionlessLaunchAllowed: true,
	})

	if preview.Model.After.Mode != "" {
		t.Fatalf("after mode = %q, want no mode change for a fresh passthrough profile", preview.Model.After.Mode)
	}
}

func TestPreviewConfigureSelectionNoticeUsesTypedReason(t *testing.T) {
	resolver := previewAgentFamilyResolver{
		"ambiguous": {"codex-acp", "codex-cli"},
		"codex-acp": {"codex-acp"},
		"codex-cli": {"codex-cli"},
	}
	rules := []wfmodels.ConfigureSessionRule{
		{AgentName: "ambiguous", Operation: wfmodels.ConfigureSessionSet, Model: "gpt-5.6-luna"},
	}
	selection := selectConfigureSessionRuleWithResolver(rules, "codex-acp", resolver)
	if selection.rule != nil {
		t.Fatalf("selection = %#v, want no rule for ambiguous reference", selection)
	}
	if got := previewConfigureSelectionNotice(selection).Code; got != "ambiguous_session_configuration" {
		t.Fatalf("notice code = %q, want ambiguous_session_configuration", got)
	}

	conflictResolver := previewAgentFamilyResolver{
		"codex-acp": {"codex-acp"},
		"codex-cli": {"codex-acp"},
	}
	selection = selectConfigureSessionRuleWithResolver([]wfmodels.ConfigureSessionRule{
		{AgentName: "codex-acp", Operation: wfmodels.ConfigureSessionSet, Model: "gpt-5.6-luna"},
		{AgentName: "codex-cli", Operation: wfmodels.ConfigureSessionSet, Model: "gpt-5.6-astra"},
	}, "codex-acp", conflictResolver)
	if selection.rule != nil {
		t.Fatalf("selection = %#v, want no rule for conflicting references", selection)
	}
	if got := previewConfigureSelectionNotice(selection).Code; got != "conflicting_session_configuration" {
		t.Fatalf("notice code = %q, want conflicting_session_configuration", got)
	}
}

func TestPreviewWorkflowMove_ProjectsConfigureSessionForFreshLaunch(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "task-fresh-config", "step-source")

	steps := newMockStepGetter()
	steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1", Position: 0}
	target := configureSessionStep("step-target", "codex", "set", "gpt-5.6-astra", map[string]string{"reasoning_effort": "max"})
	target.WorkflowID = "wf1"
	target.Position = 1
	target.Events.OnEnter = append(target.Events.OnEnter, wfmodels.OnEnterAction{Type: wfmodels.OnEnterAutoStartAgent})
	steps.steps[target.ID] = target
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID: "profile-task", ProfileName: "Task profile", AgentName: "codex", Model: "gpt-5.6-luna",
		},
	})
	task, err := repo.GetTask(ctx, "task-fresh-config")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata[models.MetaKeyAgentProfileID] = "profile-task"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
		TaskID: "task-fresh-config", WorkflowID: "wf1", WorkflowStepID: target.ID,
	})
	if err != nil {
		t.Fatalf("PreviewWorkflowMove: %v", err)
	}
	if preview.Outcome != WorkflowMovePreviewOutcomeCreateNew {
		t.Fatalf("outcome = %q, want create_new", preview.Outcome)
	}
	if preview.Model.After.ID != "gpt-5.6-astra" || preview.Model.After.ConfigOptions["reasoning_effort"] != "max" {
		t.Fatalf("after model/config = %#v, want rule-projected fresh-session settings", preview.Model.After)
	}
}

func TestPreviewWorkflowMove_KeepsCurrentSessionWhenOnlyTaskProfileFallbackExists(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-current-routing", "session-current-routing", "step-source")
	session, err := repo.GetTaskSession(ctx, "session-current-routing")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile-current"
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, session.ID); err != nil {
		t.Fatalf("set primary session: %v", err)
	}
	task, err := repo.GetTask(ctx, "task-current-routing")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata[models.MetaKeyAgentProfileID] = "profile-task-fallback"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1", Position: 0}
	steps.steps["step-target"] = &wfmodels.WorkflowStep{ID: "step-target", WorkflowID: "wf1", Position: 1}
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{})
	preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
		TaskID: "task-current-routing", WorkflowID: "wf1", WorkflowStepID: "step-target",
	})
	if err != nil {
		t.Fatalf("PreviewWorkflowMove: %v", err)
	}
	if preview.Outcome != WorkflowMovePreviewOutcomeReuseCurrent || preview.Recipient == nil || preview.Recipient.SessionID != session.ID {
		t.Fatalf("preview = %#v, want current-session reuse", preview)
	}
}

func TestPreviewWorkflowMoveAuthorizesTaskBeforeRepositoryAccess(t *testing.T) {
	denied := errors.New("task is not visible")
	svc := &Service{}
	svc.SetTaskAccessChecker(func(_ context.Context, taskID string) error {
		if taskID != "task-private" {
			return errors.New("unexpected task")
		}
		return denied
	})

	_, err := svc.PreviewWorkflowMove(context.Background(), WorkflowMovePreviewRequest{
		TaskID: "task-private", WorkflowID: "workflow-1", WorkflowStepID: "step-1",
	})
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want task authorization error", err)
	}
}

type previewAgentFamilyResolver map[string][]string

func (r previewAgentFamilyResolver) ResolveFamilyIDs(name string) []string {
	return append([]string(nil), r[name]...)
}

func TestBuildWorkflowMovePreview_ConditionalSetRetainsUnnamedRuntimeOptions(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "codex",
			"model":      "gpt-5.6-luna",
			"config_options": map[string]interface{}{
				"reasoning_effort": "medium",
				"verbosity":        "high",
			},
		},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyOrigin: models.SessionOriginTaskInitial,
		},
	}

	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:        "task-1",
		SourceSession: current,
		Sessions:      []*models.TaskSession{current},
		Destination: &wfmodels.WorkflowStep{
			ID: "step-review",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"agent_name": "codex",
					"operation":  "set",
					"model":      "gpt-5.6-astra",
					"config_options": map[string]interface{}{
						"reasoning_effort": "max",
					},
				}}},
			}}},
		},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
		SourceEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	})

	if preview.Model.After.ID != "gpt-5.6-astra" {
		t.Fatalf("after model = %q, want gpt-5.6-astra", preview.Model.After.ID)
	}
	if preview.Model.After.ConfigOptions["verbosity"] != "high" {
		t.Fatalf("after options = %#v, want unnamed verbosity retained", preview.Model.After.ConfigOptions)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].Key != "reasoning_effort" || preview.Changes[0].After != "max" {
		t.Fatalf("changes = %#v, want one reasoning_effort change", preview.Changes)
	}
}

func TestPreviewSettingChangesUsesStableKandevKeysAndSafeProviderLabels(t *testing.T) {
	before := models.SessionRuntimeConfig{
		Mode: "default",
		ConfigOptions: map[string]string{
			"provider.option-name": "off",
		},
	}
	after := models.SessionRuntimeConfig{
		Mode: "plan",
		ConfigOptions: map[string]string{
			"provider.option-name": "on",
		},
	}

	changes := previewSettingChanges(before, true, after, true)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v, want mode and provider option", changes)
	}
	if changes[0].Key != "mode" || changes[0].Label != "mode" {
		t.Fatalf("Kandev change = %#v, want stable mode key", changes[0])
	}
	if changes[1].Key != "provider.option-name" || changes[1].Label != "Provider Option Name" {
		t.Fatalf("provider change = %#v, want safe display label", changes[1])
	}
}

func TestBuildWorkflowMovePreview_IsReadOnlyForSessionMetadata(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{
				Model: "gpt-5.6-astra",
			},
		},
	}
	preview := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current},
		Destination:     &wfmodels.WorkflowStep{ID: "step-review"},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
		SourceEndPolicy: models.WorkflowProfileSessionEndPolicyPark,
	})

	if preview == nil || preview.SourceSessionID != current.ID {
		t.Fatalf("preview = %#v, want a current-session result", preview)
	}
	if _, ok := current.Metadata[models.MetaKeyWorkflowInitialSession]; ok {
		t.Fatal("preview wrote workflow initial-session metadata")
	}
}

func TestBuildWorkflowMovePreview_PreservesDiagnosticNotices(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "codex",
			"model":      "gpt-5.6-luna",
		},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{Model: "gpt-5.6-astra"},
		},
	}
	retained := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current},
		Destination:     &wfmodels.WorkflowStep{ID: "step-review"},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
	})
	if len(retained.Notices) != 1 || retained.Notices[0].Code != "retained_model_override" {
		t.Fatalf("retained notices = %#v, want retained_model_override", retained.Notices)
	}

	startedAt := time.Now().UTC()
	ambiguousSessions := []*models.TaskSession{
		{
			ID:             "session-current",
			TaskID:         "task-1",
			AgentProfileID: "profile-analysis",
			State:          models.TaskSessionStateWaitingForInput,
			IsPrimary:      true,
			StartedAt:      startedAt,
			AgentProfileSnapshot: map[string]interface{}{
				"agent_name": "codex",
			},
		},
		{
			ID:             "session-other",
			TaskID:         "task-1",
			AgentProfileID: "profile-other",
			State:          models.TaskSessionStateIdle,
			StartedAt:      startedAt,
		},
	}
	ambiguous := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:        "task-1",
		SourceSession: ambiguousSessions[0],
		Sessions:      ambiguousSessions,
		Destination: &wfmodels.WorkflowStep{
			ID: "step-review",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"agent_name": "codex",
					"operation":  "set",
					"model":      "gpt-5.6-astra",
				}}},
			}}},
		},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
	})
	if len(ambiguous.Notices) != 1 || ambiguous.Notices[0].Code != "missing_original_snapshot" {
		t.Fatalf("missing-original notices = %#v, want missing_original_snapshot", ambiguous.Notices)
	}
}

func TestPreviewWorkflowMove_DoesNotBackfillLegacyInitialSessionMetadata(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-preview", "session-preview", "step-current")

	session, err := repo.GetTaskSession(ctx, "session-preview")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile-analysis"
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, session.ID); err != nil {
		t.Fatalf("set primary session: %v", err)
	}
	if _, err := repo.RemoveTaskMetadataKey(ctx, "task-preview", models.MetaKeyWorkflowInitialSession); err != nil {
		t.Fatalf("remove initial-session marker: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["step-current"] = &wfmodels.WorkflowStep{ID: "step-current", WorkflowID: "wf1", Position: 0}
	steps.steps["step-target"] = &wfmodels.WorkflowStep{
		ID:                        "step-target",
		WorkflowID:                "wf1",
		Position:                  1,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	agent := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID:   "profile-analysis",
		ProfileName: "Analysis",
		AgentName:   "codex",
		Model:       "gpt-5.6-luna",
	}}
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), agent)

	if _, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
		TaskID:         "task-preview",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-target",
	}); err != nil {
		t.Fatalf("PreviewWorkflowMove: %v", err)
	}

	task, err := repo.GetTask(ctx, "task-preview")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := task.Metadata[models.MetaKeyWorkflowInitialSession]; ok {
		t.Fatal("preview backfilled the legacy initial-session marker")
	}
}

func TestPreviewWorkflowMove_NoSessionLaunchGatesMatchActualMove(t *testing.T) {
	t.Run("step without auto-start stays idle", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-no-auto-start", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "profile-task"},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1", AgentProfileID: "profile-task",
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna"},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-no-auto-start", WorkflowID: "wf1", WorkflowStepID: "step-target",
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeNoSession || preview.Recipient != nil {
			t.Fatalf("preview = %#v, want no_session without a recipient", preview)
		}

		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "task-no-auto-start", ToStepID: "step-target"})
		sessions, err := repo.ListTaskSessions(ctx, "task-no-auto-start")
		if err != nil {
			t.Fatalf("ListTaskSessions: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("actual move created %d sessions, want idle task", len(sessions))
		}
	})

	t.Run("skip prompt without instructions suppresses auto-start", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		options := &workflowmove.EntryOptions{SkipStepPrompt: true}
		encoded, err := workflowmove.EncodeEntryOptionsJSON(options)
		if err != nil {
			t.Fatalf("EncodeEntryOptionsJSON: %v", err)
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-skip-prompt", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentProfileID: "profile-task",
				models.MetaKeyWorkflowMovePending: map[string]interface{}{
					"from_step_id": "step-source", "move_id": "move-1", "options": string(encoded),
				},
			},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna"},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-skip-prompt", WorkflowID: "wf1", WorkflowStepID: "step-target", EntryOptions: options,
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeNoSession || preview.Recipient != nil {
			t.Fatalf("preview = %#v, want no_session without a recipient", preview)
		}

		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "task-skip-prompt", ToStepID: "step-target"})
		sessions, err := repo.ListTaskSessions(ctx, "task-skip-prompt")
		if err != nil {
			t.Fatalf("ListTaskSessions: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("actual move created %d sessions, want idle task", len(sessions))
		}
		task, err := repo.GetTask(ctx, "task-skip-prompt")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
			t.Fatal("actual move left the one-shot marker after suppressing auto-start")
		}
	})

	t.Run("allowed auto-start retains the task profile fallback", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-profile-fallback", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "profile-task"},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{
				ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna",
			},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-profile-fallback", WorkflowID: "wf1", WorkflowStepID: "step-target",
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeCreateNew {
			t.Fatalf("preview outcome = %q, want create_new", preview.Outcome)
		}
		if preview.Recipient == nil || preview.Recipient.ProfileID != "profile-task" {
			t.Fatalf("preview recipient = %#v, want task profile fallback", preview.Recipient)
		}
		if preview.Model.After.ID != "gpt-5.6-luna" {
			t.Fatalf("preview model = %#v, want task profile model", preview.Model.After)
		}
	})
}
