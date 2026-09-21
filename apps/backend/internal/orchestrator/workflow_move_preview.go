package orchestrator

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

// WorkflowMovePreviewRequest identifies the same destination and one-shot
// options that a manual workflow move will use. It intentionally has no
// source session field: the server resolves the current session exactly as
// the move path does.
type WorkflowMovePreviewRequest struct {
	TaskID         string
	WorkflowID     string
	WorkflowStepID string
	EntryOptions   *workflowmove.EntryOptions
}

type WorkflowMovePreviewOutcome string

const (
	WorkflowMovePreviewOutcomeReuseCurrent WorkflowMovePreviewOutcome = "reuse_current"
	WorkflowMovePreviewOutcomeReuseOther   WorkflowMovePreviewOutcome = "reuse_other"
	WorkflowMovePreviewOutcomeCreateNew    WorkflowMovePreviewOutcome = "create_new"
	WorkflowMovePreviewOutcomeNoSession    WorkflowMovePreviewOutcome = "no_session"
	WorkflowMovePreviewOutcomeUnknown      WorkflowMovePreviewOutcome = "unknown"
)

const (
	previewSettingMode            = "mode"
	previewSettingStepPrompt      = "step_prompt"
	previewSettingReasoningEffort = "reasoning_effort"
	previewSettingVerbosity       = "verbosity"
	previewSourceOverride         = "override"
)

type WorkflowMovePreviewSourceDisposition string

const (
	WorkflowMovePreviewSourceDispositionKeep     WorkflowMovePreviewSourceDisposition = "keep"
	WorkflowMovePreviewSourceDispositionPark     WorkflowMovePreviewSourceDisposition = "park"
	WorkflowMovePreviewSourceDispositionComplete WorkflowMovePreviewSourceDisposition = "complete"
	WorkflowMovePreviewSourceDispositionUnknown  WorkflowMovePreviewSourceDisposition = "unknown"
)

type WorkflowMovePreviewApplicability string

const (
	WorkflowMovePreviewPlanned   WorkflowMovePreviewApplicability = "planned"
	WorkflowMovePreviewUnchanged WorkflowMovePreviewApplicability = "unchanged"
	WorkflowMovePreviewSkipped   WorkflowMovePreviewApplicability = "skipped"
	WorkflowMovePreviewUnknown   WorkflowMovePreviewApplicability = "unknown"
)

type WorkflowMovePreviewDispatch string

const (
	WorkflowMovePreviewDispatchPrompt    WorkflowMovePreviewDispatch = "prompt"
	WorkflowMovePreviewDispatchNoPrompt  WorkflowMovePreviewDispatch = "no_prompt"
	WorkflowMovePreviewDispatchNoSession WorkflowMovePreviewDispatch = "no_session"
	WorkflowMovePreviewDispatchDeferred  WorkflowMovePreviewDispatch = "deferred"
	WorkflowMovePreviewDispatchUnknown   WorkflowMovePreviewDispatch = "unknown"
)

// WorkflowMovePreview is a read-only prediction of the recipient and
// destination settings. It contains display-safe values only; prompts and
// one-shot instruction text never leave the request boundary.
type WorkflowMovePreview struct {
	TaskID            string                               `json:"task_id"`
	WorkflowStepID    string                               `json:"workflow_step_id"`
	SourceSessionID   string                               `json:"source_session_id,omitempty"`
	EvaluatedAt       time.Time                            `json:"evaluated_at"`
	Outcome           WorkflowMovePreviewOutcome           `json:"outcome"`
	Recipient         *WorkflowMovePreviewRecipient        `json:"recipient,omitempty"`
	Model             WorkflowMovePreviewModel             `json:"model"`
	Changes           []WorkflowMovePreviewChange          `json:"changes,omitempty"`
	ContextReset      bool                                 `json:"context_reset"`
	ContextResetState WorkflowMovePreviewApplicability     `json:"context_reset_state"`
	SourceDisposition WorkflowMovePreviewSourceDisposition `json:"source_disposition"`
	Dispatch          WorkflowMovePreviewDispatch          `json:"dispatch"`
	Notices           []WorkflowMovePreviewNotice          `json:"notices,omitempty"`
}

type WorkflowMovePreviewRecipient struct {
	SessionID   string `json:"session_id,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	ProfileID   string `json:"profile_id,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	AgentFamily string `json:"agent_family,omitempty"`
}

type WorkflowMovePreviewModel struct {
	Before       WorkflowMovePreviewModelValue `json:"before"`
	After        WorkflowMovePreviewModelValue `json:"after"`
	BeforeSource string                        `json:"before_source,omitempty"`
	AfterSource  string                        `json:"after_source,omitempty"`
}

type WorkflowMovePreviewModelValue struct {
	ID            string            `json:"id,omitempty"`
	Label         string            `json:"label,omitempty"`
	Known         bool              `json:"known"`
	Mode          string            `json:"mode,omitempty"`
	ConfigOptions map[string]string `json:"config_options,omitempty"`
}

type WorkflowMovePreviewChange struct {
	Key           string                           `json:"key"`
	Label         string                           `json:"label"`
	Before        string                           `json:"before,omitempty"`
	After         string                           `json:"after,omitempty"`
	Applicability WorkflowMovePreviewApplicability `json:"applicability"`
}

type WorkflowMovePreviewNotice struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

type workflowMovePreviewInput struct {
	TaskID                   string
	WorkflowStepID           string
	SourceSession            *models.TaskSession
	Sessions                 []*models.TaskSession
	Destination              *wfmodels.WorkflowStep
	Source                   *wfmodels.WorkflowStep
	TargetSession            *models.TaskSession
	TargetProfileID          string
	ProfileName              string
	ProfileInfo              *executor.AgentProfileInfo
	DynamicProfile           bool
	TargetPassthrough        bool
	ExplicitTarget           bool
	NoStepChange             bool
	OriginalSession          *models.TaskSession
	StartPolicy              models.WorkflowProfileSessionStartPolicy
	SourceEndPolicy          models.WorkflowProfileSessionEndPolicy
	SessionlessLaunchAllowed bool
	AgentResolver            AgentFamilyResolver
	EntryOptions             *workflowmove.EntryOptions
	Notices                  []WorkflowMovePreviewNotice
}

// PreviewWorkflowMove resolves a manual move without reserving a route,
// launching a provider, changing a session, or writing metadata. The actual
// move still repeats its atomic validation after this advisory response.
func (s *Service) PreviewWorkflowMove(ctx context.Context, request WorkflowMovePreviewRequest) (*WorkflowMovePreview, error) {
	if strings.TrimSpace(request.TaskID) == "" {
		return nil, fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(request.WorkflowID) == "" || strings.TrimSpace(request.WorkflowStepID) == "" {
		return nil, fmt.Errorf("workflow id and workflow step id are required")
	}
	if err := s.authorizeTask(ctx, request.TaskID); err != nil {
		return nil, err
	}
	if s.repo == nil || s.workflowStepGetter == nil {
		return nil, fmt.Errorf("workflow move preview is unavailable")
	}

	task, err := s.repo.GetTask(ctx, request.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %s not found", request.TaskID)
	}
	if task.ArchivedAt != nil {
		return nil, fmt.Errorf("archived tasks cannot be moved")
	}

	destination, err := s.workflowStepGetter.GetStep(ctx, request.WorkflowStepID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target workflow step: %w", err)
	}
	if destination == nil {
		return nil, fmt.Errorf("target workflow step %s not found", request.WorkflowStepID)
	}
	if destination.WorkflowID != request.WorkflowID {
		return nil, fmt.Errorf("target workflow step does not belong to target workflow")
	}

	input, err := s.resolveWorkflowMovePreviewInput(ctx, request, task, destination)
	if err != nil {
		return nil, err
	}
	return buildWorkflowMovePreview(input), nil
}

func (s *Service) resolveWorkflowMovePreviewInput(
	ctx context.Context,
	request WorkflowMovePreviewRequest,
	task *models.Task,
	destination *wfmodels.WorkflowStep,
) (workflowMovePreviewInput, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, request.TaskID)
	if err != nil {
		return workflowMovePreviewInput{}, fmt.Errorf("failed to list task sessions: %w", err)
	}
	current := previewCurrentSession(sessions)
	var source *wfmodels.WorkflowStep
	if task.WorkflowStepID != "" {
		source, err = s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
		if err != nil {
			return workflowMovePreviewInput{}, fmt.Errorf("failed to get current workflow step: %w", err)
		}
	}

	input := workflowMovePreviewInput{
		TaskID:                   request.TaskID,
		WorkflowStepID:           request.WorkflowStepID,
		NoStepChange:             task.WorkflowID == request.WorkflowID && task.WorkflowStepID == request.WorkflowStepID,
		SourceSession:            current,
		Sessions:                 sessions,
		Destination:              destination,
		Source:                   source,
		StartPolicy:              models.NormalizeWorkflowProfileSessionStartPolicy(string(destination.ProfileSessionStartPolicy)),
		SourceEndPolicy:          models.WorkflowProfileSessionEndPolicyPark,
		SessionlessLaunchAllowed: workflowmove.ShouldAutoStartAgent(destination, request.EntryOptions),
		AgentResolver:            s.agentFamilyResolver,
		EntryOptions:             request.EntryOptions,
	}
	if source != nil {
		input.SourceEndPolicy = s.resolveStepProfileSessionEndPolicy(source)
	}
	if _, ok := configureSessionAction(destination); ok {
		input.OriginalSession = previewOriginalTaskSessionFromTask(task, sessions)
	}

	s.resolveWorkflowMovePreviewRecipient(ctx, request.TaskID, task, destination, &input)
	return input, nil
}

func (s *Service) resolveWorkflowMovePreviewRecipient(
	ctx context.Context,
	taskID string,
	task *models.Task,
	destination *wfmodels.WorkflowStep,
	input *workflowMovePreviewInput,
) {
	if destination.SessionTarget != nil {
		input.ExplicitTarget = true
		target, profileID, err := s.resolvePreviewWorkflowSessionTarget(ctx, taskID, task, destination)
		if err != nil {
			input.Notices = append(input.Notices, workflowMovePreviewNotice("target_unavailable", nil))
		} else {
			input.TargetSession = target
			input.TargetProfileID = profileID
		}
	} else {
		profileID, err := s.previewStepAgentProfile(ctx, destination, task, input.SourceSession == nil)
		input.TargetProfileID = profileID
		if err != nil {
			input.Notices = append(input.Notices, workflowMovePreviewNotice("profile_unavailable", nil))
		}
	}
	if input.TargetProfileID == "" {
		return
	}
	if s.agentManager == nil {
		input.Notices = append(input.Notices, workflowMovePreviewNotice("profile_unavailable", nil))
		return
	}
	profile, err := s.agentManager.ResolveAgentProfile(ctx, input.TargetProfileID)
	if err != nil || profile == nil {
		input.Notices = append(input.Notices, workflowMovePreviewNotice("profile_unavailable", nil))
		return
	}
	input.ProfileInfo = profile
	input.ProfileName = profile.ProfileName
	input.TargetPassthrough = profile.CLIPassthrough
	input.DynamicProfile = profile.AgentID == agents.DynamicAgentID ||
		strings.EqualFold(profile.AgentName, agents.DynamicAgentID)
}

func (s *Service) previewStepAgentProfile(ctx context.Context, step *wfmodels.WorkflowStep, task *models.Task, sessionless bool) (string, error) {
	if step == nil || step.SessionTarget != nil {
		return "", nil
	}
	if task != nil && task.WorkflowID == step.WorkflowID {
		if replacement, ok := task.WorkflowAgentOverrides.ReplacementFor(task.WorkflowID, step.ID); ok {
			return replacement, nil
		}
	}
	if profileID := strings.TrimSpace(step.AgentProfileID); profileID != "" {
		return profileID, nil
	}
	if step.WorkflowID == "" || s.workflowStepGetter == nil {
		if task != nil && sessionless {
			return strings.TrimSpace(models.StringFromAny(task.Metadata[models.MetaKeyAgentProfileID])), nil
		}
		return "", nil
	}
	meta, err := s.getWorkflowMeta(ctx, step.WorkflowID)
	profileID := strings.TrimSpace(meta.AgentProfileID)
	if profileID == "" && task != nil && sessionless {
		profileID = strings.TrimSpace(models.StringFromAny(task.Metadata[models.MetaKeyAgentProfileID]))
	}
	if profileID != "" {
		return profileID, nil
	}
	return "", err
}

func (s *Service) resolvePreviewWorkflowSessionTarget(
	ctx context.Context,
	taskID string,
	task *models.Task,
	step *wfmodels.WorkflowStep,
) (*models.TaskSession, string, error) {
	if step == nil || step.SessionTarget == nil {
		return nil, "", fmt.Errorf("workflow session target is required")
	}
	if err := wfmodels.ValidateWorkflowSessionTarget(step.SessionTarget); err != nil {
		return nil, "", err
	}
	if step.SessionTarget.Kind == wfmodels.WorkflowSessionTargetInitial {
		return s.resolvePreviewInitialWorkflowSession(ctx, taskID)
	}
	sourceStep, err := s.loadWorkflowStepForLifecycle(ctx, step.SessionTarget.StepID, "session target source")
	if err != nil {
		return nil, "", err
	}
	if err := validateWorkflowSessionTargetSource(step, sourceStep); err != nil {
		return nil, "", err
	}
	session, err := s.resolveBoundSourceWorkflowSession(ctx, taskID, step.WorkflowID, sourceStep)
	if err != nil {
		return nil, "", err
	}
	profileID, err := s.previewStepAgentProfile(ctx, sourceStep, task, true)
	if err != nil {
		return nil, "", err
	}
	return session, profileID, nil
}

// resolvePreviewInitialWorkflowSession is the read-only sibling of
// resolveInitialWorkflowSession. In particular, it never backfills the
// legacy initial-session marker while calculating a disclosure.
func (s *Service) resolvePreviewInitialWorkflowSession(
	ctx context.Context,
	taskID string,
) (*models.TaskSession, string, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", fmt.Errorf("load task for initial session target: %w", err)
	}
	if task == nil {
		return nil, "", fmt.Errorf("task %s not found for initial session target", taskID)
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, "", fmt.Errorf("list sessions for initial session target: %w", err)
	}
	initial := previewOriginalTaskSessionFromTask(task, sessions)
	profileID := ""
	if snapshot, ok := models.LoadWorkflowInitialSessionSnapshot(task.Metadata); ok {
		profileID = strings.TrimSpace(snapshot.AgentProfileID)
	}
	if profileID == "" && initial != nil {
		profileID = strings.TrimSpace(initial.AgentProfileID)
	}
	if profileID == "" && len(sessions) == 0 {
		profileID = models.StringFromAny(task.Metadata[models.MetaKeyAgentProfileID])
	}
	if profileID == "" {
		return nil, "", fmt.Errorf("initial session provenance is unavailable for task %s", taskID)
	}
	return initial, profileID, nil
}

func previewOriginalTaskSessionFromTask(task *models.Task, sessions []*models.TaskSession) *models.TaskSession {
	if task != nil {
		if snapshot, ok := models.LoadWorkflowInitialSessionSnapshot(task.Metadata); ok {
			for _, candidate := range sessions {
				if candidate != nil && candidate.ID == snapshot.SessionID {
					return candidate
				}
			}
			return nil
		}
	}
	if original, resolved := markedOriginalTaskSession(sessions); resolved {
		return original
	}
	return legacyOriginalTaskSession(sessions)
}

func buildWorkflowMovePreview(input workflowMovePreviewInput) *WorkflowMovePreview {
	preview := &WorkflowMovePreview{
		TaskID:            input.TaskID,
		WorkflowStepID:    input.WorkflowStepID,
		EvaluatedAt:       time.Now().UTC(),
		Outcome:           WorkflowMovePreviewOutcomeUnknown,
		SourceDisposition: WorkflowMovePreviewSourceDispositionUnknown,
		Dispatch:          WorkflowMovePreviewDispatchUnknown,
	}
	if input.SourceSession != nil {
		preview.SourceSessionID = input.SourceSession.ID
	}
	if input.NoStepChange {
		preview.Outcome = WorkflowMovePreviewOutcomeUnknown
		if input.SourceSession != nil {
			preview.Outcome = WorkflowMovePreviewOutcomeReuseCurrent
			preview.Recipient = previewRecipientDTO(input.SourceSession, workflowMovePreviewInput{})
			before, known := previewSessionRuntimeConfig(input.SourceSession)
			preview.Model = WorkflowMovePreviewModel{
				Before:       previewModelValue(before, known),
				After:        previewModelValue(before, known),
				BeforeSource: previewSessionModelSource(input.SourceSession),
				AfterSource:  previewSessionModelSource(input.SourceSession),
			}
			preview.SourceDisposition = WorkflowMovePreviewSourceDispositionKeep
		} else {
			preview.Model = WorkflowMovePreviewModel{
				Before: previewModelValue(models.SessionRuntimeConfig{}, false),
				After:  previewModelValue(models.SessionRuntimeConfig{}, false),
			}
		}
		preview.ContextResetState = WorkflowMovePreviewUnchanged
		preview.Dispatch = WorkflowMovePreviewDispatchNoPrompt
		preview.Notices = append([]WorkflowMovePreviewNotice(nil), input.Notices...)
		return preview
	}

	target, outcome := previewRecipient(input)
	input.TargetSession = target
	preview.Outcome = outcome
	if outcome != WorkflowMovePreviewOutcomeUnknown && outcome != WorkflowMovePreviewOutcomeNoSession {
		preview.Recipient = previewRecipientDTO(target, input)
	}

	beforeSession := target
	if beforeSession == nil && outcome == WorkflowMovePreviewOutcomeReuseCurrent {
		beforeSession = input.SourceSession
	}
	before, beforeKnown := previewSessionRuntimeConfig(beforeSession)
	beforeSource := previewSessionModelSource(beforeSession)
	after, afterKnown, afterSource := previewDestinationRuntimeConfig(target, outcome, input)
	if beforeSession == nil && outcome == WorkflowMovePreviewOutcomeCreateNew {
		before, beforeKnown, beforeSource = models.SessionRuntimeConfig{}, false, "unknown"
	}
	if outcome == WorkflowMovePreviewOutcomeUnknown {
		after, afterKnown, afterSource = models.SessionRuntimeConfig{}, false, "unknown"
	}
	if outcome == WorkflowMovePreviewOutcomeNoSession {
		after, afterKnown, afterSource = models.SessionRuntimeConfig{}, false, "unknown"
	}

	applyPreviewConfigureSession(&after, &afterKnown, &afterSource, target, outcome, &input)
	applyPreviewSessionMode(&after, &afterKnown, target, input)
	preview.Model = WorkflowMovePreviewModel{
		Before:       previewModelValue(before, beforeKnown),
		After:        previewModelValue(after, afterKnown),
		BeforeSource: beforeSource,
		AfterSource:  afterSource,
	}
	if beforeSource == previewSourceOverride && afterSource == previewSourceOverride {
		input.Notices = appendPreviewNoticeOnce(input.Notices, workflowMovePreviewNotice("retained_model_override", nil))
	}
	preview.Notices = append([]WorkflowMovePreviewNotice(nil), input.Notices...)
	preview.Changes = previewSettingChanges(before, beforeKnown, after, afterKnown)
	preview.ContextReset, preview.ContextResetState = previewContextReset(input, outcome)
	preview.Changes = append(preview.Changes, previewPromptChange(input, outcome)...)
	preview.SourceDisposition = previewSourceDisposition(input, outcome)
	preview.Dispatch = previewDispatch(input, outcome)
	return preview
}

func previewRecipient(input workflowMovePreviewInput) (*models.TaskSession, WorkflowMovePreviewOutcome) {
	if input.ExplicitTarget {
		return previewExplicitTargetRecipient(input)
	}
	return previewProfileRecipient(input)
}

func previewExplicitTargetRecipient(input workflowMovePreviewInput) (*models.TaskSession, WorkflowMovePreviewOutcome) {
	if input.StartPolicy == models.WorkflowProfileSessionStartPolicyReuse &&
		input.TargetSession != nil && !isTerminalSessionState(input.TargetSession.State) {
		if input.SourceSession != nil && input.TargetSession.ID == input.SourceSession.ID {
			return input.TargetSession, WorkflowMovePreviewOutcomeReuseCurrent
		}
		return input.TargetSession, WorkflowMovePreviewOutcomeReuseOther
	}
	if input.TargetProfileID == "" {
		return nil, WorkflowMovePreviewOutcomeUnknown
	}
	return previewSessionlessRecipient(input)
}

func previewProfileRecipient(input workflowMovePreviewInput) (*models.TaskSession, WorkflowMovePreviewOutcome) {
	if input.TargetProfileID == "" {
		if input.SourceSession != nil {
			return input.SourceSession, WorkflowMovePreviewOutcomeReuseCurrent
		}
		return nil, WorkflowMovePreviewOutcomeUnknown
	}
	if input.SourceSession != nil && shouldKeepCurrentWorkflowStepSession(
		input.TargetProfileID,
		input.SourceSession.AgentProfileID,
		input.StartPolicy,
	) {
		return input.SourceSession, WorkflowMovePreviewOutcomeReuseCurrent
	}
	if input.StartPolicy == models.WorkflowProfileSessionStartPolicyReuse {
		if reusable := previewReusableSession(input.Sessions, input.TargetProfileID, sessionID(input.SourceSession)); reusable != nil {
			return reusable, WorkflowMovePreviewOutcomeReuseOther
		}
	}
	return previewSessionlessRecipient(input)
}

func previewSessionlessRecipient(input workflowMovePreviewInput) (*models.TaskSession, WorkflowMovePreviewOutcome) {
	if input.SourceSession == nil && !input.SessionlessLaunchAllowed {
		return nil, WorkflowMovePreviewOutcomeNoSession
	}
	return nil, WorkflowMovePreviewOutcomeCreateNew
}

func previewReusableSession(sessions []*models.TaskSession, profileID, excludeID string) *models.TaskSession {
	return selectReusableWorkflowSession(sessions, profileID, excludeID)
}

// selectReusableWorkflowSession is the pure candidate selector shared by the
// preview and the side-effecting workflow switch path. Both paths must reject
// terminal and completion-follow-up conversations in the same way.
func selectReusableWorkflowSession(sessions []*models.TaskSession, profileID, excludeID string) *models.TaskSession {
	if profileID == "" {
		return nil
	}
	var best *models.TaskSession
	for _, session := range sessions {
		if session == nil || session.ID == excludeID || session.AgentProfileID != profileID ||
			models.IsCompletionFollowUpSession(session.Metadata) || isTerminalSessionState(session.State) {
			continue
		}
		if best == nil || session.UpdatedAt.After(best.UpdatedAt) {
			best = session
		}
	}
	return best
}

func previewCurrentSession(sessions []*models.TaskSession) *models.TaskSession {
	for _, session := range sessions {
		if session != nil && session.IsPrimary && isPreviewActiveSession(session.State) {
			return session
		}
	}
	var latest *models.TaskSession
	for _, session := range sessions {
		if session == nil || !isPreviewActiveSession(session.State) {
			continue
		}
		if latest == nil || session.StartedAt.After(latest.StartedAt) ||
			(session.StartedAt.Equal(latest.StartedAt) && session.ID > latest.ID) {
			latest = session
		}
	}
	return latest
}

func isPreviewActiveSession(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting,
		models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

func sessionID(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func previewRecipientDTO(session *models.TaskSession, input workflowMovePreviewInput) *WorkflowMovePreviewRecipient {
	recipient := &WorkflowMovePreviewRecipient{
		ProfileID:   input.TargetProfileID,
		ProfileName: input.ProfileName,
	}
	if session == nil {
		return recipient
	}
	recipient.SessionID = session.ID
	recipient.SessionName = session.Name
	if recipient.ProfileID == "" {
		recipient.ProfileID = session.AgentProfileID
	}
	if recipient.ProfileName == "" {
		recipient.ProfileName = previewSnapshotString(session, "name")
	}
	recipient.AgentFamily = previewSnapshotString(session, "agent_name")
	return recipient
}

func previewSessionRuntimeConfig(session *models.TaskSession) (models.SessionRuntimeConfig, bool) {
	if session == nil {
		return models.SessionRuntimeConfig{}, false
	}
	return models.LoadEffectiveSessionRuntimeConfig(session)
}

func previewDestinationRuntimeConfig(
	target *models.TaskSession,
	outcome WorkflowMovePreviewOutcome,
	input workflowMovePreviewInput,
) (models.SessionRuntimeConfig, bool, string) {
	if target != nil && (outcome == WorkflowMovePreviewOutcomeReuseCurrent || outcome == WorkflowMovePreviewOutcomeReuseOther) {
		config, known := previewSessionRuntimeConfig(target)
		return config, known, previewSessionModelSource(target)
	}
	if input.DynamicProfile {
		return models.SessionRuntimeConfig{}, false, "unknown"
	}
	if input.ProfileInfo == nil {
		return models.SessionRuntimeConfig{}, false, "unknown"
	}
	config := models.SessionRuntimeConfig{
		Model:         strings.TrimSpace(input.ProfileInfo.Model),
		Mode:          strings.TrimSpace(input.ProfileInfo.Mode),
		ConfigOptions: maps.Clone(input.ProfileInfo.ConfigOptions),
	}
	return config, !config.IsZero(), "profile"
}

func previewSessionModelSource(session *models.TaskSession) string {
	if session == nil {
		return "unknown"
	}
	if overrides, ok := models.LoadSessionRuntimeConfigOverrides(session.Metadata); ok && overrides.Model != "" {
		return previewSourceOverride
	}
	if runtime, ok := models.LoadSessionRuntimeConfig(session.Metadata); ok && runtime.Model != "" {
		return "runtime"
	}
	if previewSnapshotString(session, "model") != "" {
		return "profile"
	}
	return "unknown"
}

func previewModelValue(config models.SessionRuntimeConfig, known bool) WorkflowMovePreviewModelValue {
	return WorkflowMovePreviewModelValue{
		ID:            config.Model,
		Label:         config.Model,
		Known:         known && config.Model != "",
		Mode:          config.Mode,
		ConfigOptions: maps.Clone(config.ConfigOptions),
	}
}
