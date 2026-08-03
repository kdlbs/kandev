package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// sessionConfigOptionSetter is intentionally narrower than the executor's
// agent-manager contract. Older test and plugin implementations can continue
// to run; providers that support ACP config options opt into this seam.
type sessionConfigOptionSetter interface {
	SetSessionConfigOptionBySessionID(context.Context, string, string, string) error
}

type sessionConfigurationTarget struct {
	model         string
	configOptions map[string]string
}

// applyWorkflowSessionConfigBeforeLaunch stores a matching rule as a runtime
// override before lifecycle initialization. The ACP lifecycle then applies it
// after the profile layer and before the first prompt.
func (s *Service) applyWorkflowSessionConfigBeforeLaunch(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
) {
	s.applyWorkflowSessionConfig(ctx, taskID, session, step, false)
}

// applyWorkflowSessionConfigOnEnter applies a matching rule to an already
// initialized original session. It runs after context reset and before the
// step's auto-start prompt.
func (s *Service) applyWorkflowSessionConfigOnEnter(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
) {
	s.applyWorkflowSessionConfig(ctx, taskID, session, step, true)
}

func (s *Service) applyWorkflowSessionConfig(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	preferLive bool,
) {
	if session == nil || step == nil || s.repo == nil {
		return
	}
	session = s.latestSession(ctx, session)
	target, ok := s.resolveWorkflowSessionConfigTarget(ctx, taskID, session, step)
	if !ok {
		return
	}

	// A launch path has no ready ACP session. Persisting the target is enough;
	// lifecycle initialization consumes it as the runtime layer. On-enter paths
	// prefer the live ACP session, but fall back to the same durable layer when
	// the process is still booting or has been stopped.
	if !preferLive || !s.agentManager.IsAgentReadyForPrompt(ctx, session.ID) {
		s.persistWorkflowSessionConfigBeforeStart(ctx, taskID, session, step, target)
		return
	}

	failed := s.applyLiveWorkflowSessionConfig(ctx, session.ID, target)
	if len(failed) > 0 {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			fmt.Sprintf("Some session settings could not be applied: %s.", strings.Join(failed, ", ")))
	}
}

func (s *Service) latestSession(ctx context.Context, session *models.TaskSession) *models.TaskSession {
	if session == nil || s.repo == nil {
		return session
	}
	latest, err := s.repo.GetTaskSession(ctx, session.ID)
	if err == nil && latest != nil {
		return latest
	}
	return session
}

func (s *Service) resolveWorkflowSessionConfigTarget(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
) (sessionConfigurationTarget, bool) {
	action, ok := configureSessionAction(step)
	if !ok {
		return sessionConfigurationTarget{}, false
	}
	rules, err := wfmodels.ParseConfigureSessionRules(action)
	if err != nil {
		s.logger.Warn("skipping invalid configure_session action",
			zap.String("task_id", taskID), zap.String("step_id", step.ID), zap.Error(err))
		return sessionConfigurationTarget{}, false
	}
	original, err := s.originalTaskSession(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to resolve original task session for configure_session",
			zap.String("task_id", taskID), zap.Error(err))
		return sessionConfigurationTarget{}, false
	}
	if original == nil {
		return sessionConfigurationTarget{}, false
	}
	if original.ID != session.ID {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"This step's session settings were not applied because workflow routing is using a different session tab.")
		return sessionConfigurationTarget{}, false
	}
	if session.IsPassthrough {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"This step's ACP session settings were not applied because the original agent uses passthrough mode.")
		return sessionConfigurationTarget{}, false
	}
	agentName := sessionAgentFamily(session)
	if agentName == "" {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"This step's session settings were not applied because the original agent family is unknown.")
		return sessionConfigurationTarget{}, false
	}
	rule := matchingConfigureSessionRule(rules, agentName)
	if rule == nil || rule.Operation == wfmodels.ConfigureSessionKeep {
		return sessionConfigurationTarget{}, false
	}
	target, ok := sessionConfigurationTargetForRule(session, *rule)
	if !ok {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"This step requested restoring the original session settings, but no immutable original configuration is available.")
		return sessionConfigurationTarget{}, false
	}
	return target, true
}

func matchingConfigureSessionRule(rules []wfmodels.ConfigureSessionRule, agentName string) *wfmodels.ConfigureSessionRule {
	for index := range rules {
		if rules[index].AgentName == agentName {
			return &rules[index]
		}
	}
	return nil
}

func (s *Service) persistWorkflowSessionConfigBeforeStart(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	target sessionConfigurationTarget,
) {
	if err := s.persistWorkflowSessionConfigOverride(ctx, session.ID, target); err != nil {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"The requested session settings could not be saved for the next agent start.")
	}
}

func (s *Service) applyLiveWorkflowSessionConfig(
	ctx context.Context,
	sessionID string,
	target sessionConfigurationTarget,
) []string {
	failed := make([]string, 0)
	applied := sessionConfigurationTarget{configOptions: make(map[string]string)}
	if target.model != "" {
		if err := s.agentManager.SetSessionModelBySessionID(ctx, sessionID, target.model); err != nil {
			failed = append(failed, "model")
		} else {
			applied.model = target.model
		}
	}
	setter, supported := s.agentManager.(sessionConfigOptionSetter)
	for configID, value := range target.configOptions {
		if !supported {
			failed = append(failed, configID)
			continue
		}
		if err := setter.SetSessionConfigOptionBySessionID(ctx, sessionID, configID, value); err != nil {
			failed = append(failed, configID)
			continue
		}
		applied.configOptions[configID] = value
	}
	if applied.model != "" || len(applied.configOptions) > 0 {
		if err := s.persistWorkflowSessionConfigOverride(ctx, sessionID, applied); err != nil {
			failed = append(failed, "settings persistence")
		}
	}
	return failed
}

func configureSessionAction(step *wfmodels.WorkflowStep) (wfmodels.OnEnterAction, bool) {
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterConfigureSession {
			return action, true
		}
	}
	return wfmodels.OnEnterAction{}, false
}

func sessionAgentFamily(session *models.TaskSession) string {
	if session == nil || session.AgentProfileSnapshot == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(session.AgentProfileSnapshot["agent_name"]))
}

func sessionConfigurationTargetForRule(session *models.TaskSession, rule wfmodels.ConfigureSessionRule) (sessionConfigurationTarget, bool) {
	if rule.Operation == wfmodels.ConfigureSessionSet {
		return sessionConfigurationTarget{model: strings.TrimSpace(rule.Model), configOptions: cleanSessionConfigOptions(rule.ConfigOptions)}, true
	}
	original, ok := models.LoadOriginalSessionEffectiveConfiguration(session.Metadata)
	if !ok {
		return sessionConfigurationTarget{}, false
	}
	return sessionConfigurationTarget{model: original.Model, configOptions: cleanSessionConfigOptions(original.ConfigOptions)}, true
}

func cleanSessionConfigOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return nil
	}
	clean := make(map[string]string, len(options))
	for key, value := range options {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || key == "model" || key == "mode" {
			continue
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func (s *Service) persistWorkflowSessionConfigOverride(ctx context.Context, sessionID string, target sessionConfigurationTarget) error {
	if s.repo == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session %q not found", sessionID)
	}
	overrides, _ := models.LoadSessionRuntimeConfigOverrides(session.Metadata)
	if target.model != "" {
		overrides.Model = target.model
	}
	if len(target.configOptions) > 0 {
		if overrides.ConfigOptions == nil {
			overrides.ConfigOptions = make(map[string]string)
		}
		for key, value := range target.configOptions {
			overrides.ConfigOptions[key] = value
		}
	}
	return s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyRuntimeConfigOverrides, overrides)
}

// originalTaskSession returns the immutable task-initial session. The marker
// is authoritative; the timestamp fallback is deliberately conservative for
// sessions created before the marker existed.
func (s *Service) originalTaskSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if original, resolved := markedOriginalTaskSession(sessions); resolved {
		return original, nil
	}
	return legacyOriginalTaskSession(sessions), nil
}

func markedOriginalTaskSession(sessions []*models.TaskSession) (*models.TaskSession, bool) {
	marked := make([]*models.TaskSession, 0, 1)
	for _, candidate := range sessions {
		if candidate != nil && models.IsOriginalTaskSession(candidate.Metadata) {
			marked = append(marked, candidate)
		}
	}
	if len(marked) == 1 {
		return marked[0], true
	}
	if len(marked) > 1 {
		return nil, true
	}
	return nil, false
}

func legacyOriginalTaskSession(sessions []*models.TaskSession) *models.TaskSession {
	legacy := make([]*models.TaskSession, 0, len(sessions))
	for _, candidate := range sessions {
		if candidate == nil || candidate.Metadata[models.SessionMetaKeyCreatedBy] == models.SessionCreatedByWorkflowSwitch {
			continue
		}
		legacy = append(legacy, candidate)
	}
	if len(legacy) == 0 {
		return nil
	}
	sort.SliceStable(legacy, func(i, j int) bool {
		left, right := legacy[i].StartedAt, legacy[j].StartedAt
		if left.Equal(right) {
			return legacy[i].ID < legacy[j].ID
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})
	if len(legacy) > 1 && legacy[0].StartedAt.Equal(legacy[1].StartedAt) {
		return nil
	}
	return legacy[0]
}

func (s *Service) warnWorkflowSessionConfig(ctx context.Context, taskID, sessionID, stepID, content string) {
	s.logger.Warn("workflow session configuration warning",
		zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.String("step_id", stepID), zap.String("message", content))
	if s.messageCreator == nil {
		return
	}
	metadata := map[string]interface{}{
		"variant":                 "warning",
		"workflow_session_config": true,
		"workflow_step_id":        stepID,
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, content, sessionID, string(v1.MessageTypeStatus), s.getActiveTurnID(sessionID), metadata, false,
	); err != nil {
		s.logger.Warn("failed to persist workflow session configuration warning",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}

func stringFromAny(value interface{}) string {
	valueString, _ := value.(string)
	return valueString
}
