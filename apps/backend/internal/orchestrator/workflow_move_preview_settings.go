package orchestrator

import (
	"sort"
	"strings"
	"unicode"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func applyPreviewConfigureSession(
	after *models.SessionRuntimeConfig,
	afterKnown *bool,
	afterSource *string,
	target *models.TaskSession,
	input *workflowMovePreviewInput,
) {
	if input == nil || input.Destination == nil || target == nil {
		return
	}
	action, ok := configureSessionAction(input.Destination)
	if !ok {
		return
	}
	rules, err := wfmodels.ParseConfigureSessionRules(action)
	if err != nil {
		input.Notices = append(input.Notices, workflowMovePreviewNotice("invalid_session_configuration", nil))
		return
	}
	rule, ok := previewConfigureSessionRule(target, input, rules)
	if !ok || rule.Operation == wfmodels.ConfigureSessionKeep {
		return
	}
	targetConfig, ok := sessionConfigurationTargetForRule(target, *rule)
	if !ok {
		input.Notices = appendPreviewNoticeOnce(input.Notices, workflowMovePreviewNotice("session_configuration_unavailable", nil))
		return
	}
	applyPreviewConfigurationTarget(after, afterKnown, afterSource, targetConfig, rule.Operation)
}

func applyPreviewConfigurationTarget(
	after *models.SessionRuntimeConfig,
	afterKnown *bool,
	afterSource *string,
	targetConfig sessionConfigurationTarget,
	operation wfmodels.ConfigureSessionOperation,
) {
	if targetConfig.model != "" {
		after.Model = targetConfig.model
	}
	if len(targetConfig.configOptions) > 0 {
		if after.ConfigOptions == nil {
			after.ConfigOptions = make(map[string]string)
		}
		for key, value := range targetConfig.configOptions {
			after.ConfigOptions[key] = value
		}
	}
	*afterKnown = *afterKnown || targetConfig.model != "" || len(targetConfig.configOptions) > 0
	if operation == wfmodels.ConfigureSessionRestoreOriginal {
		*afterSource = "original"
	} else {
		*afterSource = "step_rule"
	}
}

func previewConfigureSessionRule(
	target *models.TaskSession,
	input *workflowMovePreviewInput,
	rules []wfmodels.ConfigureSessionRule,
) (*wfmodels.ConfigureSessionRule, bool) {
	original := input.OriginalSession
	if original == nil {
		original = previewOriginalFromSessions(input.Sessions)
	}
	if original == nil {
		input.Notices = appendPreviewNoticeOnce(input.Notices, workflowMovePreviewNotice("missing_original_snapshot", nil))
		return nil, false
	}
	if original.ID != target.ID || target.IsPassthrough || input.TargetPassthrough {
		input.Notices = appendPreviewNoticeOnce(input.Notices, workflowMovePreviewNotice("session_configuration_skipped", nil))
		return nil, false
	}
	agentName := sessionAgentFamily(target)
	if agentName == "" {
		input.Notices = appendPreviewNoticeOnce(input.Notices, workflowMovePreviewNotice("session_configuration_unavailable", nil))
		return nil, false
	}
	selection := selectConfigureSessionRuleWithResolver(rules, agentName, input.AgentResolver)
	if selection.rule == nil {
		if selection.warning != "" {
			input.Notices = appendPreviewNoticeOnce(input.Notices, previewConfigureSelectionNotice(selection.warning))
		}
		return nil, false
	}
	return selection.rule, true
}

func previewOriginalFromSessions(sessions []*models.TaskSession) *models.TaskSession {
	if original, resolved := markedOriginalTaskSession(sessions); resolved {
		return original
	}
	return legacyOriginalTaskSession(sessions)
}

func applyPreviewSessionMode(
	after *models.SessionRuntimeConfig,
	afterKnown *bool,
	target *models.TaskSession,
	input workflowMovePreviewInput,
) {
	if input.Destination == nil {
		return
	}
	for _, action := range input.Destination.Events.OnEnter {
		if action.Type != wfmodels.OnEnterSetSessionMode {
			continue
		}
		mode := strings.TrimSpace(models.StringFromAny(action.Config[previewSettingMode]))
		if mode == "" {
			continue
		}
		if target != nil && (target.IsPassthrough || input.TargetPassthrough) {
			return
		}
		after.Mode = mode
		*afterKnown = true
	}
}

func previewSettingChanges(
	before models.SessionRuntimeConfig,
	beforeKnown bool,
	after models.SessionRuntimeConfig,
	afterKnown bool,
) []WorkflowMovePreviewChange {
	changes := make([]WorkflowMovePreviewChange, 0)
	if before.Mode != after.Mode {
		changes = append(changes, WorkflowMovePreviewChange{
			Key: previewSettingMode, Label: previewSettingLabel(previewSettingMode), Before: before.Mode, After: after.Mode,
			Applicability: previewApplicability(beforeKnown, afterKnown),
		})
	}
	keys := make(map[string]struct{}, len(before.ConfigOptions)+len(after.ConfigOptions))
	for key := range before.ConfigOptions {
		keys[key] = struct{}{}
	}
	for key := range after.ConfigOptions {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		beforeValue, afterValue := before.ConfigOptions[key], after.ConfigOptions[key]
		if beforeValue == afterValue {
			continue
		}
		changes = append(changes, WorkflowMovePreviewChange{
			Key: key, Label: previewSettingLabel(key), Before: beforeValue, After: afterValue,
			Applicability: previewApplicability(beforeKnown, afterKnown),
		})
	}
	return changes
}

// previewSettingLabel keeps provider-owned option labels displayable without
// exposing raw option identifiers as UI copy. Kandev-owned fields keep their
// stable keys so the web client can localize them; provider fields use a
// bounded, readable label because their catalog is provider-defined.
func previewSettingLabel(key string) string {
	key = strings.TrimSpace(key)
	if key == previewSettingMode || key == previewSettingStepPrompt || key == previewSettingReasoningEffort || key == previewSettingVerbosity {
		return key
	}

	var label strings.Builder
	capitalize := true
	runes := 0
	for _, r := range key {
		if runes >= 64 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if capitalize {
				r = unicode.ToUpper(r)
				capitalize = false
			}
			label.WriteRune(r)
			runes++
			continue
		}
		if label.Len() > 0 && !capitalize {
			label.WriteByte(' ')
			capitalize = true
		}
	}
	if value := strings.TrimSpace(label.String()); value != "" {
		return value
	}
	return "Provider option"
}

func previewApplicability(beforeKnown, afterKnown bool) WorkflowMovePreviewApplicability {
	if beforeKnown && afterKnown {
		return WorkflowMovePreviewPlanned
	}
	return WorkflowMovePreviewUnknown
}

func previewContextReset(input workflowMovePreviewInput, outcome WorkflowMovePreviewOutcome) (bool, WorkflowMovePreviewApplicability) {
	requested := input.EntryOptions != nil && input.EntryOptions.ResetContext
	declared := input.Destination != nil && input.Destination.HasOnEnterAction(wfmodels.OnEnterResetAgentContext)
	if !requested && !declared {
		return false, WorkflowMovePreviewUnchanged
	}
	if outcome == WorkflowMovePreviewOutcomeUnknown || outcome == WorkflowMovePreviewOutcomeNoSession {
		return requested || declared, WorkflowMovePreviewUnknown
	}
	return requested || declared, WorkflowMovePreviewPlanned
}

func previewPromptChange(input workflowMovePreviewInput, outcome WorkflowMovePreviewOutcome) []WorkflowMovePreviewChange {
	if input.EntryOptions == nil || !input.EntryOptions.SkipStepPrompt {
		return nil
	}
	applicability := WorkflowMovePreviewPlanned
	if outcome == WorkflowMovePreviewOutcomeUnknown {
		applicability = WorkflowMovePreviewUnknown
	}
	if outcome == WorkflowMovePreviewOutcomeNoSession {
		applicability = WorkflowMovePreviewSkipped
	}
	return []WorkflowMovePreviewChange{{
		Key: previewSettingStepPrompt, Label: previewSettingLabel(previewSettingStepPrompt), Before: "configured", After: "skipped", Applicability: applicability,
	}}
}

func previewSourceDisposition(input workflowMovePreviewInput, outcome WorkflowMovePreviewOutcome) WorkflowMovePreviewSourceDisposition {
	if input.SourceSession == nil {
		return WorkflowMovePreviewSourceDispositionUnknown
	}
	if outcome == WorkflowMovePreviewOutcomeReuseCurrent || input.Source == nil {
		return WorkflowMovePreviewSourceDispositionKeep
	}
	switch models.NormalizeWorkflowProfileSessionEndPolicy(string(input.SourceEndPolicy)) {
	case models.WorkflowProfileSessionEndPolicyComplete:
		return WorkflowMovePreviewSourceDispositionComplete
	case models.WorkflowProfileSessionEndPolicyPark:
		return WorkflowMovePreviewSourceDispositionPark
	default:
		return WorkflowMovePreviewSourceDispositionUnknown
	}
}

func previewDispatch(input workflowMovePreviewInput, outcome WorkflowMovePreviewOutcome) WorkflowMovePreviewDispatch {
	if outcome == WorkflowMovePreviewOutcomeUnknown || input.Destination == nil {
		return WorkflowMovePreviewDispatchUnknown
	}
	if outcome == WorkflowMovePreviewOutcomeNoSession {
		return WorkflowMovePreviewDispatchNoPrompt
	}
	if !input.Destination.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		return WorkflowMovePreviewDispatchNoPrompt
	}
	if input.EntryOptions != nil && input.EntryOptions.SkipStepPrompt && strings.TrimSpace(input.EntryOptions.Instructions) == "" {
		return WorkflowMovePreviewDispatchNoPrompt
	}
	if previewSessionDispatchDeferred(input.SourceSession) || previewSessionDispatchDeferred(input.TargetSession) {
		return WorkflowMovePreviewDispatchDeferred
	}
	return WorkflowMovePreviewDispatchPrompt
}

func previewSessionDispatchDeferred(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	return session.State == models.TaskSessionStateStarting || session.State == models.TaskSessionStateRunning
}

func previewSnapshotString(session *models.TaskSession, key string) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(models.StringFromAny(session.AgentProfileSnapshot[key]))
}

func workflowMovePreviewNotice(code string, params map[string]string) WorkflowMovePreviewNotice {
	return WorkflowMovePreviewNotice{Code: code, Params: params}
}

func appendPreviewNoticeOnce(notices []WorkflowMovePreviewNotice, notice WorkflowMovePreviewNotice) []WorkflowMovePreviewNotice {
	for _, existing := range notices {
		if existing.Code == notice.Code {
			return notices
		}
	}
	return append(notices, notice)
}

func previewConfigureSelectionNotice(warning string) WorkflowMovePreviewNotice {
	switch {
	case strings.Contains(warning, "ambiguous"):
		return workflowMovePreviewNotice("ambiguous_session_configuration", nil)
	case strings.Contains(warning, "conflicting"):
		return workflowMovePreviewNotice("conflicting_session_configuration", nil)
	default:
		return workflowMovePreviewNotice("session_configuration_unavailable", nil)
	}
}
