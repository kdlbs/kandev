package orchestrator

import (
	"context"
	"fmt"
	"slices"
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
	selection := s.selectConfigureSessionRule(rules, agentName)
	if selection.rule == nil {
		// A rule for a real but different agent is a deliberate no-op and stays
		// silent. Every other way of not applying a rule is a workflow-authoring
		// problem, and dropping those silently is what let per-step model
		// selection fail unnoticed.
		if selection.warning != "" {
			s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID, selection.warning)
		}
		return sessionConfigurationTarget{}, false
	}
	if selection.rule.Operation == wfmodels.ConfigureSessionKeep {
		return sessionConfigurationTarget{}, false
	}
	target, ok := sessionConfigurationTargetForRule(session, *selection.rule)
	if !ok {
		s.warnWorkflowSessionConfig(ctx, taskID, session.ID, step.ID,
			"This step requested restoring the original session settings, but no immutable original configuration is available.")
		return sessionConfigurationTarget{}, false
	}
	return target, true
}

// configureSessionRuleSelection is the outcome of choosing the rule that governs
// a session's agent family. A nil rule with an empty warning is the one silent
// outcome: the step deliberately configures some other agent.
type configureSessionRuleSelection struct {
	rule    *wfmodels.ConfigureSessionRule
	warning string
}

// selectConfigureSessionRule returns the single rule governing the session's
// agent family. Workflow rules are hand-authored and name families the way a
// person writes them ("Claude") while sessions store the canonical agent ID
// ("claude-acp"), so both sides are resolved before comparison.
//
// Resolution makes two authoring mistakes reachable that a raw string comparison
// could not express, and neither may be guessed at: a reference that names more
// than one installed agent, and two textually different rules that turn out to
// name the same family. Both refuse to apply anything and say why, because
// picking one silently is indistinguishable from the defect this resolution
// exists to fix.
//
// Both refusals are scoped to rules that could actually govern this session. A
// step configuring several agents carries rules this session will never match,
// and a mistake in one of those is not a reason to drop the rule that does
// match — that would turn one colliding agent name into per-step model
// selection silently failing for every agent at once.
func (s *Service) selectConfigureSessionRule(
	rules []wfmodels.ConfigureSessionRule,
	agentName string,
) configureSessionRuleSelection {
	sessionFamily, sessionAmbiguous := s.resolveSessionAgentFamily(agentName)
	if sessionAmbiguous {
		return configureSessionRuleSelection{warning: ambiguousAgentFamilyWarning(agentName)}
	}

	matched := make([]*wfmodels.ConfigureSessionRule, 0, 1)
	anyKnownFamily := false
	for index := range rules {
		switch s.classifyRuleAgentFamily(rules[index].AgentName, sessionFamily) {
		case ruleGovernsSession:
			anyKnownFamily = true
			matched = append(matched, &rules[index])
		case ruleAmbiguousForSession:
			return configureSessionRuleSelection{warning: ambiguousAgentFamilyWarning(rules[index].AgentName)}
		case ruleNamesOtherAgents:
			anyKnownFamily = true
		case ruleNamesNothingKnown:
		}
	}

	switch {
	case len(matched) > 1:
		return configureSessionRuleSelection{warning: conflictingRulesWarning(len(matched), sessionFamily)}
	case len(matched) == 1:
		return configureSessionRuleSelection{rule: matched[0]}
	case !anyKnownFamily:
		return configureSessionRuleSelection{warning: noKnownAgentFamilyWarning}
	default:
		return configureSessionRuleSelection{}
	}
}

// ruleAgentFamilyVerdict is how one rule's agent reference relates to the agent
// family of the session being configured.
type ruleAgentFamilyVerdict int

const (
	// ruleNamesNothingKnown: the reference names no agent the registry knows and
	// is not the session's family verbatim either.
	ruleNamesNothingKnown ruleAgentFamilyVerdict = iota
	// ruleGovernsSession: the reference names this session's family.
	ruleGovernsSession
	// ruleNamesOtherAgents: the reference names one or more known agents, none of
	// them this session's family. A deliberate no-op for this step.
	ruleNamesOtherAgents
	// ruleAmbiguousForSession: the reference names several agents and this
	// session's family is one of them, so which agent the author meant decides
	// this session's model and cannot be guessed.
	ruleAmbiguousForSession
)

const noKnownAgentFamilyWarning = "This step's session settings were not applied because its " +
	"configure_session rules name no known agent."

func ambiguousAgentFamilyWarning(name string) string {
	return fmt.Sprintf("This step's session settings were not applied because %q matches more than one "+
		"agent installed here. Name the agent by its exact ID in the workflow rule.", name)
}

func conflictingRulesWarning(count int, family string) string {
	return fmt.Sprintf("This step's session settings were not applied because %d configure_session rules "+
		"target the same agent (%s). Keep one rule per agent.", count, family)
}

// resolveSessionAgentFamily maps the session's stored agent family onto its
// canonical ID, reporting whether the stored value is itself ambiguous. Sessions
// store the canonical ID, so this is normally an exact hit; without a resolver
// wired, or for a value the registry does not know, the trimmed input is
// returned so rule matching degrades to the exact comparison this used to
// perform.
func (s *Service) resolveSessionAgentFamily(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if s.agentFamilyResolver == nil {
		return trimmed, false
	}
	switch candidates := s.agentFamilyResolver.ResolveFamilyIDs(trimmed); len(candidates) {
	case 0:
		return trimmed, false
	case 1:
		return candidates[0], false
	default:
		return trimmed, true
	}
}

// classifyRuleAgentFamily reports how one rule's agent reference relates to the
// session's already-canonical family. An unrecognized reference falls back to an
// exact comparison, so it can still match an equally unrecognized session family
// the way it did before resolution existed.
func (s *Service) classifyRuleAgentFamily(ruleAgentName, sessionFamily string) ruleAgentFamilyVerdict {
	trimmed := strings.TrimSpace(ruleAgentName)
	if s.agentFamilyResolver == nil {
		// Nothing is knowable about an agent family without a resolver, so a
		// reference that does not match verbatim is treated as naming some other
		// agent rather than reported to the user as a workflow typo.
		return verbatimRuleVerdict(trimmed, sessionFamily)
	}
	candidates := s.agentFamilyResolver.ResolveFamilyIDs(trimmed)
	switch {
	case len(candidates) == 0:
		if trimmed == sessionFamily {
			return ruleGovernsSession
		}
		return ruleNamesNothingKnown
	case len(candidates) == 1:
		if candidates[0] == sessionFamily {
			return ruleGovernsSession
		}
		return ruleNamesOtherAgents
	case slices.Contains(candidates, sessionFamily):
		return ruleAmbiguousForSession
	default:
		return ruleNamesOtherAgents
	}
}

func verbatimRuleVerdict(trimmed, sessionFamily string) ruleAgentFamilyVerdict {
	if trimmed == sessionFamily {
		return ruleGovernsSession
	}
	return ruleNamesOtherAgents
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
